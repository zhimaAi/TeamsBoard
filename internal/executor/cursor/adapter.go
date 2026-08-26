package cursor

import (
	"bufio"
	"context"
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

const defaultDisplayName = "Cursor Agent"

// Adapter Cursor `agent` CLI adapter.
//
// The Cursor `agent` CLI is invoked as `agent`; the model list is obtained via
// `agent models` (see internal/executor/discovery.go -> resolveCursorModels).
//
// The `agent` CLI refuses to run headless until the workspace is trusted
// ("⚠ Workspace Trust Required"), so every invocation passes --trust to bypass
// the interactive prompt. The exact run-command flags / output schema of `agent`
// are not pinned down here; this adapter streams stdout as plain text (each
// non-empty line becomes a message event) and terminates the whole process tree
// on Stop. The prompt is written to stdin (Windows-safe, matching the other
// agent adapters); pass extra flags via RunOptions.ExtraArgs if a different
// invocation is needed.
type Adapter struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	pid         int
	displayName string
}

// NewAdapter creates a Cursor agent adapter.
func NewAdapter() executor.Adapter {
	return &Adapter{displayName: defaultDisplayName}
}

// NewConversation new conversation: agent --trust [--model <id>] [extraArgs]
func (a *Adapter) NewConversation(ctx context.Context, opts executor.RunOptions) (<-chan executor.ExecutorEvent, error) {
	args := []string{"--trust"}
	if opts.ModelProfile != "" {
		args = append(args, "--model", opts.ModelProfile)
	}
	args = append(args, opts.ExtraArgs...)

	return a.runCommand(ctx, opts, args)
}

// ResumeConversation continues a conversation: agent --trust --resume <session_id> [--model <id>] [extraArgs]
func (a *Adapter) ResumeConversation(ctx context.Context, opts executor.ResumeOptions) (<-chan executor.ExecutorEvent, error) {
	args := []string{"--trust"}
	if opts.ExternalSessionID != "" {
		args = append(args, "--resume", opts.ExternalSessionID)
	}
	if opts.ModelProfile != "" {
		args = append(args, "--model", opts.ModelProfile)
	}
	args = append(args, opts.ExtraArgs...)

	return a.runCommand(ctx, opts.RunOptions, args)
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

	// Retain stderr so that real failure reasons can be shown on non-zero exit.
	var stderrDiagnostic strings.Builder
	var stderrWG sync.WaitGroup
	stderrWG.Add(1)
	go func() {
		defer stderrWG.Done()
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			stderrDiagnostic.WriteString(scanner.Text())
			stderrDiagnostic.WriteString("\n")
		}
	}()

	eventCh := make(chan executor.ExecutorEvent, 100)

	go func() {
		defer close(eventCh)
		defer cancel()

		eventCh <- executor.NewEvent(executor.EventStart)

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			eventCh <- executor.ExecutorEvent{
				Type:      executor.EventMessage,
				Content:   line,
				Timestamp: nowMillis(),
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

		if stdoutScanErr != nil || waitErr != nil || stdinErr != nil {
			details := make([]string, 0, 3)
			if stdinErr != nil {
				details = append(details, "写入 "+a.displayLabel()+" Prompt 失败: "+stdinErr.Error())
			}
			if stderrDiagnostic.Len() > 0 {
				details = append(details, strings.TrimSpace(stderrDiagnostic.String()))
			}
			if stdoutScanErr != nil {
				details = append(details, "读取 "+a.displayLabel()+" 输出失败: "+stdoutScanErr.Error())
			}
			eventCh <- executor.ExecutorEvent{
				Type:      executor.EventError,
				Error:     formatCursorError(a.displayLabel(), strings.Join(details, "\n"), waitErr),
				Timestamp: nowMillis(),
			}
			return
		}

		eventCh <- executor.NewCompleteEvent("", 0, 0)
	}()

	return eventCh, nil
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
		a.pid = 0
	}
	a.mu.Unlock()
}

func (a *Adapter) displayLabel() string {
	if a.displayName != "" {
		return a.displayName
	}
	return defaultDisplayName
}

func formatCursorError(displayName, detail string, waitErr error) string {
	detail = strings.TrimSpace(detail)
	exitCode := -1
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		exitCode = exitErr.ExitCode()
	}
	exitLabel := ""
	if exitCode >= 0 {
		exitLabel = fmt.Sprintf("（退出码 %d）", exitCode)
	}
	if detail != "" {
		return fmt.Sprintf("%s 运行失败%s：%s", displayName, exitLabel, detail)
	}
	if waitErr != nil && exitCode < 0 {
		reason := strings.TrimSpace(waitErr.Error())
		if reason != "" && !strings.HasPrefix(reason, "exit status") {
			return fmt.Sprintf("%s 运行失败：%s", displayName, reason)
		}
	}
	return fmt.Sprintf(
		"%s 运行失败%s：CLI 未输出错误详情，请检查 %s 登录状态、网络、模型和 CLI 配置",
		displayName, exitLabel, displayName,
	)
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}
