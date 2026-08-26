// Package opencode provides OpenCode CLI execution adaptation.
package opencode

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"goteams-client/internal/executor"
)

const displayName = "OpenCode"

// Adapter OpenCode CLI adapter
type Adapter struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
	pid    int
}

// NewAdapter creates the OpenCode adapter
func NewAdapter() executor.Adapter {
	return &Adapter{}
}

// NewConversation New conversation: opencode run --format json --auto -m <model> (prompt on stdin)
func (a *Adapter) NewConversation(ctx context.Context, opts executor.RunOptions) (<-chan executor.ExecutorEvent, error) {
	args := []string{"run", "--format", "json", "--auto"}
	if opts.ModelProfile != "" {
		args = append(args, "-m", opts.ModelProfile)
	}
	args = append(args, opts.ExtraArgs...)

	return a.runCommand(ctx, opts, args)
}

// ResumeConversation Continue the conversation:
// opencode run --format json --auto --session <id> -m <model> (prompt on stdin)
func (a *Adapter) ResumeConversation(ctx context.Context, opts executor.ResumeOptions) (<-chan executor.ExecutorEvent, error) {
	args := []string{"run", "--format", "json", "--auto", "--session", opts.ExternalSessionID}
	if opts.ModelProfile != "" {
		args = append(args, "-m", opts.ModelProfile)
	}
	args = append(args, opts.ExtraArgs...)

	return a.runCommand(ctx, opts.RunOptions, args)
}

// runCommand executes the command and returns the event channel
func (a *Adapter) runCommand(ctx context.Context, opts executor.RunOptions, args []string) (<-chan executor.ExecutorEvent, error) {
	innerCtx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()

	cmd := exec.CommandContext(innerCtx, opts.ExecPath, args...)
	executor.CreateProcessGroup(cmd)

	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	//Set environment variables
	if len(opts.EnvVars) > 0 {
		env := append([]string{}, os.Environ()...)
		for k, v := range opts.EnvVars {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}

	// Prompt must be passed via stdin. On Windows, `opencode` is normally
	// installed as a .cmd/.bat shim that is launched through cmd.exe; a
	// multi-line prompt in the command line gets mangled by cmd.exe and the CLI
	// only receives the first line. `opencode run` reads the message from stdin
	// when stdin is not a TTY, so the prompt is delivered there instead.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("创建 stdin 管道失败: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("创建 stdout 管道失败: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("创建 stderr 管道失败: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("启动 OpenCode 失败: %w", err)
	}

	a.mu.Lock()
	a.cmd = cmd
	a.pid = cmd.Process.Pid
	a.mu.Unlock()

	stdinDone := make(chan error, 1)
	go func() {
		_, writeErr := io.WriteString(stdin, opts.Prompt)
		closeErr := stdin.Close()
		if writeErr == nil {
			writeErr = closeErr
		}
		stdinDone <- writeErr
	}()

	// Continue reading stderr for diagnostics
	var stderrDiag diagnosticTail
	var stderrWG sync.WaitGroup
	stderrWG.Add(1)
	go func() {
		defer stderrWG.Done()
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			stderrDiag.AppendLine(scanner.Text())
		}
	}()

	eventCh := make(chan executor.ExecutorEvent, 100)

	go func() {
		defer close(eventCh)
		defer cancel()

		eventCh <- executor.NewEvent(executor.EventStart)

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

		sessionID := ""
		inputTokens := 0
		outputTokens := 0
		resultError := ""
		sawCompletion := false
		sawError := false
		sawJSON := false
		var stdoutDiag diagnosticTail

		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}

			var event opencodeEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				stdoutDiag.AppendLine(line)
				continue
			}
			sawJSON = true

			if sessionID == "" && event.getSessionID() != "" {
				sessionID = event.getSessionID()
			}

			switch event.Type {
			case "assistant", "text", "message":
				content := extractContent(event)
				if content != "" {
					eventCh <- executor.ExecutorEvent{
						Type:      executor.EventMessage,
						Content:   content,
						SessionID: sessionID,
						Timestamp: nowMillis(),
					}
				}
			case "tool_use", "tool_call":
				// New format (>=1.18): the tool call, its input and its completed
				// result are all carried in the single "part" field.
				if event.Part != nil {
					name := event.Part.Tool
					if name == "" {
						name = event.Name
					}
					input := ""
					if event.Part.State != nil {
						input = rawText(event.Part.State.Input)
					}
					if input == "" {
						input = rawText(event.Input)
					}
					callContent := strings.TrimSpace(name + " " + input)
					if event.Part.State != nil && event.Part.State.Status == "completed" && event.Part.State.Output != "" {
						// emit both the call and its result so the console can show them
						if callContent != "" {
							eventCh <- executor.ExecutorEvent{
								Type:      executor.EventToolCall,
								Content:   callContent,
								SessionID: sessionID,
								Timestamp: nowMillis(),
							}
						}
						eventCh <- executor.ExecutorEvent{
							Type:      executor.EventToolResult,
							Content:   event.Part.State.Output,
							SessionID: sessionID,
							Timestamp: nowMillis(),
						}
						continue
					}
					if event.Part.State != nil && event.Part.State.Error != "" {
						eventCh <- executor.ExecutorEvent{
							Type:      executor.EventToolResult,
							Content:   "错误: " + event.Part.State.Error,
							SessionID: sessionID,
							Timestamp: nowMillis(),
						}
					}
					eventCh <- executor.ExecutorEvent{
						Type:      executor.EventToolCall,
						Content:   callContent,
						SessionID: sessionID,
						Timestamp: nowMillis(),
					}
					continue
				}
				// Legacy format: name/input at the top level.
				content := strings.TrimSpace(event.Name + " " + rawText(event.Input))
				eventCh <- executor.ExecutorEvent{
					Type:      executor.EventToolCall,
					Content:   content,
					SessionID: sessionID,
					Timestamp: nowMillis(),
				}
			case "tool_result":
				eventCh <- executor.ExecutorEvent{
					Type:      executor.EventToolResult,
					Content:   rawText(event.Content),
					SessionID: sessionID,
					Timestamp: nowMillis(),
				}
			case "step_finish":
				// opencode >=1.18 finishes each step with type=step_finish; a final
				// "stop" reason means the whole run is done.
				if event.Part != nil && event.Part.Reason == "stop" {
					sawCompletion = true
				}
				if event.Part != nil && event.Part.Reason == "error" {
					sawError = true
					sawCompletion = true
				}
			case "result", "complete", "done":
				sawCompletion = true
				if sid := event.getSessionID(); sid != "" {
					sessionID = sid
				}
				if event.Usage.InputTokens > 0 {
					inputTokens = event.Usage.InputTokens
				}
				if event.Usage.OutputTokens > 0 {
					outputTokens = event.Usage.OutputTokens
				}
				if event.IsError {
					msg := extractErrorMessage(event)
					if msg != "" {
						resultError = msg
					} else {
						resultError = "OpenCode 返回错误结果"
					}
					sawError = true
				}
			case "error":
				msg := extractErrorMessage(event)
				if msg != "" {
					resultError = msg
				}
				sawError = true
				sawCompletion = true
			}
		}

		stdoutScanErr := scanner.Err()
		stderrWG.Wait()

		waitErr := cmd.Wait()
		stdinErr := <-stdinDone
		a.clearProcess(cmd)

		// Build error message
		if sawError && resultError == "" {
			resultError = "OpenCode 返回错误事件"
		}
		failureDetails := make([]string, 0, 4)
		if resultError != "" {
			failureDetails = append(failureDetails, resultError)
		}
		if stdinErr != nil {
			failureDetails = append(failureDetails, "写入 OpenCode Prompt 失败: "+stdinErr.Error())
		}
		if !sawCompletion {
			// OpenCode 以 0 退出码正常结束且已输出过 JSON 事件时，视为执行成功：
			// 部分 OpenCode 版本不会显式输出 complete/result/done 等结束事件，
			// 仅以关闭 stdout 表示任务结束，此时不应误报“未输出有效的结束事件”。
			if waitErr == nil && sawJSON {
				sawCompletion = true
			} else {
				failureDetails = append(failureDetails, "OpenCode 未输出有效的结束事件")
				if output := stdoutDiag.String(); output != "" {
					failureDetails = append(failureDetails, "未解析的 OpenCode 输出:\n"+output)
				}
			}
		}

		if len(failureDetails) > 0 || stdoutScanErr != nil || waitErr != nil {
			eventCh <- executor.ExecutorEvent{
				Type:      executor.EventError,
				Error:     formatError(strings.Join(failureDetails, "\n"), stderrDiag.String(), stdoutScanErr, waitErr),
				SessionID: sessionID,
				Timestamp: nowMillis(),
			}
			return
		}

		eventCh <- executor.NewCompleteEvent(sessionID, inputTokens, outputTokens)
	}()

	return eventCh, nil
}

// Stop stops execution
func (a *Adapter) Stop() error {
	a.mu.Lock()
	cmd := a.cmd
	pid := a.pid
	cancel := a.cancel
	a.mu.Unlock()

	var terminateErr error
	if cmd != nil && cmd.Process != nil && pid > 0 {
		terminateErr = executor.TerminateProcessTree(pid)
	}
	if terminateErr == nil && cancel != nil {
		cancel()
	}
	return terminateErr
}

func (a *Adapter) clearProcess(cmd *exec.Cmd) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cmd != cmd {
		return
	}
	a.cmd = nil
	a.cancel = nil
	a.pid = 0
}

// opencodeEvent OpenCode JSON event
type opencodeEvent struct {
	Type         string          `json:"type"`
	Content      json.RawMessage `json:"content"`
	Name         string          `json:"name"`
	Input        json.RawMessage `json:"input"`
	SessionID    string          `json:"session_id"`
	SessionIDAlt string          `json:"sessionID"`
	Error        *opencodeError  `json:"error"`
	IsError      bool            `json:"is_error"`
	Message      struct {
		Content []opencodeContentBlock `json:"content"`
	} `json:"message"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	// opencode may use the parts structure
	Parts []opencodePart `json:"parts"`
	// opencode >= 1.18 emits every event with a single "part" payload:
	// {"type":"text","part":{"type":"text","text":"..."}}
	// {"type":"tool_use","part":{"type":"tool","tool":"read","state":{...}}}
	// {"type":"step_finish","part":{"reason":"stop",...}}
	Part *opencodePart `json:"part"`
}

// opencodeState carries the execution state of a tool part.
type opencodeState struct {
	Status string          `json:"status"` // pending | completed | error
	Input  json.RawMessage `json:"input"`
	Output string          `json:"output"`
	Error  string          `json:"error"`
}

// getSessionID returns the session id, tolerant of both the snake_case
// (session_id) and camelCase (sessionID) field names used by OpenCode.
func (e opencodeEvent) getSessionID() string {
	if e.SessionID != "" {
		return e.SessionID
	}
	return e.SessionIDAlt
}

// opencodeError OpenCode JSON error event payload.
// OpenCode emits errors as {"type":"error","error":{"name":...,"message":...,"data":{"message":...}}}
type opencodeError struct {
	Name    string         `json:"name"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

type opencodeContentBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Name    string          `json:"name"`
	Input   json.RawMessage `json:"input"`
	Content json.RawMessage `json:"content"`
}

type opencodePart struct {
	Type string `json:"type"`
	Text string `json:"text"`
	// tool part fields
	Tool    string         `json:"tool"`
	CallID  string         `json:"callID"`
	State   *opencodeState `json:"state"`
	Reason  string         `json:"reason"`
	Session string         `json:"sessionID"`
}

func extractContent(event opencodeEvent) string {
	// New format (>=1.18): text lives in the single "part" field.
	if event.Part != nil && event.Part.Type == "text" && event.Part.Text != "" {
		return event.Part.Text
	}
	// Extract from message.content first
	for _, block := range event.Message.Content {
		if block.Type == "text" && block.Text != "" {
			return block.Text
		}
	}
	//Extract from parts
	var parts []string
	for _, part := range event.Parts {
		if part.Type == "text" && part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	// Fall back to the content field
	return rawText(event.Content)
}

// extractErrorMessage extracts the human-readable message from an OpenCode
// error event. OpenCode nests the message at either error.message or
// error.data.message, and may also carry a plain content payload.
func extractErrorMessage(event opencodeEvent) string {
	if event.Error != nil {
		if msg := strings.TrimSpace(event.Error.Message); msg != "" {
			return msg
		}
		if event.Error.Data != nil {
			if m, ok := event.Error.Data["message"].(string); ok && strings.TrimSpace(m) != "" {
				return strings.TrimSpace(m)
			}
		}
		if name := strings.TrimSpace(event.Error.Name); name != "" {
			return name
		}
	}
	if content := strings.TrimSpace(rawText(event.Content)); content != "" {
		return content
	}
	return ""
}

func rawText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var value interface{}
	if json.Unmarshal(raw, &value) == nil {
		encoded, _ := json.Marshal(value)
		return string(encoded)
	}
	return string(raw)
}

func formatError(resultError, stderrText string, stdoutScanErr, waitErr error) string {
	details := make([]string, 0, 3)
	appendDetail := func(detail string) {
		detail = strings.TrimSpace(detail)
		if detail == "" {
			return
		}
		for _, existing := range details {
			if existing == detail {
				return
			}
		}
		details = append(details, detail)
	}

	appendDetail(resultError)
	appendDetail(stderrText)
	if stdoutScanErr != nil {
		appendDetail("读取 OpenCode 输出失败: " + stdoutScanErr.Error())
	}

	exitCode := -1
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		exitCode = exitErr.ExitCode()
	}

	exitLabel := ""
	if exitCode >= 0 {
		exitLabel = fmt.Sprintf("（退出码 %d）", exitCode)
	}
	if len(details) > 0 {
		return fmt.Sprintf("OpenCode 运行失败%s：%s", exitLabel, strings.Join(details, "\n"))
	}
	if waitErr != nil && exitCode < 0 {
		waitReason := strings.TrimSpace(waitErr.Error())
		if waitReason != "" && !strings.HasPrefix(waitReason, "exit status") {
			return fmt.Sprintf("OpenCode 运行失败：%s", waitReason)
		}
	}
	return fmt.Sprintf(
		"OpenCode 运行失败%s：CLI 未输出错误详情，请检查 OpenCode 登录状态、网络和 CLI 配置",
		exitLabel,
	)
}

// diagnosticTail only retains the end of the CLI diagnostic output
type diagnosticTail struct {
	data []byte
}

const maxDiagnosticBytes = 64 * 1024

func (b *diagnosticTail) AppendLine(line string) {
	if len(b.data) > 0 {
		b.append([]byte{'\n'})
	}
	b.append([]byte(line))
}

func (b *diagnosticTail) String() string {
	return strings.TrimSpace(string(b.data))
}

func (b *diagnosticTail) append(chunk []byte) {
	if len(chunk) >= maxDiagnosticBytes {
		b.data = append(b.data[:0], chunk[len(chunk)-maxDiagnosticBytes:]...)
		return
	}
	overflow := len(b.data) + len(chunk) - maxDiagnosticBytes
	if overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
	}
	b.data = append(b.data, chunk...)
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}
