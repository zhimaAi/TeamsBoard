package log

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// EventLogEntry is a log event entry
type EventLogEntry struct {
	ID          int64  `json:"id"`
	SessionUUID string `json:"session_uuid"`
	Sequence    int    `json:"sequence"`
	EventType   string `json:"event_type"`
	Payload     string `json:"payload_json"`
	CreatedAt   int64  `json:"created_at"`
}

// EventStore is the event store
type EventStore struct {
	db *sql.DB
}

// NewEventStore creates the event store
func NewEventStore(db *sql.DB) *EventStore {
	return &EventStore{db: db}
}

// AppendBatch writes events in batch
func (s *EventStore) AppendBatch(events []EventLogEntry) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT OR REPLACE INTO gt_session_events (session_uuid, sequence, event_type, payload_json, created_at)
		 VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("准备语句失败: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UnixMilli()
	for _, event := range events {
		created := event.CreatedAt
		if created == 0 {
			created = now
		}
		_, err := stmt.Exec(event.SessionUUID, event.Sequence, event.EventType, event.Payload, created)
		if err != nil {
			return fmt.Errorf("写入事件失败: %w", err)
		}
	}

	return tx.Commit()
}

// Append appends a single event
func (s *EventStore) Append(event EventLogEntry) error {
	return s.AppendBatch([]EventLogEntry{event})
}

// QueryAfterSequence queries events with sequence > afterSeq in the given session
func (s *EventStore) QueryAfterSequence(sessionUUID string, afterSeq int) ([]EventLogEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, session_uuid, sequence, event_type, payload_json, created_at
		 FROM gt_session_events
		 WHERE session_uuid = ? AND sequence > ?
		 ORDER BY sequence ASC`,
		sessionUUID, afterSeq)
	if err != nil {
		return nil, fmt.Errorf("查询事件失败: %w", err)
	}
	defer rows.Close()

	var events []EventLogEntry
	for rows.Next() {
		var e EventLogEntry
		if err := rows.Scan(&e.ID, &e.SessionUUID, &e.Sequence, &e.EventType, &e.Payload, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描事件失败: %w", err)
		}
		events = append(events, e)
	}
	// Must check rows.Err(): if an error occurs mid-iteration, Next() returns false normally,
	// missing it would return an incomplete event list as a complete result, causing the frontend to backfill missing frames.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历事件失败: %w", err)
	}
	return events, nil
}

// NextSequence gets the next sequence number for the given session
func (s *EventStore) NextSequence(sessionUUID string) (int, error) {
	var maxSeq sql.NullInt64
	err := s.db.QueryRow(
		`SELECT MAX(sequence) FROM gt_session_events WHERE session_uuid = ?`,
		sessionUUID).Scan(&maxSeq)
	if err != nil {
		return 0, fmt.Errorf("查询最大 sequence 失败: %w", err)
	}
	if !maxSeq.Valid {
		return 1, nil
	}
	return int(maxSeq.Int64) + 1, nil
}

// AppendFromJSON appends events from a JSON payload
func (s *EventStore) AppendFromJSON(sessionUUID string, sequence int, eventType string, payload interface{}) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化 payload 失败: %w", err)
	}
	return s.Append(EventLogEntry{
		SessionUUID: sessionUUID,
		Sequence:    sequence,
		EventType:   eventType,
		Payload:     string(payloadJSON),
		CreatedAt:   time.Now().UnixMilli(),
	})
}
