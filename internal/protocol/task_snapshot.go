package protocol

// TaskSnapshotData is the task.snapshot message payload (full task projection)
type TaskSnapshotData struct {
	LocalTaskUUID      string                 `json:"local_task_uuid"`
	LocalRevision      int                    `json:"local_revision"`
	ProjectionHash     string                 `json:"projection_hash"`
	WorkItem           WorkItemSnapshot       `json:"work_item"`
	Agent              AgentSnapshot          `json:"agent"`
	TaskStatus         string                 `json:"task_status"`
	ExecutionStatus    string                 `json:"execution_status"`
	CurrentStepKey     string                 `json:"current_step_key"`
	ExecutionSummary   string                 `json:"execution_summary"`
	StartedAt          *string                `json:"started_at"`
	FinishedAt         *string                `json:"finished_at"`
	LastLocalUpdatedAt string                 `json:"last_local_updated_at"`
	Usage              UsageStats             `json:"usage"`
	Steps              []StepSnapshot         `json:"steps"`
	Conversations      []ConversationSnapshot `json:"conversations"`
}

// WorkItemSnapshot is a work item snapshot
type WorkItemSnapshot struct {
	Type        string `json:"type"` // requirement | defect
	ID          int64  `json:"id"`
	WorkspaceID int64  `json:"workspace_id"`
	Title       string `json:"title"`
}

// AgentSnapshot is an Agent snapshot
type AgentSnapshot struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Color           string `json:"color"`
	CLIType         string `json:"cli_type"`
	WorkflowVersion int    `json:"workflow_version"`
}

// UsageStats is usage statistics
type UsageStats struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	ConversationRounds int `json:"conversation_rounds"`
}

// StepSnapshot is a step snapshot
type StepSnapshot struct {
	StepKey       string     `json:"step_key"`
	StepName      string     `json:"step_name"`
	SortOrder     int        `json:"sort_order"`
	Status        string     `json:"status"`
	PromptSummary string     `json:"prompt_summary"`
	OutputSummary string     `json:"output_summary"`
	ErrorSummary  string     `json:"error_summary"`
	Usage         UsageStats `json:"usage"`
	StartedAt     *string    `json:"started_at"`
	FinishedAt    *string    `json:"finished_at"`
	DurationMs    int64      `json:"duration_ms"`
}

// SessionMessageSnapshot is an AI-visible message that must be displayed independently in the cloud.
// Detailed events such as tool calls and reasoning are still stored only locally.
type SessionMessageSnapshot struct {
	Sequence  int    `json:"sequence"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

// ConversationRoundSnapshot is one user question and its AI reply within a conversation.
type ConversationRoundSnapshot struct {
	RoundNo           int                      `json:"round_no"`
	SessionUUID       string                   `json:"session_uuid"`
	Status            string                   `json:"status"`
	UserMessage       string                   `json:"user_message"`
	AssistantMessages []SessionMessageSnapshot `json:"assistant_messages"`
	ErrorSummary      string                   `json:"error_summary"`
	Usage             UsageStats               `json:"usage"`
	StartedAt         *string                  `json:"started_at"`
	FinishedAt        *string                  `json:"finished_at"`
	DurationMs        int64                    `json:"duration_ms"`
}

// ConversationContentSnapshot is the full conversation body stored in a cloud JSON field.
type ConversationContentSnapshot struct {
	RoundCount int                         `json:"round_count"`
	Rounds     []ConversationRoundSnapshot `json:"rounds"`
}

// ConversationSnapshot is the cloud conversation aggregated by conversation_uuid.
type ConversationSnapshot struct {
	ConversationUUID string                      `json:"conversation_uuid"`
	StepKey          string                      `json:"step_key"`
	Status           string                      `json:"status"`
	CLIType          string                      `json:"cli_type"`
	Content          ConversationContentSnapshot `json:"conversation"`
	ErrorSummary     string                      `json:"error_summary"`
	Usage            UsageStats                  `json:"usage"`
	StartedAt        *string                     `json:"started_at"`
	FinishedAt       *string                     `json:"finished_at"`
	DurationMs       int64                       `json:"duration_ms"`
}

// SessionSnapshot is the local Session intermediate structure used when building conversation projections.
type SessionSnapshot struct {
	SessionUUID       string                   `json:"session_uuid"`
	ConversationUUID  string                   `json:"conversation_uuid"`
	ParentSessionUUID string                   `json:"parent_session_uuid"`
	StepKey           string                   `json:"step_key"`
	RunNo             int                      `json:"run_no"`
	Status            string                   `json:"status"`
	CLIType           string                   `json:"cli_type"`
	RequestSummary    string                   `json:"request_summary"`
	ResponseSummary   string                   `json:"response_summary"`
	ErrorSummary      string                   `json:"error_summary"`
	Messages          []SessionMessageSnapshot `json:"messages"`
	Usage             UsageStats               `json:"usage"`
	StartedAt         *string                  `json:"started_at"`
	FinishedAt        *string                  `json:"finished_at"`
	DurationMs        int64                    `json:"duration_ms"`
}

// GroupSessionSnapshots aggregates local Sessions into cloud conversations by conversation_uuid.
func GroupSessionSnapshots(sessions []SessionSnapshot) []ConversationSnapshot {
	conversations := make([]ConversationSnapshot, 0)
	indexes := make(map[string]int)

	for _, session := range sessions {
		conversationUUID := session.ConversationUUID
		if conversationUUID == "" {
			conversationUUID = session.SessionUUID
		}
		index, exists := indexes[conversationUUID]
		if !exists {
			index = len(conversations)
			indexes[conversationUUID] = index
			conversations = append(conversations, ConversationSnapshot{
				ConversationUUID: conversationUUID,
				StepKey:          session.StepKey,
				Status:           session.Status,
				CLIType:          session.CLIType,
				Content: ConversationContentSnapshot{
					Rounds: make([]ConversationRoundSnapshot, 0),
				},
				ErrorSummary: session.ErrorSummary,
				StartedAt:    session.StartedAt,
			})
		}

		conversation := &conversations[index]
		conversation.Status = session.Status
		conversation.ErrorSummary = session.ErrorSummary
		conversation.FinishedAt = session.FinishedAt
		conversation.DurationMs += session.DurationMs
		conversation.Usage.InputTokens += session.Usage.InputTokens
		conversation.Usage.OutputTokens += session.Usage.OutputTokens
		conversation.Usage.TotalTokens += session.Usage.TotalTokens
		conversation.Content.Rounds = append(conversation.Content.Rounds, ConversationRoundSnapshot{
			RoundNo:           len(conversation.Content.Rounds) + 1,
			SessionUUID:       session.SessionUUID,
			Status:            session.Status,
			UserMessage:       session.RequestSummary,
			AssistantMessages: session.Messages,
			ErrorSummary:      session.ErrorSummary,
			Usage:             session.Usage,
			StartedAt:         session.StartedAt,
			FinishedAt:        session.FinishedAt,
			DurationMs:        session.DurationMs,
		})
		conversation.Content.RoundCount = len(conversation.Content.Rounds)
		conversation.Usage.ConversationRounds = conversation.Content.RoundCount
	}

	return conversations
}

// Task status enum
const (
	TaskStatusActive     = "active"
	TaskStatusPending    = "pending"
	TaskStatusDeveloping = "developing"
	TaskStatusDeveloped  = "developed"
)

// Execution status enum
const (
	ExecStatusIdle    = "idle"
	ExecStatusRunning = "running"
	ExecStatusSuccess = "success"
	ExecStatusFailed  = "failed"
	ExecStatusStopped = "stopped"
)

// Work item type enum
const (
	WorkItemTypeRequirement = "requirement"
	WorkItemTypeDefect      = "defect"
)

// ACK result enum
const (
	ACKResultAccepted  = "accepted"
	ACKResultDuplicate = "duplicate"
)

// Sync request mode
const (
	SyncModeDirty = "dirty"
)

// Sync request reason
const (
	SyncReasonConnectionReady = "connection_ready"
	SyncReasonAdminManual     = "admin_manual"
	SyncReasonServerReconcile = "server_reconcile"
)

// Agent authorization change reason
const (
	AuthReasonMemberRemoved  = "member_removed"
	AuthReasonAgentDisabled  = "agent_disabled"
	AuthReasonAgentDeleted   = "agent_deleted"
	AuthReasonTenantInactive = "tenant_inactive"
)
