package workitem

import (
	"database/sql"
	"fmt"
	"time"
)

// Store is the work-item local storage
type Store struct {
	db *sql.DB
}

// NewStore creates the work-item storage
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// UpsertSnapshot writes or updates a work-item snapshot
func (s *Store) UpsertSnapshot(adminID, userID, workItemType, workItemID, snapshotJSON string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO gt_work_item_snapshots (admin_id, user_id, work_item_type, work_item_id, snapshot_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(admin_id, user_id, work_item_type, work_item_id) DO UPDATE SET snapshot_json = excluded.snapshot_json`,
		adminID, userID, workItemType, workItemID, snapshotJSON, now)
	if err != nil {
		return fmt.Errorf("写入工作项快照失败: %w", err)
	}
	return nil
}

// ListSnapshots lists work-item snapshots
func (s *Store) ListSnapshots(adminID, userID string) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(
		`SELECT id, admin_id, user_id, work_item_type, work_item_id, snapshot_json, created_at
		 FROM gt_work_item_snapshots WHERE admin_id = ? AND user_id = ? ORDER BY created_at DESC`,
		adminID, userID)
	if err != nil {
		return nil, fmt.Errorf("查询工作项快照失败: %w", err)
	}
	defer rows.Close()

	return rowsToMaps(rows)
}

// Exists checks whether a work item already exists (dedup)
func (s *Store) Exists(adminID, userID, workItemType, workItemID string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM gt_work_item_snapshots WHERE admin_id = ? AND user_id = ? AND work_item_type = ? AND work_item_id = ?`,
		adminID, userID, workItemType, workItemID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// rowsToMaps converts sql.Rows to []map[string]interface{}
func rowsToMaps(rows *sql.Rows) ([]map[string]interface{}, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			if b, ok := values[i].([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = values[i]
			}
		}
		results = append(results, row)
	}
	return results, nil
}
