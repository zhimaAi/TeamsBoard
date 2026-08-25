package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"goteams-client/internal/applog"
	"goteams-client/internal/cloud"
	"goteams-client/internal/identity"
	"goteams-client/internal/localauth"
	"goteams-client/internal/localserver"
	"goteams-client/internal/secrets"
)

// AccountSession contains only the optional cloud-login lifetime state.
type AccountSession struct {
	AdminID   string
	UserID    string
	UserName  string
	DeviceMgr *identity.DeviceManager
	JWTMgr    *localauth.JWTManager
	// CloudClient The actual cloud client used in this session: the official login is the default client,
	// Custom address login is an independent client built based on domain name.
	// The HTTP proxy layer (task/work item, etc.) must use it to avoid sending the JWT of the custom cloud to the official cloud.
	CloudClient *cloud.Client
	// ServerURL 登录目标云端地址（normalized），随登录索引写入。官方登录按域名
	// 隔离恢复；custom 登录地址由用户主动选择，不受当前配置约束。
	ServerURL string
	// ServerType 登录类型（official/custom），随会话保存，登出时用于决定是否清理 custom 登录索引。
	ServerType string
	cancelFunc context.CancelFunc
}

// AccountManager manages the life cycle of account sessions
type AccountManager struct {
	mu              sync.RWMutex
	current         *AccountSession
	cloudClient     *cloud.Client
	secretStore     *secrets.DelegatingStore
	bootstrapStore  secrets.Store // Boot storage (configDir), only carries account index last_login
	configDir       string
	localAPIBaseURL string
	localRuntime    *LocalRuntime
	// officialServerURL 当前配置的官方云端地址（normalized）。写入索引时按"最后
	// 登录"语义互斥清理：官方登录清除 custom 固定索引，custom 登录清除官方
	// per-address 索引，避免切换登录方式后重启恢复错误会话。
	officialServerURL string
}

// SetOfficialServerURL sets the normalized official cloud address of the current
// configuration, used by the last-login index rules in writeLastLogin/Logout.
func (m *AccountManager) SetOfficialServerURL(normalized string) {
	m.mu.Lock()
	m.officialServerURL = normalized
	m.mu.Unlock()
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
	configDir string,
	localRuntime *LocalRuntime,
) *AccountManager {
	return &AccountManager{
		cloudClient:    cloudClient,
		secretStore:    secretStore,
		bootstrapStore: bootstrapStore,
		configDir:      configDir,
		localRuntime:   localRuntime,
	}
}

// SetLocalAPIBaseURL sets the loopback request address that the current process actually listens to.
func (m *AccountManager) SetLocalAPIBaseURL(baseURL string) {
	m.mu.Lock()
	m.localAPIBaseURL = baseURL
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
// Local databases and task execution are application-scoped and never change here.
func (m *AccountManager) Login(ctx context.Context, adminID, userID, userName, token string, opts localserver.LoginOptions) (*AccountSession, error) {
	// Always rebuild the account session when logging in explicitly or resuming login, ensuring the latest JWT and device credentials are used.
	m.mu.RLock()
	hasCurrent := m.current != nil
	m.mu.RUnlock()
	if hasCurrent {
		m.Logout()
	}

	// Select the cloud client. Local task storage is application-scoped and is
	// never selected, created, or migrated as part of login.
	cloudClient := m.cloudClient
	if opts.CloudClient != nil {
		cloudClient = opts.CloudClient
	}

	if m.localRuntime == nil || m.localRuntime.DB == nil {
		return nil, fmt.Errorf("本地任务运行时未初始化")
	}

	// Parse and persist the JWT.
	// The key backend has been switched to the current Profile (base/custom) by Server before logging in.
	// Therefore, the JWT will be saved to Profile's secrets.json, which has the same origin as the local function key.
	// - Explicit login: The processor has verified the password and obtained the token, and the order is placed directly here;
	// -Restore login: Read the persisted JWT from Profile storage and renew it to the cloud; if the renewal fails, the login status will be considered invalid.
	if token == "" {
		persisted, getErr := m.secretStore.Get(secrets.KeyJWT)
		if getErr != nil || persisted == "" {
			return nil, fmt.Errorf("无可用登录凭证（JWT 缺失且未提供）")
		}
		refreshed, rErr := cloudClient.RefreshToken(ctx, persisted)
		if rErr != nil || refreshed == nil || refreshed.Token == "" {
			// The login state has expired: clear the JWT in the Profile and the account index in the boot store to avoid another recovery failure after restarting.
			if dErr := m.secretStore.Delete(secrets.KeyJWT); dErr != nil {
				applog.Warn("[Account] 清除 JWT 失败", "error", dErr)
			}
			if dErr := m.bootstrapStore.Delete(localserver.LastLoginKey(opts.ServerURL)); dErr != nil {
				applog.Warn("[Account] 清除上次登录信息失败", "error", dErr)
			}
			if opts.ServerType == "custom" {
				if dErr := m.bootstrapStore.Delete(secrets.KeyLastLoginCustom); dErr != nil {
					applog.Warn("[Account] 清除自定义登录索引失败", "error", dErr)
				}
			}
			return nil, fmt.Errorf("云端登录态已失效，请重新登录")
		}
		token = refreshed.Token
	}
	if err := m.secretStore.Set(secrets.KeyJWT, token); err != nil {
		return nil, fmt.Errorf("保存 JWT 失败: %w", err)
	}

	db := m.localRuntime.DB

	//Initialize JWT manager
	jwtMgr := localauth.NewJWTManager(m.secretStore)

	//Initialize device manager
	deviceMgr := identity.NewDeviceManager(m.configDir, db, m.secretStore)

	//The account session must be independent of this HTTP login request.
	// Gin will cancel the request context after the request returns; long-term tasks are only explicitly canceled by Logout/application exit.
	sessionCtx, cancel := context.WithCancel(context.Background())

	session := &AccountSession{
		AdminID:     adminID,
		UserID:      userID,
		UserName:    userName,
		DeviceMgr:   deviceMgr,
		JWTMgr:      jwtMgr,
		CloudClient: cloudClient,
		ServerURL:   opts.ServerURL,
		ServerType:  opts.ServerType,
		cancelFunc:  cancel,
	}

	//Re-register the same device UUID every time a cloud account session is created.
	// The registration interface is idempotent, so there is no need to manually log out, log back in, or restart after the server rebuilds data or the credentials become invalid.
	if deviceUUID, deviceErr := deviceMgr.GetOrCreateUUID(); deviceErr != nil {
		applog.Warn("准备设备注册失败", "error", deviceErr)
	} else if deviceCred, registerErr := cloudClient.RegisterDevice(
		sessionCtx, deviceUUID, "",
	); registerErr != nil {
		applog.Warn("设备注册失败，后续请求可能无法通过设备认证", "error", registerErr)
	} else if saveErr := deviceMgr.SaveRegistration(
		adminID, deviceUUID, deviceCred.Credential,
	); saveErr != nil {
		applog.Warn("保存设备凭据失败，后续请求可能无法通过设备认证", "error", saveErr)
	}

	// The cloud token is a standard JWT, which is automatically refreshed before expiration driven by exp.
	if token, tokenErr := m.secretStore.Get(secrets.KeyJWT); tokenErr == nil {
		if claims, claimsErr := localauth.ParseClaims(token); claimsErr == nil && claims.Exp > 0 {
			if saveErr := jwtMgr.SaveToken(token, "", claims.Exp); saveErr == nil {
				jwtMgr.StartRefreshLoop(sessionCtx, cloudClient, func() {
					applog.Warn("云端登录态已过期，请重新登录")
				})
			}
		}
	}

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
		DeviceMgr:   session.DeviceMgr,
		JWTMgr:      session.JWTMgr,
		CloudClient: session.CloudClient,
		AdminID:     session.AdminID,
		UserID:      session.UserID,
		UserName:    session.UserName,
		ServerURL:   session.ServerURL,
		ServerType:  opts.ServerType,
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

	// Stop JWT refresh
	if session.JWTMgr != nil {
		session.JWTMgr.Stop()
	}

	// Cancel session context
	if session.cancelFunc != nil {
		session.cancelFunc()
	}

	// The DB, Orchestrator, WSHub and task services are application-owned and
	// intentionally continue running after cloud logout.
}

// Logout actively logs out the current account session.
// In addition to releasing resources, it clears the account index in the boot
// storage to avoid automatic recovery on the next startup.
// Only call it when the user actively logs out or switches accounts.
func (m *AccountManager) Logout() {
	m.mu.RLock()
	serverURL := ""
	serverType := ""
	officialURL := m.officialServerURL
	if m.current != nil {
		serverURL = m.current.ServerURL
		serverType = m.current.ServerType
	}
	m.mu.RUnlock()

	m.Close()

	// Actively log out/switch accounts: clear the account index in the boot storage to avoid automatic recovery by mistake at next startup.
	// 清除当前会话索引、custom 固定索引与官方索引，登出后重启不恢复任何会话；
	// 共享目录下其他配置实例的索引不受影响。
	if m.bootstrapStore != nil {
		if err := m.bootstrapStore.Delete(localserver.LastLoginKey(serverURL)); err != nil {
			applog.Warn("[Account] 清除上次登录信息失败", "error", err)
		}
		if serverType == "custom" {
			if err := m.bootstrapStore.Delete(secrets.KeyLastLoginCustom); err != nil {
				applog.Warn("[Account] 清除自定义登录索引失败", "error", err)
			}
		}
		if officialURL != "" {
			if err := m.bootstrapStore.Delete(localserver.LastLoginKey(officialURL)); err != nil {
				applog.Warn("[Account] 清除官方登录索引失败", "error", err)
			}
		}
	}
}

// writeLastLogin writes the last login account index into the boot storage (configDir), isolated from the account key.
// The index key is derived from the normalized cloud address (LastLoginKey), so different remote
// domains keep independent indexes and never overwrite each other's login state.
func (m *AccountManager) writeLastLogin(ll accountLastLogin) {
	data, err := json.Marshal(ll)
	if err != nil {
		applog.Warn("[Account] 序列化上次登录信息失败", "error", err)
		return
	}
	if err := m.bootstrapStore.Set(localserver.LastLoginKey(ll.ServerURL), string(data)); err != nil {
		applog.Warn("[Account] 写入上次登录信息失败", "error", err)
		return
	}
	m.mu.RLock()
	officialURL := m.officialServerURL
	m.mu.RUnlock()
	if ll.ServerType == "custom" {
		// custom 登录的地址由用户主动选择，额外写固定索引供恢复定位；
		// 同时清除官方索引，保证重启恢复的是最后一次登录的会话。
		if err := m.bootstrapStore.Set(secrets.KeyLastLoginCustom, string(data)); err != nil {
			applog.Warn("[Account] 写入自定义登录索引失败", "error", err)
		}
		if officialURL != "" {
			if err := m.bootstrapStore.Delete(localserver.LastLoginKey(officialURL)); err != nil {
				applog.Warn("[Account] 清除官方登录索引失败", "error", err)
			}
		}
	} else if officialURL != "" {
		// 官方登录后，最后一次登录不再是 custom，清除其固定索引。
		if err := m.bootstrapStore.Delete(secrets.KeyLastLoginCustom); err != nil {
			applog.Warn("[Account] 清除自定义登录索引失败", "error", err)
		}
	}
}
