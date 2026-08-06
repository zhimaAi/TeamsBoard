package storage

import (
	"database/sql"
	"sync"
)

// DBRef is an updatable database reference (supports multi-account isolation)
// All handlers share one DBRef; the underlying *sql.DB is updated at login
type DBRef struct {
	mu  sync.RWMutex
	ptr *sql.DB
}

// NewDBRef creates a DBRef
func NewDBRef() *DBRef {
	return &DBRef{}
}

// Set sets the underlying database connection
func (r *DBRef) Set(db *sql.DB) {
	r.mu.Lock()
	r.ptr = db
	r.mu.Unlock()
}

// Get returns the underlying database connection
func (r *DBRef) Get() *sql.DB {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ptr
}

// IsNil reports whether it is unset
func (r *DBRef) IsNil() bool {
	return r.Get() == nil
}
