package workflow

import (
	"goteams-client/internal/protocol"
)

// Task status transition table
var taskStatusTransitions = map[string]map[string]bool{
	protocol.TaskStatusPending: {
		protocol.TaskStatusActive:  true,
		protocol.TaskStatusBlocked: true,
	},
	protocol.TaskStatusActive: {
		protocol.TaskStatusDone:    true,
		protocol.TaskStatusBlocked: true,
		protocol.TaskStatusPending: true,
	},
	protocol.TaskStatusBlocked: {
		protocol.TaskStatusActive:  true,
		protocol.TaskStatusDone:    true,
		protocol.TaskStatusPending: true,
	},
	protocol.TaskStatusDone: {
		protocol.TaskStatusActive: true,
	},
}

// Execution status transition table
var execStatusTransitions = map[string]map[string]bool{
	protocol.ExecStatusIdle: {
		protocol.ExecStatusRunning: true,
	},
	protocol.ExecStatusRunning: {
		protocol.ExecStatusSuccess: true,
		protocol.ExecStatusFailed:  true,
		protocol.ExecStatusStopped: true,
		protocol.ExecStatusIdle:    true,
	},
}

// CanTransition checks whether the task status can transition
func CanTransition(from, to string) bool {
	if transitions, ok := taskStatusTransitions[from]; ok {
		return transitions[to]
	}
	return false
}

// CanExecTransition checks whether the execution status can transition
func CanExecTransition(from, to string) bool {
	if transitions, ok := execStatusTransitions[from]; ok {
		return transitions[to]
	}
	return false
}

// ValidateTaskStatus validates the task status
func ValidateTaskStatus(status string) bool {
	switch status {
	case protocol.TaskStatusActive,
		protocol.TaskStatusPending,
		protocol.TaskStatusDone,
		protocol.TaskStatusBlocked:
		return true
	}
	return false
}

// ValidateExecStatus validates the execution status
func ValidateExecStatus(status string) bool {
	switch status {
	case protocol.ExecStatusIdle,
		protocol.ExecStatusRunning,
		protocol.ExecStatusSuccess,
		protocol.ExecStatusFailed,
		protocol.ExecStatusStopped:
		return true
	}
	return false
}

// Session status constants
const (
	SessionStatusCreated       = "created"
	SessionStatusRunning       = "running"
	SessionStatusWaiting       = "waiting_input"
	SessionStatusStopRequested = "stop_requested"
	SessionStatusSuccess       = "success"
	SessionStatusFailed        = "failed"
	SessionStatusStopped       = "stopped"
	SessionStatusInterrupted   = "interrupted"
)

// sessionStatusTransitions is the Session status transition table
var sessionStatusTransitions = map[string]map[string]bool{
	SessionStatusCreated: {
		SessionStatusRunning:       true,
		SessionStatusStopRequested: true,
		SessionStatusFailed:        true,
		SessionStatusStopped:       true,
	},
	SessionStatusRunning: {
		SessionStatusWaiting:       true,
		SessionStatusStopRequested: true,
		SessionStatusSuccess:       true,
		SessionStatusFailed:        true,
		SessionStatusStopped:       true,
		SessionStatusInterrupted:   true,
	},
	SessionStatusWaiting: {
		SessionStatusRunning:       true,
		SessionStatusStopRequested: true,
		SessionStatusStopped:       true,
		SessionStatusInterrupted:   true,
	},
	SessionStatusStopRequested: {
		SessionStatusStopped:     true,
		SessionStatusInterrupted: true,
	},
}

// CanSessionTransition checks whether the Session status can transition
func CanSessionTransition(from, to string) bool {
	if transitions, ok := sessionStatusTransitions[from]; ok {
		return transitions[to]
	}
	return false
}

// IsTerminalStatus reports whether a Session is terminal
func IsTerminalStatus(status string) bool {
	switch status {
	case SessionStatusSuccess,
		SessionStatusFailed,
		SessionStatusStopped,
		SessionStatusInterrupted:
		return true
	}
	return false
}
