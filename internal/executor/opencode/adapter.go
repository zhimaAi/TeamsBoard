// Package opencode provides OpenCode CLI execution adaptation.
package opencode

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// NewConversation New conversation: opencode run --format json --auto -m <model> <prompt>
func (a *Adapter) NewConversation(ctx context.Context, opts executor.RunOptions) (<-chan executor.ExecutorEvent, error) {
	args := []string{"run", "--format", "json", "--auto"}
	if opts.ModelProfile != "" {
		args = append(args, "-m", opts.ModelProfile)
	}
	args = append(args, opts.ExtraArgs...)
	args = append(args, opts.Prompt)

	return a.runCommand(ctx, opts, args)
}

// ResumeConversation Continue the conversation: opencode run --format json --auto --session <id> -m <model> <prompt>
func (a *Adapter) ResumeConversation(ctx context.Context, opts executor.ResumeOptions) (<-chan executor.ExecutorEvent, error) {
	args := []string{"run", "--format", "json", "--auto", "--session", opts.ExternalSessionID}
	if opts.ModelProfile != "" {
		args = append(args, "-m", opts.ModelProfile)
	}
	args = append(args, opts.ExtraArgs...)
	args = append(args, opts.Prompt)

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

			if sessionID == "" && event.SessionID != "" {
				sessionID = event.SessionID
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
			case "result", "complete", "done":
				sawCompletion = true
				if event.SessionID != "" {
					sessionID = event.SessionID
				}
				if event.Usage.InputTokens > 0 {
					inputTokens = event.Usage.InputTokens
				}
				if event.Usage.OutputTokens > 0 {
					outputTokens = event.Usage.OutputTokens
				}
				if event.IsError {
					resultError = strings.TrimSpace(rawText(event.Content))
					if resultError == "" {
						resultError = "OpenCode 返回错误结果"
					}
				}
			case "error":
				resultError = strings.TrimSpace(rawText(event.Content))
				if resultError == "" {
					resultError = event.Error
				}
				sawCompletion = true
			}
		}

		stdoutScanErr := scanner.Err()
		stderrWG.Wait()

		waitErr := cmd.Wait()
		a.clearProcess(cmd)

		// Build error message
		failureDetails := make([]string, 0, 4)
		if resultError != "" {
			failureDetails = append(failureDetails, resultError)
		}
		if !sawCompletion {
			failureDetails = append(failureDetails, "OpenCode 未输出有效的结束事件")
			if output := stdoutDiag.String(); output != "" {
				failureDetails = append(failureDetails, "未解析的 OpenCode 输出:\n"+output)
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
	Type      string          `json:"type"`
	Content   json.RawMessage `json:"content"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	SessionID string          `json:"session_id"`
	Error     string          `json:"error"`
	IsError   bool            `json:"is_error"`
	Message   struct {
		Content []opencodeContentBlock `json:"content"`
	} `json:"message"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	// opencode may use the parts structure
	Parts []opencodePart `json:"parts"`
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
}

func extractContent(event opencodeEvent) string {
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
