package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"goteams-client/internal/executor"
)

const fullPermissionArg = "--dangerously-bypass-approvals-and-sandbox"

// Adapter Codex CLI adapter
type Adapter struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
	pid    int
}

// NewAdapter creates Codex adapter
func NewAdapter() executor.Adapter {
	return &Adapter{}
}

// NewConversation New conversation: codex exec - --json
func (a *Adapter) NewConversation(ctx context.Context, opts executor.RunOptions) (<-chan executor.ExecutorEvent, error) {
	args := []string{"exec", fullPermissionArg, "-", "--json"}
	if opts.ModelProfile != "" {
		args = append(args, "-m", opts.ModelProfile)
	}
	args = append(args, opts.ExtraArgs...)

	return a.runCommand(ctx, opts, args)
}

// ResumeConversation continues the conversation: codex exec resume {thread_id} --json -
func (a *Adapter) ResumeConversation(ctx context.Context, opts executor.ResumeOptions) (<-chan executor.ExecutorEvent, error) {
	args := []string{"exec", "resume", fullPermissionArg, opts.ExternalSessionID, "--json", "-"}
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
	createProcessGroupForCmd(cmd)

	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	//Set environment variables
	if len(opts.EnvVars) > 0 {
		env := append([]string{}, execEnv()...)
		for k, v := range opts.EnvVars {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}

	// Get stdin pipe
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("创建 stdin 管道失败: %w", err)
	}

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("创建 stdout 管道失败: %w", err)
	}

	// Get stderr pipe
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("创建 stderr 管道失败: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("启动 Codex 失败: %w", err)
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
		if scanErr := scanner.Err(); scanErr != nil {
			stderrDiagnostic.AppendLine("读取 Codex stderr 失败: " + scanErr.Error())
		}
	}()

	eventCh := make(chan executor.ExecutorEvent, 100)

	// Read JSONL output
	go func() {
		defer close(eventCh)
		defer cancel()

		eventCh <- executor.NewEvent(executor.EventStart)

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB buffer

		sessionID := ""
		// There are two sets of mutually incompatible token reporting standards in Codex, which must be divided into buckets and accumulated before merging:
		// 1) usage / token_usage / complete event: each item is an increment
		// 2) turn.completed event: each item is the usage of "this turn"
		// The old implementation mixed the two on the same variable (one +=, one =), and the latter one in multiple rounds of sessions
		// turn.completed will directly overwrite the previous accumulated value, causing the token statistics to be lost.
		// Take the larger one after bucketing: when there is a single source, it is equal to the total amount of that source; when two sets are reported at the same time, it will not double.
		deltaInput, deltaOutput := 0, 0
		turnInput, turnOutput := 0, 0
		executionError := ""
		sawTurnCompleted := false
		var stdoutDiagnostic diagnosticTail

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var event codexEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				stdoutDiagnostic.AppendLine(line)
				continue
			}

			//Extract thread_id
			if sessionID == "" && event.ThreadID != "" {
				sessionID = event.ThreadID
			}

			timestamp := event.Timestamp
			if timestamp == 0 {
				timestamp = nowMillis()
			}

			//Convert by event type
			switch event.Type {
			case "message", "assistant", "response":
				if content := rawJSONText(event.Content); content != "" {
					eventCh <- executor.ExecutorEvent{
						Type:      executor.EventMessage,
						Content:   content,
						SessionID: sessionID,
						Timestamp: timestamp,
					}
				}
			case "tool_call", "function_call":
				eventCh <- executor.ExecutorEvent{
					Type:      executor.EventToolCall,
					Content:   rawJSONText(event.Content),
					SessionID: sessionID,
					Timestamp: timestamp,
				}
			case "tool_result", "function_call_output":
				eventCh <- executor.ExecutorEvent{
					Type:      executor.EventToolResult,
					Content:   rawJSONText(event.Content),
					SessionID: sessionID,
					Timestamp: timestamp,
				}
			case "item.started":
				if content := event.Item.toolLabel(); content != "" {
					eventCh <- executor.ExecutorEvent{
						Type:      executor.EventToolCall,
						Content:   content,
						SessionID: sessionID,
						Timestamp: timestamp,
					}
				}
			case "item.completed":
				switch event.Item.Type {
				case "agent_message":
					if event.Item.Text != "" {
						eventCh <- executor.ExecutorEvent{
							Type:      executor.EventMessage,
							Content:   event.Item.Text,
							SessionID: sessionID,
							Timestamp: timestamp,
						}
					}
				default:
					if content := event.Item.resultText(); content != "" {
						eventCh <- executor.ExecutorEvent{
							Type:      executor.EventToolResult,
							Content:   content,
							SessionID: sessionID,
							Timestamp: timestamp,
						}
					}
				}
			case "usage", "token_usage":
				if event.InputTokens > 0 {
					deltaInput += event.InputTokens
				}
				if event.OutputTokens > 0 {
					deltaOutput += event.OutputTokens
				}
				eventCh <- executor.ExecutorEvent{
					Type:         executor.EventUsage,
					InputTokens:  event.InputTokens,
					OutputTokens: event.OutputTokens,
					SessionID:    sessionID,
					Timestamp:    timestamp,
				}
			case "turn.completed":
				sawTurnCompleted = true
				// The usage of each turn must be accumulated. Multiple rounds of dialogue cannot only retain the last round.
				turnInput += event.Usage.InputTokens
				turnOutput += event.Usage.OutputTokens
				eventCh <- executor.ExecutorEvent{
					Type:         executor.EventUsage,
					InputTokens:  event.Usage.InputTokens,
					OutputTokens: event.Usage.OutputTokens,
					SessionID:    sessionID,
					Timestamp:    timestamp,
				}
			case "turn.failed", "error":
				errText := event.Error.Message
				if errText == "" {
					errText = rawJSONText(event.Content)
				}
				if errText == "" {
					errText = "Codex 执行失败"
				}
				if executionError == "" {
					executionError = errText
				}
			case "complete", "done", "finished":
				if event.InputTokens > 0 {
					deltaInput += event.InputTokens
				}
				if event.OutputTokens > 0 {
					deltaOutput += event.OutputTokens
				}
			}
		}

		stdoutScanErr := scanner.Err()
		if stdoutScanErr != nil {
			cancel()
		}
		stderrWG.Wait()

		// Wait for the process to exit and combine stderr, unresolved stdout and exit code into a single final error.
		waitErr := cmd.Wait()
		stdinErr := <-stdinDone
		a.clearProcess(cmd)

		failureDetails := make([]string, 0, 6)
		if executionError != "" {
			failureDetails = append(failureDetails, executionError)
		}
		if stdinErr != nil {
			failureDetails = append(failureDetails, "写入 Codex Prompt 失败: "+stdinErr.Error())
		}
		if stdoutScanErr != nil {
			failureDetails = append(failureDetails, "读取 Codex stdout 失败: "+stdoutScanErr.Error())
		}
		if waitErr != nil {
			failureDetails = append(failureDetails, formatWaitError(waitErr))
		}
		if !sawTurnCompleted && executionError == "" {
			failureDetails = append(failureDetails, "Codex CLI 未输出有效的 turn.completed 结束事件")
		}
		if len(failureDetails) > 0 {
			if output := stdoutDiagnostic.String(); output != "" {
				failureDetails = append(failureDetails, "未解析的 Codex 输出:\n"+output)
			}
			if output := stderrDiagnostic.String(); output != "" {
				failureDetails = append(failureDetails, "Codex stderr:\n"+output)
			}
			eventCh <- executor.ExecutorEvent{
				Type:      executor.EventError,
				Error:     "Codex CLI 运行失败：" + strings.Join(failureDetails, "\n"),
				SessionID: sessionID,
				Timestamp: nowMillis(),
			}
			return
		}

		eventCh <- executor.NewCompleteEvent(sessionID, maxInt(deltaInput, turnInput), maxInt(deltaOutput, turnOutput))
	}()

	return eventCh, nil
}

// maxInt returns the larger value of the two, which is used to merge the two sets of token reporting standards of Codex.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// codexEvent Codex JSONL event
type codexEvent struct {
	Type         string          `json:"type"`
	Content      json.RawMessage `json:"content"`
	ThreadID     string          `json:"thread_id"`
	InputTokens  int             `json:"input_tokens"`
	OutputTokens int             `json:"output_tokens"`
	Timestamp    int64           `json:"timestamp"`
	Item         codexItem       `json:"item"`
	Usage        codexUsage      `json:"usage"`
	Error        struct {
		Message string `json:"message"`
	} `json:"error"`
}

type codexUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type codexItem struct {
	Type             string          `json:"type"`
	Text             string          `json:"text"`
	Command          string          `json:"command"`
	AggregatedOutput string          `json:"aggregated_output"`
	Name             string          `json:"name"`
	Server           string          `json:"server"`
	Arguments        json.RawMessage `json:"arguments"`
	Result           json.RawMessage `json:"result"`
	Error            json.RawMessage `json:"error"`
	Status           string          `json:"status"`
}

func (item codexItem) toolLabel() string {
	switch item.Type {
	case "command_execution":
		return item.Command
	case "mcp_tool_call":
		name := item.Name
		if item.Server != "" {
			name = item.Server + "." + name
		}
		if args := rawJSONText(item.Arguments); args != "" {
			return strings.TrimSpace(name + " " + args)
		}
		return name
	case "web_search":
		return item.Text
	}
	return ""
}

func (item codexItem) resultText() string {
	switch item.Type {
	case "command_execution":
		if item.AggregatedOutput != "" {
			return item.AggregatedOutput
		}
		return item.Status
	case "mcp_tool_call":
		if result := rawJSONText(item.Result); result != "" {
			return result
		}
		return rawJSONText(item.Error)
	case "file_change":
		if item.Text != "" {
			return item.Text
		}
		return item.Status
	}
	return ""
}

func rawJSONText(raw json.RawMessage) string {
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

// Stop stops execution
func (a *Adapter) Stop() error {
	a.mu.Lock()
	cmd := a.cmd
	pid := a.pid
	cancel := a.cancel
	a.mu.Unlock()

	var terminateErr error
	if cmd != nil && cmd.Process != nil && pid > 0 {
		terminateErr = terminateProcessTreeForPID(pid)
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

// nowMillis returns the current Unix millisecond timestamp
func nowMillis() int64 {
	return timeNowMillis()
}

// execEnv returns the current environment variables
func execEnv() []string {
	return environ()
}
