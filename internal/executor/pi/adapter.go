// Package pi provides Pi CLI execution adaptation.
package pi

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

const displayName = "Pi Agent"

// Adapter Pi CLI adapter.
//
// Pi CLI's `--mode json` outputs type-based JSONL events:
// session / agent_start / agent_end / turn_start / turn_end /
// message_start / message_update / message_end /
// tool_execution_start / tool_execution_update / tool_execution_end.
type Adapter struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
	pid    int
}

// NewAdapter creates the Pi CLI adapter
func NewAdapter() executor.Adapter {
	return &Adapter{}
}

// NewConversation starts a new conversation: pi --mode json [--provider <p>] [--model <m>]
// The prompt is passed via stdin, which avoids cmd.exe reinterpreting multi-line prompts on Windows.
func (a *Adapter) NewConversation(ctx context.Context, opts executor.RunOptions) (<-chan executor.ExecutorEvent, error) {
	args := []string{"--mode", "json"}
	args = append(args, modelArgs(opts.ModelProfile)...)
	args = append(args, opts.ExtraArgs...)

	return a.runCommand(ctx, opts, args)
}

// ResumeConversation continues a conversation: pi --session <id> --mode json [...]
func (a *Adapter) ResumeConversation(ctx context.Context, opts executor.ResumeOptions) (<-chan executor.ExecutorEvent, error) {
	args := []string{"--session", opts.ExternalSessionID, "--mode", "json"}
	args = append(args, modelArgs(opts.ModelProfile)...)
	args = append(args, opts.ExtraArgs...)

	return a.runCommand(ctx, opts.RunOptions, args)
}

// modelArgs converts "provider/model" or bare "model" into --provider / --model flags.
// "auto" is skipped since pi uses it as the default.
func modelArgs(modelProfile string) []string {
	if modelProfile == "" || modelProfile == "auto" {
		return nil
	}
	if strings.Contains(modelProfile, "/") {
		parts := strings.SplitN(modelProfile, "/", 2)
		return []string{"--provider", parts[0], "--model", parts[1]}
	}
	return []string{"--model", modelProfile}
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

	// Set environment variables
	if len(opts.EnvVars) > 0 {
		env := append([]string{}, os.Environ()...)
		for k, v := range opts.EnvVars {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}

	// Prompt is passed via stdin.
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

	// Retain stderr for diagnostics.
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

	// Read --mode json output
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
		// Pi 的 text_delta 按字符/短片段输出，直接转发会让前端逐字渲染。
		// 与其它 CLI 一致，缓冲到 message_end 完整事件后再输出。
		messageBuffer := ""
		var stdoutDiagnostic diagnosticTail

		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}

			var event piEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				stdoutDiagnostic.AppendLine(line)
				continue
			}

			switch event.Type {
			case "session":
				sessionID = event.ID

			case "agent_start":
				// Agent started — no content to emit

			case "agent_end":
				sawResult = true
				if event.Usage.InputTokens > 0 || event.Usage.OutputTokens > 0 {
					inputTokens = event.Usage.InputTokens
					outputTokens = event.Usage.OutputTokens
				}

			case "turn_start", "turn_end":
				// Turn lifecycle — no content to emit

			case "message_start":
				// 新消息开始，重置流式缓冲
				messageBuffer = ""
				if content := extractMessageText(event.Message); content != "" {
					eventCh <- executor.ExecutorEvent{
						Type:      executor.EventMessage,
						Content:   content,
						SessionID: sessionID,
						Timestamp: nowMillis(),
					}
				}

			case "message_update":
				// Deltas: text_delta carries streaming text; toolcall_start
				// announces a new tool call generated by the model.
				if event.AssistantEvent != nil {
					switch event.AssistantEvent.Type {
					case "text_delta":
						// 累积流式片段，message_end 时随完整文本一并输出
						messageBuffer += event.AssistantEvent.Delta
					case "toolcall_start":
						toolName := event.AssistantEvent.ToolName
						if toolName == "" {
							toolName = "tool_call"
						}
						eventCh <- executor.ExecutorEvent{
							Type:      executor.EventToolCall,
							Content:   toolName,
							SessionID: sessionID,
							Timestamp: nowMillis(),
						}
					}
				}

			case "message_end":
				// Message complete — usage may be present
				if event.Usage.InputTokens > 0 {
					inputTokens = event.Usage.InputTokens
				}
				if event.Usage.OutputTokens > 0 {
					outputTokens = event.Usage.OutputTokens
				}
				content := extractMessageText(event.Message)
				if content == "" {
					content = strings.TrimSpace(messageBuffer)
				}
				messageBuffer = ""
				if content != "" {
					eventCh <- executor.ExecutorEvent{
						Type:      executor.EventMessage,
						Content:   content,
						SessionID: sessionID,
						Timestamp: nowMillis(),
					}
				}

			case "tool_execution_start":
				content := event.ToolName
				if argStr := renderToolArgs(event.Args); argStr != "" {
					content = strings.TrimSpace(content + " " + argStr)
				}
				eventCh <- executor.ExecutorEvent{
					Type:      executor.EventToolCall,
					Content:   content,
					SessionID: sessionID,
					Timestamp: nowMillis(),
				}

			case "tool_execution_update":
				// Tool execution progress — forward as tool result content
				if event.Content != "" {
					eventCh <- executor.ExecutorEvent{
						Type:      executor.EventToolResult,
						Content:   event.Content,
						SessionID: sessionID,
						Timestamp: nowMillis(),
					}
				}

			case "tool_execution_end":
				content := event.Result
				if content == "" {
					content = event.Content
				}
				if content != "" {
					eventCh <- executor.ExecutorEvent{
						Type:      executor.EventToolResult,
						Content:   content,
						SessionID: sessionID,
						Timestamp: nowMillis(),
					}
				}

			case "bash_execution_update":
				// Streaming terminal output during bash execution
				if event.Content != "" {
					eventCh <- executor.ExecutorEvent{
						Type:      executor.EventToolResult,
						Content:   event.Content,
						SessionID: sessionID,
						Timestamp: nowMillis(),
					}
				}

			case "queue_update":
				// Queue changes — no content to emit

			case "compaction_start", "compaction_end":
				// Compaction events — no content to emit

			case "auto_retry_start", "auto_retry_end":
				// Auto-retry lifecycle — no content to emit

			case "error":
				sawResult = true
				resultFailed = true
				resultError = event.Error
				if resultError == "" {
					resultError = event.Content
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
			failureDetails = append(failureDetails, displayName+" 未输出有效的 agent_end 或 error 结束事件")
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

// piEvent is a tolerant union of all Pi CLI --mode json event types.
// Fields that may carry arrays or objects use json.RawMessage so a single
// struct can decode every event without type-mismatch failures.
type piEvent struct {
	Type           string            `json:"type"`
	ID             string            `json:"id"`
	Version        json.RawMessage   `json:"version"` // number in JSON, skip direct mapping
	Name           string            `json:"name"`
	ToolName       string            `json:"toolName"`
	ToolCallID     string            `json:"toolCallId"`
	Args           json.RawMessage   `json:"args"`
	Content        string            `json:"content"`
	Result         string            `json:"result"`
	Error          string            `json:"error"`
	IsError        bool              `json:"isError"`
	Message        *piMessage        `json:"message"`
	AssistantEvent *piAssistantEvent `json:"assistantMessageEvent"`
	Usage          piUsage           `json:"usage"`
	CWD            string            `json:"cwd"`
}

type piMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // array of content blocks, not a plain string
}

type piAssistantEvent struct {
	Type         string `json:"type"`
	ContentIndex int    `json:"contentIndex"`
	Delta        string `json:"delta"`
	ID           string `json:"id"`       // present for toolcall_start
	ToolName     string `json:"toolName"` // present for toolcall_start
}

type piUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}

// extractMessageText extracts human-readable text from a message object.
// The content field is a JSON array of content blocks; we rebuild a simple
// concatenation of text-delta blocks for display.
func extractMessageText(msg *piMessage) string {
	if msg == nil || len(msg.Content) == 0 {
		return ""
	}
	// Try string (unlikely for --mode json, but tolerate)
	var text string
	if json.Unmarshal(msg.Content, &text) == nil && strings.TrimSpace(text) != "" {
		return strings.TrimSpace(text)
	}
	// Try array of content blocks: [{"type":"text","text":"..."}, ...]
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(msg.Content, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				parts = append(parts, strings.TrimSpace(b.Text))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return ""
}

// renderToolArgs produces a short human-readable representation of tool
// execution arguments for the tool-call event content.
func renderToolArgs(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return ""
	}
	// Unmarshal into a generic map and join key=value pairs.
	var args map[string]interface{}
	if json.Unmarshal(raw, &args) != nil {
		return string(raw)
	}
	var pairs []string
	for k, v := range args {
		vs := fmt.Sprintf("%v", v)
		if vs == "" || vs == "null" {
			continue
		}
		pairs = append(pairs, k+"="+vs)
	}
	if len(pairs) == 0 {
		return ""
	}
	return strings.Join(pairs, " ")
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
