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
)

// Event type constant
const (
	EventStart      = "start"
	EventMessage    = "message"
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
	Type               string              `json:"type"`
	Content            string              `json:"content,omitempty"`
	SessionID          string              `json:"session_id,omitempty"` // External session ID (thread_id / session_id)
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
// The specific implementation is provided by codex, claude, codebuddy, and opencode sub-packages.
// The construction entry is NewAdapter() of each sub-package, which is injected by the workflow factory according to the CLI type.
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
