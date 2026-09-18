package executor

import (
	"context"
	"time"
)

// CLI type constant
const (
	CLITypeCodex     = "codex"
	CLITypeClaude    = "claude"
	CLITypeCodeBuddy = "codebuddy"
	CLITypeOpenCode  = "opencode"
	CLITypeCursor    = "cursor"
	CLITypeCopilot   = "copilot"
	CLITypeGrok      = "grok"
	CLITypeHermes    = "hermes"
	CLITypeKimi      = "kimi"
	CLITypeQoder     = "qoder"
	CLITypeQoderCN   = "qoder-cn"
	CLITypeQwen      = "qwen"
	CLITypeOpenClaw  = "openclaw"
	CLITypePi        = "pi"
)

// Event type constant
const (
	EventStart      = "start"
	EventMessage    = "message"
	EventThinking   = "thinking"
	EventToolCall   = "tool_call"
	EventToolResult = "tool_result"
	EventPermission = "permission_request"
	EventComplete   = "complete"
	EventError      = "error"
	EventUsage      = "usage"
)

// PermissionRequest CLI rejects and returns a tool permission request in non-interactive mode.
type PermissionRequest struct {
	ToolName  string `json:"tool_name"`
	ToolUseID string `json:"tool_use_id"`
	ToolInput string `json:"tool_input,omitempty"`
}

// ExecutorEvent CLI executor generated events
type ExecutorEvent struct {
	Type      string `json:"type"`
	Content   string `json:"content,omitempty"`
	SessionID string `json:"session_id,omitempty"` // External session ID (thread_id / session_id)
	// ToolUseID 是协议侧的工具调用 ID，用于把 tool_result 精确归属到对应的 tool_call。
	// 并行工具调用的结果并不按调用顺序返回（实测 pi 会乱序回传），仅靠事件顺序无法配对。
	// 协议未提供该字段时为空，此时不得按顺序猜测归属。
	ToolUseID          string              `json:"tool_use_id,omitempty"`
	InputTokens        int                 `json:"input_tokens,omitempty"`
	OutputTokens       int                 `json:"output_tokens,omitempty"`
	Error              string              `json:"error,omitempty"`
	PermissionRequests []PermissionRequest `json:"permission_requests,omitempty"`
	Timestamp          int64               `json:"timestamp"`
}

// RunOptions New dialog execution options
type RunOptions struct {
	ExecPath     string            // CLI executable file path
	Prompt       string            // prompt word
	WorkDir      string            // working directory
	ModelProfile string            //Model name
	ExtraArgs    []string          // Additional command line parameters
	EnvVars      map[string]string // environment variables
}

// ResumeOptions Continue dialog execution options
type ResumeOptions struct {
	RunOptions
	ExternalSessionID string // External session ID (codex thread_id / claude/codebuddy session_id)
}

// Adapter CLI adapter interface.
// The concrete implementation lives in the internal/executor/<cli> sub-packages.
// The construction entry is NewAdapter() of each sub-package, registered into the
// workflow adapter registry at bootstrap and looked up by CLI type at run time;
// most adapters are assembled from executor.NewStreamAdapter with an AdapterSpec.
// CLI version and model detection are unified through DiscoverCLIs, and are not implemented repeatedly on the adapter.
type Adapter interface {
	// NewConversation new conversation
	NewConversation(ctx context.Context, opts RunOptions) (<-chan ExecutorEvent, error)
	// ResumeConversation continues the conversation
	ResumeConversation(ctx context.Context, opts ResumeOptions) (<-chan ExecutorEvent, error)
	// Stop stops execution
	Stop() error
}

// NewEvent creates event
func NewEvent(eventType string) ExecutorEvent {
	return ExecutorEvent{
		Type:      eventType,
		Timestamp: time.Now().UnixMilli(),
	}
}

// NewCompleteEvent creates a completion event
func NewCompleteEvent(sessionID string, inputTokens, outputTokens int) ExecutorEvent {
	return ExecutorEvent{
		Type:         EventComplete,
		SessionID:    sessionID,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Timestamp:    time.Now().UnixMilli(),
	}
}
