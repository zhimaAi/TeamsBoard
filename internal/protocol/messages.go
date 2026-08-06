package protocol

// ConnectionReadyData is the connection.ready message payload
type ConnectionReadyData struct {
	ConnectionID          string `json:"connection_id"`
	ServerTime            string `json:"server_time"`
	ProtocolVersion       int    `json:"protocol_version"`
	HeartbeatIntervalSecs int    `json:"heartbeat_interval_seconds"`
	PongTimeoutSecs       int    `json:"pong_timeout_seconds"`
	MaxMessageBytes       int    `json:"max_message_bytes"`
}

// TaskSyncRequestData is the task.sync.request message payload
type TaskSyncRequestData struct {
	RequestID string        `json:"request_id"`
	Mode      string        `json:"mode"`       // The first phase only allows "dirty"
	WorkItems []WorkItemRef `json:"work_items"` // Empty means all dirty tasks
	MaxTasks  int           `json:"max_tasks"`
	Reason    string        `json:"reason"` // connection_ready | admin_manual | server_reconcile
}

// WorkItemRef is a work item reference
type WorkItemRef struct {
	Type string `json:"type"` // requirement | defect
	ID   int64  `json:"id"`
}

// TaskSyncCompletedData is the task.sync.completed message payload
type TaskSyncCompletedData struct {
	RequestID      string `json:"request_id"`
	Attempted      int    `json:"attempted"`
	Acked          int    `json:"acked"`
	Failed         int    `json:"failed"`
	RemainingDirty int    `json:"remaining_dirty"`
}

// TaskDeleteData is the task.delete message payload: after local deletion, notify the cloud to sync-delete
type TaskDeleteData struct {
	LocalTaskUUID string `json:"local_task_uuid"`
}

// TaskSnapshotACKData is the task.snapshot.ack message payload
type TaskSnapshotACKData struct {
	RequestEventID string `json:"request_event_id"`
	CloudTaskID    int64  `json:"cloud_task_id"`
	LocalRevision  int    `json:"local_revision"`
	ProjectionHash string `json:"projection_hash"`
	LastUploadedAt string `json:"last_uploaded_at"`
	Result         string `json:"result"` // accepted | duplicate
}

// AgentAuthorizationChangedData is the agent.authorization.changed message payload
type AgentAuthorizationChangedData struct {
	AgentID int64  `json:"agent_id"`
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"` // member_removed | agent_disabled | agent_deleted | tenant_inactive
}

// ErrorData is the error message payload
type ErrorData struct {
	RequestEventID string `json:"request_event_id"`
	Code           string `json:"code"`
	Message        string `json:"message"`
	Retryable      bool   `json:"retryable"`
}

// DeviceRevokedData is the device.revoked message payload
type DeviceRevokedData struct {
	DeviceUUID string `json:"device_uuid"`
	Reason     string `json:"reason"`
}

// ClientUpgradeRequiredData is the client.upgrade_required message payload
type ClientUpgradeRequiredData struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
}
