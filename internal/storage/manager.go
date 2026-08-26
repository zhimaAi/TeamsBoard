package storage

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"

	"goteams-client"
	"goteams-client/internal/applog"
	"goteams-client/internal/storage/migration"
)

// Manager manages the GoTeams local SQLite database
type Manager struct {
	db   *sql.DB
	role DatabaseRole
}

// DatabaseRole identifies a physical database and its migration set.
type DatabaseRole string

const (
	DatabaseBase    DatabaseRole = "base"
	DatabaseGoTeams DatabaseRole = "goteams"
)

func (r DatabaseRole) migrationTable() string {
	if r == DatabaseBase {
		return "gt_base_schema_migrations"
	}
	return "gt_goteams_schema_migrations"
}

func (r DatabaseRole) filename() string {
	if r == DatabaseBase {
		return "base.db"
	}
	return "goteams.db"
}

// NewManager creates the database manager and opens a connection
func NewManager(dataDir string) (*Manager, error) {
	return NewManagerForRole(dataDir, DatabaseGoTeams)
}

// NewManagerForRole opens the database belonging to role under dataDir.
func NewManagerForRole(dataDir string, role DatabaseRole) (*Manager, error) {
	return NewManagerFileForRole(filepath.Join(dataDir, role.filename()), role)
}

// NewManagerFile opens a SQLite database at the given path (used for the local Profile DB base.db, etc.).
func NewManagerFile(dbPath string) (*Manager, error) {
	role := DatabaseGoTeams
	if filepath.Base(dbPath) == DatabaseBase.filename() {
		role = DatabaseBase
	}
	return NewManagerFileForRole(dbPath, role)
}

// NewManagerFileForRole opens an explicit SQLite path with an explicit schema role.
func NewManagerFileForRole(dbPath string, role DatabaseRole) (*Manager, error) {
	if role != DatabaseBase && role != DatabaseGoTeams {
		return nil, fmt.Errorf("未知数据库角色: %q", role)
	}
	m := &Manager{role: role}

	var err error
	m.db, err = openSQLite(dbPath)
	if err != nil {
		applog.Error("打开本地 SQLite 数据库失败", "path", dbPath, "role", role, "error", err)
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	return m, nil
}

// DB returns the database connection
func (m *Manager) DB() *sql.DB {
	return m.db
}

// Migrate runs DB migrations using the in-house migration runner (migration SQL is embedded in the binary at compile time)
func (m *Manager) Migrate(ctx context.Context) error {
	migrationDir := filepath.ToSlash(filepath.Join("migrations", string(m.role)))
	fsys, err := fs.Sub(assets.MigrationsFS, migrationDir)
	if err != nil {
		return fmt.Errorf("打开嵌入的迁移文件失败: %w", err)
	}

	runner := migration.NewRunnerWithTable(m.db, fsys, m.role.migrationTable())
	if err := runner.Run(ctx); err != nil {
		applog.Error("执行 SQLite 数据库迁移失败", "role", m.role, "error", err)
		return err
	}
	return nil
}

// Close closes the database connection
func (m *Manager) Close() {
	if m.db != nil {
		m.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		m.db.Close()
	}
}

// openSQLite opens the SQLite database and sets PRAGMA
func openSQLite(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000&_foreign_keys=ON&_temp_store=MEMORY", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		applog.Error("创建 SQLite 连接失败", "path", path, "error", err)
		return nil, err
	}

	// Single writer connection
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		applog.Error("连接 SQLite 数据库失败", "path", path, "error", err)
		return nil, err
	}

	return db, nil
}
