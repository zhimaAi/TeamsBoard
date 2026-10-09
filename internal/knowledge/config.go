package knowledge

import (
	"database/sql"
	"time"
)

// RootKey 知识库根目录（KR）在 gt_client_config 中的配置键（DA-01 / IN-05）。
const RootKey = "knowledge_root_dir"

// GetRootDir 读取 KR；读不到或为空时回退 defaultDir（DA-01 验收②③）。
func GetRootDir(db *sql.DB, defaultDir string) string {
	if db == nil {
		return defaultDir
	}
	var v string
	err := db.QueryRow(`SELECT config_value FROM gt_client_config WHERE config_key=?`, RootKey).Scan(&v)
	if err == nil && v != "" {
		return v
	}
	return defaultDir
}

// SetRootDir 持久化 KR（UPSERT gt_client_config）。
func SetRootDir(db *sql.DB, dir string) error {
	if db == nil {
		return nil
	}
	now := time.Now().UnixMilli()
	_, err := db.Exec(`
		INSERT INTO gt_client_config (config_key, config_value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(config_key) DO UPDATE SET config_value=excluded.config_value, updated_at=excluded.updated_at`,
		RootKey, dir, now)
	return err
}
