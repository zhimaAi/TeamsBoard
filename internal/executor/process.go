package executor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// PromptFilePlaceholder 是启动参数中代表临时 prompt 文件路径的占位符。
// 用于只能通过文件接收长 prompt 的 CLI（如 openclaw 的 --message-file）。
const PromptFilePlaceholder = "{prompt_file}"

// promptFilePermission 是临时 prompt 文件的权限（仅当前用户可读写）。
const promptFilePermission os.FileMode = 0o600

// PromptFileSpec 描述通过临时文件传递的 prompt。
type PromptFileSpec struct {
	Prefix  string // 临时文件名前缀
	Content string // 文件内容
}

// RunSpec 描述一次 CLI 进程执行。
// prompt 的投递方式由调用方决定：StdinText 走标准输入，PromptFile 走临时文件，
// 也可以直接拼进 Args，运行器不预设策略。
type RunSpec struct {
	ExecPath   string
	Args       []string
	WorkDir    string
	EnvVars    map[string]string
	StdinText  string // 非空时写入子进程 stdin；为空则不占用 stdin
	PromptFile *PromptFileSpec
}

// RunResult 汇总一次进程执行的收尾信息。
type RunResult struct {
	ExitCode int
	WaitErr  error
	ScanErr  error
	StdinErr error
	Stderr   string
}

// Process 表示一个正在运行的 CLI 进程，可被停止并等待收尾。
type Process struct {
	mu         sync.Mutex
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	pid        int
	stopped    bool
	stopErr    error
	scanErr    error
	lines      chan string
	stderrTail *DiagnosticTail
	stderrDone chan struct{}
	stdinErr   chan error
	promptPath string
	waitOnce   sync.Once
	result     *RunResult
}

// StartProcess 启动 CLI 进程并开始按行读取 stdout。
// Lines 通道由调用方消费；通道关闭后再调用 WaitFor 获取收尾结果。
func StartProcess(parent context.Context, spec RunSpec) (*Process, error) {
	innerCtx, cancel := context.WithCancel(parent)

	args := append([]string{}, spec.Args...)
	promptPath := ""
	if spec.PromptFile != nil {
		path, err := writePromptFile(spec.PromptFile)
		if err != nil {
			cancel()
			return nil, err
		}
		promptPath = path
		for index, arg := range args {
			args[index] = strings.ReplaceAll(arg, PromptFilePlaceholder, path)
		}
	}

	cmd := exec.CommandContext(innerCtx, spec.ExecPath, args...)
	CreateProcessGroup(cmd)
	if spec.WorkDir != "" {
		cmd.Dir = spec.WorkDir
	}
	if len(spec.EnvVars) > 0 {
		env := append([]string{}, os.Environ()...)
		for key, value := range spec.EnvVars {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
		cmd.Env = env
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		removePromptFile(promptPath)
		return nil, fmt.Errorf("创建 stdout 管道失败: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		removePromptFile(promptPath)
		return nil, fmt.Errorf("创建 stderr 管道失败: %w", err)
	}

	stdinErrCh := make(chan error, 1)
	var stdin io.WriteCloser
	if spec.StdinText != "" {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			cancel()
			removePromptFile(promptPath)
			return nil, fmt.Errorf("创建 stdin 管道失败: %w", err)
		}
	}

	if err := cmd.Start(); err != nil {
		cancel()
		removePromptFile(promptPath)
		return nil, err
	}

	proc := &Process{
		cmd:        cmd,
		cancel:     cancel,
		pid:        cmd.Process.Pid,
		lines:      make(chan string, 1024),
		stderrTail: &DiagnosticTail{},
		stderrDone: make(chan struct{}),
		stdinErr:   stdinErrCh,
		promptPath: promptPath,
	}

	if stdin != nil {
		prompt := spec.StdinText
		go func() {
			_, writeErr := io.WriteString(stdin, prompt)
			closeErr := stdin.Close()
			if writeErr == nil {
				writeErr = closeErr
			}
			stdinErrCh <- writeErr
		}()
	} else {
		stdinErrCh <- nil
	}

	// stderr 只保留尾部，进程非零退出时作为错误详情回传
	go func() {
		defer close(proc.stderrDone)
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			proc.stderrTail.AppendLine(scanner.Text())
		}
		if scanErr := scanner.Err(); scanErr != nil {
			proc.stderrTail.AppendLine("读取 stderr 失败: " + scanErr.Error())
		}
	}()

	go func() {
		defer close(proc.lines)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
		for scanner.Scan() {
			proc.lines <- scanner.Text()
		}
		proc.mu.Lock()
		proc.scanErr = scanner.Err()
		proc.mu.Unlock()
	}()

	return proc, nil
}

// Lines 返回 stdout 的行通道，通道关闭表示输出读取结束。
func (p *Process) Lines() <-chan string {
	return p.lines
}

// Pid 返回进程组根进程 ID。
func (p *Process) Pid() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pid
}

// Stopped 报告进程是否收到过停止请求。
func (p *Process) Stopped() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopped
}

// Stop 终止进程树并取消上下文。
// 必须先按存活 PID 终止整棵进程树再 cancel：先 cancel 会让根进程先消失，
// 子进程会变成孤儿进程继续运行。
func (p *Process) Stop() error {
	p.mu.Lock()
	cmd := p.cmd
	pid := p.pid
	cancel := p.cancel
	p.stopped = true
	p.mu.Unlock()

	var terminateErr error
	if cmd != nil && cmd.Process != nil && pid > 0 {
		terminateErr = TerminateProcessTree(pid)
	}
	if terminateErr == nil && cancel != nil {
		cancel()
	}
	p.mu.Lock()
	p.stopErr = terminateErr
	p.mu.Unlock()
	return terminateErr
}

// WaitFor 等待进程退出并汇总 stdout/stderr/stdin 与退出码信息。
// 必须在 Lines 通道关闭（编码器已消费完输出）之后调用；可重复调用，只有第一次真正等待。
func (p *Process) WaitFor() *RunResult {
	p.waitOnce.Do(func() {
		<-p.stderrDone

		waitErr := p.cmd.Wait()
		stdinErr := <-p.stdinErr

		p.mu.Lock()
		scanErr := p.scanErr
		p.mu.Unlock()

		removePromptFile(p.promptPath)

		p.result = &RunResult{
			ExitCode: exitCodeOf(waitErr),
			WaitErr:  waitErr,
			ScanErr:  scanErr,
			StdinErr: stdinErr,
			Stderr:   p.stderrTail.String(),
		}
	})
	return p.result
}

func writePromptFile(spec *PromptFileSpec) (string, error) {
	prefix := strings.TrimSpace(spec.Prefix)
	if prefix == "" {
		prefix = "goteams-prompt-"
	}
	file, err := os.CreateTemp("", prefix+"*.md")
	if err != nil {
		return "", fmt.Errorf("创建 prompt 临时文件失败: %w", err)
	}
	path := file.Name()
	if _, err := file.WriteString(spec.Content); err != nil {
		file.Close()
		removePromptFile(path)
		return "", fmt.Errorf("写入 prompt 临时文件失败: %w", err)
	}
	if err := file.Close(); err != nil {
		removePromptFile(path)
		return "", fmt.Errorf("关闭 prompt 临时文件失败: %w", err)
	}
	_ = os.Chmod(path, promptFilePermission)
	return path, nil
}

func removePromptFile(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
}

// exitCodeOf 从等待错误中解析退出码，无法解析时返回 -1。
func exitCodeOf(waitErr error) int {
	if waitErr == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// FormatExitCode 返回退出码的可读文本，用于错误详情与提示。
func FormatExitCode(code int) string {
	return strconv.Itoa(code)
}

// FormatExecutionError 统一拼装 CLI 执行失败原因：
// 依次合并协议层错误、协议特有细节、stderr 尾部、stdout 读取错误与退出码。
func FormatExecutionError(displayName, failureReason, stderrText string, extraDetails []string, scanErr, waitErr error) string {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = "CLI"
	}
	details := make([]string, 0, 4+len(extraDetails))
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

	appendDetail(failureReason)
	for _, extra := range extraDetails {
		appendDetail(extra)
	}
	appendDetail(stderrText)
	if scanErr != nil {
		appendDetail("读取 " + displayName + " 输出失败: " + scanErr.Error())
	}

	exitCode := exitCodeOf(waitErr)
	exitLabel := ""
	if exitCode >= 0 {
		exitLabel = fmt.Sprintf("（退出码 %d）", exitCode)
	}

	if len(details) > 0 {
		return fmt.Sprintf("%s 运行失败%s：%s", displayName, exitLabel, strings.Join(details, "\n"))
	}
	if waitErr != nil && exitCode < 0 {
		reason := strings.TrimSpace(waitErr.Error())
		if reason != "" && !strings.HasPrefix(reason, "exit status") {
			return fmt.Sprintf("%s 运行失败：%s", displayName, reason)
		}
	}
	return fmt.Sprintf(
		"%s 运行失败%s：CLI 未输出错误详情，请检查 %s 登录状态、网络、模型和 CLI 配置",
		displayName,
		exitLabel,
		displayName,
	)
}
