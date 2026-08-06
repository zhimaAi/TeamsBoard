package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"goteams-client/internal/applog"
	"goteams-client/internal/projection"
	"goteams-client/internal/protocol"
	"goteams-client/internal/secrets"
)

// MessageHandler message processing function type
type MessageHandler func(data json.RawMessage) error

// WSClient cloud WebSocket client
type WSClient struct {
	cloudClient *Client
	db          *sql.DB
	store       secrets.Store

	mu          sync.RWMutex
	conn        *websocket.Conn
	isConnected bool
	cancelFunc  context.CancelFunc

	// writeMu serializes all write operations to conn (gorilla/websocket does not support concurrent writes).
	// heartbeat, SendMessage, and Stop belong to different goroutines, and this lock must be used to avoid frame interleaving damage.
	writeMu sync.Mutex

	handlers         map[string]MessageHandler
	connChangeCBs    []func(bool)
	pendingSnapshots chan string // Task UUID to be sent

	// syncMu protects flushDirtyTasks to avoid concurrent execution of client cycle and cloud pull request
	syncMu            sync.Mutex
	reconnectAttempts int
}

// syncFlushInterval Client synchronization cycle: periodically scan local dirty tasks and push them to the cloud.
// As a back-up to "push changes", it ensures that even if there is no connection or missing tags during the change, the final synchronization can be achieved.
// The cycle should not be too short: there may be many dirty tasks (such as hundreds), and excessive scanning + pushing will increase the burden on the cloud and local.
const syncFlushInterval = 30 * time.Minute

// NewWSClient creates a cloud WebSocket client
func NewWSClient(cloudClient *Client, db *sql.DB, store secrets.Store) *WSClient {
	wsc := &WSClient{
		cloudClient:      cloudClient,
		db:               db,
		store:            store,
		handlers:         make(map[string]MessageHandler),
		pendingSnapshots: make(chan string, 100),
	}
	wsc.registerDefaultHandlers()
	return wsc
}

// registerDefaultHandlers register default message handlers
func (w *WSClient) registerDefaultHandlers() {
	w.handlers[protocol.MsgConnectionReady] = w.handleConnectionReady
	w.handlers[protocol.MsgTaskSyncRequest] = w.handleTaskSyncRequest
	w.handlers[protocol.MsgTaskSnapshotACK] = w.handleTaskSnapshotACK
	w.handlers[protocol.MsgAgentAuthorizationChanged] = w.handleAgentAuthorizationChanged
	w.handlers[protocol.MsgError] = w.handleError
	w.handlers[protocol.MsgDeviceRevoked] = w.handleDeviceRevoked
	w.handlers[protocol.MsgClientUpgradeRequired] = w.handleClientUpgradeRequired
}

// Start starts the WebSocket client (including automatic reconnection)
func (w *WSClient) Start(ctx context.Context) {
	innerCtx, cancel := context.WithCancel(ctx)
	w.mu.Lock()
	w.cancelFunc = cancel
	w.mu.Unlock()

	go w.connectLoop(innerCtx)
	go w.syncFlushLoop(innerCtx)
}

// Stop stops the WebSocket client
func (w *WSClient) Stop() {
	w.mu.Lock()
	if w.cancelFunc != nil {
		w.cancelFunc()
		w.cancelFunc = nil
	}
	conn := w.conn
	w.conn = nil
	w.isConnected = false
	w.mu.Unlock()

	if conn != nil {
		w.writeMu.Lock()
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(protocol.CloseCodeNormal, "客户端主动关闭"))
		conn.Close()
		w.writeMu.Unlock()
	}
}

// IsConnected returns connection status
func (w *WSClient) IsConnected() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.isConnected
}

// OnConnectionChange registers connection status change callback
func (w *WSClient) OnConnectionChange(cb func(connected bool)) {
	w.mu.Lock()
	w.connChangeCBs = append(w.connChangeCBs, cb)
	w.mu.Unlock()
}

// notifyConnectionChange notifies connection status changes
func (w *WSClient) notifyConnectionChange(connected bool) {
	w.mu.RLock()
	cbs := make([]func(bool), len(w.connChangeCBs))
	copy(cbs, w.connChangeCBs)
	w.mu.RUnlock()
	for _, cb := range cbs {
		cb(connected)
	}
}

// connectLoop connection loop (including exponential backoff reconnection)
func (w *WSClient) connectLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := w.connectAndRun(ctx)
		if err != nil {
			remote := ""
			if w.cloudClient != nil {
				remote = w.cloudClient.BaseURL()
			}
			applog.Warn("[WSClient] 连接断开", "url", remote, "error", err)
		}

		w.setConnected(false)

		select {
		case <-ctx.Done():
			return
		case <-time.After(w.nextBackoff()):
		}
	}
}

// nextBackoff calculates the backoff time for the next reconnection
func (w *WSClient) nextBackoff() time.Duration {
	base := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second,
	}

	w.mu.Lock()
	idx := w.reconnectAttempts
	w.reconnectAttempts++
	w.mu.Unlock()

	if idx >= len(base) {
		idx = len(base) - 1
	}

	//Add random jitter (0-1 seconds)
	jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
	return base[idx] + jitter
}

// transportProxy reuses the proxy selection logic of the cloud HTTP client.
//
// httpClient.Transport may be wrapped by middleware into a non-*http.Transport implementation,
// Naked type assertion will directly panic and bring down the entire WS connection, so comma-ok is used here to safely downgrade:
// Return to http.ProxyFromEnvironment when the custom Transport cannot be obtained. The behavior is consistent with the default of the standard library.
func (w *WSClient) transportProxy() func(*http.Request) (*url.URL, error) {
	if w.cloudClient == nil || w.cloudClient.httpClient == nil {
		return http.ProxyFromEnvironment
	}
	tr, ok := w.cloudClient.httpClient.Transport.(*http.Transport)
	if !ok || tr == nil || tr.Proxy == nil {
		return http.ProxyFromEnvironment
	}
	return tr.Proxy
}

// connectAndRun establishes the connection and runs the read loop
func (w *WSClient) connectAndRun(ctx context.Context) error {
	if !w.cloudClient.IsConfigured() {
		return fmt.Errorf("云端 API 地址未配置")
	}

	jwt, err := w.getJWT()
	if err != nil || jwt == "" {
		return fmt.Errorf("未登录，无 JWT")
	}

	// Construct WebSocket URL
	wsURL := w.cloudClient.baseURL + "/api/ws/agent"
	// Replace http(s) with ws(s)
	if len(wsURL) > 4 && wsURL[:5] == "https" {
		wsURL = "wss" + wsURL[5:]
	} else if len(wsURL) > 4 && wsURL[:4] == "http" {
		wsURL = "ws" + wsURL[4:]
	}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+jwt)

	// WebSocket authentication uses JWT uniformly and no longer carries device-related request headers.
	if w.cloudClient.cloudCfg != nil && w.cloudClient.cloudCfg.ClientVersion != "" {
		headers.Set("X-GoTeams-Client-Version", w.cloudClient.cloudCfg.ClientVersion)
	}
	headers.Set("X-GoTeams-Protocol-Version", fmt.Sprintf("%d", protocol.ProtocolVersion))

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Subprotocols:     []string{protocol.SubProtocol},
		//Reuse the Transport of the cloud HTTP client (the system proxy has been bypassed for the cloud host)
		NetDialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		Proxy:          w.transportProxy(),
	}

	// Debug log: Print the actual handshake parameters sent (desensitized Authorization) to facilitate troubleshooting handshake rejection.
	hdrs := make(map[string]string)
	for k, vals := range headers {
		if len(vals) == 0 {
			continue
		}
		if k == "Authorization" {
			n := len(vals[0]) - len("Bearer ")
			if n < 0 {
				n = 0
			}
			hdrs[k] = fmt.Sprintf("Bearer <len=%d>", n)
		} else {
			hdrs[k] = strings.Join(vals, ", ")
		}
	}
	applog.Debug("[WSClient] 发起 WebSocket 握手",
		"url", wsURL,
		"subprotocol", protocol.SubProtocol,
		"has_jwt", jwt != "",
		"headers", hdrs)

	conn, resp, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			applog.Warn("[WSClient] WebSocket 握手失败诊断",
				"url", wsURL,
				"status", resp.StatusCode,
				"resp_body", redactResponseBody(string(body)),
				"resp_headers", redactResponseHeaders(resp.Header))
			return fmt.Errorf("WebSocket 握手失败: url=%s status=%d", wsURL, resp.StatusCode)
		}
		return fmt.Errorf("WebSocket 连接失败: url=%s %w", wsURL, err)
	}

	w.mu.Lock()
	w.conn = conn
	w.reconnectAttempts = 0
	w.mu.Unlock()

	// Start heartbeat
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	go w.heartbeatLoop(heartbeatCtx)

	// read loop
	err = w.readLoop(ctx)

	heartbeatCancel()
	conn.Close()

	w.mu.Lock()
	w.conn = nil
	w.mu.Unlock()

	return err
}

// readLoop reads message loop
func (w *WSClient) readLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		w.mu.RLock()
		conn := w.conn
		w.mu.RUnlock()
		if conn == nil {
			return fmt.Errorf("连接已关闭")
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("读取消息失败: %w", err)
		}

		var envelope protocol.Envelope
		if err := json.Unmarshal(message, &envelope); err != nil {
			applog.Warn("[WSClient] 解析消息失败", "error", err)
			continue
		}

		// Distribute to processor
		if handler, ok := w.handlers[envelope.Type]; ok {
			if err := handler(envelope.Data); err != nil {
				applog.Warn("[WSClient] 处理消息失败", "type", envelope.Type, "error", err)
			}
		} else {
			applog.Debug("[WSClient] 未处理的消息类型", "type", envelope.Type)
		}
	}
}

// heartbeatLoop heartbeat loop
func (w *WSClient) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(protocol.PingIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.mu.RLock()
			conn := w.conn
			w.mu.RUnlock()
			if conn == nil {
				return
			}
			w.writeMu.Lock()
			err := conn.WriteMessage(websocket.PingMessage, nil)
			w.writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// setConnected sets the connection status
func (w *WSClient) setConnected(connected bool) {
	w.mu.Lock()
	w.isConnected = connected
	w.mu.Unlock()
	w.notifyConnectionChange(connected)
}

// SendMessage Send message
func (w *WSClient) SendMessage(msgType, eventID string, data interface{}) error {
	w.mu.RLock()
	conn := w.conn
	w.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("WebSocket 未连接")
	}

	envelope, err := protocol.NewEnvelope(msgType, eventID, time.Now().UTC().Format(time.RFC3339), data)
	if err != nil {
		return fmt.Errorf("创建消息失败: %w", err)
	}

	message, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	w.writeMu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, message)
	w.writeMu.Unlock()
	if err != nil {
		return fmt.Errorf("发送消息失败: %w", err)
	}

	return nil
}

// SendTaskSnapshot Send task snapshot
func (w *WSClient) SendTaskSnapshot(snapshot *protocol.TaskSnapshotData) error {
	eventID := ""
	if w.db != nil {
		// Generate event_id
		var pendingID string
		err := w.db.QueryRow("SELECT pending_event_id FROM gt_task_sync_state WHERE task_uuid = ?", snapshot.LocalTaskUUID).Scan(&pendingID)
		if err == nil && pendingID != "" {
			eventID = pendingID
		}
	}

	return w.SendMessage(protocol.MsgTaskSnapshot, eventID, snapshot)
}

// SendTaskSyncCompleted Send synchronization completed
func (w *WSClient) SendTaskSyncCompleted(reqID string, attempted, acked, failed, dirty int) error {
	data := protocol.TaskSyncCompletedData{
		RequestID:      reqID,
		Attempted:      attempted,
		Acked:          acked,
		Failed:         failed,
		RemainingDirty: dirty,
	}
	return w.SendMessage(protocol.MsgTaskSyncCompleted, "", data)
}

// SendTaskDelete notifies the cloud to delete the corresponding local task
func (w *WSClient) SendTaskDelete(localTaskUUID string) error {
	data := protocol.TaskDeleteData{LocalTaskUUID: localTaskUUID}
	return w.SendMessage(protocol.MsgTaskDelete, "", data)
}

// flushPendingDeletes reissues the local deletion notification generated during the offline period, and clears the records to be deleted after success.
// Note: Under MaxOpenConns(1), you need to collect the UUID first and then close the cursor to avoid nested query deadlock.
func (w *WSClient) flushPendingDeletes() {
	if w.db == nil || !w.IsConnected() {
		return
	}
	w.syncMu.Lock()
	defer w.syncMu.Unlock()

	rows, err := w.db.Query("SELECT local_task_uuid FROM gt_pending_task_deletes")
	if err != nil {
		applog.Warn("[WSClient] 查询待删任务失败", "error", err)
		return
	}
	var pending []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err == nil {
			pending = append(pending, u)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		applog.Warn("[WSClient] 遍历待删任务失败", "error", err)
		return
	}

	for _, u := range pending {
		if err := w.SendTaskDelete(u); err != nil {
			applog.Warn("[WSClient] 发送删除通知失败", "task", u, "error", err)
			continue
		}
		if _, err := w.db.Exec("DELETE FROM gt_pending_task_deletes WHERE local_task_uuid = ?", u); err != nil {
			applog.Warn("[WSClient] 清除待删记录失败", "task", u, "error", err)
		}
	}
}

// ========== Default message handler ==========

// handleConnectionReady handles connection readiness
func (w *WSClient) handleConnectionReady(data json.RawMessage) error {
	var ready protocol.ConnectionReadyData
	if err := json.Unmarshal(data, &ready); err != nil {
		return err
	}
	if ready.ProtocolVersion != protocol.ProtocolVersion {
		return fmt.Errorf("云端协议版本不兼容: client=%d server=%d",
			protocol.ProtocolVersion, ready.ProtocolVersion)
	}
	w.setConnected(true)
	// Immediately push backlogged dirty tasks and to-be-deleted tasks after successful reconnection
	go w.flushDirtyTasks()
	go w.flushPendingDeletes()
	applog.Info("[WSClient] 连接就绪", "connection_id", ready.ConnectionID)
	return nil
}

// handleTaskSyncRequest handles the synchronization request issued by the cloud
func (w *WSClient) handleTaskSyncRequest(data json.RawMessage) error {
	var req protocol.TaskSyncRequestData
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}

	if w.db == nil {
		return nil
	}

	attempted, acked, failed, dirty := w.flushDirtyTasks()
	return w.SendTaskSyncCompleted(req.RequestID, attempted, acked, failed, dirty)
}

// flushDirtyTasks scans and pushes all tasks with sync_dirty=1.
// For reuse by the client's flush cycle (syncFlushLoop) and cloud pull request (handleTaskSyncRequest).
// Return the number of attempts, successes, failures, and remaining dirty numbers.
func (w *WSClient) flushDirtyTasks() (attempted, acked, failed, dirty int) {
	if w.db == nil {
		return 0, 0, 0, 0
	}

	//Only one flush is allowed to be executed at the same time (periodic loops and cloud pull requests may be triggered concurrently)
	w.syncMu.Lock()
	defer w.syncMu.Unlock()

	//Scan tasks with sync_dirty=1
	rows, err := w.db.Query("SELECT task_uuid FROM gt_task_sync_state WHERE sync_dirty = 1")
	if err != nil {
		applog.Warn("[WSClient] 查询脏任务失败", "error", err)
		return 0, 0, 0, 0
	}
	// Collect all dirty task UUIDs first, then close the cursor.
	// Note: The entire application under MaxOpenConns(1) shares the same SQLite connection.
	// If the cursor is not closed, calling buildSnapshotForSync to initiate a nested query will cause a deadlock——
	// The outer cursor occupies the only connection, and nested queries can never get the connection.
	var dirtyTaskUUIDs []string
	for rows.Next() {
		var taskUUID string
		if err := rows.Scan(&taskUUID); err != nil {
			continue
		}
		dirtyTaskUUIDs = append(dirtyTaskUUIDs, taskUUID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		applog.Warn("[WSClient] 遍历脏任务失败", "error", err)
		return 0, 0, 0, 0
	}

	for _, taskUUID := range dirtyTaskUUIDs {
		attempted++

		// Construct a complete projection and send it (at this time, the outer cursor has been closed, the connection has been released, and nested queries can be safely initiated)
		snapshot, err := projection.BuildSnapshot(w.db, taskUUID)
		if err != nil {
			applog.Warn("[WSClient] 构建快照失败", "task", taskUUID, "error", err)
			failed++
			continue
		}

		if err := w.SendTaskSnapshot(snapshot); err != nil {
			applog.Warn("[WSClient] 发送快照失败", "task", taskUUID, "error", err)
			failed++
			continue
		}
		acked++
	}

	dirty = len(dirtyTaskUUIDs) - acked
	return attempted, acked, failed, dirty
}

// syncFlushLoop client cycle: periodically push local dirty tasks to the cloud,
// Ensure that even if the connection is not connected when the change occurs, or a certain change path misses the dirty mark, the dirty tasks will eventually be synchronized.
func (w *WSClient) syncFlushLoop(ctx context.Context) {
	ticker := time.NewTicker(syncFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !w.IsConnected() {
				continue
			}
			w.flushDirtyTasks()
			w.flushPendingDeletes()
		}
	}
}

// handleTaskSnapshotACK handles snapshot confirmation
func (w *WSClient) handleTaskSnapshotACK(data json.RawMessage) error {
	var ack protocol.TaskSnapshotACKData
	if err := json.Unmarshal(data, &ack); err != nil {
		return err
	}

	if w.db == nil {
		return nil
	}

	// Verify that local_revision + projection_hash matches before clearing the dirty mark
	// Late ACKs must not clear dirty marks for newer revisions
	result, err := w.db.Exec(
		`UPDATE gt_task_sync_state
		 SET sync_dirty = 0, cloud_task_id = ?, last_uploaded_at = ?, last_error_code = '', last_error_message = ''
		 WHERE pending_event_id = ?
		   AND local_revision = ?
		   AND projection_hash = ?
		   AND sync_dirty = 1`,
		fmt.Sprintf("%d", ack.CloudTaskID), time.Now().UnixMilli(),
		ack.RequestEventID, ack.LocalRevision, ack.ProjectionHash)
	if err != nil {
		return fmt.Errorf("清除 dirty 标记失败: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取 ACK 影响行数失败: %w", err)
	}
	if rows == 0 {
		// It may be late ACK or revision has changed, no error will be reported
		applog.Debug("[WSClient] ACK 未匹配（可能迟到或 revision 已变化）",
			"event", ack.RequestEventID, "revision", ack.LocalRevision)
	}

	return nil
}

// handleAgentAuthorizationChanged handles Agent authorization changes
func (w *WSClient) handleAgentAuthorizationChanged(data json.RawMessage) error {
	var changed protocol.AgentAuthorizationChangedData
	if err := json.Unmarshal(data, &changed); err != nil {
		return err
	}
	applog.Info("[WSClient] Agent 授权变更",
		"agent_id", changed.AgentID, "allowed", changed.Allowed, "reason", changed.Reason)
	return nil
}

// handleError handles error messages
func (w *WSClient) handleError(data json.RawMessage) error {
	var errData protocol.ErrorData
	if err := json.Unmarshal(data, &errData); err != nil {
		return err
	}
	applog.Warn("[WSClient] 云端错误",
		"code", errData.Code, "message", errData.Message, "retryable", errData.Retryable)
	return nil
}

// handleDeviceRevoked handles device revocation
func (w *WSClient) handleDeviceRevoked(data json.RawMessage) error {
	var revoked protocol.DeviceRevokedData
	if err := json.Unmarshal(data, &revoked); err != nil {
		return err
	}
	applog.Warn("[WSClient] 设备已撤销", "device_uuid", revoked.DeviceUUID, "reason", revoked.Reason)
	// Clear device credentials
	if w.store != nil {
		if err := w.store.Delete(secrets.KeyDeviceCredential); err != nil {
			applog.Warn("[WSClient] 清除设备凭据失败", "error", err)
		}
	}
	return nil
}

// handleClientUpgradeRequired handles client upgrade requirements
func (w *WSClient) handleClientUpgradeRequired(data json.RawMessage) error {
	var upgrade protocol.ClientUpgradeRequiredData
	if err := json.Unmarshal(data, &upgrade); err != nil {
		return err
	}
	applog.Warn("[WSClient] 需要升级", "current", upgrade.CurrentVersion, "latest", upgrade.LatestVersion)
	return nil
}

// getJWT reads JWT from SecretStore
func (w *WSClient) getJWT() (string, error) {
	if w.store == nil {
		return "", fmt.Errorf("SecretStore 未初始化")
	}
	return w.store.Get(secrets.KeyJWT)
}

// getDeviceUUID Read device UUID from SecretStore
func (w *WSClient) getDeviceUUID() string {
	if w.store == nil {
		return ""
	}
	uuid, err := w.store.Get(secrets.KeyPrefix + "device_uuid")
	if err != nil {
		return ""
	}
	return uuid
}

// getDeviceCredential reads device credentials from SecretStore
func (w *WSClient) getDeviceCredential() string {
	if w.store == nil {
		return ""
	}
	cred, err := w.store.Get(secrets.KeyDeviceCredential)
	if err != nil {
		return ""
	}
	return cred
}

// redactResponseBody desensitizes the response body: truncates the length and hides the values of sensitive fields such as token/secret/password.
func redactResponseBody(body string) string {
	const maxLen = 500
	if len(body) > maxLen {
		body = body[:maxLen] + "...(truncated)"
	}
	re := regexp.MustCompile(`(?i)"(token|secret|password|credential|authorization|api[_ ]?key|cookie|set[- ]?cookie)"\s*:\s*"[^"]*"`)
	return re.ReplaceAllString(body, `"$1":"<redacted>"`)
}

// redactResponseHeaders desensitizes the response headers: hides headers that may carry session credentials such as Set-Cookie/Authorization.
func redactResponseHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vals := range h {
		if len(vals) == 0 {
			continue
		}
		if isSensitiveHeader(k) {
			out[k] = "<redacted>"
			continue
		}
		out[k] = strings.Join(vals, ", ")
	}
	return out
}

func isSensitiveHeader(name string) bool {
	lower := strings.ToLower(name)
	return lower == "set-cookie" || lower == "cookie" ||
		lower == "authorization" ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "credential") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "api_key") ||
		strings.Contains(lower, "apikey")
}
