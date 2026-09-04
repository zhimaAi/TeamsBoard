package localserver

import (
	"bytes"
	"context"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"goteams-client/internal/apimanager"
	"goteams-client/internal/applog"
	"goteams-client/internal/cloud"
	"goteams-client/internal/command"
	"goteams-client/internal/config"
	"goteams-client/internal/configcenter"
	"goteams-client/internal/identity"
	"goteams-client/internal/knowledge"
	"goteams-client/internal/localauth"
	"goteams-client/internal/secrets"
	"goteams-client/internal/storage"
	"goteams-client/internal/tools"
	"goteams-client/internal/workflow"
)

// AccountManagerInterface account manager interface (implemented by bootstrap.AccountManager)
type AccountManagerInterface interface {
	LoginAndCreateSession(ctx context.Context, adminID, userID, userName, token string, opts LoginOptions) (*SessionInfo, error)
	Logout()
	IsLoggedIn() bool
}

// LoginOptions configures the optional cloud session. It never changes local storage.
type LoginOptions struct {
	// CloudClient Customize the cloud address client; pass nil for official login (use the default client).
	CloudClient *cloud.Client
	// PersistLastLogin Whether to persist the last login index for automatic session recovery after restart.
	// Both official and custom logins are persisted (when the custom login is restored, the client is rebuilt according to the ServerURL and renewed to the cloud).
	PersistLastLogin bool
	// ServerType login target: official | custom. Required only for restore, used to select cloud client.
	ServerType string
	// ServerURL Custom cloud address (after normalization); custom is filled in when logging in, official is empty.
	ServerURL string
}

// Config local service configuration
type Config struct {
	DataDir         string
	ConfigDir       string
	RuntimeDir      string
	TaskRoot        string
	BaseProfileDir  string
	BootstrapStore  secrets.Store
	SecretStore     *secrets.DelegatingStore
	CloudConfig     *config.CloudConfig
	BrowserTicket   string
	DesktopToken    string
	APIToken        string
	APITokenFromEnv bool
	Shutdown        func()
	CloudClient     *cloud.Client
	AccountMgr      AccountManagerInterface
	TaskDB          *sql.DB
	Orchestrator    *workflow.Orchestrator
	WSHub           *WSHub
}

// SessionInfo session information of the current login account
type SessionInfo struct {
	DeviceMgr *identity.DeviceManager
	JWTMgr    *localauth.JWTManager
	// CloudClient The cloud client actually used by the current session (custom login is an independent client).
	// It must be used to proxy cloud requests (task/work item/device registration, etc.), and the static official client cannot be used directly.
	CloudClient *cloud.Client
	AdminID     string
	UserID      string
	UserName    string
	// ServerURL 登录目标云端地址（normalized）。官方登录用它校验会话与当前配置的
	// 远程域名一致，防止共享本地数据目录的多个实例互相串用登录态；custom 登录的
	// 地址由用户主动选择，不受内置配置约束。
	ServerURL string
	// ServerType 登录目标类型（official | custom）。官方会话恢复与校验要求域名与
	// 当前配置一致；custom 会话以自身地址为目标，不做域名一致性校验。
	ServerType string
}

// sessionHolder thread-safely holds the current account session
type sessionHolder struct {
	mu      sync.RWMutex
	current *SessionInfo
}

func (h *sessionHolder) get() *SessionInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.current
}

func (h *sessionHolder) set(s *SessionInfo) {
	h.mu.Lock()
	h.current = s
	h.mu.Unlock()
}

func (h *sessionHolder) clear() {
	h.mu.Lock()
	h.current = nil
	h.mu.Unlock()
}

// Server local HTTP service
type Server struct {
	config     Config
	router     *gin.Engine
	sessions   *sessionHolder
	dbRef      *storage.DBRef
	lastLogin  *lastLoginInfo
	profile    *LocalProfile
	profileMu  sync.Mutex
	profileDir string
	ticketMu   sync.Mutex
	ticket     string
}

// New creates a local service instance
func New(config Config) *Server {
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		config:   config,
		sessions: &sessionHolder{},
		dbRef:    storage.NewDBRef(),
		profile:  NewLocalProfile(),
		ticket:   config.BrowserTicket,
	}
	if config.WSHub != nil {
		config.WSHub.SetAPIToken(config.APIToken)
	}
	s.router = s.buildRouter()

	// Restore the browser session from disk: the browser-side 30-day cookie remains valid across reboots,
	// The memory session table is reloaded here so that old cookies can continue to be mapped to the session without re-logging in.
	s.loadBrowserSessions()

	// Open the default (base) local Profile library: interface/configuration/command/knowledge base, which can be used when not logged in.
	//The data falls in <BaseProfileDir>/base.db, and the key backend switches to the secrets.json of this Profile.
	baseDir := config.BaseProfileDir
	if baseDir == "" {
		baseDir = config.DataDir
	}
	if err := s.switchProfile(context.Background(), baseDir, baseDir); err != nil {
		// Failure to open the Profile library does not block service startup: the relevant page will report an error before logging in.
		// But basic capabilities such as health check/login/configuration of cloud address are still available.
		s.profileMu.Lock()
		s.profileDir = baseDir
		s.profileMu.Unlock()
	}
	return s
}

// SetSession sets the current account session (called when logging in).
// Local functions (interface/configuration/command/knowledge base) use an independent Profile library and do not switch dbRef with the session.
func (s *Server) SetSession(info *SessionInfo) {
	s.sessions.set(info)
}

// ClearSession clears the current account session (called when logging out)
func (s *Server) ClearSession() {
	s.sessions.clear()
}

// currentSession gets the current session
func (s *Server) currentSession() *SessionInfo {
	return s.sessions.get()
}

// currentAccountID returns the account ID of the current login account.
// Note: Local function keys are isolated by Profile and no longer isolated by account (see noopAccountID).
func (s *Server) currentAccountID() (string, error) {
	sess := s.sessions.get()
	if sess == nil {
		return "", fmt.Errorf("当前未登录")
	}
	return sess.UserID, nil
}

// switchProfile opens the fixed base profile and selects its credential store.
func (s *Server) switchProfile(ctx context.Context, dbDir, secretDir string) error {
	s.profileMu.Lock()
	defer s.profileMu.Unlock()

	if dbDir == "" {
		return fmt.Errorf("Profile 数据目录为空")
	}
	if secretDir == "" {
		return fmt.Errorf("密钥存储目录为空")
	}

	db, err := s.profile.Open(ctx, dbDir)
	if err != nil {
		return err
	}

	store, err := secrets.New(secretDir)
	if err != nil {
		return fmt.Errorf("初始化密钥存储失败: %w", err)
	}
	if s.config.SecretStore != nil {
		s.config.SecretStore.SetBackend(store)
	}

	s.dbRef.Set(db)
	s.profileDir = dbDir
	return nil
}

// currentProfileDir returns the current local Profile data directory (empty when not initialized).
func (s *Server) currentProfileDir() string {
	s.profileMu.Lock()
	defer s.profileMu.Unlock()
	return s.profileDir
}

// currentSessionDB returns the application-scoped local task database.
func (s *Server) currentSessionDB() *sql.DB {
	return s.config.TaskDB
}

// noopAccountID local function key is isolated by Profile (not account): the key reference does not have the account ID,
// Ensure that the same Profile library has consistent references and no aliasing in the logged in/unlogged state.
func noopAccountID() (string, error) {
	return "", nil
}

// Handler returns HTTP Handler
func (s *Server) Handler() http.Handler {
	return s.router
}

// Close releases server-owned base profile resources. Application-level task
// runtime resources are owned and closed by bootstrap.App.
func (s *Server) Close() {
	if s.profile != nil {
		s.profile.Close()
	}
}

// buildRouter builds the route
func (s *Server) buildRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		applog.Error("本地 HTTP 服务发生未处理异常",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"error", recovered,
		)
		c.AbortWithStatus(http.StatusInternalServerError)
	}))
	r.Use(noStoreAPI())
	r.Use(logAllRequests())
	r.Use(logFailedRequest())
	r.Use(s.requireAPIToken())
	r.Use(s.validateSession())

	// Health check (no Session required)
	r.GET("/api/local/health", s.handleHealth)

	//Authentication routing group
	auth := r.Group("/api/local/auth")
	auth.GET("/ticket", s.handleBrowserTicket)
	auth.GET("/session", s.handleSessionStatus)
	auth.POST("/login", s.handleLogin)
	auth.POST("/logout", s.handleLogout)

	if s.config.DesktopToken != "" && s.config.Shutdown != nil {
		r.POST("/api/local/desktop/shutdown", s.handleDesktopShutdown)
	}

	// GoTeams status
	r.GET("/api/local/goteams/status", s.handleGoTeamsStatus)

	// WebSocket endpoint
	r.GET("/api/local/ws", s.handleWebSocket)

	//Configuration center
	configHandler := configcenter.NewHandler(s.dbRef, s.config.SecretStore, s.config.CloudConfig, noopAccountID)
	// Cloud connection configuration requires no login (access is required when starting for the first time and not logging in)
	configHandler.RegisterCloudRoutes(r.Group("/api/local/config"))

	// The following local function routing groups can be used without cloud login, and the data is uniformly read and written to the local general Profile library (data/base.db, shared by all accounts);
	// Configuration center (Git/SSH/Docker/database/model), interface management, command center, knowledge base.
	// Cloud credentials use the fixed application credential store.
	local := r.Group("/api/local")
	configHandler.RegisterRoutes(local.Group("/config"))

	//Interface management
	apisGroup := local.Group("/apis")
	apimanager.RegisterRoutes(apisGroup, s.dbRef, noopAccountID, s.config.SecretStore)

	// command center
	commandGroup := local.Group("/commands")
	command.RegisterRoutes(commandGroup, s.dbRef, s.config.SecretStore)

	//Knowledge base (content files belong to the current Profile directory)
	knowledgeGroup := local.Group("/knowledge")
	knowledge.RegisterRoutes(knowledgeGroup, s.dbRef, func() (string, error) {
		dir := s.currentProfileDir()
		if dir == "" {
			return "", fmt.Errorf("本地 Profile 未初始化")
		}
		return filepath.Join(dir, "knowledge", "content"), nil
	})

	// Tool Center
	toolsHandler := tools.NewHandler(s.dbRef, s.config.SecretStore)
	toolsGroup := local.Group("/tools")
	toolsHandler.RegisterRoutes(toolsGroup)
	loggedIn := r.Group("", s.requireLogin())

	// Local task management and CLI execution are available without cloud login.
	// Individual cloud-import endpoints validate the cloud session in their handler.
	tasksHandler := NewTasksHandler(s.currentSessionDB, s.currentOrchestrator, s.currentCloudClient, s.config.WSHub, s.config.TaskRoot)
	tasksGroup := local.Group("/tasks")
	tasksHandler.RegisterRoutes(tasksGroup)
	iconStore := NewIconStore(s.config.DataDir)
	local.GET("/assets/icons/:filename", iconStore.Serve)
	pipelinesHandler := NewPipelinesHandler(s.currentSessionDB, iconStore, s.currentCloudClient)
	pipelinesHandler.RegisterRoutes(local.Group("/pipelines"))
	NewProjectsHandler(s.currentSessionDB, iconStore).RegisterRoutes(local.Group("/projects"))
	NewNotificationsHandler(s.currentSessionDB).RegisterRoutes(local.Group("/notifications"))
	tasksHandler.RegisterTeamRoutes(loggedIn.Group("/api/local/team"))
	pipelinesHandler.RegisterCloudRoutes(loggedIn.Group("/api/local/team"))
	pipelinesHandler.RegisterCloudRoutes(loggedIn.Group("/api/local"))

	registerWebUI(r)
	return r
}

// logAllRequests records every local API request so connectivity issues between
// the front-end (Vite proxy) and the back-end are visible in the application log.
func logAllRequests() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		applog.Info("本地 HTTP 请求",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"cost_ms", time.Since(start).Milliseconds(),
			"remote", c.Request.RemoteAddr,
		)
	}
}

// noStoreAPI marks every loopback API response as non-cacheable. A stale
// cached response (e.g. index.html) previously served to /api/local/* masked
// the real session state in the Electron renderer.
func noStoreAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Header("Cache-Control", "no-store")
		}
		c.Next()
	}
}

// logFailedRequest records every server-side HTTP error in the application log.
// Database query failures eventually returned by handlers are therefore visible
// even when Electron owns (and does not display) the sidecar's stderr.
func logFailedRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		writer := &errorResponseWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		c.Next()
		if c.Writer.Status() >= http.StatusInternalServerError {
			applog.Error("本地 HTTP 请求失败",
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"status", c.Writer.Status(),
				// Handlers include the original SQLite/SQL error in their JSON error
				// response. Capture that response so the log has the useful database
				// diagnostic without recording request bodies or SQL statements.
				"response", strings.TrimSpace(writer.body.String()),
			)
		}
	}
}

const maxLoggedErrorResponseBytes = 4096

// errorResponseWriter retains a bounded copy only of responses so error logs
// can carry handler diagnostics while keeping normal traffic unlogged.
type errorResponseWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *errorResponseWriter) Write(data []byte) (int, error) {
	w.capture(data)
	return w.ResponseWriter.Write(data)
}

func (w *errorResponseWriter) WriteString(value string) (int, error) {
	w.capture([]byte(value))
	return w.ResponseWriter.WriteString(value)
}

func (w *errorResponseWriter) capture(data []byte) {
	remaining := maxLoggedErrorResponseBytes - w.body.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = w.body.Write(data)
	}
}

// requireAPIToken 强制除启动引导路径外的所有本地 API 请求携带本次启动的
// 鉴权 token（header X-GoTeams-Api-Token，WS 在 handler 内校验 query）。
// 浏览器无法为静态页面和 img 请求添加 header，因此非 /api/ 路径（页面与静态
// 资源）以及只读受管图标路径放行；health 由 Electron 探测、auth/* 是登录与
// 握手入口、desktop/shutdown 已有独立 token，均放行。
func (s *Server) requireAPIToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/api/local/health" ||
			path == "/api/local/desktop/shutdown" ||
			path == "/api/local/ws" ||
			(c.Request.Method == http.MethodGet && strings.HasPrefix(path, ManagedIconURLPrefix)) ||
			strings.HasPrefix(path, "/api/local/auth/") ||
			!strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}
		expected := s.config.APIToken
		if expected == "" {
			applog.Warn("本地 API token 未配置，跳过校验", "path", path)
			c.Next()
			return
		}
		token := c.GetHeader("X-GoTeams-Api-Token")
		if len(token) != len(expected) || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无效的访问令牌"})
			return
		}
		c.Next()
	}
}

// currentOrchestratorGetter returns the current Orchestrator
func (s *Server) currentOrchestrator() *workflow.Orchestrator {
	return s.config.Orchestrator
}

// currentCloudClient returns the cloud client used by the current session (fallback to the default client when the session does not exist).
// When logging in with a custom address, the session holds an independent client, and proxy cloud requests must go here.
// Otherwise, the JWT of the custom cloud will be sent to the official cloud, resulting in 401.
func (s *Server) currentCloudClient() *cloud.Client {
	sess := s.currentSession()
	if sess != nil && sess.CloudClient != nil {
		return sess.CloudClient
	}
	return s.config.CloudClient
}

// requireLogin middleware that requires logged in
func (s *Server) requireLogin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.currentSession() == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
			return
		}
		c.Next()
	}
}

// handleHealth health check
func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "goteams-client",
	})
}

// handleWebSocket WebSocket endpoint
func (s *Server) handleWebSocket(c *gin.Context) {
	if s.config.WSHub == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "本地任务运行时未初始化"})
		return
	}
	s.config.WSHub.HandleWebSocket(c)
}
