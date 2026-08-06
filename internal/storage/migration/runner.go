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
	db   *sql.DB
	fsys fs.FS
}

// NewRunner creates the migration runner. fsys is a filesystem containing *.sql migration files (usually from the embedded assets.MigrationsFS).
func NewRunner(db *sql.DB, fsys fs.FS) *Runner {
	return &Runner{
		db:   db,
		fsys: fsys,
	}
}

// Run executes migrations
func (r *Runner) Run(ctx context.Context) error {
	if r.fsys == nil {
		return nil
	}

	// Fix a legacy issue: early versions pre-created an empty gt_schema_migrations table (missing goose's version=0 record),
	// causing goose to wrongly think the table is absent and error on duplicate table creation. An empty table is deleted and left to goose to recreate.
	if err := r.dropEmptyLegacyTable(ctx); err != nil {
		return fmt.Errorf("清理遗留迁移记录表失败: %w", err)
	}

	// Use goose to run migrations (the version table is created and managed by goose itself)
	provider, err := goose.NewProvider(
		goose.DialectSQLite3,
		r.db,
		r.fsys,
		goose.WithTableName("gt_schema_migrations"),
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

// dropEmptyLegacyTable deletes the empty migration record table pre-created by early versions
// A version table created by goose contains at least one version=0 record; 0 rows means it is a legacy pre-built empty table
func (r *Runner) dropEmptyLegacyTable(ctx context.Context) error {
	var name string
	err := r.db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'gt_schema_migrations'`).Scan(&name)
	if err == sql.ErrNoRows {
		return nil // Table does not exist; nothing to do
	}
	if err != nil {
		return err
	}

	var count int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM gt_schema_migrations`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		_, err := r.db.ExecContext(ctx, `DROP TABLE gt_schema_migrations`)
		return err
	}
	return nil
}
