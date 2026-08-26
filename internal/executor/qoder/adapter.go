// Package qoder provides Qoder CLI execution adaptation.
package qoder

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

const displayName = "Qoder CLI"

// fullPermissionArg skips every permission confirmation so unattended tasks run
// to completion (mirroring the codex/claude/copilot adapters' behavior).
// `--yolo` is the documented boolean equivalent of `--permission-mode
// bypass_permissions`, passed as a single token to avoid splitting.
const fullPermissionArg = "--yolo"

// Adapter Qoder CLI adapter.
//
// Qoder CLI's `--output-format stream-json` uses a type-based JSONL contract
// (assistant / text / tool_use / tool_result / result) in the same family as
// Claude Code, so it gets its own streaming parser modeled on that contract.
type Adapter struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
	pid    int
}

// NewAdapter creates the Qoder CLI adapter
func NewAdapter() executor.Adapter {
	return &Adapter{}
}

// NewConversation New conversation: qodercli -p --output-format stream-json [-m <model>]
// The prompt is passed via stdin (qodercli -p reads it from stdin), which avoids
// cmd.exe reinterpreting multi-line prompts on Windows.
func (a *Adapter) NewConversation(ctx context.Context, opts executor.RunOptions) (<-chan executor.ExecutorEvent, error) {
	args := []string{"-p", fullPermissionArg, "--output-format", "stream-json"}
	if opts.ModelProfile != "" {
		args = append(args, "--model", opts.ModelProfile)
	}
	args = append(args, opts.ExtraArgs...)

	return a.runCommand(ctx, opts, args)
}

// ResumeConversation Continue the conversation: qodercli -p --resume <id> --output-format stream-json [-m <model>]
// NOTE: must use --resume (not --session-id). --session-id means "create a new session with this ID";
// for an ID that already exists in qodercli's project session store it exits with code 42
// ("Session ID ... is already in use"), while --resume loads the existing conversation.
func (a *Adapter) ResumeConversation(ctx context.Context, opts executor.ResumeOptions) (<-chan executor.ExecutorEvent, error) {
	args := []string{
		"-p",
		"--resume", opts.ExternalSessionID,
		fullPermissionArg,
		"--output-format", "stream-json",
	}
	if opts.ModelProfile != "" {
		args = append(args, "--model", opts.ModelProfile)
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

	// Prompt is passed via stdin, matching the `echo "..." | qodercli -p` headless form.
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
		return nil, fmt.Errorf("启动 %s 失败: %w", displayName, err)
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

	//Continue to read and retain stderr; when execution fails, the real reason will be passed to the task execution window.
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

	//Read stream-json output
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
		resultFailed := false
		sawResult := false
		var stdoutDiagnostic diagnosticTail

		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}

			var event qoderEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				stdoutDiagnostic.AppendLine(line)
				continue
			}

			//Extract session_id
			if sessionID == "" && event.SessionID != "" {
				sessionID = event.SessionID
			}

			switch event.Type {
			case "assistant":
				for _, block := range event.Message.Content {
					switch block.Type {
					case "text":
						if block.Text != "" {
							eventCh <- executor.ExecutorEvent{
								Type:      executor.EventMessage,
								Content:   block.Text,
								SessionID: sessionID,
								Timestamp: nowMillis(),
							}
						}
					case "tool_use":
						content := block.Name
						if input := qoderRawText(block.Input); input != "" {
							content = strings.TrimSpace(content + " " + input)
						}
						eventCh <- executor.ExecutorEvent{
							Type:      executor.EventToolCall,
							Content:   content,
							SessionID: sessionID,
							Timestamp: nowMillis(),
						}
					case "tool_result":
						eventCh <- executor.ExecutorEvent{
							Type:      executor.EventToolResult,
							Content:   qoderRawText(block.Content),
							SessionID: sessionID,
							Timestamp: nowMillis(),
						}
					}
				}
				if content := qoderRawText(event.Content); content != "" {
					eventCh <- executor.ExecutorEvent{
						Type:      executor.EventMessage,
						Content:   content,
						SessionID: sessionID,
						Timestamp: nowMillis(),
					}
				}
			case "text", "message":
				if content := qoderRawText(event.Content); content != "" {
					eventCh <- executor.ExecutorEvent{
						Type:      executor.EventMessage,
						Content:   content,
						SessionID: sessionID,
						Timestamp: nowMillis(),
					}
				}
			case "tool_use", "tool_call":
				eventCh <- executor.ExecutorEvent{
					Type:      executor.EventToolCall,
					Content:   strings.TrimSpace(event.Name + " " + qoderRawText(event.Input)),
					SessionID: sessionID,
					Timestamp: nowMillis(),
				}
			case "tool_result":
				eventCh <- executor.ExecutorEvent{
					Type:      executor.EventToolResult,
					Content:   qoderRawText(event.Content),
					SessionID: sessionID,
					Timestamp: nowMillis(),
				}
			case "result":
				sawResult = true
				if event.Usage.InputTokens > 0 {
					inputTokens = event.Usage.InputTokens
				}
				if event.Usage.OutputTokens > 0 {
					outputTokens = event.Usage.OutputTokens
				}
				if event.SessionID != "" {
					sessionID = event.SessionID
				}
				if len(event.PermissionDenials) > 0 {
					requests := make([]executor.PermissionRequest, 0, len(event.PermissionDenials))
					for _, denial := range event.PermissionDenials {
						requests = append(requests, executor.PermissionRequest{
							ToolName:  denial.ToolName,
							ToolUseID: denial.ToolUseID,
							ToolInput: qoderRawText(denial.ToolInput),
						})
					}
					eventCh <- executor.ExecutorEvent{
						Type:               executor.EventPermission,
						Content:            summarizePermissionRequests(requests),
						SessionID:          sessionID,
						PermissionRequests: requests,
						Timestamp:          nowMillis(),
					}
				}
				if event.IsError {
					resultFailed = true
					resultError = strings.TrimSpace(event.Result)
				}
			}
		}

		stdoutScanErr := scanner.Err()
		if stdoutScanErr != nil {
			cancel()
		}
		stderrWG.Wait()

		waitErr := cmd.Wait()
		stdinErr := <-stdinDone
		a.clearProcess(cmd)

		failureDetails := make([]string, 0, 4)
		if resultError != "" {
			failureDetails = append(failureDetails, resultError)
		} else if resultFailed {
			failureDetails = append(failureDetails, displayName+" 返回错误结果，但未提供错误详情")
		}
		if stdinErr != nil {
			failureDetails = append(failureDetails, "写入 "+displayName+" Prompt 失败: "+stdinErr.Error())
		}
		if !sawResult {
			failureDetails = append(failureDetails, displayName+" 未输出有效的 result 结束事件")
			if output := stdoutDiagnostic.String(); output != "" {
				failureDetails = append(failureDetails, "未解析的 "+displayName+" 输出:\n"+output)
			}
		}

		if len(failureDetails) > 0 || stdoutScanErr != nil || waitErr != nil {
			eventCh <- executor.ExecutorEvent{
				Type:      executor.EventError,
				Error:     formatExecutionError(strings.Join(failureDetails, "\n"), stderrDiagnostic.String(), stdoutScanErr, waitErr),
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

	// The entire process tree must be terminated by the surviving root PID before canceling the CommandContext.
	// If you cancel first, the root process will disappear first and Windows taskkill /T will not be found later.
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

// qoderEvent Qoder CLI stream-json event (type-based JSONL)
type qoderEvent struct {
	Type              string          `json:"type"`
	Content           json.RawMessage `json:"content"`
	Name              string          `json:"name"`
	Input             json.RawMessage `json:"input"`
	SessionID         string          `json:"session_id"`
	Result            string          `json:"result"`
	IsError           bool            `json:"is_error"`
	PermissionDenials []struct {
		ToolName  string          `json:"tool_name"`
		ToolUseID string          `json:"tool_use_id"`
		ToolInput json.RawMessage `json:"tool_input"`
	} `json:"permission_denials"`
	Message struct {
		Content []qoderContentBlock `json:"content"`
	} `json:"message"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type qoderContentBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Name    string          `json:"name"`
	Input   json.RawMessage `json:"input"`
	Content json.RawMessage `json:"content"`
}

// qoderRawText renders a JSON field that may be a string, an object or an array
// as a single readable string.
func qoderRawText(raw json.RawMessage) string {
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

func summarizePermissionRequests(requests []executor.PermissionRequest) string {
	names := make([]string, 0, len(requests))
	seen := make(map[string]struct{}, len(requests))
	for _, request := range requests {
		name := strings.TrimSpace(request.ToolName)
		if name == "" {
			name = "未知工具"
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return fmt.Sprintf("%s 请求授权使用：%s", displayName, strings.Join(names, "、"))
}

func formatExecutionError(resultError, stderrText string, stdoutScanErr, waitErr error) string {
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
		displayName,
		exitLabel,
		displayName,
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
