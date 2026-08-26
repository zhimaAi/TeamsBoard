package migration

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

// Runner is the migration runner
type Runner struct {
	db        *sql.DB
	fsys      fs.FS
	tableName string
}

// NewRunner creates the migration runner. fsys is a filesystem containing *.sql migration files (usually from the embedded assets.MigrationsFS).
func NewRunner(db *sql.DB, fsys fs.FS) *Runner {
	return NewRunnerWithTable(db, fsys, "gt_schema_migrations")
}

// NewRunnerWithTable creates a migration runner with a database-specific goose
// version table. Keeping the version tables separate prevents a base.db version
// from being mistaken for a goteams.db version (and vice versa).
func NewRunnerWithTable(db *sql.DB, fsys fs.FS, tableName string) *Runner {
	return &Runner{
		db:        db,
		fsys:      fsys,
		tableName: tableName,
	}
}

// Run executes migrations
func (r *Runner) Run(ctx context.Context) error {
	if r.fsys == nil {
		return nil
	}

	// Use goose to run migrations (the version table is created and managed by goose itself)
	provider, err := goose.NewProvider(
		goose.DialectSQLite3,
		r.db,
		r.fsys,
		goose.WithTableName(r.tableName),
	)
	if err != nil {
		return fmt.Errorf("创建 goose provider 失败: %w", err)
	}

	_, err = provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("goose 迁移执行失败: %w", err)
	}

	return nil
}
