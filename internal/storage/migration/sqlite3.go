package migration

import (
	"database/sql"

	"modernc.org/sqlite"
)

// Uses the pure-Go modernc.org/sqlite driver, registered under the "sqlite3" alias,
// to seamlessly replace the formerly CGO-dependent mattn/go-sqlite3.
// This way the upper-layer sql.Open("sqlite3", dsn) and goose's DialectSQLite3 calls need no changes.
func init() {
	sql.Register("sqlite3", &sqlite.Driver{})
}
