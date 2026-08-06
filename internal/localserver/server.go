package localserver

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/gin-gonic/gin"

	"goteams-client/internal/apimanager"
	"goteams-client/internal/capability"
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
	"goteams-client/internal/workitem"
)

// AccountManagerInterface account manager interface (implemented by bootstrap.AccountManager)
type AccountManagerInterface interface {
	LoginAndCreateSession(ctx context.Context, adminID, userID, userName, token string, opts LoginOptions) (*SessionInfo, error)
	Logout()
	IsLoggedIn() bool
}

// LoginOptions Login options: Use a separate CloudClient and data directory when customizing the cloud address.
type LoginOptions struct {
	// CloudClient Customize the cloud address client; pass nil for official login (use the default client).
	CloudClient *cloud.Client
	// DataDir account data (cloud session library) container directory: the layer directly containing the <userID> subdirectory.
	// Both official and custom logins derive it from the server address as data/accounts_<key>, so the same address maps to the same directory.
	// Required: must always be provided by the caller (empty means an error).
	// Note: This field only determines the location of the account container and is independent of the local Profile library (base.db/key) directory.
	DataDir string
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
	DataDir        string
	ConfigDir      string
	RuntimeDir     string
	BaseProfileDir string
	BootstrapStore secrets.Store
	SecretStore    *secrets.DelegatingStore
	CloudConfig    *config.CloudConfig
	BrowserTicket  string
	CloudClient    *cloud.Client
	AccountMgr     AccountManagerInterface
	Capabilities   *capability.Registry
}

// SessionInfo session information of the current login account
type SessionInfo struct {
	DB           *sql.DB
	Orchestrator *workflow.Orchestrator
	WSHub        *WSHub
	WorkitemSvc  *workitem.Service
	DeviceMgr    *identity.DeviceManager
	JWTMgr       *localauth.JWTManager
	WSClient     *cloud.WSClient
	// CloudClient The cloud client actually used by the current session (custom login is an independent client).
	// It must be used to proxy cloud requests (task/work item/device registration, etc.), and the static official client cannot be used directly.
	CloudClient *cloud.Client
	AdminID     string
	UserID      string
	UserName    string
	DataDir     string
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

func (h *sessionHolder) db() *sql.DB {
	s := h.get()
	if s == nil {
		return nil
	}
	return s.DB
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
}

// New creates a local service instance
func New(config Config) *Server {
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		config:   config,
		sessions: &sessionHolder{},
		dbRef:    storage.NewDBRef(),
		profile:  NewLocalProfile(),
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

// currentDB gets the current database (returns nil if not logged in)
func (s *Server) currentDB() *sql.DB {
	return s.sessions.db()
}

// switchProfile switches local data Profile:
// - dbDir is the local function library (base.db) directory, which is always a common Profile (data) and is shared by all accounts;
// - secretDir is the key backend directory, isolated by login target - the official one is general Profile (data),
// The custom address login is the account container directory corresponding to the self-built cloud (data/accounts_<key>),
// Make the JWT/device keys of different clouds non-interfering with each other, and no longer generate the top-level data_cus_<key> directory.
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

// currentSessionDB returns the current cloud account session database (returns nil if not logged in).
func (s *Server) currentSessionDB() *sql.DB {
	sess := s.sessions.get()
	if sess == nil {
		return nil
	}
	return sess.DB
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

// buildRouter builds the route
func (s *Server) buildRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(s.validateSession())

	// Health check (no Session required)
	r.GET("/api/local/health", s.handleHealth)

	//Authentication routing group
	auth := r.Group("/api/local/auth")
	auth.GET("/ticket", s.handleBrowserTicket)
	auth.GET("/session", s.handleSessionStatus)
	auth.POST("/login", s.handleLogin)
	auth.POST("/logout", s.handleLogout)

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
	// Note: The JWT/device key is isolated in data/accounts_<key>/secrets.json according to the server address (official and custom share the same rule).
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

	// The following route verifies the cloud login status through requireLogin middleware
	loggedIn := r.Group("", s.requireLogin())

	// Tool Center
	toolsHandler := tools.NewHandler(s.dbRef, s.config.SecretStore)
	toolsGroup := loggedIn.Group("/api/local/tools")
	toolsHandler.RegisterRoutes(toolsGroup)

	//Task management
	tasksHandler := NewTasksHandler(s.dbRef, s.currentSessionDB, s.currentOrchestrator, s.currentWorkitemSvc, s.currentCloudClient)
	tasksGroup := loggedIn.Group("/api/local/tasks")
	tasksHandler.RegisterRoutes(tasksGroup)

	registerWebUI(r)
	return r
}

// currentOrchestratorGetter returns the current Orchestrator
func (s *Server) currentOrchestrator() *workflow.Orchestrator {
	sess := s.currentSession()
	if sess == nil {
		return nil
	}
	return sess.Orchestrator
}

// currentWorkitemSvc returns the current WorkitemSvc
func (s *Server) currentWorkitemSvc() *workitem.Service {
	sess := s.currentSession()
	if sess == nil {
		return nil
	}
	return sess.WorkitemSvc
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
	sess := s.currentSession()
	if sess == nil || sess.WSHub == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	sess.WSHub.HandleWebSocket(c)
}
