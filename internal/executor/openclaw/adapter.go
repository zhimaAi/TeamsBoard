// Package openclaw provides OpenClaw CLI execution adaptation.
package openclaw

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
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

const displayName = "OpenClaw"

// runTimeoutSeconds extends openclaw's default 600-second agent-turn deadline so
// long unattended coding tasks (long builds or test runs) are not killed
// mid-run. The orchestrator still cancels the process tree on stop, so this is
// only the CLI-internal run deadline.
const runTimeoutSeconds = "7200"

// Adapter OpenClaw CLI adapter.
//
// OpenClaw is managed through a Gateway, but `openclaw agent` is the
// self-contained headless entry point recommended for CI and coding automation:
// it owns setup, cleanup, output projection and process status, runs under the
// full execution policy a headless turn needs, and with `--json` reserves
// stdout for one stable JSON envelope. New conversations pass a fresh
// `--session-key` (bare keys are scoped to the configured default agent by the
// CLI itself); resuming a native session uses the documented
// `openclaw agent --session-id <id>` selector, which routes the new turn into
// the existing session.
type Adapter struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
	pid    int
}

// NewAdapter creates the OpenClaw adapter
func NewAdapter() executor.Adapter {
	return &Adapter{}
}

// NewConversation New conversation:
// openclaw agent --session-key <fresh-key> [--model <ref>] --timeout <s> --message-file <tempfile> --json
// The current CLI (>=2026.7) replaced `agent exec` with a plain `agent` command
// that requires an explicit session selector and dropped the `--cwd` option.
// The prompt is passed via `--message-file` pointing at a temp file because the
// CLI no longer reads the message from stdin (`--message-file -` was removed).
func (a *Adapter) NewConversation(ctx context.Context, opts executor.RunOptions) (<-chan executor.ExecutorEvent, error) {
	args := buildNewConversationArgs(opts)
	return a.runCommand(ctx, opts, args)
}

// buildNewConversationArgs assembles the CLI arguments for a new conversation.
// The fresh random session key starts a brand-new session under the configured
// default agent (bare keys are scoped by the CLI itself).
func buildNewConversationArgs(opts executor.RunOptions) []string {
	args := []string{"agent", "--session-key", newSessionKey()}
	if opts.ModelProfile != "" {
		args = append(args, "--model", opts.ModelProfile)
	}
	args = append(args, "--timeout", runTimeoutSeconds, "--json")
	args = append(args, opts.ExtraArgs...)
	return args
}

// ResumeConversation Continue the conversation:
// openclaw agent --session-id <id> [--model <ref>] --timeout <s> --message-file <tempfile> --json
// This path is Gateway-backed when a Gateway is reachable: the session selected
// by the user still lives under the running OpenClaw Gateway, which owns its
// state and routing; without a Gateway the CLI falls back to the embedded
// agent against the same local session store.
func (a *Adapter) ResumeConversation(ctx context.Context, opts executor.ResumeOptions) (<-chan executor.ExecutorEvent, error) {
	args := buildResumeConversationArgs(opts)
	return a.runCommand(ctx, opts.RunOptions, args)
}

// buildResumeConversationArgs assembles the CLI arguments for resuming a
// native session.
func buildResumeConversationArgs(opts executor.ResumeOptions) []string {
	args := []string{"agent", "--session-id", opts.ExternalSessionID}
	if opts.ModelProfile != "" {
		args = append(args, "--model", opts.ModelProfile)
	}
	args = append(args, "--timeout", runTimeoutSeconds, "--json")
	args = append(args, opts.ExtraArgs...)
	return args
}

// openClawUsage token usage counters.
type openClawUsage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	Total  int `json:"total"`
}

// openClawAgentMeta agent-level metadata carried inside the result meta.
type openClawAgentMeta struct {
	SessionID string        `json:"sessionId"`
	Usage     openClawUsage `json:"usage"`
}

// openClawResultMeta metadata envelope of one agent run result.
type openClawResultMeta struct {
	AgentMeta openClawAgentMeta `json:"agentMeta"`
}

// openClawPayload one assistant reply payload.
type openClawPayload struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// openClawRunResult the nested run result of the gateway relay envelope.
type openClawRunResult struct {
	Payloads []openClawPayload  `json:"payloads"`
	Meta     openClawResultMeta `json:"meta"`
}

// openClawExecResult captures the fields of the `--json` envelope across the
// shapes the CLI emits today plus the legacy flat envelope of older releases:
//   - Gateway relay: {runId, status, summary, result: {payloads, meta}}
//   - Embedded/local: {payloads, meta}
//   - Legacy flat: {ok, status, final, sessionId, payloads, usage, error}
type openClawExecResult struct {
	Status    string             `json:"status"`
	Summary   string             `json:"summary"`
	Result    *openClawRunResult `json:"result"`
	Payloads  []openClawPayload  `json:"payloads"`
	Meta      openClawResultMeta `json:"meta"`
	OK        bool               `json:"ok"`
	Final     string             `json:"final"`
	SessionID string             `json:"sessionId"`
	Usage     openClawUsage      `json:"usage"`
	Error     struct {
		Message string `json:"message"`
		Kind    string `json:"kind"`
		Code    any    `json:"code"` // numeric error code in gateway envelopes
	} `json:"error"`
}

// parseOpenClawEnvelope decodes the stable --json envelope emitted by
// `openclaw agent --json`.
func parseOpenClawEnvelope(output string) (openClawExecResult, error) {
	var envelope openClawExecResult
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		return envelope, err
	}
	return envelope, nil
}

// succeeded reports whether the run reached a successful terminal state.
func (e *openClawExecResult) succeeded() bool {
	// The gateway relay envelope carries an explicit terminal status; the
	// embedded/local envelope has no status field and reaching a parseable
	// envelope there means the run finished (failures exit non-zero with
	// diagnostics on stderr and no JSON on stdout).
	if e.Status == "error" || e.Status == "timeout" || e.Status == "in_flight" {
		return false
	}
	return true
}

// payloads returns the assistant reply payloads from whichever envelope shape
// the CLI emitted (nested under `result`, or the flat shape).
func (e *openClawExecResult) payloads() []openClawPayload {
	if e.Result != nil && len(e.Result.Payloads) > 0 {
		return e.Result.Payloads
	}
	return e.Payloads
}

// sessionID extracts the native session id from the envelope.
func (e *openClawExecResult) sessionID() string {
	if e.Result != nil && e.Result.Meta.AgentMeta.SessionID != "" {
		return e.Result.Meta.AgentMeta.SessionID
	}
	if e.Meta.AgentMeta.SessionID != "" {
		return e.Meta.AgentMeta.SessionID
	}
	return e.SessionID
}

// usage extracts the token usage from the envelope.
func (e *openClawExecResult) usage() openClawUsage {
	if e.Result != nil {
		if u := e.Result.Meta.AgentMeta.Usage; u.Input > 0 || u.Output > 0 || u.Total > 0 {
			return u
		}
	}
	if u := e.Meta.AgentMeta.Usage; u.Input > 0 || u.Output > 0 || u.Total > 0 {
		return u
	}
	return e.Usage
}

// failureSummary renders a human-readable reason for a failed run envelope.
func (e *openClawExecResult) failureSummary() string {
	if msg := strings.TrimSpace(e.Error.Message); msg != "" {
		return msg
	}
	if msg := strings.TrimSpace(e.Summary); msg != "" && msg != "completed" && msg != "aborted" {
		return msg
	}
	switch e.Status {
	case "timeout":
		return displayName + " 任务执行超时（超过 --timeout 限制）"
	case "in_flight":
		return displayName + " 会话已有任务正在运行（in_flight），请稍后重试"
	case "error":
		return displayName + " 任务执行失败"
	default:
		return displayName + " 任务未正常完成"
	}
}

// newSessionKey generates a fresh session key for a new conversation. Bare
// keys (no agent prefix) are scoped to the configured default agent by the
// CLI, so no agent id needs to be hardcoded here.
func newSessionKey() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "goteams-" + hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("goteams-%d", time.Now().UnixNano())
}

// writePromptFile stages the prompt in a UTF-8 temp file for `--message-file`.
// The current CLI reads the file during command-line handling, so it must
// exist before the process starts. The caller removes the file after the run.
func writePromptFile(prompt string) (string, error) {
	f, err := os.CreateTemp("", "goteams-openclaw-*.txt")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := io.WriteString(f, prompt); err != nil {
		f.Close()
		os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

// runCommand executes the command and returns the event channel
func (a *Adapter) runCommand(ctx context.Context, opts executor.RunOptions, args []string) (<-chan executor.ExecutorEvent, error) {
	innerCtx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()

	// The prompt is passed via `--message-file <tempfile>`: the current CLI no
	// longer supports the stdin shorthand `-`. The temp file also avoids
	// cmd.exe reinterpreting multi-line prompts on Windows.
	messageFile, err := writePromptFile(opts.Prompt)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("创建 %s Prompt 临时文件失败: %w", displayName, err)
	}
	args = append(args, "--message-file", messageFile)

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
		os.Remove(messageFile)
		return nil, fmt.Errorf("创建 stdout 管道失败: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		os.Remove(messageFile)
		return nil, fmt.Errorf("创建 stderr 管道失败: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		os.Remove(messageFile)
		return nil, fmt.Errorf("启动 %s 失败: %w", displayName, err)
	}

	a.mu.Lock()
	a.cmd = cmd
	a.pid = cmd.Process.Pid
	a.mu.Unlock()

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

	//Read the --json envelope (single JSON document on stdout)
	go func() {
		defer close(eventCh)
		defer cancel()

		eventCh <- executor.NewEvent(executor.EventStart)

		var stdoutDiagnostic diagnosticTail
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
		for scanner.Scan() {
			stdoutDiagnostic.AppendLine(scanner.Text())
		}
		stdoutScanErr := scanner.Err()
		if stdoutScanErr != nil {
			cancel()
		}

		stderrWG.Wait()
		waitErr := cmd.Wait()
		os.Remove(messageFile)
		a.clearProcess(cmd)

		envelope, parseErr := parseOpenClawEnvelope(stdoutDiagnostic.String())

		if parseErr != nil || !envelope.succeeded() {
			failureDetails := make([]string, 0, 4)
			if parseErr != nil {
				failureDetails = append(failureDetails, displayName+" 未输出有效的 JSON 结果")
				if output := stdoutDiagnostic.String(); output != "" {
					failureDetails = append(failureDetails, "未解析的 "+displayName+" 输出:\n"+output)
				}
			} else {
				failureDetails = append(failureDetails, envelope.failureSummary())
			}
			eventCh <- executor.ExecutorEvent{
				Type:      executor.EventError,
				Error:     formatExecutionError(strings.Join(failureDetails, "\n"), stderrDiagnostic.String(), stdoutScanErr, waitErr),
				Timestamp: nowMillis(),
			}
			return
		}

		//Stream the assistant replies into messages, then finish with usage.
		payloads := envelope.payloads()
		for _, payload := range payloads {
			if content := strings.TrimSpace(payload.Text); content != "" {
				eventCh <- executor.ExecutorEvent{
					Type:      executor.EventMessage,
					Content:   content,
					Timestamp: nowMillis(),
				}
			}
		}
		if len(payloads) == 0 {
			if content := strings.TrimSpace(envelope.Final); content != "" {
				eventCh <- executor.ExecutorEvent{
					Type:      executor.EventMessage,
					Content:   content,
					Timestamp: nowMillis(),
				}
			}
		}

		usage := envelope.usage()
		eventCh <- executor.NewCompleteEvent(envelope.sessionID(), usage.Input, usage.Output)
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
		"%s 运行失败%s：CLI 未输出错误详情，请检查 OpenClaw Gateway 状态、登录状态、网络和 CLI 配置",
		displayName,
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
