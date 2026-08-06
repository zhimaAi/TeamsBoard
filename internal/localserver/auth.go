package localserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	"goteams-client/internal/capability"
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

	//Verify Ticket
	if ticket != s.config.BrowserTicket {
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

// accountServerDir returns the per-server account container directory under the data
// root, derived from the normalized server URL. Both official and custom logins follow
// the same rule: the same server address maps to the same data/accounts_<key> directory.
func (s *Server) accountServerDir(normalized string) string {
	dataDir := s.config.DataDir
	if dataDir == "" {
		dataDir = s.config.BaseProfileDir
	}
	return filepath.Join(dataDir, "accounts_"+serverProfileKey(normalized))
}

// copyFile copies a single file to the target path, creating parent directories on demand.
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	target, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer target.Close()
	if _, err := io.Copy(target, source); err != nil {
		return err
	}
	return nil
}

// migrateLegacyServerData migrates legacy data directories to the unified accounts_<key>
// layout (both official and custom logins use the same per-domain rule):
//   - data/accounts_cus_<key>/ -> data/accounts_<key>/ (old custom layout, renamed as a whole)
//   - data_cus_<key>/secrets.json -> data/accounts_<key>/secrets.json (even older custom layout)
//   - data/secrets.json (old official base profile keys) is COPIED into the target when the
//     target is missing: the base copy must remain so the logged-out state can still use it.
//
// The function is idempotent and only migrates when the target does not exist yet.
func (s *Server) migrateLegacyServerData(normalized string, official bool) {
	if normalized == "" {
		return
	}
	key := serverProfileKey(normalized)
	targetDir := s.accountServerDir(normalized)
	dataDir := s.config.DataDir
	if dataDir == "" {
		dataDir = s.config.BaseProfileDir
	}

	// 1. Old custom layout: rename the whole directory (secrets.json + account data dirs).
	oldCustom := filepath.Join(dataDir, "accounts_cus_"+key)
	if _, err := os.Stat(oldCustom); err == nil {
		if _, terr := os.Stat(targetDir); terr != nil {
			if rerr := os.Rename(oldCustom, targetDir); rerr == nil {
				applog.Info("已迁移旧自定义账号目录", "from", oldCustom, "to", targetDir)
				return
			} else {
				applog.Warn("迁移旧自定义账号目录失败", "from", oldCustom, "error", rerr)
			}
		}
	}

	// 2. Old official base secrets: copy into the target when missing.
	targetSecrets := filepath.Join(targetDir, "secrets.json")
	if _, err := os.Stat(targetSecrets); err == nil {
		return
	}
	if official {
		baseSecrets := filepath.Join(s.config.BaseProfileDir, "secrets.json")
		if _, err := os.Stat(baseSecrets); err == nil {
			if cerr := copyFile(baseSecrets, targetSecrets); cerr != nil {
				applog.Warn("迁移旧官方密钥失败", "error", cerr)
			} else {
				applog.Info("已迁移旧官方密钥到账号目录", "key", key)
			}
			return
		}
	}

	// 3. Even older top-level custom layout.
	oldTop := filepath.Join(filepath.Dir(s.config.BaseProfileDir), "data_cus_"+key, "secrets.json")
	if _, err := os.Stat(oldTop); err == nil {
		if cerr := copyFile(oldTop, targetSecrets); cerr != nil {
			applog.Warn("迁移旧自定义密钥失败", "error", cerr)
		} else {
			applog.Info("已迁移旧自定义密钥", "key", key)
		}
	}
}

// handleLogin logs in: the agent calls the cloud API and creates an account-isolated database.
// Both official and custom logins derive the account container directory from the (normalized)
// server URL as data/accounts_<key> (secrets.json + <userID>), so the same server address always
// maps to the same directory. The local function library (data/base.db) is a general library shared
// by all accounts, and keys/account data of different clouds are completely isolated from each other.
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

	// 1. Select cloud client and local profile based on login target.
	// Both official and custom logins derive the account container directory from the
	// (normalized) server URL: data/accounts_<key>, so the same server address always
	// maps to the same directory.
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
		// The local function library (base.db) is a general library shared by all accounts;
		// only the key backend and each account's data are isolated to data/accounts_<key>
		// according to the server address, so different clouds never interfere with each other.
		profileDir = s.config.BaseProfileDir
		if profileDir == "" {
			profileDir = s.config.DataDir
		}
		secretDir = s.accountServerDir(normalized)
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
		// The official address is also normalized to derive the same account directory as a
		// custom login that points to the same address.
		if s.config.CloudConfig != nil && s.config.CloudConfig.APIBaseURL != "" {
			if norm, err := normalizeServerURL(s.config.CloudConfig.APIBaseURL); err == nil {
				normalized = norm
			}
		}
		if normalized == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "官方云端 API 地址无效，请检查 config.ini 中的 api_base_url"})
			return
		}
		secretDir = s.accountServerDir(normalized)
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

	// Forward compatibility: migrate legacy data directories to the unified accounts_<key> layout.
	s.migrateLegacyServerData(normalized, serverType != "custom")

	// 3. Switch local Profile (generic base.db + key backend by target), then create an account session
	if err := s.switchProfile(c.Request.Context(), profileDir, secretDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "切换本地数据目录失败: " + err.Error()})
		return
	}

	if s.config.AccountMgr == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "账号管理器未初始化"})
		return
	}

	opts := LoginOptions{
		// Account data (cloud session library) container directory: unified per-server
		// directory data/accounts_<key> (the same as the key backend), official and custom
		// logins are derived by the same rule. base.db is always the universal library
		// (BaseProfileDir) shared by all accounts.
		DataDir:          secretDir,
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
func (s *Server) handleSessionStatus(c *gin.Context) {
	session, ok := s.browserSessionFromCookie(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"local_authenticated": false,
			"cloud_logged_in":     false,
		})
		return
	}

	// The cloud login state is determined by the server-side account session (restored
	// from the persistent login state on startup). The browser session's profile info
	// is only used to enrich the displayed user and must not gate the login state.
	cur := s.currentSession()
	cloudLoggedIn := cur != nil
	var user interface{}
	if cloudLoggedIn {
		// Fall back to the server-side session for any field the browser session
		// is missing, so a valid login state is never reported as logged out just
		// because the browser session lacks profile info.
		userID := session.UserID
		if userID == "" {
			userID = cur.UserID
		}
		userName := session.UserName
		if userName == "" {
			userName = cur.UserName
		}
		user = gin.H{
			"id":           userID,
			"username":     userName,
			"display_name": session.DisplayName,
			"avatar":       session.Avatar,
			"role":         session.Role,
			"admin_id":     session.AdminID,
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"local_authenticated": true,
		"cloud_logged_in":     cloudLoggedIn,
		"user":                user,
	})
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

// validateSession middleware: validate local browser session
func (s *Server) validateSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		// Public path (basic capabilities + local login-free function) does not require a local browser session
		if isSessionExemptPath(path) {
			c.Next()
			return
		}

		if token := c.GetHeader("X-GoTeams-Local-Token"); token != "" {
			requiredSkill := capabilitySkillForRequest(c.Request.Method, path)
			grant, valid := s.config.Capabilities.Lookup(token)
			if requiredSkill == "" || !valid || !grant.Allows(requiredSkill) || !isLoopbackRequest(c.Request.RemoteAddr) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "本地 Skill 能力令牌无效或无权访问该资源"})
				return
			}
			c.Set(capability.GinContextGrantKey, grant)
			c.Next()
			return
		}

		session, ok := s.browserSessionFromCookie(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "会话已过期"})
			return
		}

		c.Set("session", session)
		c.Next()
	}
}

// isSessionExemptPath determines whether the path requires a local browser session.
// Basic capabilities (health check/login/session/WebSocket/cloud configuration) and local login-free function
// (Configuration Center / Interface Management / Command Center / Knowledge Base) does not require a session and can be accessed directly.
func isSessionExemptPath(path string) bool {
	if path == "/api/local/health" ||
		path == "/api/local/auth/ticket" ||
		path == "/api/local/auth/login" ||
		path == "/api/local/auth/logout" ||
		path == "/api/local/auth/session" ||
		path == "/api/local/ws" ||
		path == "/api/local/config/cloud" {
		return true
	}
	// Local login-free function: general local Profile library for data reading and writing (data/base.db)
	if strings.HasPrefix(path, "/api/local/config/") ||
		strings.HasPrefix(path, "/api/local/apis/") ||
		strings.HasPrefix(path, "/api/local/commands/") ||
		strings.HasPrefix(path, "/api/local/knowledge/") {
		return true
	}
	return false
}

func capabilitySkillForRequest(method, path string) string {
	switch {
	case strings.HasPrefix(path, "/api/local/apis/"):
		return "goteams-api"
	case strings.HasPrefix(path, "/api/local/tools/db/"):
		return "goteams-db"
	case method == http.MethodGet && path == "/api/local/config/database-profiles":
		return "goteams-db"
	default:
		return ""
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
// so the user does not need to log in again after restarting the program. Both official and custom logins persist their indexes:
// - official: uses the default client; the key backend and account data fall into data/accounts_<key> derived from the official address;
// - custom: rebuilds an independent client according to the persisted ServerURL and switches to the same data/accounts_<key> rule,
// so the same server address always maps to the same directory, and the login state stays pointed at the right domain after restarting.
//
// The cloud validity verification and renewal of JWT are completed internally by Login.
func (s *Server) RestoreSession(ctx context.Context) {
	if s.config.AccountMgr == nil || s.config.BootstrapStore == nil {
		return
	}

	//The account index is persisted in the boot storage (configDir) and is isolated from the Profile key.
	last := readLastLogin(s.config.BootstrapStore)
	if last == nil || last.AdminID == "" {
		return
	}

	// Resolve the target server address: new versions always persist the normalized
	// ServerURL (official logins too). Old-version official indexes have no ServerURL,
	// so fall back to the currently configured official address.
	serverURL := strings.TrimSpace(last.ServerURL)
	if serverURL == "" && s.config.CloudConfig != nil {
		serverURL = s.config.CloudConfig.APIBaseURL
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
		// Unified per-server directory data/accounts_<key> shared by official and custom logins.
		secretDir = s.accountServerDir(normalized)
		// Forward compatibility: migrate legacy data directories to the unified layout.
		s.migrateLegacyServerData(normalized, last.ServerType != "custom")
		opts = LoginOptions{
			CloudClient:      client,
			DataDir:          secretDir,
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
		return
	}
	s.SetSession(sessionInfo)
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

// readLastLogin reads the last login information from SecretStore
func readLastLogin(store secrets.Store) *lastLoginInfo {
	if store == nil {
		return nil
	}
	data, err := store.Get(secrets.KeyLastLogin)
	if err != nil || data == "" {
		return nil
	}
	var info lastLoginInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		return nil
	}
	return &info
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

// serverProfileKey assigns a stable and file system-safe subdirectory key from the standardized cloud address.
// Used to isolate the local Profile and account data of each cloud address into a separate directory.
// The http/https protocol is excluded from the derivation (only host + path participate), so the same
// host always maps to the same directory regardless of which protocol is used.
// Take the first 12 hexadecimal digits of SHA-256 of the address, with negligible collision probability and no special characters.
func serverProfileKey(normalizedURL string) string {
	key := normalizedURL
	if u, err := url.Parse(normalizedURL); err == nil && u.Host != "" {
		key = u.Host + u.Path
	}
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])[:12]
}
