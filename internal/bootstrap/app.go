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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"goteams-client/internal/applog"
	"goteams-client/internal/capability"
	"goteams-client/internal/cloud"
	"goteams-client/internal/config"
	"goteams-client/internal/localserver"
	"goteams-client/internal/secrets"
)

// App is the main structure of the client application
type App struct {
	dataDir        string
	configDir      string
	runtimeDir     string
	cloud          *config.CloudConfig
	secretStore    *secrets.DelegatingStore
	bootstrapStore secrets.Store
	server         *localserver.Server
	listener       net.Listener
	browserTicket  string
	lock           interface{}

	//Basic components (not dependent on database)
	cloudClient  *cloud.Client
	accountMgr   *AccountManager
	capabilities *capability.Registry
}

// New creates and initializes the application instance
func New(ctx context.Context) (*App, error) {
	app := &App{}

	// 1. Load cloud connection configuration (from INI file, does not rely on database)
	cloudCfg, err := loadCloudConfig()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}
	app.cloud = cloudCfg

	// 2. Initialize directory
	if err := app.initDirs(); err != nil {
		return nil, fmt.Errorf("初始化目录失败: %w", err)
	}
	if err := app.installBuiltinSkills(); err != nil {
		return nil, fmt.Errorf("安装内置 Skills 失败: %w", err)
	}

	// 3. Single instance lock
	if err := app.acquireLockImpl(); err != nil {
		return nil, fmt.Errorf("获取实例锁失败: %w", err)
	}

	// 4. Initialize SecretStore (entrusted storage: the backend switches with the local Profile, and the physical file falls in the Profile directory secrets.json)
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
	app.browserTicket = generateTicket()

	// 6. Initialize the cloud client (does not rely on database)
	app.cloudClient = cloud.NewClient(cloudCfg, app.secretStore)
	app.capabilities = capability.NewRegistry()

	// 7. Initialize the account manager (create the database after logging in)
	app.accountMgr = NewAccountManager(
		app.cloudClient, app.secretStore, bootstrapStore,
		app.dataDir, app.configDir, app.runtimeDir, app.capabilities,
	)

	return app, nil
}

// Run starts the local HTTP service and blocks until the context is canceled
func (a *App) Run(ctx context.Context) error {
	// Start the loopback HTTP service (the address comes from config.ini [client] listen_addr, default 127.0.0.1:18900, consistent with web/vite.config.ts proxy target)
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
		DataDir:        a.dataDir,
		ConfigDir:      a.configDir,
		RuntimeDir:     a.runtimeDir,
		BaseProfileDir: a.dataDir,
		BootstrapStore: a.bootstrapStore,
		SecretStore:    a.secretStore,
		CloudConfig:    a.cloud,
		BrowserTicket:  a.browserTicket,
		CloudClient:    a.cloudClient,
		AccountMgr:     a.accountMgr,
		Capabilities:   a.capabilities,
	})

	srv := &http.Server{
		Handler: a.server.Handler(),
	}

	//Restore the account session from the persistent login state (no need to log in again after restart)
	a.server.RestoreSession(ctx)

	// Open the browser asynchronously (set GOTEAMS_NO_OPEN_BROWSER in dev mode to disable it,
	// Avoid jumping to 18900 and missing vite's 5173 hot update)
	if os.Getenv("GOTEAMS_NO_OPEN_BROWSER") == "" {
		go a.openBrowser(addr)
	}

	// Wait for exit
	go func() {
		<-ctx.Done()
		applog.Info("正在关闭服务...")
		// Release the resources of the current account session (close database, etc.)
		// without clearing the last login index, so that the login state
		// can be restored automatically on the next startup.
		if a.accountMgr != nil {
			a.accountMgr.Close()
		}
		srv.Close()
		a.releaseLockImpl()
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

	dirs := []string{
		a.dataDir,
		// Per-server account container directories (data/accounts_<key>, official and custom share
		// the same rule) are created on demand when logging in, no need to pre-build them here.
		a.configDir,
		a.runtimeDir,
		filepath.Join(root, "skills"),
		filepath.Join(root, "logs"),
		filepath.Join(a.runtimeDir, "sessions"),
		filepath.Join(a.runtimeDir, "tasks"),
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

// openBrowser opens the default browser (with Ticket)
func (a *App) openBrowser(addr string) {
	// Wait for the service to be ready
	time.Sleep(200 * time.Millisecond)

	// dev mode (task dev / Vite) explicitly specifies the front-end address by Taskfile.
	// The ticket request is also initiated from the Vite entrance and then proxied to the backend by its /api/local. This browser
	// The first hop and the final page are both at 5173, and the host-only session cookie will also correctly land on localhost.
	if devWebURL := strings.TrimSpace(os.Getenv("GOTEAMS_DEV_WEB_URL")); devWebURL != "" {
		a.openBrowserDev(addr, devWebURL)
		return
	}

	browserURL := fmt.Sprintf("http://%s/api/local/auth/ticket?ticket=%s", addr, a.browserTicket)
	a.launchBrowser(browserURL)
}

// openBrowserDev opens the Vite dev server in dev mode and ensures that the session cookie is available.
func (a *App) openBrowserDev(addr, devWebURL string) {
	// Wait for the Vite dev server to be ready (up to about 15s) to avoid 5173 not starting when opening.
	if !waitForReachable(devWebURL, 15*time.Second) {
		// If Vite is not started (if not npm installed), go back and open the compiled front end.
		a.launchBrowser(fmt.Sprintf("http://%s/api/local/auth/ticket?ticket=%s", addr, a.browserTicket))
		return
	}

	// The browser directly accesses Vite's proxy entrance; the backend returns relative Location: /, and the browser will continue
	// Stay in Vite origin and will not enter 18900 in the address bar.
	browserURL := fmt.Sprintf("%s/api/local/auth/ticket?ticket=%s",
		strings.TrimRight(devWebURL, "/"), a.browserTicket)
	a.launchBrowser(browserURL)
}

// waitForReachable polls the target URL until 2xx/3xx is returned or times out.
// Use direct connection (disable system HTTP(S)_PROXY) to avoid localhost detection being intercepted by the proxy and mistakenly determined to be unreachable.
func waitForReachable(target string, timeout time.Duration) bool {
	client := &http.Client{
		Timeout: 800 * time.Millisecond,
		Transport: &http.Transport{
			Proxy: nil, // Direct connection, no proxy
		},
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(target)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 400 {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

// launchBrowser opens a browser to access the given URL by platform.
func (a *App) launchBrowser(browserURL string) {
	// The ticket parameter in the URL is a one-time local authentication credential and must not be written to the log.
	if u := strings.SplitN(browserURL, "?", 2)[0]; u != "" {
		applog.Debug("openBrowser", "target", u)
	} else {
		applog.Debug("openBrowser")
	}
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", "", browserURL}
	case "darwin":
		cmd = "open"
		args = []string{browserURL}
	default:
		return
	}

	exec.Command(cmd, args...).Start()
}

// generateTicket generates a one-time browser Ticket
func generateTicket() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
