package localserver

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"goteams-client/internal/applog"
	"goteams-client/internal/executor"
	"goteams-client/internal/storage/log"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins (local loopback service, no CSRF required)
		return true
	},
	HandshakeTimeout: 10 * time.Second,
}

// wsWriteTimeout is the timeout for a single write to the local browser connection.
const wsWriteTimeout = 10 * time.Second

// PushFunc push function signature: returns the push type and data, and returns nil to indicate that no push is required in this round.
type PushFunc func() (pushType string, data interface{})

// WSHub local WebSocket manager
type WSHub struct {
	mu       sync.RWMutex
	clients  map[*websocket.Conn]*wsClient
	logDB    *sql.DB
	apiToken string

	// Universal push
	pushMu     sync.RWMutex
	pushFuncs  []PushFunc
	pushCancel context.CancelFunc
}

// wsClient local WebSocket client
type wsClient struct {
	conn        *websocket.Conn
	taskUUID    string
	sessionUUID string
	writeMu     sync.Mutex
}

// NewWSHub creates a WebSocket manager
func NewWSHub(logDB *sql.DB) *WSHub {
	return &WSHub{
		clients: make(map[*websocket.Conn]*wsClient),
		logDB:   logDB,
	}
}

// SetAPIToken records the startup API token for handshake validation.
func (h *WSHub) SetAPIToken(token string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.apiToken = token
}

// RegisterPushFunc registers a periodic push function.
// Each push loop will call all registered pushFuncs, and when non-nil data is returned, it will be broadcast to all clients.
func (h *WSHub) RegisterPushFunc(fn PushFunc) {
	h.pushMu.Lock()
	h.pushFuncs = append(h.pushFuncs, fn)
	h.pushMu.Unlock()
}

// StartPushLoop starts a periodic push loop and broadcasts the result of registering pushFunc to all connected clients.
// Multiple calls will first stop the old loop and then start the new loop.
func (h *WSHub) StartPushLoop(interval time.Duration) {
	h.StopPushLoop()
	ctx, cancel := context.WithCancel(context.Background())
	h.pushMu.Lock()
	h.pushCancel = cancel
	h.pushMu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.runPushCycle()
			}
		}
	}()
}

// StopPushLoop Stops the periodic push loop.
func (h *WSHub) StopPushLoop() {
	h.pushMu.Lock()
	if h.pushCancel != nil {
		h.pushCancel()
		h.pushCancel = nil
	}
	h.pushMu.Unlock()
}

// runPushCycle executes a round of push: traverses all pushFuncs and broadcasts the results.
func (h *WSHub) runPushCycle() {
	h.pushMu.RLock()
	fns := make([]PushFunc, len(h.pushFuncs))
	copy(fns, h.pushFuncs)
	h.pushMu.RUnlock()

	for _, fn := range fns {
		pushType, data := fn()
		if data == nil {
			continue
		}
		h.BroadcastToAll(pushType, data)
	}
}

// BroadcastToAll broadcasts a message to all connected clients (not limited to specific task/session subscribers).
func (h *WSHub) BroadcastToAll(msgType string, data interface{}) {
	msg := map[string]interface{}{
		"type": msgType,
		"data": data,
	}

	h.mu.RLock()
	clients := make([]*wsClient, 0, len(h.clients))
	for _, c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		if err := writeClientJSON(c, msg); err != nil {
			applog.Warn("ws broadcast to all failed", "error", err)
			h.mu.Lock()
			delete(h.clients, c.conn)
			h.mu.Unlock()
			c.conn.Close()
		}
	}
}

// BroadcastTaskChanged notifies task views that persisted task/step state has
// changed and should be reloaded from the HTTP API.
func (h *WSHub) BroadcastTaskChanged(taskUUID string) {
	if taskUUID == "" {
		return
	}
	h.BroadcastToAll("task.changed", map[string]interface{}{"task_uuid": taskUUID})
}

// BroadcastActivity pushes the throttled latest CLI activity to all clients.
// Task views filter by task_uuid and patch the running-step subtitle locally,
// so it stays fresh without polling /progress.
func (h *WSHub) BroadcastActivity(taskUUID, sessionUUID, eventType, content string, at int64) {
	if taskUUID == "" || sessionUUID == "" {
		return
	}
	h.BroadcastToAll("executor.activity", map[string]interface{}{
		"task_uuid":    taskUUID,
		"session_uuid": sessionUUID,
		"event_type":   eventType,
		"content":      content,
		"at":           at,
	})
}

// Close Closes all local WebSocket client connections (browser connections).
// Called on account switch/logout: proactively sending a close frame triggers the browser-side onclose,
// so it auto-reconnects and binds to the new account session's WSHub, avoiding the old connection becoming an orphan.
func (h *WSHub) Close() {
	h.StopPushLoop()

	h.mu.Lock()
	clients := make([]*wsClient, 0, len(h.clients))
	for _, c := range h.clients {
		clients = append(clients, c)
	}
	h.clients = make(map[*websocket.Conn]*wsClient)
	h.mu.Unlock()

	// The close frame must also go through writeMu: the broadcast goroutine may still hold the old client reference and be writing.
	for _, c := range clients {
		_ = writeClientControl(c, websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "账号会话已切换"))
		c.conn.Close()
	}
}

// HandleWebSocket handles the WebSocket connection
func (h *WSHub) HandleWebSocket(c *gin.Context) {
	// Validate the startup API token passed via ?token= (browsers cannot set
	// custom headers on WebSocket handshakes).
	token := c.Query("token")
	h.mu.RLock()
	expected := h.apiToken
	h.mu.RUnlock()
	if expected != "" && (len(token) != len(expected) ||
		subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无效的访问令牌"})
		return
	}

	// Validate the Session Cookie
	sessionID, err := c.Cookie("goteams_local_session")
	if err != nil || sessionID == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	sessionMgr.mu.RLock()
	session, ok := sessionMgr.sessions[sessionID]
	sessionMgr.mu.RUnlock()
	if !ok || time.Now().After(session.ExpiresAt) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "会话已过期"})
		return
	}

	// Upgrade HTTP to WebSocket
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &wsClient{conn: conn}
	h.mu.Lock()
	h.clients[conn] = client
	h.mu.Unlock()

	// The Ping handler must be registered before the read loop starts:
	// 1. Registering it after ReadMessage races with the read goroutine for the conn's handler field;
	// 2. Pong is sent by the read goroutine concurrently with the broadcast goroutine, so they must share writeMu.
	conn.SetPingHandler(func(appData string) error {
		return writeClientControl(client, websocket.PongMessage, nil)
	})

	// read loop
	go h.readLoop(conn, client)
}

// readLoop reads client messages
func (h *WSHub) readLoop(conn *websocket.Conn, client *wsClient) {
	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		conn.Close()
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var msg struct {
			Type        string `json:"type"`
			TaskUUID    string `json:"task_uuid"`
			SessionUUID string `json:"session_uuid"`
			AfterSeq    int    `json:"after_sequence"`
		}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "executor.subscribe":
			client.taskUUID = msg.TaskUUID
			client.sessionUUID = msg.SessionUUID
			h.backfillEvents(client, msg.SessionUUID, msg.AfterSeq)
		case "executor.unsubscribe":
			client.taskUUID = ""
			client.sessionUUID = ""
		}
	}
}

// backfillEvents re-sends historical events
func (h *WSHub) backfillEvents(client *wsClient, sessionUUID string, afterSeq int) {
	if h.logDB == nil || sessionUUID == "" {
		return
	}

	eventStore := log.NewEventStore(h.logDB)
	events, err := eventStore.QueryAfterSequence(sessionUUID, afterSeq)
	if err != nil {
		return
	}

	for _, event := range events {
		msg := map[string]interface{}{
			"type":         "executor.event",
			"session_uuid": event.SessionUUID,
			"sequence":     event.Sequence,
			"event_type":   event.EventType,
			"event":        json.RawMessage(event.Payload),
		}
		if err := writeClientJSON(client, msg); err != nil {
			return
		}
	}
}

// BroadcastEvent broadcasts an event to clients subscribed to the session
func (h *WSHub) BroadcastEvent(sessionUUID string, sequence int, event executor.ExecutorEvent) {
	h.mu.RLock()
	clients := make([]*wsClient, 0, len(h.clients))
	for _, c := range h.clients {
		if c.sessionUUID == sessionUUID {
			clients = append(clients, c)
		}
	}
	h.mu.RUnlock()

	msg := map[string]interface{}{
		"type":         "executor.event",
		"session_uuid": sessionUUID,
		"sequence":     sequence,
		"event_type":   event.Type,
		"event":        event,
	}

	for _, c := range clients {
		if err := writeClientJSON(c, msg); err != nil {
			// Slow browser disconnected
			h.mu.Lock()
			delete(h.clients, c.conn)
			h.mu.Unlock()
			c.conn.Close()
		}
	}
}

// BroadcastStateChanged broadcasts state changes
func (h *WSHub) BroadcastStateChanged(taskUUID, stepKey, sessionUUID, status string) {
	h.mu.RLock()
	clients := make([]*wsClient, 0, len(h.clients))
	for _, c := range h.clients {
		if c.taskUUID == taskUUID {
			clients = append(clients, c)
		}
	}
	h.mu.RUnlock()

	msg := map[string]interface{}{
		"type":         "executor.state_changed",
		"task_uuid":    taskUUID,
		"step_key":     stepKey,
		"session_uuid": sessionUUID,
		"status":       status,
	}

	for _, c := range clients {
		if err := writeClientJSON(c, msg); err != nil {
			h.mu.Lock()
			delete(h.clients, c.conn)
			h.mu.Unlock()
			c.conn.Close()
		}
	}

	// executor.state_changed is scoped to explicit session subscribers. Task
	// pages use the shared socket, so publish a lightweight global invalidation
	// event as well and let each page filter by task_uuid.
	h.BroadcastTaskChanged(taskUUID)
}

func writeClientJSON(client *wsClient, value interface{}) error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	_ = client.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
	return client.conn.WriteJSON(value)
}

// writeClientControl sends control frames (Pong / Close) under lock.
// gorilla/websocket explicitly forbids concurrent writes to the same connection; all write paths must go through client.writeMu,
// otherwise the Ping handler (read goroutine) and the broadcast / close paths would cause concurrent write panics or frame interleaving.
func writeClientControl(client *wsClient, messageType int, data []byte) error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	_ = client.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
	return client.conn.WriteMessage(messageType, data)
}
