// Package hermes provides Hermes Agent CLI execution adaptation.
//
// Hermes is a Python-based agent CLI. Its `hermes model` entry point is an
// interactive TTY-gated wizard, so this adapter drives the non-interactive
// chat surface instead:
//
//	hermes chat -q <prompt> -Q [--resume <session-id>] [-m <model>]
//
// `-q/--query` runs a one-shot (non-interactive) chat and `-Q/--quiet` enables
// programmatic mode (no banner/spinner/tool previews), leaving the assistant's
// replies as the primary stdout content. `--resume <session-id>` continues a
// saved conversation. New turns use a fresh process; later turns resume the
// saved session id parsed from the exit banner.
package hermes

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"goteams-client/internal/executor"
)

const defaultDisplayName = "Hermes"

// Adapter Hermes CLI adapter.
type Adapter struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	pid         int
	displayName string
}

// NewAdapter creates the Hermes adapter.
func NewAdapter() executor.Adapter {
	return &Adapter{displayName: defaultDisplayName}
}

// NewConversation starts a new conversation:
//
//	hermes chat -q <prompt> -Q [-m <model>] [extraArgs...]
func (a *Adapter) NewConversation(ctx context.Context, opts executor.RunOptions) (<-chan executor.ExecutorEvent, error) {
	args := hermesChatArgs(opts, nil)
	args = append(args, opts.ExtraArgs...)
	args = append(args, "-q", opts.Prompt)

	return a.runCommand(ctx, opts, args)
}

// ResumeConversation continues a saved conversation:
//
//	hermes chat -q <prompt> -Q --resume <session-id> [-m <model>] [extraArgs...]
func (a *Adapter) ResumeConversation(ctx context.Context, opts executor.ResumeOptions) (<-chan executor.ExecutorEvent, error) {
	args := hermesChatArgs(opts.RunOptions, []string{"--resume", opts.ExternalSessionID})
	args = append(args, opts.ExtraArgs...)
	args = append(args, "-q", opts.Prompt)

	return a.runCommand(ctx, opts.RunOptions, args)
}

// hermesChatArgs builds the common argument prefix for every invocation:
// non-interactive chat, programmatic (quiet) mode and the chosen model.
// resumeArgs (if any) are injected before the prompt.
func hermesChatArgs(opts executor.RunOptions, resumeArgs []string) []string {
	args := []string{"chat", "-Q"}
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

	// Continue reading stderr for diagnostics; when exiting with non-zero
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

	go func() {
		defer close(eventCh)
		defer cancel()

		eventCh <- executor.NewEvent(executor.EventStart)

		stdoutData, _ := io.ReadAll(stdout)
		waitErr := cmd.Wait()
		stderrWG.Wait()
		a.clearProcess(cmd)

		text := strings.TrimSpace(stripANSI(string(stdoutData)))
		sessionID := ""
		sawOutput := false

		// Extract the resumable session id from the exit banner
		// ("Resume this session with: hermes --resume <id>").
		sessionID = extractHermesSessionID(text)

		// Drop the exit banner lines from the message content.
		body := trimHermesExitBanner(text)

		if body != "" {
			sawOutput = true
			eventCh <- executor.ExecutorEvent{
				Type:      executor.EventMessage,
				Content:   body,
				SessionID: sessionID,
				Timestamp: nowMillis(),
			}
		}

		// Hermes 正常完成时会将会话摘要（Session: / --resume <id> 等）打到
		// stderr，因此 stderr 非空不代表失败。只有当没有 stdout 输出时，
		// stderr 才作为失败原因说明；进程异常退出（waitErr != nil）仍判失败。
		failureDetails := make([]string, 0, 4)
		if !sawOutput {
			if stderrText := stderrDiagnostic.String(); stderrText != "" {
				failureDetails = append(failureDetails, stderrText)
			}
			failureDetails = append(failureDetails, a.displayLabel()+" 未输出任何内容")
		}

		if len(failureDetails) > 0 || waitErr != nil {
			eventCh <- executor.ExecutorEvent{
				Type:      executor.EventError,
				Error:     formatHermesError(a.displayLabel(), strings.Join(failureDetails, "\n"), waitErr),
				SessionID: sessionID,
				Timestamp: nowMillis(),
			}
			return
		}

		eventCh <- executor.NewCompleteEvent(sessionID, 0, 0)
	}()

	return eventCh, nil
}

// hermesResumeBannerRe matches the session id printed in Hermes' exit banner:
// "Resume this session with:  hermes --resume <id>" or "Session:  <id>".
var hermesResumeBannerRe = regexp.MustCompile(`(?i)(?:--resume\s+([A-Za-z0-9_\-]+)|Session:\s*([A-Za-z0-9_\-]+))`)

// extractHermesSessionID pulls the resumable session id out of the CLI output.
func extractHermesSessionID(output string) string {
	for _, m := range hermesResumeBannerRe.FindAllStringSubmatch(output, -1) {
		for _, g := range m[1:] {
			if strings.TrimSpace(g) != "" {
				return strings.TrimSpace(g)
			}
		}
	}
	return ""
}

// hermesExitBannerLines are the summary lines Hermes prints when a chat
// session ends; they are stripped from the message content.
var hermesExitBannerLines = []string{
	"Resume this session with:",
	"Session:",
	"Duration:",
	"Messages:",
}

// trimHermesExitBanner removes the session summary block that Hermes prints at
// the end of a chat run.
func trimHermesExitBanner(output string) string {
	lines := strings.Split(output, "\n")
	end := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		matched := false
		for _, prefix := range hermesExitBannerLines {
			if strings.HasPrefix(line, prefix) {
				matched = true
				break
			}
		}
		if matched {
			end = i
			continue
		}
		break
	}
	return strings.TrimSpace(strings.Join(lines[:end], "\n"))
}

// ansiEscapePattern strips ANSI escape sequences (color codes etc.).
var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func stripANSI(s string) string {
	return ansiEscapePattern.ReplaceAllString(s, "")
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

func formatHermesError(displayName, resultError string, waitErr error) string {
	details := make([]string, 0, 2)
	resultError = strings.TrimSpace(resultError)
	if resultError != "" {
		details = append(details, resultError)
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
