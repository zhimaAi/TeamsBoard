package storage

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"

	"goteams-client"
	"goteams-client/internal/storage/migration"
)

// Manager manages the GoTeams local SQLite database
type Manager struct {
	dataDir string
	db      *sql.DB
}

// NewManager creates the database manager and opens a connection
func NewManager(dataDir string) (*Manager, error) {
	return NewManagerFile(filepath.Join(dataDir, "goteams.db"))
}

// NewManagerFile opens a SQLite database at the given path (used for the local Profile DB base.db, etc.).
func NewManagerFile(dbPath string) (*Manager, error) {
	m := &Manager{}

	var err error
	m.db, err = openSQLite(dbPath)
	if err != nil {
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
	fsys, err := fs.Sub(assets.MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("打开嵌入的迁移文件失败: %w", err)
	}

	runner := migration.NewRunner(m.db, fsys)
	return runner.Run(ctx)
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
		return nil, err
	}

	// Single writer connection
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
