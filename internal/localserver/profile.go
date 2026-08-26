package localserver

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"goteams-client/internal/storage"
)

// LocalProfile local data Profile.
//
// Interface management, configuration center (Git/SSH/Docker/database/model), command center, knowledge base and other local functions
// No longer rely on cloud login, unified reading and writing of "local Profile library":
// - General Profile: ~/.goteams/data/base.db (shared by all accounts, both official and custom address logins are written into this library)
//
// The fixed underlying *sql.DB is exposed to local handlers through dbRef. Cloud
// login does not select another database or create an account-specific directory.
type LocalProfile struct {
	mu  sync.Mutex
	db  *sql.DB
	dir string //Current Profile data directory
}

// NewLocalProfile creates a local Profile manager.
func NewLocalProfile() *LocalProfile {
	return &LocalProfile{}
}

// Open opens and migrates base.db in the specified directory.
// If the target directory is consistent with the current one and the connection is still valid, reuse it directly to avoid repeatedly switching libraries.
func (p *LocalProfile) Open(ctx context.Context, dir string) (*sql.DB, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.db != nil && p.dir == dir {
		return p.db, nil
	}

	db, err := p.openAndMigrate(ctx, dir)
	if err != nil {
		return nil, err
	}

	// Close the old Profile library (the current login account library is not affected and is managed by AccountManager)
	if p.db != nil {
		p.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		p.db.Close()
	}

	p.db = db
	p.dir = dir
	return db, nil
}

// openAndMigrate opens dir/base.db and performs migration.
func (p *LocalProfile) openAndMigrate(ctx context.Context, dir string) (*sql.DB, error) {
	// The directory may not have been created yet (such as manually deleting data), make sure it exists first
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("创建 Profile 数据目录失败: %w", err)
	}
	manager, err := storage.NewManagerFileForRole(filepath.Join(dir, "base.db"), storage.DatabaseBase)
	if err != nil {
		return nil, fmt.Errorf("打开本地 Profile 数据库失败: %w", err)
	}
	if err := manager.Migrate(ctx); err != nil {
		manager.Close()
		return nil, fmt.Errorf("本地 Profile 数据库迁移失败: %w", err)
	}
	return manager.DB(), nil
}

// DB returns the current Profile database connection (nil when not initialized).
func (p *LocalProfile) DB() *sql.DB {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.db
}

// Dir returns the current Profile data directory.
func (p *LocalProfile) Dir() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dir
}

// Close closes the current Profile database connection.
func (p *LocalProfile) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.db != nil {
		p.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		p.db.Close()
		p.db = nil
		p.dir = ""
	}
}
