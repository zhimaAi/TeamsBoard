package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"goteams-client/internal/applog"
	"goteams-client/internal/cloud"
	"goteams-client/internal/cloudsync"
	"goteams-client/internal/config"
	"goteams-client/internal/executor"
	"goteams-client/internal/localserver"
	"goteams-client/internal/secrets"
	"goteams-client/internal/storage"
)

// App is the main structure of the client application
type App struct {
	dataDir         string
	configDir       string
	runtimeDir      string
	taskDir         string
	cloud           *config.CloudConfig
	secretStore     *secrets.DelegatingStore
	bootstrapStore  secrets.Store
	server          *localserver.Server
	listener        net.Listener
	browserTicket   string
	apiToken        string
	apiTokenFromEnv bool
	lock            interface{}

	//Basic components (not dependent on database)
	cloudClient  *cloud.Client
	accountMgr   *AccountManager
	localRuntime *LocalRuntime
}

// New creates and initializes the application instance
func New(ctx context.Context) (*App, error) {
	app := &App{}

	// Initialize the directory and logger before reading any runtime settings so
	// even a malformed config.ini is captured in the file log.
	if err := app.initDirs(); err != nil {
		return nil, fmt.Errorf("初始化目录失败: %w", err)
	}
	// File logging must be ready before any database is opened. Previously the
	// logger was only configured for an account directory (and was never called),
	// which left ~/.goteams/logs empty and discarded sidecar stderr in desktop use.
	if err := applog.InitLogger(filepath.Join(filepath.Dir(app.dataDir), "logs")); err != nil {
		return nil, fmt.Errorf("初始化应用日志失败: %w", err)
	}
	applog.Info("正在初始化 GoTeams 客户端", "data_dir", app.dataDir)

	// Load cloud connection configuration (from INI file, does not rely on database).
	cloudCfg, err := loadCloudConfig()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}
	app.cloud = cloudCfg

	// Single instance lock.
	if err := app.acquireLockImpl(); err != nil {
		return nil, fmt.Errorf("获取实例锁失败: %w", err)
	}

	// 4. Initialize SecretStore.
	store := secrets.NewDelegating()
	app.secretStore = store

	// Boot storage: carries the account index last_login, which always points to configDir.
	// In this way, RestoreSession can restore the official account last logged in before logging in, which is isolated from the Profile key.
	bootstrapStore, bErr := secrets.New(app.configDir)
	if bErr != nil {
		return nil, fmt.Errorf("初始化引导密钥存储失败: %w", bErr)
	}
	app.bootstrapStore = bootstrapStore
	store.SetBackend(bootstrapStore)

	// 5. Generate browser Ticket
	app.browserTicket = strings.TrimSpace(os.Getenv("GOTEAMS_BROWSER_TICKET"))
	if app.browserTicket == "" {
		app.browserTicket = generateTicket()
	}

	// Generate the startup API token: the Electron main process injects it via
	// GOTEAMS_API_TOKEN; when running without Electron, self-generate so the
	// token enforcement is always on and the frontend can read it from /auth/session.
	app.apiToken = strings.TrimSpace(os.Getenv("GOTEAMS_API_TOKEN"))
	app.apiTokenFromEnv = app.apiToken != ""
	if app.apiToken == "" {
		app.apiToken = generateTicket()
	}

	// 6. Initialize the cloud client (does not rely on database)
	app.cloudClient = cloud.NewClient(cloudCfg, app.secretStore)

	// 7. Initialize the application-level local task runtime. This is deliberately
	// independent from cloud login and remains alive across logout/account switches.
	app.localRuntime, err = NewLocalRuntime(ctx, app.dataDir, app.cloudClient)
	if err != nil {
		app.releaseLockImpl()
		return nil, fmt.Errorf("初始化本地任务运行时失败: %w", err)
	}

	// Establish the base.db baseline without deleting historical files or tables.
	baseManager, err := storage.NewManagerForRole(app.dataDir, storage.DatabaseBase)
	if err != nil {
		app.localRuntime.Close()
		app.releaseLockImpl()
		return nil, fmt.Errorf("打开基础数据库失败: %w", err)
	}
	if err := baseManager.Migrate(ctx); err != nil {
		baseManager.Close()
		app.localRuntime.Close()
		app.releaseLockImpl()
		return nil, fmt.Errorf("基础数据库迁移失败: %w", err)
	}
	baseManager.Close()
	// 8. Initialize the optional cloud account manager.
	app.accountMgr = NewAccountManager(
		app.cloudClient, app.secretStore, bootstrapStore,
		app.configDir, app.localRuntime,
	)

	return app, nil
}

// Run starts the local HTTP service and blocks until the context is canceled
func (a *App) Run(ctx context.Context) error {
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	// Start the loopback HTTP service (address from GOTEAMS_LISTEN_ADDR, default 127.0.0.1:18900)
	listener, err := net.Listen("tcp", a.cloud.ListenAddr)
	if err != nil {
		return fmt.Errorf("启动监听失败: %w", err)
	}
	a.listener = listener

	addr := listener.Addr().String()
	localAPIBaseURL := "http://" + addr
	applog.Info("GoTeams 客户端已启动",
		"addr", localAPIBaseURL,
		"cloud_api", a.cloud.APIBaseURL,
		"config", a.cloud.ConfigSource(),
	)
	a.accountMgr.SetLocalAPIBaseURL(localAPIBaseURL)

	//Write to runtime.json
	if err := a.writeRuntimeInfo(addr); err != nil {
		return fmt.Errorf("写入运行时信息失败: %w", err)
	}

	//Create local HTTP service
	a.server = localserver.New(localserver.Config{
		DataDir:         a.dataDir,
		ConfigDir:       a.configDir,
		RuntimeDir:      a.runtimeDir,
		TaskRoot:        a.taskDir,
		BaseProfileDir:  a.dataDir,
		BootstrapStore:  a.bootstrapStore,
		SecretStore:     a.secretStore,
		CloudConfig:     a.cloud,
		BrowserTicket:   a.browserTicket,
		DesktopToken:    strings.TrimSpace(os.Getenv("GOTEAMS_DESKTOP_TOKEN")),
		APIToken:        a.apiToken,
		APITokenFromEnv: a.apiTokenFromEnv,
		Shutdown:        cancelRun,
		CloudClient:     a.cloudClient,
		AccountMgr:      a.accountMgr,
		TaskDB:          a.localRuntime.DB,
		Orchestrator:    a.localRuntime.Orchestrator,
		WSHub:           a.localRuntime.WSHub,
	})

	srv := &http.Server{
		Handler: a.server.Handler(),
	}

	//Restore the account session from the persistent login state (no need to log in again after restart)
	a.server.RestoreSession(runCtx)

	// Start the cloud session sync service: it periodically pushes the created/updated
	// CLI session records of cloud tasks to the cloud, in per-task creation order.
	// It uses the current account session's cloud client and skips rounds while logged out.
	cloudsync.NewSessionSyncService(a.localRuntime.DB, func() *cloud.Client {
		if sess := a.accountMgr.Current(); sess != nil {
			return sess.CloudClient
		}
		return nil
	}).Start(runCtx)

	// Push the task projection whenever a CLI session finishes asynchronously; the HTTP
	// handlers already push for user-triggered actions. Non-cloud tasks are skipped
	// inside the pusher.
	a.localRuntime.Orchestrator.AddStateListener(func(taskUUID, stepKey, sessionUUID, status string) {
		var client *cloud.Client
		if sess := a.accountMgr.Current(); sess != nil {
			client = sess.CloudClient
		}
		if client == nil {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := cloudsync.DefaultPusher().PushTaskSync(ctx, a.localRuntime.DB, client, taskUUID); err != nil {
				applog.Warn("[CloudSync] 会话结束后推送任务结果失败", "task_uuid", taskUUID, "error", err.Error())
			}
		}()
	})

	// Preload and refresh the CLI/model discovery cache in the background (see
	// executor.StartCLIDiscoveryRefresh). It re-detects installed CLIs and their
	// available models right after startup and every few minutes, persists the
	// results to config/clis.json and config/models.json, and the "选择执行 CLI"
	// popup reads those snapshots via /tasks/cli-discovery and /tasks/cli-models
	// without spawning each CLI on every request.
	executor.StartCLIDiscoveryRefresh(runCtx, a.configDir)

	// Wait for exit
	go func() {
		<-runCtx.Done()
		applog.Info("正在关闭服务...")
		// Release the resources of the current account session (close database, etc.)
		// without clearing the last login index, so that the login state
		// can be restored automatically on the next startup.
		if a.accountMgr != nil {
			a.accountMgr.Close()
		}
		if a.localRuntime != nil {
			a.localRuntime.Close()
		}
		if a.server != nil {
			a.server.Close()
		}
		srv.Close()
		a.releaseLockImpl()
		applog.Info("GoTeams 客户端已关闭")
		applog.CloseLogger()
	}()

	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// initDirs initializes ~/.goteams directory structure
func (a *App) initDirs() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	root := filepath.Join(home, ".goteams")
	a.dataDir = filepath.Join(root, "data")
	a.configDir = filepath.Join(root, "config")
	a.runtimeDir = filepath.Join(root, "runtime")
	a.taskDir = filepath.Join(root, "task")

	dirs := []string{
		a.dataDir,
		a.configDir,
		a.runtimeDir,
		a.taskDir,
		filepath.Join(root, "logs"),
		filepath.Join(a.runtimeDir, "sessions"),
		filepath.Join(a.runtimeDir, "processes"),
		filepath.Join(a.runtimeDir, "temp"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
		}
	}

	return nil
}

// writeRuntimeInfo writes runtime information
func (a *App) writeRuntimeInfo(addr string) error {
	info := map[string]interface{}{
		"address":      addr,
		"pid":          os.Getpid(),
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"api_base_url": a.cloud.APIBaseURL,
		"started_at":   time.Now().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(a.runtimeDir, "runtime.json"), data, 0600)
}

// generateTicket generates a one-time browser Ticket
func generateTicket() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
