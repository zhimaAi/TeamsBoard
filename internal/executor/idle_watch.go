package executor

import (
	"strings"
	"time"
)

// idleWatchState 由 Process.mu 保护；协议事件在发送给编排层之前更新。
// 子进程存在只能作为存活线索，不能证明任务有进展；硬期限始终独立检查。
type idleWatchState struct {
	tools               map[string]watchedTool
	anonymous           []watchedTool
	permissions         map[string]bool
	anonymousPermission bool
	pausedAt            time.Time
	lastChildSeen       time.Time
	version             uint64
}

type watchedTool struct {
	started    time.Time
	subprocess bool
}

func (w *idleWatchState) paused() bool { return w.anonymousPermission || len(w.permissions) > 0 }

func (w *idleWatchState) observe(e ExecutorEvent, now time.Time) bool {
	wasPaused := w.paused()
	id := strings.TrimSpace(e.ToolUseID)
	switch e.Type {
	case EventPermission:
		if !wasPaused {
			w.pausedAt = now
		}
		if w.permissions == nil {
			w.permissions = make(map[string]bool)
		}
		identified := false
		for _, request := range e.PermissionRequests {
			if key := strings.TrimSpace(request.ToolUseID); key != "" {
				w.permissions[key] = true
				identified = true
			}
		}
		if id != "" {
			w.permissions[id] = true
			identified = true
		}
		if !identified {
			w.anonymousPermission = true
		}
	case EventToolCall:
		tool := watchedTool{started: now, subprocess: isSubprocessTool(e.Content)}
		if id == "" {
			w.anonymous = append(w.anonymous, tool)
		} else {
			if w.tools == nil {
				w.tools = make(map[string]watchedTool)
			}
			// 同一调用的 started/updated 事件不重置其期限。
			if _, exists := w.tools[id]; !exists {
				w.tools[id] = tool
			}
		}
		w.resolvePermission(id)
	case EventToolResult:
		// 有 ID 的进度只代表该工具已获准运行，不代表其他授权请求已解决。
		w.resolvePermission(id)
		if !e.ToolResultPartial {
			if id != "" {
				delete(w.tools, id)
			} else if len(w.tools)+len(w.anonymous) == 1 {
				// 协议没有 ID 时，只允许唯一候选；不猜并行结果的归属。
				clear(w.tools)
				w.anonymous = nil
			}
		}
	case EventComplete, EventError:
		clear(w.tools)
		w.anonymous = nil
		clear(w.permissions)
		w.anonymousPermission = false
	default:
		// 普通消息、usage 和思考不能解除授权等待。
		return false
	}
	resumed := wasPaused && !w.paused()
	if resumed {
		// 授权等待不消耗工具执行预算；在等待期间开始的新工具仅扣除重叠部分。
		for key, tool := range w.tools {
			tool.started = excludePermissionWait(tool.started, w.pausedAt, now)
			w.tools[key] = tool
		}
		for i := range w.anonymous {
			w.anonymous[i].started = excludePermissionWait(w.anonymous[i].started, w.pausedAt, now)
		}
		w.pausedAt = time.Time{}
		w.lastChildSeen = now
	}
	w.version++
	return resumed
}

func (w *idleWatchState) resolvePermission(id string) {
	if id != "" {
		delete(w.permissions, id)
	}
	// 对无标识的授权只在工具真正执行且仅有一个候选时恢复。
	if w.anonymousPermission && len(w.tools)+len(w.anonymous) == 1 {
		w.anonymousPermission = false
	}
}

func excludePermissionWait(started, pausedAt, now time.Time) time.Time {
	if pausedAt.IsZero() {
		return started
	}
	if started.After(pausedAt) {
		pausedAt = started
	}
	if now.After(pausedAt) {
		return started.Add(now.Sub(pausedAt))
	}
	return started
}

func isSubprocessTool(content string) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return false
	}
	switch strings.ToLower(fields[0]) {
	case "bash", "shell", "run_shell_command", "exec_command", "command_execution":
		return true
	default:
		// read、HTTP、MCP 等可能在 CLI 进程内部执行，没有子进程并不意味着挂起。
		return false
	}
}

func (w *idleWatchState) hardExpired(now time.Time) bool {
	for _, tool := range w.tools {
		if now.Sub(tool.started) >= OutputToolHardTimeout {
			return true
		}
	}
	for _, tool := range w.anonymous {
		if now.Sub(tool.started) >= OutputToolHardTimeout {
			return true
		}
	}
	return false
}

func (w *idleWatchState) hasSubprocessTool() bool {
	for _, tool := range w.tools {
		if tool.subprocess {
			return true
		}
	}
	for _, tool := range w.anonymous {
		if tool.subprocess {
			return true
		}
	}
	return false
}

func (w *idleWatchState) reason(now time.Time, lastOutput time.Time, childAlive bool) string {
	if w.paused() {
		return ""
	}
	if w.hardExpired(now) {
		return OutputToolHardStopMessage
	}
	if len(w.tools)+len(w.anonymous) == 0 {
		if now.Sub(lastOutput) >= OutputIdleTimeout {
			return OutputIdleStopMessage
		}
		return ""
	}
	if childAlive {
		w.lastChildSeen = now
	}
	// 任一工具仍有执行证据就不按会话静默误杀；每个调用的硬期限仍然生效。
	waiting := false
	check := func(tool watchedTool) {
		if !tool.subprocess || childAlive {
			waiting = true
			return
		}
		since := lastOutput
		if tool.started.After(since) {
			since = tool.started
		}
		if w.lastChildSeen.After(since) {
			since = w.lastChildSeen
		}
		if now.Sub(since) < OutputIdleTimeout {
			waiting = true
		}
	}
	for _, tool := range w.tools {
		check(tool)
	}
	for _, tool := range w.anonymous {
		check(tool)
	}
	if waiting {
		return ""
	}
	return OutputIdleStopMessage
}
