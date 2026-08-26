package localserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"goteams-client/internal/applog"
	"goteams-client/internal/cloud"
	"goteams-client/internal/config"
	"goteams-client/internal/secrets"
)

// sessionValidity Validity period of local browser session and login state (30 days)
const sessionValidity = 30 * 24 * time.Hour

// lastLoginInfo The account information of the last successful login, persisted to SecretStore to restore the session after restart.
type lastLoginInfo struct {
	AdminID     string `json:"admin_id"`
	UserID      string `json:"user_id"`
	UserName    string `json:"user_name"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar,omitempty"`
	Role        string `json:"role,omitempty"`
	// ServerType login target: official | custom. Used to select the correct cloud client after restarting.
	ServerType string `json:"server_type,omitempty"`
	// ServerURL Custom cloud address (after normalization); custom is filled in when logging in, official is empty.
	ServerURL string `json:"server_url,omitempty"`
}

// sessionManager local browser session management
type sessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*browserSession
}

// browserSession local browser session
type browserSession struct {
	ID          string
	UserID      string
	UserName    string
	DisplayName string
	Avatar      string
	Role        string
	AdminID     string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

var sessionMgr = &sessionManager{
	sessions: make(map[string]*browserSession),
}

// browserSessionsFile browser session (including cloud login user information) persistence file,
// Located in ConfigDir (~/.goteams/config/). Browser session cookies are valid for 30 days and survive
// On the browser side, the cookie is still valid after the process is restarted; after clearing sessionMgr.sessions in the memory, restart
// Old cookies carried by the browser can be remapped to the session, eliminating the need to log in again.
func (s *Server) browserSessionsFile() string {
	return filepath.Join(s.config.ConfigDir, "browser_sessions.json")
}

// persistBrowserSessions writes the current in-memory browser session to disk.
// The session ID is the Bearer token issued by the server, and the file is protected with 0600 permissions.
func (s *Server) persistBrowserSessions() {
	if s.config.ConfigDir == "" {
		return
	}
	sessionMgr.mu.RLock()
	data, err := json.Marshal(sessionMgr.sessions)
	sessionMgr.mu.RUnlock()
	if err != nil {
		applog.Warn("[LocalServer] 序列化浏览器会话失败", "error", err)
		return
	}
	if err := os.WriteFile(s.browserSessionsFile(), data, 0600); err != nil {
		applog.Warn("[LocalServer] 持久化浏览器会话失败", "error", err)
	}
}

// loadBrowserSessions Restore browser sessions from disk on startup (automatically skipping expired entries).
func (s *Server) loadBrowserSessions() {
	if s.config.ConfigDir == "" {
		return
	}
	data, err := os.ReadFile(s.browserSessionsFile())
	if err != nil {
		if !os.IsNotExist(err) {
			applog.Warn("[LocalServer] 读取浏览器会话失败", "error", err)
		}
		return
	}
	var loaded map[string]*browserSession
	if err := json.Unmarshal(data, &loaded); err != nil {
		applog.Warn("[LocalServer] 解析浏览器会话失败", "error", err)
		return
	}
	now := time.Now()
	count := 0
	sessionMgr.mu.Lock()
	for id, sess := range loaded {
		if sess == nil || now.After(sess.ExpiresAt) {
			continue
		}
		sessionMgr.sessions[id] = sess
		count++
	}
	sessionMgr.mu.Unlock()
	if count > 0 {
		applog.Info("[LocalServer] 已从磁盘恢复浏览器会话", "count", count)
	}
}

// handleBrowserTicket browser Ticket verification, create a local Session after passing the verification
func (s *Server) handleBrowserTicket(c *gin.Context) {
	ticket := c.Query("ticket")
	if ticket == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 ticket 参数"})
		return
	}

	// Consume the launch ticket atomically so it cannot be replayed by another
	// process after the desktop window has established its session.
	if !s.consumeBrowserTicket(ticket) {
		c.JSON(http.StatusForbidden, gin.H{"error": "无效的 ticket"})
		return
	}

	// Ticket verification passed, create a local browser session
	sessionID := generateSessionID()
	session := &browserSession{
		ID:        sessionID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(sessionValidity),
	}

	// If the server has restored the cloud account session (retaining the login status after restarting), reuse its user information,
	// Make the browser in the "cloud logged in" state without having to log in again.
	if s.lastLogin != nil {
		session.AdminID = s.lastLogin.AdminID
		session.UserID = s.lastLogin.UserID
		session.UserName = s.lastLogin.UserName
		session.DisplayName = s.lastLogin.DisplayName
		session.Avatar = s.lastLogin.Avatar
		session.Role = s.lastLogin.Role
	}

	sessionMgr.mu.Lock()
	sessionMgr.sessions[sessionID] = session
	sessionMgr.mu.Unlock()
	s.persistBrowserSessions()

	//Set HttpOnly Cookie (valid for 30 days)
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("goteams_local_session", sessionID, int(sessionValidity.Seconds()), "/", "", false, true)

	// Redirect to the front-end home page. In development mode, this request enters through the Vite proxy, so the relative address will still
	// Return to localhost:5173; in publishing mode, return to the home page of the backend hosting.
	c.Redirect(http.StatusFound, "/")
}

func (s *Server) consumeBrowserTicket(candidate string) bool {
	s.ticketMu.Lock()
	defer s.ticketMu.Unlock()
	if s.ticket == "" || len(candidate) != len(s.ticket) ||
		subtle.ConstantTimeCompare([]byte(candidate), []byte(s.ticket)) != 1 {
		return false
	}
	s.ticket = ""
	return true
}

func (s *Server) handleDesktopShutdown(c *gin.Context) {
	if !isLoopbackRequest(c.Request.RemoteAddr) {
		c.JSON(http.StatusForbidden, gin.H{"error": "desktop shutdown only accepts loopback requests"})
		return
	}
	token := c.GetHeader("X-GoTeams-Desktop-Token")
	expected := s.config.DesktopToken
	if expected == "" || len(token) != len(expected) ||
		subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid desktop token"})
		return
	}
	c.Status(http.StatusAccepted)
	go s.config.Shutdown()
}

// handleLogin creates only an optional cloud session. Local task and base data
// remain in the fixed application databases for official and custom cloud login.
func (s *Server) handleLogin(c *gin.Context) {
	var body struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		ServerType string `json:"server_type"` // official | custom
		ServerURL  string `json:"server_url"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	if body.Username == "" || body.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名和密码不能为空"})
		return
	}

	serverType := body.ServerType
	if serverType == "" {
		serverType = "official"
	}

	// 1. Select the cloud client. Both login modes keep the fixed local profile.
	var client *cloud.Client
	var profileDir string
	var secretDir string
	var normalized string // Normalized server URL, used to derive the account subdirectory
	switch serverType {
	case "custom":
		serverURL := strings.TrimSpace(body.ServerURL)
		if serverURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请输入自定义云端地址"})
			return
		}
		norm, err := normalizeServerURL(serverURL)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "云端地址无效: " + err.Error()})
			return
		}
		normalized = norm
		client = s.customCloudClient(normalized)
		// The local function library and credential file remain application-scoped.
		profileDir = s.config.BaseProfileDir
		if profileDir == "" {
			profileDir = s.config.DataDir
		}
		secretDir = profileDir
	default: // official
		if s.config.CloudClient == nil || !s.config.CloudClient.IsConfigured() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "官方云端 API 地址未配置，请先设置 config.ini 中的 api_base_url 或使用自定义地址登录"})
			return
		}
		client = s.config.CloudClient
		profileDir = s.config.BaseProfileDir
		if profileDir == "" {
			profileDir = s.config.DataDir
		}
		// Persist the normalized official endpoint for restoring the correct client.
		if s.config.CloudConfig != nil && s.config.CloudConfig.APIBaseURL != "" {
			if norm, err := normalizeServerURL(s.config.CloudConfig.APIBaseURL); err == nil {
				normalized = norm
			}
		}
		if normalized == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "官方云端 API 地址无效，请检查 config.ini 中的 api_base_url"})
			return
		}
		secretDir = profileDir
	}

	// 2. Call the cloud to log in (any account is acceptable, as long as the password is correct)
	loginResp, err := client.Login(c.Request.Context(), body.Username, body.Password)
	if err != nil {
		errMsg := "登录失败: " + err.Error()
		if apiErr, ok := err.(*cloud.APIError); ok {
			errMsg = "登录失败: " + apiErr.Message
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": errMsg})
		return
	}

	// 3. Keep the fixed local Profile and create the optional cloud account session.
	if err := s.switchProfile(c.Request.Context(), profileDir, secretDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "切换本地数据目录失败: " + err.Error()})
		return
	}

	if s.config.AccountMgr == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "账号管理器未初始化"})
		return
	}

	opts := LoginOptions{
		PersistLastLogin: true, // Both official and custom logins persist the last login index, and automatically restore the session after restart.
		ServerType:       serverType,
		ServerURL:        normalized, // official and custom both record the normalized server address
	}
	if serverType == "custom" {
		opts.CloudClient = client
	}

	sessionInfo, err := s.config.AccountMgr.LoginAndCreateSession(
		c.Request.Context(),
		loginResp.AdminID,
		loginResp.User.ID,
		loginResp.User.Username,
		loginResp.Token,
		opts,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建账号会话失败: " + err.Error()})
		return
	}

	// 4. Set the current session of the server
	s.SetSession(sessionInfo)
	// Refresh the read-only cloud pipeline cache after both official and custom
	// login. A cloud outage must not turn a successful login into a failure.
	s.triggerCloudPipelineSync()

	// 5. Device registration check (using DeviceMgr in the account session)
	//
	// Note: SetSession has been executed here and the server is already logged in. Any early return will cause
	// The status of "the server is logged in and the browser did not get the cookie" is split. Device-related failures will be downgraded.
	// (Consistent with the handling of RegisterDevice failure below), let the login process complete normally and issue cookies.
	// Users can retry device registration on the settings page.
	deviceRegistered := false
	if sessionInfo.DeviceMgr != nil {
		deviceRegistered = sessionInfo.DeviceMgr.IsRegistered()
		if !deviceRegistered {
			deviceUUID, err := sessionInfo.DeviceMgr.GetOrCreateUUID()
			if err != nil {
				applog.Warn("生成设备 UUID 失败，登录继续但设备未注册",
					"admin_id", loginResp.AdminID, "error", err.Error())
			} else {
				// Register the device (no boot keys are used, the cloud has been simplified to just JWT).
				// Must use the session cloud client: when customizing the login, the registration target is a custom cloud,
				// Otherwise, the JWT issued by the cloud will be sent to the official cloud, resulting in 401/registration failure.
				registerClient := s.config.CloudClient
				if sessionInfo.CloudClient != nil {
					registerClient = sessionInfo.CloudClient
				}
				deviceCred, err := registerClient.RegisterDevice(c.Request.Context(), deviceUUID, "")
				if err != nil {
					// Device registration failure does not block login, and the user can try again later.
					applog.Warn("设备注册失败，登录继续",
						"admin_id", loginResp.AdminID, "error", err.Error())
					deviceRegistered = false
				} else {
					if err := sessionInfo.DeviceMgr.SaveRegistration(loginResp.AdminID, deviceUUID, deviceCred.Credential); err != nil {
						applog.Warn("保存设备注册信息失败，登录继续",
							"admin_id", loginResp.AdminID, "error", err.Error())
						deviceRegistered = false
					} else {
						deviceRegistered = true
					}
				}
			}
		}
	}

	// 6. Create a local browser session
	sessionID := generateSessionID()
	browserSess := &browserSession{
		ID:          sessionID,
		UserID:      loginResp.User.ID,
		UserName:    loginResp.User.Username,
		DisplayName: loginResp.User.DisplayName,
		Avatar:      loginResp.User.Avatar,
		Role:        loginResp.User.Role,
		AdminID:     loginResp.AdminID,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(sessionValidity),
	}

	// Record this login information (persistent to the account index, Login is responsible for writing to the boot storage)
	s.lastLogin = &lastLoginInfo{
		AdminID:     loginResp.AdminID,
		UserID:      loginResp.User.ID,
		UserName:    loginResp.User.Username,
		DisplayName: loginResp.User.DisplayName,
		Avatar:      loginResp.User.Avatar,
		Role:        loginResp.User.Role,
		ServerType:  serverType,
		ServerURL:   normalized,
	}

	sessionMgr.mu.Lock()
	sessionMgr.sessions[sessionID] = browserSess
	sessionMgr.mu.Unlock()
	s.persistBrowserSessions()

	//Set HttpOnly Cookie (valid for 30 days)
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("goteams_local_session", sessionID, int(sessionValidity.Seconds()), "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"status":            "ok",
		"user":              loginResp.User,
		"device_registered": deviceRegistered,
		"admin_id":          loginResp.AdminID,
	})
}

// handleLogout log out
func (s *Server) handleLogout(c *gin.Context) {
	//Clear browser session
	sessionID, err := c.Cookie("goteams_local_session")
	if err == nil && sessionID != "" {
		sessionMgr.mu.Lock()
		delete(sessionMgr.sessions, sessionID)
		sessionMgr.mu.Unlock()
		s.persistBrowserSessions()
	}
	c.SetCookie("goteams_local_session", "", -1, "/", "", false, true)

	//Clear account session (close database, etc.)
	s.ClearSession()
	if s.config.AccountMgr != nil {
		s.config.AccountMgr.Logout()
	}

	// Clear JWT (current Profile key backend)
	if s.config.SecretStore != nil {
		if err := s.config.SecretStore.Delete(secrets.KeyJWT); err != nil {
			applog.Warn("[LocalServer] 清除 JWT 失败", "error", err)
		}
		if err := s.config.SecretStore.Delete(secrets.KeyPrefix + "jwt-refresh"); err != nil {
			applog.Warn("[LocalServer] 清除 JWT Refresh 失败", "error", err)
		}
	}
	s.lastLogin = nil

	// Return to the universal base Profile after logging out (the local function data is restored to the universal library, and the key backend is switched back to the universal directory)
	if err := s.switchProfile(c.Request.Context(), s.config.BaseProfileDir, s.config.BaseProfileDir); err != nil {
		applog.Warn("[LocalServer] 登出后切回 base Profile 失败", "error", err)
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleSessionStatus returns the current HttpOnly browser session status for restoring the UI when the page starts.
// It does not return JWTs, device credentials, or other Secrets.
// The local loopback service is trusted: local_authenticated is always true, and the cloud login state is
// determined by the server-side account session (restored from the persistent login state on startup).
func (s *Server) handleSessionStatus(c *gin.Context) {
	cur := s.currentSession()
	if !s.sessionMatchesConfig(cur) {
		// 官方会话域名与当前配置不一致（共享本地数据目录的其他实例登录写入），
		// 视为未登录，避免不同远程域名之间串用登录态；custom 会话由
		// sessionMatchesConfig 放行，不在此列。
		s.ClearSession()
		cur = nil
	}
	cloudLoggedIn := cur != nil
	var user interface{}
	if cloudLoggedIn {
		// 浏览器 session 档案更完整（头像、角色等）；缺失时回退账号会话，
		// 保证即使没有浏览器 cookie，前端也能拿到可用的登录状态。
		userID, userName, displayName, avatar, role, adminID := "", "", "", "", "", ""
		if session, ok := s.browserSessionFromCookie(c); ok {
			userID = session.UserID
			userName = session.UserName
			displayName = session.DisplayName
			avatar = session.Avatar
			role = session.Role
			adminID = session.AdminID
		}
		if userID == "" {
			userID = cur.UserID
		}
		if userName == "" {
			userName = cur.UserName
		}
		if displayName == "" {
			displayName = userName
		}
		if adminID == "" {
			adminID = cur.AdminID
		}
		user = gin.H{
			"id":           userID,
			"username":     userName,
			"display_name": displayName,
			"avatar":       avatar,
			"role":         role,
			"admin_id":     adminID,
		}
		applog.Info("会话状态响应-已登录",
			"user_id", userID,
			"user_name", userName,
		)
	}
	c.JSON(http.StatusOK, gin.H{
		"local_authenticated": true,
		"cloud_logged_in":     cloudLoggedIn,
		"user":                user,
		// 无 Electron 注入（纯 Go 运行/浏览器直连 dev）时，前端需要从本接口
		// 获取本次启动的 API token 用于后续请求；Electron 模式经 bridge 提供，不返回。
		"api_token": apiTokenForSession(s),
	})
	applog.Info("会话状态响应",
		"cloud_logged_in", cloudLoggedIn,
	)
}

// apiTokenForSession 仅在 token 非由 Electron 注入时返回，避免 token 出现在
// Electron 模式的握手响应中（该模式下前端通过 bridge 获取）。
func apiTokenForSession(s *Server) string {
	if s.config.APITokenFromEnv {
		return ""
	}
	return s.config.APIToken
}

func (s *Server) browserSessionFromCookie(c *gin.Context) (*browserSession, bool) {
	sessionID, err := c.Cookie("goteams_local_session")
	if err != nil || sessionID == "" {
		return nil, false
	}
	sessionMgr.mu.RLock()
	session, ok := sessionMgr.sessions[sessionID]
	sessionMgr.mu.RUnlock()
	if !ok || time.Now().After(session.ExpiresAt) {
		if ok {
			sessionMgr.mu.Lock()
			delete(sessionMgr.sessions, sessionID)
			sessionMgr.mu.Unlock()
			s.persistBrowserSessions()
		}
		return nil, false
	}
	return session, true
}

// validateSession 中间件：本地 loopback 服务可信，不要求浏览器 session cookie。
// ticket 偶发失败不应导致前端误判未登录；有 cookie 时优先映射浏览器 session，
// 缺失时回退账号会话（云登录态），保证本地功能能拿到用户信息。
// 回退前校验会话域名与当前配置一致（仅官方会话），防止共享数据目录时串用其他实例的登录态。
func (s *Server) validateSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if session, ok := s.browserSessionFromCookie(c); ok {
			c.Set("session", session)
		} else if cur := s.currentSession(); s.sessionMatchesConfig(cur) {
			c.Set("session", &browserSession{
				ID:        "account",
				UserID:    cur.UserID,
				UserName:  cur.UserName,
				AdminID:   cur.AdminID,
				CreatedAt: time.Now(),
				ExpiresAt: time.Now().Add(sessionValidity),
			})
		}
		c.Next()
	}
}

func isLoopbackRequest(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// RestoreSession restores the account session from the persistent account index (bootstrap storage) + JWT in the Profile library,
// so the user does not need to log in again after restarting the program. Official
// uses the default client; custom rebuilds a client from the persisted ServerURL.
//
// Domain consistency only constrains official logins: the session domain must match
// the cloud address of the current configuration (http/https are considered the same
// domain), so instances that share the local data directory but point to different
// remote domains never take over each other's official login state. Custom logins
// use the address chosen by the user and are restored regardless of the configured
// domain.
//
// The cloud validity verification and renewal of JWT are completed internally by Login.
func (s *Server) RestoreSession(ctx context.Context) {
	if s.config.AccountMgr == nil || s.config.BootstrapStore == nil {
		return
	}

	// The account index is persisted in the boot storage (configDir) and is isolated from the Profile key.
	// No official address configured means the target domain is unknown; skip restore so that
	// the stale index of another instance cannot silently log this instance in.
	configured := s.currentConfiguredServerURL()
	if configured == "" {
		applog.Info("[LocalServer] 未配置云端地址，跳过登录态恢复")
		return
	}

	// 告知账号管理器官方地址：索引写入按"最后登录"语义互斥清理（官方登录
	// 清除 custom 固定索引、custom 登录清除官方索引）。
	if mgr, ok := s.config.AccountMgr.(interface{ SetOfficialServerURL(string) }); ok {
		mgr.SetOfficialServerURL(configured)
	}

	// Read the last login index: the per-address key first, then the fixed
	// custom index, and finally the legacy single index written by older versions.
	last := readLastLogin(s.config.BootstrapStore, configured)
	if last == nil || last.AdminID == "" {
		return
	}

	// 域名一致性只约束官方登录：会话域名必须与当前配置的云端地址一致，防止
	// 共享本地数据目录的多个实例互相串用登录态。custom 登录的地址由用户主动
	// 选择，不受内置配置约束；legacy 索引（旧版本写入、无 ServerURL）视为旧版
	// 官方登录，同样不比对，按当前配置地址恢复。
	if last.ServerType != "custom" && last.ServerURL != "" && !sameServerDomain(last.ServerURL, configured) {
		applog.Info("[LocalServer] 上次登录云端地址与当前配置不一致，跳过登录态恢复",
			"last_server", last.ServerURL,
			"configured", configured)
		return
	}

	// Resolve the target server address: new versions always persist the normalized
	// ServerURL (official logins too). Old-version official indexes have no ServerURL,
	// so fall back to the currently configured official address.
	serverURL := strings.TrimSpace(last.ServerURL)
	if serverURL == "" {
		serverURL = configured
	}
	normalized := serverURL
	if norm, err := normalizeServerURL(serverURL); err == nil {
		normalized = norm
	}

	// Select the cloud client according to the login target (directory derivation rules
	// consistent with handleLogin): official uses the default client; custom rebuilds by address.
	var client *cloud.Client
	if last.ServerType == "custom" && normalized != "" {
		client = s.customCloudClient(normalized)
	}

	opts := LoginOptions{}
	profileDir := s.config.BaseProfileDir
	if profileDir == "" {
		profileDir = s.config.DataDir
	}
	secretDir := profileDir
	if normalized != "" {
		opts = LoginOptions{
			CloudClient:      client,
			PersistLastLogin: true,
			ServerType:       last.ServerType,
			ServerURL:        normalized,
		}
	}

	// First switch to the corresponding Profile (general base.db + key backend by target), and then resume the session:
	// Pass the token empty, Login reads the persistent JWT from the current Profile secrets and renews it to the corresponding cloud.
	if err := s.switchProfile(ctx, profileDir, secretDir); err != nil {
		applog.Warn("恢复登录态失败：切换 Profile 失败", "error", err)
		return
	}
	sessionInfo, err := s.config.AccountMgr.LoginAndCreateSession(ctx, last.AdminID, last.UserID, last.UserName, "", opts)
	if err != nil {
		applog.Warn("恢复登录态失败", "error", err)
		// legacy 索引（旧版本写入、无 ServerURL）没有 per-address key 可被
		// Login 清理，失败后补删固定 legacy key，避免每次启动重复失败恢复。
		if last.ServerURL == "" {
			if dErr := s.config.BootstrapStore.Delete(secrets.KeyLastLogin); dErr != nil {
				applog.Warn("[LocalServer] 清除旧版登录索引失败", "error", dErr)
			}
		}
		return
	}
	s.SetSession(sessionInfo)
	s.triggerCloudPipelineSync()
	s.lastLogin = last
	// GetMe must use the session cloud client: user information comes from the custom cloud during custom login.
	if client := s.currentCloudClient(); client != nil {
		user, profileErr := client.GetMe(ctx)
		if profileErr == nil {
			// Only overwrite with non-empty values so that a temporarily incomplete
			// /api/client/me response cannot wipe out the persisted profile.
			if user.ID != "" {
				s.lastLogin.UserID = user.ID
			}
			if user.Username != "" {
				s.lastLogin.UserName = user.Username
			}
			if user.DisplayName != "" {
				s.lastLogin.DisplayName = user.DisplayName
			}
			if user.Avatar != "" {
				s.lastLogin.Avatar = user.Avatar
			}
			if user.Role != "" {
				s.lastLogin.Role = user.Role
			}
		}
	}
	applog.Info("已从持久化登录态恢复账号会话",
		"admin", last.AdminID, "user", last.UserName, "server_type", last.ServerType)
}

// readLastLogin reads the last login information from the boot storage.
// The official index is stored per server address (LastLoginKey), so different
// remote domains never share or overwrite each other's login state. When the
// per-address index is missing, the fixed custom index (KeyLastLoginCustom,
// written by custom logins whose address is chosen by the user and not bound
// to the configuration) is tried next, and finally the legacy single index
// (goteams:last_login) written by older versions.
func readLastLogin(store secrets.Store, serverURL string) *lastLoginInfo {
	if store == nil {
		return nil
	}
	data, err := store.Get(LastLoginKey(serverURL))
	if err != nil || data == "" {
		if data, err = store.Get(secrets.KeyLastLoginCustom); err != nil || data == "" {
			if data, err = store.Get(secrets.KeyLastLogin); err != nil || data == "" {
				return nil
			}
		}
	}
	var info lastLoginInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		return nil
	}
	return &info
}

// LastLoginKey derives the boot-storage key of the last-login index from the
// normalized cloud address, so login states of different remote domains are
// isolated even when they share the local data directory. Empty address keeps
// the legacy single key for backward compatibility.
func LastLoginKey(serverURL string) string {
	if serverURL == "" {
		return secrets.KeyLastLogin
	}
	return secrets.KeyLastLogin + "." + ServerProfileKey(serverURL)
}

// currentConfiguredServerURL returns the normalized cloud address of the current
// configuration (empty when no official address is configured).
func (s *Server) currentConfiguredServerURL() string {
	if s.config.CloudConfig == nil || s.config.CloudConfig.APIBaseURL == "" {
		return ""
	}
	norm, err := normalizeServerURL(s.config.CloudConfig.APIBaseURL)
	if err != nil {
		return ""
	}
	return norm
}

// sessionMatchesConfig reports whether the account session belongs to the cloud
// address of the current configuration. The official session domain must match the
// configured api_base_url (http/https are considered the same domain); a session
// without a server address or with a mismatched domain is treated as a
// cross-domain takeover and never kept. Custom sessions are exempt: the address
// was chosen by the user, so the session itself is the target and is never
// cleared by the configured domain.
func (s *Server) sessionMatchesConfig(sess *SessionInfo) bool {
	if sess == nil {
		return false
	}
	if sess.ServerType == "custom" {
		return sess.ServerURL != ""
	}
	configured := s.currentConfiguredServerURL()
	if configured == "" {
		return false
	}
	return sameServerDomain(sess.ServerURL, configured)
}

// sameServerDomain reports whether the two cloud addresses point to the same
// server host. The protocol (http/https) and the default port are ignored, so
// http://a.example.com and https://a.example.com are considered the same domain.
func sameServerDomain(a, b string) bool {
	da, err := serverDomainKey(a)
	if err != nil {
		return false
	}
	db, err := serverDomainKey(b)
	if err != nil {
		return false
	}
	return da == db
}

// serverDomainKey derives a comparable identity from a cloud address: lowercased
// host plus explicit non-default port. The protocol itself is excluded, and a
// port equal to the scheme default (http:80/https:443) is ignored, so
// http://a.example.com and https://a.example.com are the same identity.
func serverDomainKey(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("地址不能为空")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("无法解析地址: %w", err)
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port != "" {
		switch strings.ToLower(u.Scheme) {
		case "http":
			if port == "80" {
				port = ""
			}
		case "https":
			if port == "443" {
				port = ""
			}
		}
	}
	if port == "" {
		return host, nil
	}
	return net.JoinHostPort(host, port), nil
}

// generateSessionID generates a random session ID
func generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// normalizeServerURL normalizes the custom cloud address: complete the protocol and remove the trailing slash.
func normalizeServerURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("地址不能为空")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("无法解析地址: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("缺少主机名")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("仅支持 http/https 协议")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// customCloudClient constructs a temporary cloud client based on the custom cloud address (keeps the client version number).
func (s *Server) customCloudClient(baseURL string) *cloud.Client {
	version := ""
	if s.config.CloudConfig != nil {
		version = s.config.CloudConfig.ClientVersion
	}
	cfg := config.NewCloudConfig(baseURL, version, "")
	if s.config.CloudConfig != nil {
		cfg.ListenAddr = s.config.CloudConfig.ListenAddr
	}
	return cloud.NewClient(cfg, s.config.SecretStore)
}

// ServerProfileKey assigns a stable and file system-safe subdirectory key from the standardized cloud address.
// Used to isolate the local Profile and account data of each cloud address into a separate directory.
// The http/https protocol is excluded from the derivation (only host + path participate), so the same
// host always maps to the same directory regardless of which protocol is used.
// Take the first 12 hexadecimal digits of SHA-256 of the address, with negligible collision probability and no special characters.
func ServerProfileKey(normalizedURL string) string {
	key := normalizedURL
	if u, err := url.Parse(normalizedURL); err == nil && u.Host != "" {
		key = u.Host + u.Path
	}
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])[:12]
}
