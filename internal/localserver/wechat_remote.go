package localserver

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"

	"goteams-client/internal/applog"
	"goteams-client/internal/i18n"
	"goteams-client/internal/secrets"
	"goteams-client/internal/workflow"
)

const (
	wechatBotTokenKey        = "goteams:remote_wechat_bot_token"
	wechatContextTokenKey    = "goteams:remote_wechat_context_token"
	wechatQRStatusTimeout    = 5 * time.Second
	wechatUpdatesPollTimeout = 10 * time.Second
)

type wechatState struct {
	BotID           string
	SenderID        string
	Locale          string
	BaseURL         string
	Cursor          string
	BoundTaskUUID   string
	ContextRevision int64
	ConnectedAt     int64
}

type wechatLoginAttempt struct {
	ID           string
	QRCode       string
	ImageDataURL string
	Status       string
	VerifyCode   string
	Error        string
	BaseURL      string
	Locale       string
	CreatedAt    time.Time
	Cancel       context.CancelFunc
	Wake         chan struct{}
}

type wechatPendingFinish struct {
	id          string
	status      string
	taskUUID    string
	sessionUUID string
	reply       string
}

type wechatRemote struct {
	db           *sql.DB
	store        secrets.Store
	orchestrator *workflow.Orchestrator
	tasks        *TasksHandler
	api          *wechatAPI
	mu           sync.Mutex
	// A disconnect waits for an accepted command to finish its local handoff.
	dispatchMu            sync.RWMutex
	attempt               *wechatLoginAttempt
	pendingFinish         *wechatPendingFinish
	codexMonitors         map[string]struct{}
	lastError             string
	replyMessageIDMissing bool
	pollCancel            context.CancelFunc
	sendCancel            context.CancelFunc
	wake                  chan struct{}
	started               bool
	rootCtx               context.Context
	qrStatusTimeout       time.Duration
	updatesPollTimeout    time.Duration
}

func newWeChatRemote(db *sql.DB, store secrets.Store, orchestrator *workflow.Orchestrator, tasks *TasksHandler) *wechatRemote {
	return &wechatRemote{db: db, store: store, orchestrator: orchestrator, tasks: tasks, api: newWeChatAPI(),
		codexMonitors: make(map[string]struct{}), wake: make(chan struct{}, 1), qrStatusTimeout: wechatQRStatusTimeout,
		updatesPollTimeout: wechatUpdatesPollTimeout}
}

func (w *wechatRemote) registerRoutes(r *gin.RouterGroup) {
	r.GET("/wechat", w.getStatus)
	r.GET("/wechat/uncertain", w.listUncertain)
	r.POST("/wechat/uncertain/ack", w.ackUncertain)
	r.POST("/wechat/login", w.startLogin)
	r.GET("/wechat/login/:id", w.getLogin)
	r.POST("/wechat/login/:id/verify", w.verifyLogin)
	r.DELETE("/wechat", w.disconnect)
}

func (w *wechatRemote) start(ctx context.Context) {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.rootCtx = ctx
	w.mu.Unlock()
	// An interrupted handoff has an unknown external outcome. Do not replay it.
	rows, err := w.db.QueryContext(ctx, `SELECT message_id FROM gt_remote_wechat_inbox WHERE status='processing'`)
	if err == nil {
		var uncertain []string
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				uncertain = append(uncertain, id)
			}
		}
		_ = rows.Close()
		for _, id := range uncertain {
			_, _ = w.db.ExecContext(ctx, `UPDATE gt_remote_wechat_inbox SET status='uncertain', updated_at=? WHERE message_id=? AND status='processing'`, time.Now().UnixMilli(), id)
			_ = w.enqueue(ctx, "uncertain:"+id, "", w.tr(ctx, "remote_wechat_uncertain", nil))
		}
	}
	go w.pollLoop(ctx)
	go w.workLoop(ctx)
	go w.codexRecoveryLoop(ctx)
}

func (w *wechatRemote) codexRecoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := w.recoverCodexResults(ctx); err != nil && ctx.Err() == nil {
			applog.Warn("恢复微信 Codex 执行结果失败", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *wechatRemote) stop() {
	w.mu.Lock()
	if w.attempt != nil {
		w.attempt.Cancel()
	}
	if w.pollCancel != nil {
		w.pollCancel()
	}
	if w.sendCancel != nil {
		w.sendCancel()
	}
	w.mu.Unlock()
}

func (w *wechatRemote) signal() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *wechatRemote) readState(ctx context.Context) (wechatState, error) {
	var state wechatState
	err := w.db.QueryRowContext(ctx, `SELECT bot_id, sender_id, locale, base_url, cursor, bound_task_uuid,
		context_revision, connected_at FROM gt_remote_wechat_state WHERE id=1`).Scan(
		&state.BotID, &state.SenderID, &state.Locale, &state.BaseURL, &state.Cursor, &state.BoundTaskUUID,
		&state.ContextRevision, &state.ConnectedAt)
	return state, err
}

func remoteExecutionBlock(ctx context.Context, db *sql.DB, taskUUID string) (string, error) {
	var status string
	err := db.QueryRowContext(ctx, `SELECT status FROM gt_remote_wechat_inbox
		WHERE task_uuid=? AND status IN ('processing','dispatched','uncertain')
		ORDER BY CASE WHEN status='uncertain' THEN 1 ELSE 0 END, received_at LIMIT 1`, taskUUID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return status, err
}

func guardRemoteExecutionMutation(c *gin.Context, db *sql.DB, taskUUID string) bool {
	status, err := remoteExecutionBlock(c.Request.Context(), db, taskUUID)
	if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return false
	}
	if status == "uncertain" {
		i18n.Error(c, http.StatusConflict, "remote_wechat_uncertain_action_required", "wechat_uncertain_action_required")
		return false
	}
	if status != "" {
		i18n.Error(c, http.StatusConflict, "localserver_active_sessions", "task_has_active_sessions")
		return false
	}
	return true
}

func (w *wechatRemote) tr(ctx context.Context, key string, params i18n.Params) string {
	state, err := w.readState(ctx)
	if err != nil {
		return i18n.FormatWithLocale("zh-CN", key, params)
	}
	return i18n.FormatWithLocale(state.Locale, key, params)
}

func (w *wechatRemote) getStatus(c *gin.Context) {
	state, err := w.readState(c.Request.Context())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		i18n.Error(c, http.StatusInternalServerError, "remote_wechat_state_error", "wechat_state_error")
		return
	}
	w.mu.Lock()
	attempt := w.attempt
	lastError := w.lastError
	replyMessageIDMissing := w.replyMessageIDMissing
	loginID, loginStatus := "", ""
	if attempt != nil {
		loginID, loginStatus = attempt.ID, attempt.Status
	}
	w.mu.Unlock()
	status := "disconnected"
	if err == nil {
		status = "connected"
	} else if loginID != "" && loginStatus != "expired" && loginStatus != "error" {
		status = "connecting"
	}
	c.JSON(http.StatusOK, gin.H{"status": status, "connected_at": state.ConnectedAt, "login_id": loginID,
		"last_error": lastError, "reply_message_id_missing": status == "connected" && replyMessageIDMissing})
}

func (w *wechatRemote) listUncertain(c *gin.Context) {
	rows, err := w.db.QueryContext(c.Request.Context(), `SELECT i.message_id, i.task_uuid, COALESCE(t.title,''), i.received_at
		FROM gt_remote_wechat_inbox i LEFT JOIN gt_tasks t ON t.uuid=i.task_uuid
		WHERE i.status='uncertain' ORDER BY i.received_at, i.rowid`)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "remote_wechat_state_error", "wechat_state_error")
		return
	}
	defer rows.Close()
	items := make([]gin.H, 0)
	for rows.Next() {
		var messageID, taskUUID, title string
		var receivedAt int64
		if err := rows.Scan(&messageID, &taskUUID, &title, &receivedAt); err != nil {
			i18n.Error(c, http.StatusInternalServerError, "remote_wechat_state_error", "wechat_state_error")
			return
		}
		items = append(items, gin.H{"message_id": messageID, "task_uuid": taskUUID, "task_title": title, "received_at": receivedAt})
	}
	if rows.Err() != nil {
		i18n.Error(c, http.StatusInternalServerError, "remote_wechat_state_error", "wechat_state_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (w *wechatRemote) ackUncertain(c *gin.Context) {
	var body struct {
		MessageID string `json:"message_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.MessageID) == "" {
		i18n.Error(c, http.StatusBadRequest, "common_request_invalid", "")
		return
	}
	result, err := w.db.ExecContext(c.Request.Context(), `UPDATE gt_remote_wechat_inbox SET status='acknowledged', updated_at=?
		WHERE message_id=? AND status='uncertain'`, time.Now().UnixMilli(), body.MessageID)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "remote_wechat_state_error", "wechat_state_error")
		return
	}
	updated, err := result.RowsAffected()
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "remote_wechat_state_error", "wechat_state_error")
		return
	}
	if updated == 0 {
		i18n.Error(c, http.StatusConflict, "remote_wechat_uncertain_ack_missing", "wechat_uncertain_ack_missing")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "acknowledged"})
}

func (w *wechatRemote) startLogin(c *gin.Context) {
	if _, err := w.readState(c.Request.Context()); err == nil {
		i18n.Error(c, http.StatusConflict, "remote_wechat_connected", "wechat_connected")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		i18n.Error(c, http.StatusInternalServerError, "remote_wechat_state_error", "wechat_state_error")
		return
	}
	qr, err := w.api.getQR(c.Request.Context())
	if err != nil {
		i18n.Error(c, http.StatusBadGateway, "remote_wechat_qr_error", "wechat_qr_error")
		return
	}
	png, err := qrcode.Encode(qr.ImageContent, qrcode.Medium, 224)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "remote_wechat_qr_render_error", "wechat_qr_error")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	locale := i18n.Normalize(c.GetHeader("lang"))
	attempt := &wechatLoginAttempt{ID: uuid.NewString(), QRCode: qr.QRCode, ImageDataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), Status: "waiting", BaseURL: wechatDefaultBaseURL, Locale: locale, CreatedAt: time.Now(), Cancel: cancel, Wake: make(chan struct{}, 1)}
	w.mu.Lock()
	// A different login may have completed while getQR was in flight.
	if _, err := w.readState(c.Request.Context()); err == nil {
		w.mu.Unlock()
		cancel()
		i18n.Error(c, http.StatusConflict, "remote_wechat_connected", "wechat_connected")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		w.mu.Unlock()
		cancel()
		i18n.Error(c, http.StatusInternalServerError, "remote_wechat_state_error", "wechat_state_error")
		return
	}
	if w.attempt != nil {
		w.attempt.Cancel()
	}
	w.attempt = attempt
	w.lastError = ""
	w.mu.Unlock()
	go w.pollLogin(ctx, attempt)
	c.JSON(http.StatusCreated, gin.H{"id": attempt.ID, "status": attempt.Status, "qr_image": attempt.ImageDataURL})
}

func (w *wechatRemote) getLogin(c *gin.Context) {
	w.mu.Lock()
	attempt := w.attempt
	if attempt == nil || attempt.ID != c.Param("id") {
		w.mu.Unlock()
		i18n.Error(c, http.StatusNotFound, "remote_wechat_login_not_found", "wechat_login_not_found")
		return
	}
	response := gin.H{"id": attempt.ID, "status": attempt.Status, "qr_image": attempt.ImageDataURL, "error": attempt.Error}
	w.mu.Unlock()
	c.JSON(http.StatusOK, response)
}

func (w *wechatRemote) verifyLogin(c *gin.Context) {
	var body struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(strings.TrimSpace(body.Code)) < 4 || len(strings.TrimSpace(body.Code)) > 16 {
		i18n.Error(c, http.StatusBadRequest, "remote_wechat_verify_invalid", "wechat_verify_invalid")
		return
	}
	w.mu.Lock()
	attempt := w.attempt
	if attempt == nil || attempt.ID != c.Param("id") || attempt.Status != "need_verifycode" {
		w.mu.Unlock()
		i18n.Error(c, http.StatusConflict, "remote_wechat_login_not_waiting", "wechat_login_not_waiting")
		return
	}
	attempt.VerifyCode = strings.TrimSpace(body.Code)
	attempt.Status = "verifying"
	select {
	case attempt.Wake <- struct{}{}:
	default:
	}
	w.mu.Unlock()
	c.JSON(http.StatusAccepted, gin.H{"status": "verifying"})
}

func (w *wechatRemote) pollLogin(ctx context.Context, attempt *wechatLoginAttempt) {
	defer attempt.Cancel()
	loginCtx, cancelDeadline := context.WithDeadline(ctx, attempt.CreatedAt.Add(5*time.Minute))
	defer cancelDeadline()
loginPoll:
	for loginCtx.Err() == nil {
		w.mu.Lock()
		if w.attempt != attempt {
			w.mu.Unlock()
			return
		}
		base, code := attempt.BaseURL, attempt.VerifyCode
		w.mu.Unlock()
		// The upstream endpoint can hold an unchanged status for a long poll. A
		// shorter deadline makes a fresh request observe a scan confirmation
		// without waiting for the old long poll to finish.
		requestCtx, cancelRequest := context.WithTimeout(loginCtx, w.qrStatusTimeout)
		status, err := w.api.qrStatus(requestCtx, base, attempt.QRCode, code)
		requestTimedOut := errors.Is(requestCtx.Err(), context.DeadlineExceeded)
		cancelRequest()
		if ctx.Err() != nil {
			return
		}
		if loginCtx.Err() != nil {
			break
		}
		if err != nil && requestTimedOut {
			continue
		}
		if err != nil {
			select {
			case <-loginCtx.Done():
				if ctx.Err() != nil {
					return
				}
				break loginPoll
			case <-time.After(2 * time.Second):
				continue
			}
		}
		w.mu.Lock()
		if w.attempt != attempt {
			w.mu.Unlock()
			return
		}
		if code != "" {
			attempt.VerifyCode = ""
		}
		switch status.Status {
		case "confirmed":
			w.mu.Unlock()
			w.completeLogin(attempt, status)
			return
		case "scaned_but_redirect":
			if status.RedirectHost != "" {
				redirect := status.RedirectHost
				if !strings.HasPrefix(redirect, "https://") {
					redirect = "https://" + redirect
				}
				if next, err := validWeChatBase(redirect); err == nil {
					attempt.BaseURL = next
				}
			}
			attempt.Status = "scanned"
		case "scaned":
			attempt.Status = "scanned"
		case "need_verifycode":
			attempt.Status = "need_verifycode"
		case "verify_code_blocked", "expired":
			attempt.Status = "expired"
			w.mu.Unlock()
			return
		case "binded_redirect":
			attempt.Status = "error"
			attempt.Error = i18n.WithLocale(attempt.Locale, "remote_wechat_connect_failed")
			w.mu.Unlock()
			return
		default:
			attempt.Status = "waiting"
		}
		needVerify := attempt.Status == "need_verifycode"
		w.mu.Unlock()
		if needVerify {
			select {
			case <-loginCtx.Done():
				if ctx.Err() != nil {
					return
				}
				break loginPoll
			case <-attempt.Wake:
			}
		} else {
			select {
			case <-loginCtx.Done():
				if ctx.Err() != nil {
					return
				}
				break loginPoll
			case <-time.After(time.Second):
			}
		}
	}
	w.mu.Lock()
	if w.attempt == attempt {
		attempt.Status = "expired"
	}
	w.mu.Unlock()
}

func (w *wechatRemote) completeLogin(attempt *wechatLoginAttempt, status wechatQRStatus) {
	if status.BotID == "" || status.UserID == "" || status.BotToken == "" {
		w.loginFailed(attempt, "remote_wechat_connect_failed")
		return
	}
	base := status.BaseURL
	if base == "" {
		base = wechatDefaultBaseURL
	}
	trusted, err := validWeChatBase(base)
	if err != nil {
		w.loginFailed(attempt, "remote_wechat_connect_failed")
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.attempt != attempt {
		return
	}
	// Do not replace credentials belonging to a connection that won the login race.
	if _, err = w.readState(context.Background()); err == nil {
		attempt.Status = "error"
		attempt.Error = i18n.WithLocale(attempt.Locale, "remote_wechat_connected")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		attempt.Status = "error"
		attempt.Error = i18n.WithLocale(attempt.Locale, "remote_wechat_connect_failed")
		return
	}
	previousBotToken, botErr := w.store.Get(wechatBotTokenKey)
	if botErr != nil && !errors.Is(botErr, secrets.ErrNotFound) {
		attempt.Status = "error"
		attempt.Error = i18n.WithLocale(attempt.Locale, "remote_wechat_connect_failed")
		return
	}
	previousContextToken, contextErr := w.store.Get(wechatContextTokenKey)
	if contextErr != nil && !errors.Is(contextErr, secrets.ErrNotFound) {
		attempt.Status = "error"
		attempt.Error = i18n.WithLocale(attempt.Locale, "remote_wechat_connect_failed")
		return
	}
	restoreCredentials := func() {
		if botErr == nil {
			_ = w.store.Set(wechatBotTokenKey, previousBotToken)
		} else {
			_ = w.store.Delete(wechatBotTokenKey)
		}
		if contextErr == nil {
			_ = w.store.Set(wechatContextTokenKey, previousContextToken)
		}
	}
	if err = w.store.Set(wechatBotTokenKey, status.BotToken); err != nil {
		restoreCredentials()
		attempt.Status = "error"
		attempt.Error = i18n.WithLocale(attempt.Locale, "remote_wechat_connect_failed")
		return
	}
	if err = w.store.Delete(wechatContextTokenKey); err != nil && !errors.Is(err, secrets.ErrNotFound) {
		restoreCredentials()
		attempt.Status = "error"
		attempt.Error = i18n.WithLocale(attempt.Locale, "remote_wechat_connect_failed")
		return
	}
	now := time.Now().UnixMilli()
	_, err = w.db.Exec(`INSERT INTO gt_remote_wechat_state
		(id, bot_id, sender_id, locale, base_url, cursor, bound_task_uuid, context_revision, connected_at, updated_at)
		VALUES (1, ?, ?, ?, ?, '', '', 0, ?, ?)`, status.BotID, status.UserID, attempt.Locale, trusted, now, now)
	if err != nil {
		restoreCredentials()
		attempt.Status = "error"
		attempt.Error = i18n.WithLocale(attempt.Locale, "remote_wechat_connect_failed")
		return
	}
	attempt.Status = "connected"
	w.replyMessageIDMissing = false
	w.signal()
}

func (w *wechatRemote) loginFailed(attempt *wechatLoginAttempt, key string) {
	w.mu.Lock()
	if w.attempt == attempt {
		attempt.Status = "error"
		attempt.Error = i18n.WithLocale(attempt.Locale, key)
		w.lastError = key
	}
	w.mu.Unlock()
}

func (w *wechatRemote) disconnect(c *gin.Context) {
	notified, err := w.clearConnection(c.Request.Context(), nil, true, "")
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "remote_wechat_disconnect_failed", "wechat_disconnect_error")
		return
	}
	w.broadcastConnectionStatus("disconnected")
	c.JSON(http.StatusOK, gin.H{"status": "disconnected", "wechat_notified": notified})
}

func (w *wechatRemote) broadcastConnectionStatus(status string) {
	if w.tasks == nil || w.tasks.wsHub == nil {
		return
	}
	w.tasks.wsHub.BroadcastToAll("remote.wechat.status", map[string]string{"status": status})
}

// clearConnection stops local work for one connection. The expected snapshot
// prevents a late invalid-token response from clearing a newer QR login.
func (w *wechatRemote) clearConnection(ctx context.Context, expected *wechatState, notifyStop bool, reason string) (bool, error) {
	w.dispatchMu.Lock()
	defer w.dispatchMu.Unlock()
	w.mu.Lock()
	defer w.mu.Unlock()
	state, stateErr := w.readState(ctx)
	if stateErr != nil && !errors.Is(stateErr, sql.ErrNoRows) {
		return false, stateErr
	}
	if expected != nil && (stateErr != nil || state.BotID != expected.BotID || state.ConnectedAt != expected.ConnectedAt) {
		return false, nil
	}
	if w.attempt != nil {
		w.attempt.Cancel()
		w.attempt = nil
	}
	if w.pollCancel != nil {
		w.pollCancel()
		w.pollCancel = nil
	}
	if w.sendCancel != nil {
		w.sendCancel()
		w.sendCancel = nil
	}
	notified := errors.Is(stateErr, sql.ErrNoRows)
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return notified, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM gt_remote_wechat_state WHERE id=1`); err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE gt_remote_wechat_outbox SET status='cancelled' WHERE status='pending'`)
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE gt_remote_wechat_inbox SET status='cancelled', updated_at=? WHERE status='pending'`, time.Now().UnixMilli())
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE gt_remote_wechat_inbox SET status='uncertain', updated_at=? WHERE status IN ('processing','dispatched')`, time.Now().UnixMilli())
	}
	if err != nil {
		return notified, err
	}
	if err = tx.Commit(); err != nil {
		return notified, err
	}
	if notifyStop && stateErr == nil {
		if token, tokenErr := w.store.Get(wechatBotTokenKey); tokenErr == nil && token != "" {
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			notifyErr := w.api.notifyLifecycle(callCtx, state.BaseURL, token, false)
			cancel()
			notified = notifyErr == nil
			if notifyErr != nil {
				applog.Warn("微信通道停止通知失败", "error", notifyErr)
			}
		}
	}
	botErr := w.store.Delete(wechatBotTokenKey)
	contextErr := w.store.Delete(wechatContextTokenKey)
	if (botErr != nil && !errors.Is(botErr, secrets.ErrNotFound)) ||
		(contextErr != nil && !errors.Is(contextErr, secrets.ErrNotFound)) {
		w.lastError = "wechat_secret_error"
		return notified, errors.New("删除微信通道凭据失败")
	}
	w.lastError = reason
	w.replyMessageIDMissing = false
	w.signal()
	return notified, nil
}

func (w *wechatRemote) pollLoop(ctx context.Context) {
	announcedConnection := ""
	for ctx.Err() == nil {
		state, err := w.readState(ctx)
		if err != nil {
			announcedConnection = ""
			select {
			case <-ctx.Done():
				return
			case <-w.wake:
				continue
			case <-time.After(3 * time.Second):
				continue
			}
		}
		token, err := w.store.Get(wechatBotTokenKey)
		if err != nil || token == "" {
			w.setError("wechat_credential_missing")
			select {
			case <-ctx.Done():
				return
			case <-w.wake:
				continue
			case <-time.After(5 * time.Second):
				continue
			}
		}
		connectionID := fmt.Sprintf("%s:%d", state.BotID, state.ConnectedAt)
		if announcedConnection != connectionID {
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			notifyErr := w.api.notifyLifecycle(callCtx, state.BaseURL, token, true)
			cancel()
			if notifyErr != nil {
				applog.Warn("微信通道启动通知失败", "error", notifyErr)
			} else {
				announcedConnection = connectionID
			}
		}
		// A bounded long poll periodically rechecks token validity even when the
		// upstream server keeps an old request open after a WeChat-side change.
		pollCtx, cancel := context.WithTimeout(ctx, w.updatesPollTimeout)
		w.mu.Lock()
		w.pollCancel = cancel
		w.mu.Unlock()
		updates, err := w.api.getUpdates(pollCtx, state.BaseURL, token, state.Cursor)
		pollErr := pollCtx.Err()
		cancel()
		w.mu.Lock()
		w.pollCancel = nil
		w.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			var httpErr *wechatHTTPError
			if errors.Is(err, errWeChatSessionExpired) || (errors.As(err, &httpErr) && (httpErr.StatusCode == 401 || httpErr.StatusCode == 403)) {
				if _, clearErr := w.clearConnection(ctx, &state, false, "wechat_session_expired"); clearErr != nil {
					applog.Warn("清理失效微信连接失败", "error", clearErr)
				} else if _, stateErr := w.readState(ctx); errors.Is(stateErr, sql.ErrNoRows) {
					w.broadcastConnectionStatus("disconnected")
				}
				announcedConnection = ""
				continue
			}
			if errors.Is(pollErr, context.DeadlineExceeded) {
				continue
			}
			if pollErr == nil {
				w.setError("wechat_poll_error")
			}
			select {
			case <-ctx.Done():
				return
			case <-w.wake:
				continue
			case <-time.After(3 * time.Second):
				continue
			}
		}
		w.setError("")
		if err = w.saveUpdates(ctx, state, updates); err != nil {
			applog.Warn("保存微信消息失败", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func (w *wechatRemote) setError(message string) {
	w.mu.Lock()
	w.lastError = message
	w.mu.Unlock()
}

func (w *wechatRemote) setReplyMessageIDMissing(ctx context.Context, state wechatState, missing bool) {
	w.mu.Lock()
	current, err := w.readState(ctx)
	if err != nil || current.BotID != state.BotID || current.ConnectedAt != state.ConnectedAt {
		w.mu.Unlock()
		return
	}
	changed := w.replyMessageIDMissing != missing
	w.replyMessageIDMissing = missing
	w.mu.Unlock()
	if changed {
		w.broadcastConnectionStatus("connected")
	}
}

func (w *wechatRemote) saveUpdates(ctx context.Context, state wechatState, updates wechatUpdates) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var currentBotID string
	if err := w.db.QueryRowContext(ctx, `SELECT bot_id FROM gt_remote_wechat_state WHERE id=1`).Scan(&currentBotID); err != nil {
		return err
	}
	if currentBotID != state.BotID {
		return errors.New("微信连接已更换")
	}
	// The credential-like context token is written before advancing the cursor.
	for _, msg := range updates.Messages {
		if msg.FromUserID == state.SenderID && msg.GroupID == "" && msg.ContextToken != "" {
			if err := w.store.Set(wechatContextTokenKey, msg.ContextToken); err != nil {
				return err
			}
		}
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var botID string
	if err = tx.QueryRowContext(ctx, `SELECT bot_id FROM gt_remote_wechat_state WHERE id=1`).Scan(&botID); err != nil {
		return err
	}
	if botID != state.BotID {
		return errors.New("微信连接已更换")
	}
	now := time.Now().UnixMilli()
	for _, msg := range updates.Messages {
		if msg.FromUserID != state.SenderID || msg.GroupID != "" || msg.MessageType != 1 || msg.id() == "" || msg.text() == "" {
			continue
		}
		insertedResult, insertErr := tx.ExecContext(ctx, `INSERT OR IGNORE INTO gt_remote_wechat_inbox(message_id, text_content, received_at, updated_at) VALUES (?, ?, ?, ?)`,
			state.BotID+":"+msg.id(), msg.text(), now, now)
		if insertErr != nil {
			return insertErr
		}
		inserted, rowsErr := insertedResult.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		if inserted > 0 && msg.ContextToken != "" {
			if _, err = tx.ExecContext(ctx, `UPDATE gt_remote_wechat_state SET context_revision=context_revision+1 WHERE id=1`); err != nil {
				return err
			}
		}
	}
	if updates.Cursor != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE gt_remote_wechat_state SET cursor=?, updated_at=? WHERE id=1`, updates.Cursor, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (w *wechatRemote) workLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := w.processInbox(ctx); err != nil {
			applog.Warn("处理微信指令失败", "error", err)
		}
		if err := w.collectResults(ctx); err != nil {
			applog.Warn("收集微信任务结果失败", "error", err)
		}
		if err := w.deliverOutbox(ctx); err != nil {
			applog.Warn("发送微信消息失败", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *wechatRemote) enqueue(ctx context.Context, sourceID, taskUUID, content string) error {
	if strings.TrimSpace(content) == "" {
		content = w.tr(ctx, "remote_wechat_final_empty", nil)
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := w.enqueueTx(ctx, tx, sourceID, taskUUID, content); err != nil {
		return err
	}
	return tx.Commit()
}

func (w *wechatRemote) enqueueTx(ctx context.Context, tx *sql.Tx, sourceID, taskUUID, content string) error {
	if !utf8.ValidString(content) {
		return errors.New("微信回复包含无效 UTF-8")
	}
	// Keep each message small enough for the WeChat text endpoint, preserving rune boundaries.
	runes := []rune(content)
	for start, index := 0, 0; start < len(runes); start, index = start+1500, index+1 {
		end := min(start+1500, len(runes))
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO gt_remote_wechat_outbox(id, source_id, task_uuid, content, next_attempt_at, created_at)
			SELECT ?, ?, ?, ?, 0, ? WHERE EXISTS (SELECT 1 FROM gt_remote_wechat_state WHERE id=1)`,
			uuid.NewString(), fmt.Sprintf("%s:%d", sourceID, index), taskUUID, string(runes[start:end]), time.Now().UnixMilli())
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *wechatRemote) deliverOutbox(ctx context.Context) error {
	state, err := w.readState(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	token, err := w.store.Get(wechatBotTokenKey)
	if err != nil {
		return err
	}
	contextToken, err := w.store.Get(wechatContextTokenKey)
	if errors.Is(err, secrets.ErrNotFound) || contextToken == "" {
		return nil
	}
	if err != nil {
		return err
	}
	var id, content string
	var attempts int
	err = w.db.QueryRowContext(ctx, `SELECT id, content, attempts FROM gt_remote_wechat_outbox
		WHERE status='pending' AND next_attempt_at<=? AND blocked_context_revision<>? ORDER BY created_at, rowid LIMIT 1`, time.Now().UnixMilli(), state.ContextRevision).Scan(&id, &content, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	w.mu.Lock()
	current, stateErr := w.readState(ctx)
	if stateErr != nil || current.BotID != state.BotID || current.SenderID != state.SenderID || current.ConnectedAt != state.ConnectedAt {
		w.mu.Unlock()
		return stateErr
	}
	var pendingID string
	stateErr = w.db.QueryRowContext(ctx, `SELECT id FROM gt_remote_wechat_outbox WHERE id=? AND status='pending'`, id).Scan(&pendingID)
	if errors.Is(stateErr, sql.ErrNoRows) {
		w.mu.Unlock()
		return nil
	}
	if stateErr != nil {
		w.mu.Unlock()
		return stateErr
	}
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	w.sendCancel = cancel
	w.mu.Unlock()
	messageIDPresent, err := w.api.sendText(callCtx, state.BaseURL, token, state.SenderID, contextToken, id, content)
	w.mu.Lock()
	w.sendCancel = nil
	w.mu.Unlock()
	cancel()
	if err == nil {
		_, err = w.db.ExecContext(ctx, `UPDATE gt_remote_wechat_outbox SET status='delivered', delivered_at=? WHERE id=? AND status='pending'`, time.Now().UnixMilli(), id)
		if err == nil {
			w.setReplyMessageIDMissing(ctx, state, !messageIDPresent)
		}
		return err
	}
	backoff := time.Duration(1<<min(attempts, 8)) * time.Second
	blockedRevision := int64(-1)
	var httpErr *wechatHTTPError
	var sendErr *wechatSendError
	if (errors.As(err, &sendErr) && (sendErr.Ret == -14 || sendErr.ErrCode == -14)) ||
		(errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden)) {
		if _, clearErr := w.clearConnection(ctx, &state, false, "wechat_session_expired"); clearErr != nil {
			return clearErr
		}
		if _, stateErr := w.readState(ctx); errors.Is(stateErr, sql.ErrNoRows) {
			w.broadcastConnectionStatus("disconnected")
		}
		return err
	}
	if (errors.As(err, &sendErr) && sendErr.Ret == -2) ||
		(errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusBadRequest) {
		blockedRevision = state.ContextRevision
	}
	_, updateErr := w.db.ExecContext(ctx, `UPDATE gt_remote_wechat_outbox SET attempts=attempts+1, next_attempt_at=?, blocked_context_revision=? WHERE id=? AND status='pending'`,
		time.Now().Add(backoff).UnixMilli(), blockedRevision, id)
	if updateErr != nil {
		return updateErr
	}
	return err
}
