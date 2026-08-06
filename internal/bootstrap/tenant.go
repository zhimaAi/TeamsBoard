package bootstrap

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"goteams-client"
	"goteams-client/internal/config"
)

const defaultEmbeddedConfigPath = "configs/goteams-client/config.ini"

// embeddedConfigPathForEnv returns the corresponding embedded configuration file path based on APP_ENV.
// For example APP_ENV=dev1 → configs/goteams-client/config_dev1.ini
func embeddedConfigPathForEnv() string {
	if env := strings.TrimSpace(os.Getenv("APP_ENV")); env != "" {
		return fmt.Sprintf("configs/goteams-client/config_%s.ini", env)
	}
	return defaultEmbeddedConfigPath
}

// loadCloudConfig loads the cloud connection configuration.
// Select the corresponding embedded configuration (dev1/dev2/prod) according to the APP_ENV environment variable,
// The configuration embedded at compile time always serves as the default value; the disk configuration only overrides the non-empty fields.
// In this way, no external config.ini is needed when running a single-file program for the first time, and the empty configuration file left over from history
// Nor will packed cloud addresses be obscured.
func loadCloudConfig() (*config.CloudConfig, error) {
	embeddedPath := embeddedConfigPathForEnv()
	defaultData, err := fs.ReadFile(assets.ConfigsFS, embeddedPath)
	if err != nil {
		// Fall back to the default configuration when the environment-specific configuration does not exist
		defaultData, err = fs.ReadFile(assets.ConfigsFS, defaultEmbeddedConfigPath)
		if err != nil {
			return nil, fmt.Errorf("读取嵌入的默认配置文件失败: %w", err)
		}
	}
	defaultINI, err := config.LoadINIBytes(defaultData)
	if err != nil {
		return nil, fmt.Errorf("解析嵌入的默认配置文件失败: %w", err)
	}

	configPath := runtimeConfigPath() // Read and write the disk target during runtime (~/.goteams/config/config.ini)
	sourcePath := embeddedPath        //The actual effective configuration source (displayed first to avoid misleading)
	var overrideINI *config.INIFile
	if data, path, ok := loadDiskConfigData(); ok {
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

	cfg.ListenAddr = value("client", "listen_addr")
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:18900"
	}

	return cfg, nil
}

// loadDiskConfigData looks for optional disk overlay configurations by priority.
// The returned file can be empty; loadCloudConfig will merge by field with embedded defaults.
//
// Read disk config.ini only if APP_ENV is not set Override:
// - APP_ENV not set (release single file run, defaults to embedded config.ini):
// exe sibling configs/.../config.ini → working directory configs/.../config.ini
// → ~/.goteams/config/config.ini. Existing anywhere will override the embedded default value.
// - APP_ENV is set (development mode such as dev1/dev2/prod): strictly use embedded
// config_<env>.ini, do not read any disk config.ini at all. This can prevent
// Cover the remaining config.ini on the disk (especially ~/.goteams/config/config.ini)
// The environment-specific cloud address also prevents go run from using the warehouse root as the working directory.
// Production default config.ini mistaken for override.
//
// That is, "load config.ini only if there is no APP_ENV"; only load the corresponding one if there is APP_ENV
// config_<env>.ini.
func loadDiskConfigData() ([]byte, string, bool) {
	if env := strings.TrimSpace(os.Getenv("APP_ENV")); env != "" {
		// Environment specified: strictly use embedded config_<env>.ini, do not read any disk config.ini
		return nil, "", false
	}
	diskCandidates := []string{}
	if exe, err := os.Executable(); err == nil {
		diskCandidates = append(diskCandidates,
			filepath.Join(filepath.Dir(exe), "configs", "goteams-client", "config.ini"))
	}
	diskCandidates = append(diskCandidates,
		filepath.Join("configs", "goteams-client", "config.ini"))
	diskCandidates = append(diskCandidates, runtimeConfigPath())
	for _, p := range diskCandidates {
		if data, err := os.ReadFile(p); err == nil {
			return data, p, true
		}
	}
	return nil, "", false
}

// runtimeConfigPath returns the runtime configuration path (~/.goteams/config/config.ini)
func runtimeConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("configs", "goteams-client", "config.ini")
	}
	return filepath.Join(home, ".goteams", "config", "config.ini")
}
