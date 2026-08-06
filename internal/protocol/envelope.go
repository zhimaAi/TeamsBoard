package protocol

import "encoding/json"

// Protocol version
const ProtocolVersion = 2

// Message type constants
const (
	// Client to cloud
	MsgTaskSnapshot      = "task.snapshot"
	MsgTaskSyncCompleted = "task.sync.completed"
	MsgTaskDelete        = "task.delete"

	// Cloud to client
	MsgConnectionReady           = "connection.ready"
	MsgTaskSyncRequest           = "task.sync.request"
	MsgTaskSnapshotACK           = "task.snapshot.ack"
	MsgAgentAuthorizationChanged = "agent.authorization.changed"
	MsgError                     = "error"
	MsgDeviceRevoked             = "device.revoked"
	MsgClientUpgradeRequired     = "client.upgrade_required"

	// Cloud to cloud browser
	MsgTaskSnapshotUpdated = "task.snapshot_updated"
)

// Subprotocol identifier
const SubProtocol = "goteams.agent.v2"

// Message size limit
const (
	MaxMessageBytes = 512 * 1024 // 512 KiB
	MaxSteps        = 100
	MaxSummaryText  = 8 * 1024  // 8 KiB
	MaxResponseText = 16 * 1024 // 16 KiB
	MaxTaskTitle    = 512
)

// Heartbeat parameters
const (
	PingIntervalSeconds = 30
	PongTimeoutSeconds  = 90
)

// Envelope is the WebSocket message envelope
type Envelope struct {
	Version int             `json:"version"`
	Type    string          `json:"type"`
	EventID string          `json:"event_id"`
	SentAt  string          `json:"sent_at"`
	Data    json.RawMessage `json:"data"`
}

// NewEnvelope creates a message envelope
func NewEnvelope(msgType, eventID, sentAt string, data interface{}) (*Envelope, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return &Envelope{
		Version: ProtocolVersion,
		Type:    msgType,
		EventID: eventID,
		SentAt:  sentAt,
		Data:    raw,
	}, nil
}
