package workflow

import (
	"encoding/json"
	"strings"
)

// sessionStats collects the per-session execution statistics pushed to the cloud
// (tool-call count, changed files, added/deleted lines). Parsing is best-effort:
// every CLI adapter emits a slightly different tool-call format, so counters are
// exact only for tool calls; file and line numbers cover the adapters that expose
// them (codex file_change events, Write/Edit-style tools with a file_path argument,
// unified-diff tool results). Anything unrecognized is left at zero and the cloud
// UI hides empty stats.
type sessionStats struct {
	toolCalls    int
	files        map[string]struct{}
	linesAdded   int
	linesDeleted int
}

func newSessionStats() *sessionStats {
	return &sessionStats{files: make(map[string]struct{})}
}

// writeToolNames are tool names whose arguments carry a file path. Matched
// case-insensitively against the first token of the tool-call event content.
var writeToolNames = map[string]struct{}{
	"edit":                        {},
	"write":                       {},
	"multiedit":                   {},
	"notebookedit":                {},
	"edit_file":                   {},
	"write_file":                  {},
	"str_replace":                 {},
	"str_replace_based_edit_tool": {},
	"apply_patch":                 {},
}

// observeToolCall feeds one tool_call event; the event content is
// "<tool-name> <one-line args>" produced by the executor decoders.
func (s *sessionStats) observeToolCall(content string) {
	s.toolCalls++
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	// Codex file_change label: "file_change /a/b.go, /c/d.go".
	if strings.HasPrefix(content, "file_change ") {
		for _, part := range strings.Split(strings.TrimPrefix(content, "file_change "), ",") {
			if path := strings.TrimSpace(part); path != "" {
				s.files[path] = struct{}{}
			}
		}
		return
	}
	name, args, _ := strings.Cut(content, " ")
	if _, ok := writeToolNames[strings.ToLower(name)]; !ok {
		return
	}
	for _, field := range filePathFieldsFromArgs(args) {
		s.files[field] = struct{}{}
	}
}

// filePathFieldsFromArgs extracts file path values from tool arguments: a JSON
// object with file_path/file_name/absolute_path keys, or a leading bare path
// token when the args are not JSON.
func filePathFieldsFromArgs(args string) []string {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(args), &parsed); err == nil {
		paths := make([]string, 0, 2)
		for _, key := range []string{"file_path", "file_name", "absolute_path", "path"} {
			if value, ok := parsed[key].(string); ok && strings.TrimSpace(value) != "" {
				paths = append(paths, strings.TrimSpace(value))
			}
		}
		return paths
	}
	// Non-JSON args: accept a leading token that looks like a file path.
	if token, _, _ := strings.Cut(args, " "); looksLikePath(token) {
		return []string{token}
	}
	return nil
}

func looksLikePath(token string) bool {
	if token == "" {
		return false
	}
	return strings.HasPrefix(token, "/") || strings.HasPrefix(token, "./") ||
		strings.HasPrefix(token, "../") || strings.HasPrefix(token, "~") ||
		strings.Contains(token, "/") || strings.Contains(token, "\\")
}

// observeToolResult feeds one tool_result event; unified-diff-looking outputs
// contribute their +/- line counts. Results without a diff header are ignored so
// ordinary command output lines are never miscounted as code changes.
func (s *sessionStats) observeToolResult(content string) {
	if !hasDiffHeader(content) {
		return
	}
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") ||
			strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "diff "):
			continue
		case strings.HasPrefix(line, "+"):
			s.linesAdded++
		case strings.HasPrefix(line, "-"):
			s.linesDeleted++
		}
	}
}

func hasDiffHeader(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "--- ") ||
			strings.HasPrefix(line, "diff --git ") {
			return true
		}
	}
	return false
}
