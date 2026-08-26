// Package grok provides Grok CLI (Grok Build) execution adaptation.
package grok

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

const defaultDisplayName = "Grok CLI"

// Adapter Grok CLI adapter.
//
// The Grok CLI is run headless with `-p <prompt>` and `--format json`: it emits a
// single JSON object at the end of the run carrying the assistant text, the
// session id, the stop reason and (when present) usage info. New turns use a
// fresh process; later turns resume the saved session with `-s <session-id>`.
// The prompt is passed as the `-p` argument (Grok is a native binary, so
// multi-line prompts survive), and the working directory is set explicitly so the
// agent operates on the right repo regardless of where the backend was started.
type Adapter struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	pid         int
	displayName string
}

// NewAdapter creates the Grok adapter.
func NewAdapter() executor.Adapter {
	return &Adapter{displayName: defaultDisplayName}
}

// NewConversation starts a new conversation:
//
//	grok -p <prompt> --format json [--no-sandbox] [-m <model>] [extraArgs...]
func (a *Adapter) NewConversation(ctx context.Context, opts executor.RunOptions) (<-chan executor.ExecutorEvent, error) {
	args := grokBaseArgs(opts, nil)
	args = append(args, opts.ExtraArgs...)
	args = append(args, "-p", opts.Prompt)

	return a.runCommand(ctx, opts, args)
}

// ResumeConversation continues a saved conversation:
//
//	grok -p <prompt> --format json [-s <session-id>] [--no-sandbox] [-m <model>] [extraArgs...]
func (a *Adapter) ResumeConversation(ctx context.Context, opts executor.ResumeOptions) (<-chan executor.ExecutorEvent, error) {
	args := grokBaseArgs(opts.RunOptions, []string{"-s", opts.ExternalSessionID})
	args = append(args, opts.ExtraArgs...)
	args = append(args, "-p", opts.Prompt)

	return a.runCommand(ctx, opts.RunOptions, args)
}

// grokBaseArgs builds the common argument prefix for every invocation: JSON
// headless output, no sandbox (so the agent can edit files on the host, like the
// other adapters) and the chosen model. resumeArgs (if any) are injected before
// the prompt.
func grokBaseArgs(opts executor.RunOptions, resumeArgs []string) []string {
	args := []string{"--format", "json", "--no-sandbox"}
	if len(resumeArgs) > 0 {
		args = append(args, resumeArgs...)
	}
	if opts.ModelProfile != "" {
		args = append(args, "-m", opts.ModelProfile)
	}
	return args
}

// runCommand executes the command and returns the event channel.
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

	// The prompt is delivered via the `-p` argument; stdin is not consumed so the
	// process never blocks waiting for input.
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
		return nil, fmt.Errorf("启动 %s 失败: %w", a.displayLabel(), err)
	}

	a.mu.Lock()
	a.cmd = cmd
	a.pid = cmd.Process.Pid
	a.mu.Unlock()

	// Close stdin immediately (the prompt was passed as an argument).
	stdinDone := make(chan error, 1)
	go func() {
		stdinDone <- stdin.Close()
	}()

	// Continue retaining stderr for diagnostics; when exiting with non-zero
	// value, the real reason is passed to the task execution window.
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
			stderrDiagnostic.AppendLine("读取 " + a.displayLabel() + " stderr 失败: " + scanErr.Error())
		}
	}()

	eventCh := make(chan executor.ExecutorEvent, 100)

	// Read the JSON output
	go func() {
		defer close(eventCh)
		defer cancel()

		eventCh <- executor.NewEvent(executor.EventStart)

		stdoutData, _ := io.ReadAll(stdout)
		<-stdinDone
		waitErr := cmd.Wait()
		stderrWG.Wait()
		a.clearProcess(cmd)

		sessionID := ""
		inputTokens := 0
		outputTokens := 0
		resultError := ""
		resultFailed := false
		sawOutput := false

		if text := strings.TrimSpace(string(stdoutData)); text != "" {
			sawOutput = true
			// Handle both a single JSON object and JSONL (newer Grok builds)
			// that also emit text lines before the terminal object.
			parsed := parseGrokOutput(text)
			if parsed != nil {
				eventCh <- executor.ExecutorEvent{
					Type:      executor.EventMessage,
					Content:   parsed.Text,
					SessionID: parsed.SessionID,
					Timestamp: nowMillis(),
				}
				sessionID = parsed.SessionID
				inputTokens = parsed.InputTokens
				outputTokens = parsed.OutputTokens
				if parsed.Error != "" {
					resultFailed = true
					resultError = parsed.Error
				}
			} else {
				// Fallback: raw text output becomes a single message.
				eventCh <- executor.ExecutorEvent{
					Type:      executor.EventMessage,
					Content:   text,
					SessionID: sessionID,
					Timestamp: nowMillis(),
				}
			}
		}

		failureDetails := make([]string, 0, 4)
		if resultError != "" {
			failureDetails = append(failureDetails, resultError)
		} else if resultFailed {
			failureDetails = append(failureDetails, a.displayLabel()+" 返回错误结果，但未提供错误详情")
		}
		if !sawOutput {
			failureDetails = append(failureDetails, a.displayLabel()+" 未输出任何内容")
			if output := stderrDiagnostic.String(); output != "" {
				failureDetails = append(failureDetails, "stderr:\n"+output)
			}
		}

		if len(failureDetails) > 0 || waitErr != nil {
			eventCh <- executor.ExecutorEvent{
				Type:      executor.EventError,
				Error:     formatGrokError(a.displayLabel(), strings.Join(failureDetails, "\n"), stderrDiagnostic.String(), waitErr),
				SessionID: sessionID,
				Timestamp: nowMillis(),
			}
			return
		}

		eventCh <- executor.NewCompleteEvent(sessionID, inputTokens, outputTokens)
	}()

	return eventCh, nil
}

// grokResult is the parsed terminal JSON object produced by
// `grok -p ... --format json`.
type grokResult struct {
	Text         string
	SessionID    string
	InputTokens  int
	OutputTokens int
	Error        string
}

// parseGrokOutput extracts the assistant reply and session id from Grok headless
// output. It accepts a single JSON object ({"text":..., "sessionId":...,
// "stopReason":...}) as well as a JSONL stream where the last terminal line is
// the object. When nothing JSON-shaped is found it returns nil.
func parseGrokOutput(output string) *grokResult {
	// single object form
	if obj := parseGrokJSONObject(output); obj != nil {
		return obj
	}
	// JSONL form: look for the last line that parses as a JSON object
	var last *grokResult
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if obj := parseGrokJSONObject(line); obj != nil {
			last = obj
		}
	}
	return last
}

// parseGrokJSONObject unmarshals one line/object into a grokResult. Unknown
// field names are tolerated so newer CLI output shapes keep working.
func parseGrokJSONObject(raw string) *grokResult {
	var v struct {
		Text        string `json:"text"`
		SessionID   string `json:"sessionId"`
		SessionID2  string `json:"session_id"`
		StopReason  string `json:"stopReason"`
		Error       string `json:"error"`
		Message     string `json:"message"`
		Result      string `json:"result"`
		Type        string `json:"type"`
		Data        string `json:"data"`
		InputUsage  int    `json:"inputTokens"`
		OutputUsage int    `json:"outputTokens"`
		Usage       struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil
	}
	if v.Type != "" && v.Type != "result" && v.Type != "end" && v.Type != "message" {
		// streaming event lines (text/thought/tool_call/...) are not a terminal result
		if v.Text == "" && v.Message == "" && v.Result == "" {
			return nil
		}
	}
	res := &grokResult{}
	res.Text = strings.TrimSpace(firstNonEmpty(v.Text, v.Message, v.Result, v.Data))
	res.SessionID = firstNonEmpty(v.SessionID, v.SessionID2)
	res.InputTokens = v.InputUsage
	res.OutputTokens = v.OutputUsage
	if v.Usage.InputTokens > 0 {
		res.InputTokens = v.Usage.InputTokens
	}
	if v.Usage.OutputTokens > 0 {
		res.OutputTokens = v.Usage.OutputTokens
	}
	if msg := strings.TrimSpace(firstNonEmpty(v.Error, v.Message)); msg != "" && res.Text == "" {
		res.Error = msg
	}
	// Errors reported through a streaming "error" event.
	if v.Type == "error" {
		msg := strings.TrimSpace(firstNonEmpty(v.Message, v.Data, v.Error))
		if msg == "" {
			msg = "Grok 运行失败"
		}
		res.Error = msg
	}
	if res.Text == "" && res.SessionID == "" && res.Error == "" && res.InputTokens == 0 && res.OutputTokens == 0 {
		return nil
	}
	return res
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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

func (a *Adapter) displayLabel() string {
	if strings.TrimSpace(a.displayName) != "" {
		return a.displayName
	}
	return defaultDisplayName
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

func formatGrokError(displayName, resultError, stderrText string, waitErr error) string {
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
