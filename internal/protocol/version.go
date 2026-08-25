package protocol

// ProtocolVersion is sent with cloud HTTP requests for API compatibility.
const ProtocolVersion = 2

const (
	TaskStatusActive     = "active"
	TaskStatusInProgress = "in_progress"
	TaskStatusPending    = "pending"
	TaskStatusDeveloping = "developing"
	TaskStatusDeveloped  = "developed"
	TaskStatusDone       = "done"
	TaskStatusBlocked    = "blocked"
)

const (
	ExecStatusIdle    = "idle"
	ExecStatusRunning = "running"
	ExecStatusSuccess = "success"
	ExecStatusFailed  = "failed"
	ExecStatusStopped = "stopped"
)
