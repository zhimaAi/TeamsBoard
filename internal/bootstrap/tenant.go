package bootstrap

import (
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"

	"goteams-client"
	"goteams-client/internal/config"
)

// DefaultConfigName selects the embedded default config file (config_<name>.ini)
// at build time via -ldflags -X. Empty means the plain config.ini.
var DefaultConfigName string

// configFileName returns the config file base name for the given environment
// name; an empty name maps to the default config.ini.
func configFileName(name string) string {
	if name == "" {
		return "config.ini"
	}
	return "config_" + name + ".ini"
}

// validateConfigName rejects names that could escape the config directory
// (the name is interpolated into file paths).
func validateConfigName(name string) error {
	if name == "" {
		return nil
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return fmt.Errorf("invalid config name %q: only letters, digits, '-' and '_' are allowed", name)
		}
	}
	return nil
}

// loadCloudConfig loads the cloud connection configuration.
// The embedded config_<name>.ini at compile time always serves as the default
// value; the disk configuration only overrides the non-empty fields.
// The config name must be explicit: GOTEAMS_CONFIG_NAME wins, then the
// build-time DefaultConfigName; a missing name is a hard startup error.
func loadCloudConfig() (*config.CloudConfig, error) {
	cfgName := strings.TrimSpace(os.Getenv("GOTEAMS_CONFIG_NAME"))
	if cfgName == "" {
		cfgName = DefaultConfigName
	}
	if cfgName == "" {
		return nil, fmt.Errorf("未指定配置环境：请设置 GOTEAMS_CONFIG_NAME（dev1/dev2/cn/global）或使用带 -config 参数的构建")
	}
	if err := validateConfigName(cfgName); err != nil {
		return nil, err
	}
	embeddedPath := "configs/goteams-client/" + configFileName(cfgName)
	defaultData, err := fs.ReadFile(assets.ConfigsFS, embeddedPath)
	if err != nil {
		return nil, fmt.Errorf("读取嵌入的默认配置文件失败: %w", err)
	}
	defaultINI, err := config.LoadINIBytes(defaultData)
	if err != nil {
		return nil, fmt.Errorf("解析嵌入的默认配置文件失败: %w", err)
	}

	configPath := runtimeConfigPath(cfgName) // Read and write the disk target during runtime (~/.goteams/config/config_<name>.ini)
	sourcePath := embeddedPath
	var overrideINI *config.INIFile
	if data, path, ok := loadDiskConfigData(cfgName); ok {
		parsed, parseErr := config.LoadINIBytes(data)
		if parseErr != nil {
			return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, parseErr)
		}
		overrideINI = parsed
		configPath = path
		sourcePath = path
	}

	value := func(section, key string) string {
		if overrideINI != nil {
			if override := overrideINI.Get(section, key); override != "" {
				return override
			}
		}
		return defaultINI.Get(section, key)
	}

	clientVersion := value("client", "version")
	if clientVersion == "" {
		clientVersion = "0.1.0-dev"
	}

	cfg := config.NewCloudConfig(
		value("cloud", "api_base_url"),
		clientVersion,
		configPath,
	)
	cfg.SetConfigSource(sourcePath)

	// Electron 总会为 Go sidecar 注入 GOTEAMS_LISTEN_ADDR；独立运行（无桌面宿主）时
	// 监听随机 loopback 端口，实际地址由 runtime.json 记录。
	cfg.ListenAddr = strings.TrimSpace(os.Getenv("GOTEAMS_LISTEN_ADDR"))
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:0"
	}
	if err := validateLoopbackListenAddr(cfg.ListenAddr); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validateLoopbackListenAddr(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid local listen address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("local listen address must use a loopback IP: %q", address)
	}
	return nil
}

// loadDiskConfigData looks for optional disk overlay configurations by priority.
// The returned file can be empty; loadCloudConfig will merge by field with embedded defaults.
//
// Disk config.ini candidates in order:
// exe sibling configs/.../<config file> → working directory configs/.../<config file>
// → ~/.goteams/config/<config file>. Existing anywhere will override the embedded default value.
// The client can connect to any cloud, so the cloud address is overridden by the disk configuration
// at runtime instead of being fixed by build-time environment.
func loadDiskConfigData(cfgName string) ([]byte, string, bool) {
	fileName := configFileName(cfgName)
	diskCandidates := []string{}
	if exe, err := os.Executable(); err == nil {
		diskCandidates = append(diskCandidates,
			filepath.Join(filepath.Dir(exe), "configs", "goteams-client", fileName))
	}
	diskCandidates = append(diskCandidates,
		filepath.Join("configs", "goteams-client", fileName))
	diskCandidates = append(diskCandidates, runtimeConfigPath(cfgName))
	for _, p := range diskCandidates {
		if data, err := os.ReadFile(p); err == nil {
			return data, p, true
		}
	}
	return nil, "", false
}

// runtimeConfigPath returns the runtime configuration path (~/.goteams/config/config_<name>.ini)
func runtimeConfigPath(cfgName string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("configs", "goteams-client", configFileName(cfgName))
	}
	return filepath.Join(home, ".goteams", "config", configFileName(cfgName))
}
