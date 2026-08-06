package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"goteams-client/internal/applog"
	"goteams-client/internal/capability"
	"goteams-client/internal/cloud"
	"goteams-client/internal/executor"
	"goteams-client/internal/executor/claude"
	"goteams-client/internal/executor/codebuddy"
	"goteams-client/internal/executor/codex"
	"goteams-client/internal/executor/opencode"
	"goteams-client/internal/identity"
	"goteams-client/internal/localauth"
	"goteams-client/internal/localserver"
	"goteams-client/internal/secrets"
	"goteams-client/internal/storage"
	"goteams-client/internal/storage/log"
	"goteams-client/internal/workflow"
	"goteams-client/internal/workitem"
)

// AccountSession A complete session of a login account (including independent SQLite database)
type AccountSession struct {
	AdminID      string
	UserID       string
	UserName     string
	DataDir      string
	DB           *sql.DB
	EventStore   *log.EventStore
	Orchestrator *workflow.Orchestrator
	WSHub        *localserver.WSHub
	WorkitemSvc  *workitem.Service
	DeviceMgr    *identity.DeviceManager
	WSClient     *cloud.WSClient
	JWTMgr       *localauth.JWTManager
	// CloudClient The actual cloud client used in this session: the official login is the default client,
	// Custom address login is an independent client built based on domain name.
	// The HTTP proxy layer (task/work item, etc.) must use it to avoid sending the JWT of the custom cloud to the official cloud.
	CloudClient *cloud.Client
	cancelFunc  context.CancelFunc
}

// AccountManager manages the life cycle of account sessions
type AccountManager struct {
	mu              sync.RWMutex
	current         *AccountSession
	cloudClient     *cloud.Client
	secretStore     *secrets.DelegatingStore
	bootstrapStore  secrets.Store // Boot storage (configDir), only carries account index last_login
	baseDataDir     string
	configDir       string
	runtimeDir      string
	localAPIBaseURL string
	capabilities    *capability.Registry
}

// accountLastLogin persists to the last login account index in the boot store (field is compatible with localserver.lastLoginInfo)
type accountLastLogin struct {
	AdminID     string `json:"admin_id"`
	UserID      string `json:"user_id"`
	UserName    string `json:"user_name"`
	DisplayName string `json:"display_name"`
	// ServerType login target: official | custom. Select the cloud client accordingly when restarting and restoring.
	ServerType string `json:"server_type,omitempty"`
	// ServerURL Custom cloud address (after normalization); custom is filled in when logging in, official is empty.
	ServerURL string `json:"server_url,omitempty"`
}

// NewAccountManager creates an account manager
func NewAccountManager(
	cloudClient *cloud.Client,
	secretStore *secrets.DelegatingStore,
	bootstrapStore secrets.Store,
	baseDataDir, configDir, runtimeDir string,
	capabilities *capability.Registry,
) *AccountManager {
	return &AccountManager{
		cloudClient:    cloudClient,
		secretStore:    secretStore,
		bootstrapStore: bootstrapStore,
		baseDataDir:    baseDataDir,
		configDir:      configDir,
		runtimeDir:     runtimeDir,
		capabilities:   capabilities,
	}
}

// SetLocalAPIBaseURL sets the loopback request address that the current process actually listens to.
func (m *AccountManager) SetLocalAPIBaseURL(baseURL string) {
	m.mu.Lock()
	m.localAPIBaseURL = baseURL
	if m.current != nil {
		m.current.Orchestrator.ConfigureSkillRuntime(baseURL, filepath.Join(filepath.Dir(m.baseDataDir), "skills"), m.capabilities)
		m.current.WorkitemSvc.ConfigureSkillRuntime(baseURL, filepath.Join(filepath.Dir(m.baseDataDir), "skills"))
	}
	m.mu.Unlock()
}

// Current returns the current account session (returns nil when not logged in)
func (m *AccountManager) Current() *AccountSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// IsLoggedIn whether logged in
func (m *AccountManager) IsLoggedIn() bool {
	return m.Current() != nil
}

// Login creates or switches account sessions
//
// token is the JWT returned for cloud login: non-empty means explicit login (passed in by the login processor); empty means
// Restore login state - at this time, the persisted JWT is read from the current Profile key storage and renewed to the cloud.
//
// opts supports custom cloud address login: use an independent CloudClient to request the address.
// The account data directory (cloud session library) is placed under the account container specified
// by opts.DataDir (always data/accounts_<key>, key is derived from the normalized server URL; official
// and custom logins share the same rule, so the same address maps to the same directory). The local
// Profile repository (base.db) is switched by the caller through switchProfile and stays in the base
// data directory, shared by all accounts.
func (m *AccountManager) Login(ctx context.Context, adminID, userID, userName, token string, opts localserver.LoginOptions) (*AccountSession, error) {
	// Always rebuild the account session when logging in explicitly or resuming login, ensuring the latest JWT and device credentials are used.
	m.mu.RLock()
	hasCurrent := m.current != nil
	m.mu.RUnlock()
	if hasCurrent {
		m.Logout()
	}

	//Select the cloud client and account container directories.
	// Note: The semantics of opts.DataDir here is "account container directory" (the layer directly containing the <userID> subdirectory),
	// Instead of the "data root directory" in earlier versions. The .goteams root directory (the layer where skills/runtime is located) is always represented by
	// Stable baseDataDir parent directory derivation to avoid skill path misalignment caused by custom login directory drift.
	cloudClient := m.cloudClient
	if opts.CloudClient != nil {
		cloudClient = opts.CloudClient
	}

	// Account container directory: always specified by the caller (opts.DataDir), derived from
	// the server address as data/accounts_<key>. Official and custom logins follow the same rule,
	// so the same server address always maps to the same directory.
	if opts.DataDir == "" {
		return nil, fmt.Errorf("账号数据目录未指定（opts.DataDir 为空）")
	}
	accountsContainer := opts.DataDir

	//Create account data directory: isolate by account ID (userID), not adminID.
	// Different sub-accounts under the same organization often share the adminID. If isolated by adminID, it will cause trouble when switching accounts.
	// The sqlite database, knowledge base, interface, etc. all fall into the same directory without actually switching.
	accountDataDir := filepath.Join(accountsContainer, userID)

	switch _, statErr := os.Stat(accountDataDir); {
	case statErr == nil:
		// The target directory already exists, no need to process
	case os.IsNotExist(statErr):
		// Compatible with older versions:
		// 1) If the account data directory by adminID already exists in the old version, it will be migrated to the directory by account ID to avoid the loss of historical data.
		// 2) The old version puts official account data in <root>/data/accounts/<userID>, and custom login
		// data in <root>/data_cus/accounts/<userID>; both are migrated into the unified <root>/data/accounts_<key>/<userID>.
		legacyDirs := []string{
			filepath.Join(accountsContainer, adminID),
			filepath.Join(m.baseDataDir, "accounts", userID),
			filepath.Join(m.baseDataDir, "..", "data_cus", "accounts", userID),
		}
		migrated := false
		for _, legacyDir := range legacyDirs {
			if legacyDir == accountDataDir {
				continue
			}
			if _, lerr := os.Stat(legacyDir); lerr == nil {
				if merr := os.Rename(legacyDir, accountDataDir); merr == nil {
					migrated = true
					break
				}
			}
		}
		if !migrated {
			if mkErr := os.MkdirAll(accountDataDir, 0700); mkErr != nil {
				return nil, fmt.Errorf("创建账号数据目录失败: %w", mkErr)
			}
		}
		// Remove the legacy empty data/accounts container after its contents have been moved out.
		_ = os.Remove(filepath.Join(m.baseDataDir, "accounts"))
	default:
		return nil, fmt.Errorf("检查账号数据目录失败: %w", statErr)
	}

	// Initialize account-level logs (output to accountDataDir/logs/ on a daily basis)
	if err := applog.InitAccountLogger(accountDataDir); err != nil {
		fmt.Printf("Failed to init account logger (does not affect main flow): %v\n", err)
	}

	// Open/create account database (cloud session data such as tasks/workflows)
	dbManager, err := storage.NewManager(accountDataDir)
	if err != nil {
		return nil, fmt.Errorf("打开账号数据库失败: %w", err)
	}

	// Parse and persist the JWT.
	// The key backend has been switched to the current Profile (base/custom) by Server before logging in.
	// Therefore, the JWT will be saved to Profile's secrets.json, which has the same origin as the local function key.
	// - Explicit login: The processor has verified the password and obtained the token, and the order is placed directly here;
	// -Restore login: Read the persisted JWT from Profile storage and renew it to the cloud; if the renewal fails, the login status will be considered invalid.
	if token == "" {
		persisted, getErr := m.secretStore.Get(secrets.KeyJWT)
		if getErr != nil || persisted == "" {
			dbManager.Close()
			return nil, fmt.Errorf("无可用登录凭证（JWT 缺失且未提供）")
		}
		refreshed, rErr := cloudClient.RefreshToken(ctx, persisted)
		if rErr != nil || refreshed == nil || refreshed.Token == "" {
			// The login state has expired: clear the JWT in the Profile and the account index in the boot store to avoid another recovery failure after restarting.
			if dErr := m.secretStore.Delete(secrets.KeyJWT); dErr != nil {
				applog.Warn("[Account] 清除 JWT 失败", "error", dErr)
			}
			if dErr := m.bootstrapStore.Delete(secrets.KeyLastLogin); dErr != nil {
				applog.Warn("[Account] 清除上次登录信息失败", "error", dErr)
			}
			dbManager.Close()
			return nil, fmt.Errorf("云端登录态已失效，请重新登录")
		}
		token = refreshed.Token
	}
	if err := m.secretStore.Set(secrets.KeyJWT, token); err != nil {
		dbManager.Close()
		return nil, fmt.Errorf("保存 JWT 失败: %w", err)
	}

	//Execute migration
	if err := dbManager.Migrate(ctx); err != nil {
		dbManager.Close()
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	db := dbManager.DB()

	// Initialize event storage (table structure created by main library migration)
	eventStore := log.NewEventStore(db)

	//Initialize JWT manager
	jwtMgr := localauth.NewJWTManager(m.secretStore)

	//Initialize device manager
	deviceMgr := identity.NewDeviceManager(m.configDir, db, m.secretStore)

	// Initialize the WebSocket client (connect to the custom cloud address during custom login)
	wsClient := cloud.NewWSClient(cloudClient, db, m.secretStore)

	//Inject adapter factory
	workflow.SetCodexAdapterFactory(func() executor.Adapter { return codex.NewAdapter() })
	workflow.SetClaudeAdapterFactory(func() executor.Adapter { return claude.NewAdapter() })
	workflow.SetCodeBuddyAdapterFactory(func() executor.Adapter { return codebuddy.NewAdapter() })
	workflow.SetOpenCodeAdapterFactory(func() executor.Adapter { return opencode.NewAdapter() })

	//Initialize the orchestrator
	orchestrator := workflow.NewOrchestrator(db, db, eventStore, wsClient, cloudClient)
	// The .goteams root directory (the layer where skills/runtime is located) is always deduced from the baseDataDir parent directory, regardless of the login type.
	rootDir := filepath.Dir(m.baseDataDir)
	skillsRoot := filepath.Join(rootDir, "skills")
	orchestrator.ConfigureSkillRuntime(m.localAPIBaseURL, skillsRoot, m.capabilities)

	//Initialize WSHub
	wsHub := localserver.NewWSHub(db)

	//Set broadcast callback
	orchestrator.SetEventBroadcaster(func(sessionUUID string, sequence int, event executor.ExecutorEvent) {
		wsHub.BroadcastEvent(sessionUUID, sequence, event)
	})
	orchestrator.SetStateBroadcaster(func(taskUUID, stepKey, sessionUUID, status string) {
		wsHub.BroadcastStateChanged(taskUUID, stepKey, sessionUUID, status)
	})

	// Register for general cycle push (cli-count, etc.)
	wsHub.RegisterPushFunc(func() (string, interface{}) {
		count, err := orchestrator.GlobalRunningSessionCount()
		if err != nil {
			return "", nil
		}
		return "app.cli_count", map[string]interface{}{"count": count}
	})
	wsHub.StartPushLoop(5 * time.Second)

	// When the cloud WebSocket connection status changes, push it to the front-end status bar ("Online/Offline" at the bottom).
	// In this way, the status bar reflects the real connection of WS instead of just the login status.
	wsClient.OnConnectionChange(func(connected bool) {
		status := "offline"
		if connected {
			status = "online"
		}
		wsHub.BroadcastToAll("cloud.status", map[string]interface{}{"status": status})
	})

	//Initialize work item service
	workitemSvc := workitem.NewService(cloudClient, db, accountDataDir, rootDir)
	workitemSvc.ConfigureSkillRuntime(m.localAPIBaseURL, skillsRoot)

	//The account session must be independent of this HTTP login request.
	// Gin will cancel the request context after the request returns; long-term tasks are only explicitly canceled by Logout/application exit.
	sessionCtx, cancel := context.WithCancel(context.Background())

	session := &AccountSession{
		AdminID:      adminID,
		UserID:       userID,
		UserName:     userName,
		DataDir:      accountDataDir,
		DB:           db,
		EventStore:   eventStore,
		Orchestrator: orchestrator,
		WSHub:        wsHub,
		WorkitemSvc:  workitemSvc,
		DeviceMgr:    deviceMgr,
		WSClient:     wsClient,
		JWTMgr:       jwtMgr,
		CloudClient:  cloudClient,
		cancelFunc:   cancel,
	}

	//Re-register the same device UUID every time a cloud account session is created.
	// The registration interface is idempotent, so there is no need to manually log out, log back in, or restart after the server rebuilds data or the credentials become invalid.
	if deviceUUID, deviceErr := deviceMgr.GetOrCreateUUID(); deviceErr != nil {
		applog.Warn("准备设备注册失败", "error", deviceErr)
	} else if deviceCred, registerErr := cloudClient.RegisterDevice(
		sessionCtx, deviceUUID, "",
	); registerErr != nil {
		applog.Warn("设备注册失败，WebSocket 将继续重试", "error", registerErr)
	} else if saveErr := deviceMgr.SaveRegistration(
		adminID, deviceUUID, deviceCred.Credential,
	); saveErr != nil {
		applog.Warn("保存设备凭据失败，WebSocket 将继续重试", "error", saveErr)
	}

	// The cloud token is a standard JWT, which is automatically refreshed before expiration driven by exp.
	if token, tokenErr := m.secretStore.Get(secrets.KeyJWT); tokenErr == nil {
		if claims, claimsErr := localauth.ParseClaims(token); claimsErr == nil && claims.Exp > 0 {
			if saveErr := jwtMgr.SaveToken(token, "", claims.Exp); saveErr == nil {
				jwtMgr.StartRefreshLoop(sessionCtx, cloudClient, func() {
					wsClient.Stop()
					applog.Warn("云端登录态已过期，请重新登录")
				})
			}
		}
	}

	// Start crash recovery
	go func() {
		if err := orchestrator.HandleCrashRecovery(); err != nil {
			applog.Error("崩溃恢复失败", "error", err)
		}
		if err := orchestrator.ReconcileExecutionStates(); err != nil {
			applog.Error("执行状态修复失败", "error", err)
		}
	}()

	// Start the cloud WebSocket after the device credentials are prepared to avoid handshake races during the login phase.
	wsClient.Start(sessionCtx)

	// Persist the last login account index (written to boot storage, independent of the Profile key) for session restoration after restart.
	// Both official and custom logins are persistent: when the custom login is restored, rebuild the client according to the ServerURL and renew it to the cloud.
	if opts.PersistLastLogin {
		m.writeLastLogin(accountLastLogin{
			AdminID:    adminID,
			UserID:     userID,
			UserName:   userName,
			ServerType: opts.ServerType,
			ServerURL:  opts.ServerURL,
		})
	}

	m.mu.Lock()
	m.current = session
	m.mu.Unlock()

	return session, nil
}

// LoginAndCreateSession Log in and create local service session information (implement localserver.AccountManagerInterface)
func (m *AccountManager) LoginAndCreateSession(ctx context.Context, adminID, userID, userName, token string, opts localserver.LoginOptions) (*localserver.SessionInfo, error) {
	session, err := m.Login(ctx, adminID, userID, userName, token, opts)
	if err != nil {
		return nil, err
	}
	return &localserver.SessionInfo{
		DB:           session.DB,
		Orchestrator: session.Orchestrator,
		WSHub:        session.WSHub,
		WorkitemSvc:  session.WorkitemSvc,
		DeviceMgr:    session.DeviceMgr,
		JWTMgr:       session.JWTMgr,
		WSClient:     session.WSClient,
		CloudClient:  session.CloudClient,
		AdminID:      session.AdminID,
		UserID:       session.UserID,
		UserName:     session.UserName,
		DataDir:      session.DataDir,
	}, nil
}

// Close releases the resources of the current account session.
// Called when the program exits: it keeps the last login index
// so that the login state can be restored on the next startup.
func (m *AccountManager) Close() {
	m.mu.Lock()
	session := m.current
	m.current = nil
	m.mu.Unlock()

	if session == nil {
		return
	}

	// Stop WebSocket
	if session.WSClient != nil {
		session.WSClient.Stop()
	}
	if session.WSHub != nil {
		session.WSHub.Close()
	}

	// Stop JWT refresh
	if session.JWTMgr != nil {
		session.JWTMgr.Stop()
	}

	// Cancel session context
	if session.cancelFunc != nil {
		session.cancelFunc()
	}

	//Close database
	if session.DB != nil {
		session.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		session.DB.Close()
	}

	//Close account-level log files
	applog.CloseAccountLogger()
}

// Logout actively logs out the current account session.
// In addition to releasing resources, it clears the account index in the boot
// storage to avoid automatic recovery on the next startup.
// Only call it when the user actively logs out or switches accounts.
func (m *AccountManager) Logout() {
	m.Close()

	// Actively log out/switch accounts: clear the account index in the boot storage to avoid automatic recovery by mistake at next startup.
	if m.bootstrapStore != nil {
		if err := m.bootstrapStore.Delete(secrets.KeyLastLogin); err != nil {
			applog.Warn("[Account] 清除上次登录信息失败", "error", err)
		}
	}
}

// writeLastLogin writes the last login account index into the boot storage (configDir), isolated from the account key.
func (m *AccountManager) writeLastLogin(ll accountLastLogin) {
	data, err := json.Marshal(ll)
	if err != nil {
		applog.Warn("[Account] 序列化上次登录信息失败", "error", err)
		return
	}
	if err := m.bootstrapStore.Set(secrets.KeyLastLogin, string(data)); err != nil {
		applog.Warn("[Account] 写入上次登录信息失败", "error", err)
	}
}
