package workflow

// The core implementation of snapshot building, hashing, and sync marking has been extracted into the internal/projection package,
// shared by workflow (orchestrator live push) and cloud (WSClient offline backfill).
// This file keeps a thin proxy for in-package tests and orchestrator calls.

import (
	"database/sql"

	"goteams-client/internal/projection"
	"goteams-client/internal/protocol"
)

// BuildSnapshot builds a task projection from the local database
func BuildSnapshot(db *sql.DB, taskUUID string) (*protocol.TaskSnapshotData, error) {
	return projection.BuildSnapshot(db, taskUUID)
}

// ComputeProjectionHash computes the projection hash
func ComputeProjectionHash(snapshot *protocol.TaskSnapshotData) string {
	return projection.ComputeProjectionHash(snapshot)
}

// MarkDirty marks a task as dirty (needs sync)
func MarkDirty(db *sql.DB, taskUUID string, revision int, hash, eventID string) error {
	return projection.MarkDirty(db, taskUUID, revision, hash, eventID)
}

// truncate safely truncates a UTF-8 string by bytes
func truncate(s string, maxLen int) string {
	return projection.Truncate(s, maxLen)
}
