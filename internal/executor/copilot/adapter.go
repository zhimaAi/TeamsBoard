// Package copilot provides GitHub Copilot CLI execution adaptation.
package copilot

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

const defaultDisplayName = "GitHub Copilot CLI"

// Adapter GitHub Copilot CLI adapter.
//
// The Copilot CLI is run headless with --output-format json (JSONL), every tool
// is auto-approved (--allow-all, the non-interactive requirement), autonomous
// mode disables ask_user (--no-ask-user) and the prompt is streamed over stdin
// so multi-line prompts survive the Windows npm wrapper. Text output arrives as
// "assistant.message" events, tool activity as "tool.execution_start" /
// "tool.execution_complete", errors as "session.error"/"model.call_failure" and
// the run ends with a terminal "result" or "session.shutdown" event. The
// session id from the final "result" event is used to resume later turns.
type Adapter struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	pid         int
	displayName string
}

// NewAdapter creates the Copilot adapter.
func NewAdapter() executor.Adapter {
	return &Adapter{displayName: defaultDisplayName}
}

// NewConversation new conversation:
//
//	copilot --output-format json --stream off --allow-all --no-ask-user
//	        [--model <model>] [extraArgs]   (prompt on stdin)
func (a *Adapter) NewConversation(ctx context.Context, opts executor.RunOptions) (<-chan executor.ExecutorEvent, error) {
	return a.runCommand(ctx, opts, nil)
}

// ResumeConversation continues the conversation:
//
//	copilot --output-format json --stream off --allow-all --no-ask-user
//	        --resume <session_id> [--model <model>] [extraArgs]   (prompt on stdin)
func (a *Adapter) ResumeConversation(ctx context.Context, opts executor.ResumeOptions) (<-chan executor.ExecutorEvent, error) {
	return a.runCommand(ctx, opts.RunOptions, []string{"--resume", opts.ExternalSessionID})
}

// runCommand executes the command and returns the event channel.
func (a *Adapter) runCommand(ctx context.Context, opts executor.RunOptions, resumeArgs []string) (<-chan executor.ExecutorEvent, error) {
	innerCtx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()

	args := []string{
		"--output-format", "json",
		"--stream", "off",
		"--allow-all",
		"--no-ask-user",
		"--no-auto-update",
		"--no-color",
	}
	args = append(args, resumeArgs...)
	if opts.ModelProfile != "" && opts.ModelProfile != "auto" {
		args = append(args, "--model", opts.ModelProfile)
	}
	args = append(args, opts.ExtraArgs...)

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

	// Prompt is written to stdin (Windows-safe; avoids cmd.exe mangling multi-line prompts).
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

	// Copilot writes the non-interactive auth error to stderr; retain it so real
	// failure reasons surface on non-zero exit.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("创建 stderr 管道失败: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("启动 %s 失败: %w", a.displayLabel(), err)
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

	var stderrDiagnostic diagnosticTail
	var stderrWG sync.WaitGroup
	stderrWG.Add(1)
	go func() {
		defer stderrWG.Done()
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			stderrDiagnostic.AppendLine(scanner.Text())
		}
	}()

	eventCh := make(chan executor.ExecutorEvent, 100)

	go func() {
		defer close(eventCh)
		defer cancel()

		eventCh <- executor.NewEvent(executor.EventStart)

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

		state := &copilotStreamState{}
		var stdoutDiagnostic diagnosticTail

		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}

			var event copilotEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				stdoutDiagnostic.AppendLine(line)
				continue
			}
			state.sawJSON = true

			for _, out := range a.handleEvent(state, event) {
				eventCh <- out
			}
		}

		stdoutScanErr := scanner.Err()
		stderrWG.Wait()

		stdinErr := <-stdinDone
		waitErr := cmd.Wait()
		a.clearProcess(cmd)

		sessionID := state.sessionID
		resultError := state.resultError
		resultFailed := state.resultFailed
		sawCompletion := state.sawCompletion
		sawJSON := state.sawJSON

		if resultError == "" && resultFailed {
			resultError = a.displayLabel() + " 返回错误结果，但未提供错误详情"
		}

		failureDetails := make([]string, 0, 4)
		if resultError != "" {
			failureDetails = append(failureDetails, resultError)
		}
		if stdinErr != nil {
			failureDetails = append(failureDetails, "写入 "+a.displayLabel()+" Prompt 失败: "+stdinErr.Error())
		}
		if !sawCompletion {
			// Copilot exits with code 0 and has already emitted JSON events when
			// the task completed without a terminal marker in some versions.
			if waitErr == nil && sawJSON && !resultFailed {
				sawCompletion = true
			} else {
				failureDetails = append(failureDetails, a.displayLabel()+" 未输出有效的结束事件")
				if output := stdoutDiagnostic.String(); output != "" {
					failureDetails = append(failureDetails, "未解析的 "+a.displayLabel()+" 输出:\n"+output)
				}
			}
		}

		if len(failureDetails) > 0 || stdoutScanErr != nil || waitErr != nil {
			eventCh <- executor.ExecutorEvent{
				Type:      executor.EventError,
				Error:     formatCopilotError(a.displayLabel(), strings.Join(failureDetails, "\n"), stderrDiagnostic.String(), stdoutScanErr, waitErr),
				SessionID: sessionID,
				Timestamp: nowMillis(),
			}
			return
		}

		eventCh <- executor.NewCompleteEvent(sessionID, state.inputTokens, state.outputTokens)
	}()

	return eventCh, nil
}

// copilotStreamState tracks the mutable state accumulated while consuming the
// Copilot CLI JSONL event stream (extracted from the goroutine for testability).
type copilotStreamState struct {
	sawJSON       bool
	sessionID     string
	inputTokens   int
	outputTokens  int
	resultError   string
	resultFailed  bool
	sawCompletion bool
}

// handleEvent processes a single parsed JSONL event and returns the executor
// events to emit (possibly none).
func (a *Adapter) handleEvent(state *copilotStreamState, event copilotEvent) []executor.ExecutorEvent {
	sessionID := state.sessionID

	if sessionID == "" && event.Data.SessionID != "" {
		state.sessionID = event.Data.SessionID
		sessionID = state.sessionID
	}

	switch event.Type {
	case "assistant.message", "assistant.message_delta":
		content := strings.TrimSpace(event.Data.Content)
		if event.Type == "assistant.message_delta" {
			content = strings.TrimSpace(event.Data.DeltaContent)
		}
		if event.Data.OutputTokens > 0 {
			state.outputTokens += event.Data.OutputTokens
		}
		if content != "" {
			return []executor.ExecutorEvent{{
				Type:      executor.EventMessage,
				Content:   content,
				SessionID: sessionID,
				Timestamp: nowMillis(),
			}}
		}
	case "assistant.usage", "usage", "usage_info":
		if event.Data.InputTokens > 0 {
			state.inputTokens += event.Data.InputTokens
		}
		if event.Data.OutputTokens > 0 {
			state.outputTokens += event.Data.OutputTokens
		}
	case "tool.execution_start":
		content := strings.TrimSpace(event.Data.ToolName)
		if args := rawText(event.Data.Arguments); args != "" {
			content = strings.TrimSpace(content + " " + args)
		}
		return []executor.ExecutorEvent{{
			Type:      executor.EventToolCall,
			Content:   content,
			SessionID: sessionID,
			Timestamp: nowMillis(),
		}}
	case "tool.execution_complete", "tool.result":
		content := event.Data.ResultContent()
		if content == "" && event.Data.ErrorMessage != "" {
			content = "错误: " + event.Data.ErrorMessage
		}
		return []executor.ExecutorEvent{{
			Type:      executor.EventToolResult,
			Content:   content,
			SessionID: sessionID,
			Timestamp: nowMillis(),
		}}
	case "session.error":
		msg := strings.TrimSpace(event.Data.Message)
		if msg == "" {
			msg = event.Data.ErrorType
		}
		if msg != "" {
			state.resultError = msg
		}
		state.resultFailed = true
		state.sawCompletion = true
	case "model.call_failure":
		msg := strings.TrimSpace(event.Data.ErrorMessage)
		if msg == "" {
			msg = event.Data.Message
		}
		if msg != "" {
			state.resultError = msg
		}
		state.resultFailed = true
		state.sawCompletion = true
	case "session.shutdown":
		// "routine" = normal end of the session; "error" = fatal failure.
		if event.Data.ShutdownType == "routine" {
			state.sawCompletion = true
		} else if event.Data.ShutdownType == "error" {
			msg := strings.TrimSpace(event.Data.ErrorReason)
			if msg != "" {
				state.resultError = msg
			}
			state.resultFailed = true
			state.sawCompletion = true
		}
	case "result", "session.task_complete", "session.done":
		state.sawCompletion = true
		if event.Data.SessionID != "" {
			state.sessionID = event.Data.SessionID
			sessionID = state.sessionID
		}
		if event.Data.ExitCode != 0 {
			state.resultFailed = true
			if msg := strings.TrimSpace(event.Data.Message); msg != "" {
				state.resultError = msg
			}
		}
		if event.Data.Summary != "" && event.Type == "session.task_complete" {
			return []executor.ExecutorEvent{{
				Type:      executor.EventMessage,
				Content:   event.Data.Summary,
				SessionID: sessionID,
				Timestamp: nowMillis(),
			}}
		}
	}
	return nil
}

// Stop terminates the whole process tree by root PID before cancelling the context.
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
	if a.cmd == cmd {
		a.cmd = nil
		a.cancel = nil
		a.pid = 0
	}
	a.mu.Unlock()
}

func (a *Adapter) displayLabel() string {
	if strings.TrimSpace(a.displayName) != "" {
		return a.displayName
	}
	return defaultDisplayName
}

// copilotEvent A single JSONL line emitted by `copilot --output-format json`.
// Fields are flat on the payload object ("data").
type copilotEvent struct {
	Type string      `json:"type"`
	Data copilotData `json:"data"`
}

// ResultContent returns the human-readable result of a completed tool.
func (d copilotData) ResultContent() string {
	// Prefer the full detail block, then the concise model-facing content.
	if strings.TrimSpace(d.Result.DetailedContent) != "" {
		return strings.TrimSpace(d.Result.DetailedContent)
	}
	if strings.TrimSpace(d.Result.Content) != "" {
		return strings.TrimSpace(d.Result.Content)
	}
	return strings.TrimSpace(d.Content)
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

func formatCopilotError(displayName, resultError, stderrText string, stdoutScanErr, waitErr error) string {
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
		appendDetail("读取 " + displayName + " 输出失败: " + stdoutScanErr.Error())
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
		return fmt.Sprintf("%s 运行失败%s：%s", displayName, exitLabel, strings.Join(details, "\n"))
	}
	if waitErr != nil && exitCode < 0 {
		waitReason := strings.TrimSpace(waitErr.Error())
		if waitReason != "" && !strings.HasPrefix(waitReason, "exit status") {
			return fmt.Sprintf("%s 运行失败：%s", displayName, waitReason)
		}
	}
	return fmt.Sprintf(
		"%s 运行失败%s：CLI 未输出错误详情，请检查 %s 登录状态、网络、模型和 CLI 配置",
		displayName, exitLabel, displayName,
	)
}

// copilotData carries the payload fields referenced by the adapter. Unknown
// fields are ignored so newer CLI event shapes degrade gracefully.
type copilotData struct {
	SessionID    string          `json:"sessionId"`
	Content      string          `json:"content"`
	DeltaContent string          `json:"deltaContent"`
	Summary      string          `json:"summary"`
	Message      string          `json:"message"`
	ErrorMessage string          `json:"errorMessage"`
	ErrorType    string          `json:"errorType"`
	ErrorReason  string          `json:"errorReason"`
	ShutdownType string          `json:"shutdownType"`
	ExitCode     int             `json:"exitCode"`
	InputTokens  int             `json:"inputTokens"`
	OutputTokens int             `json:"outputTokens"`
	ToolName     string          `json:"toolName"`
	Arguments    json.RawMessage `json:"arguments"`
	Result       copilotResult   `json:"result"`
}

// copilotResult carries the tool execution result of a completed tool.
type copilotResult struct {
	Content         string `json:"content"`
	DetailedContent string `json:"detailedContent"`
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
