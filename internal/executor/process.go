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
	"sync/atomic"
	"time"

	"goteams-client/internal/applog"
)

// PromptFilePlaceholder 是启动参数中代表临时 prompt 文件路径的占位符。
// 用于只能通过文件接收长 prompt 的 CLI（如 openclaw 的 --message-file）。
const PromptFilePlaceholder = "{prompt_file}"

// promptFilePermission 是临时 prompt 文件的权限（仅当前用户可读写）。
const promptFilePermission os.FileMode = 0o600

// maxStdoutLineBytes 是单行 stdout 允许进入解码器的最大字节数。
// 超过后丢弃这一行并继续读，避免 Scanner 的长度上限让读取提前结束：
// 读取一端停止后，CLI 会堵在写满的管道上，事件通道永不关闭，会话就一直停在执行中。
const maxStdoutLineBytes = 16 << 20

// OutputIdleTimeout 是没有未完成工具调用时，stdout 允许静默的时间。
// 流式增量会持续写 stdout，所以这 90 秒不是「界面没刷出新消息」，而是「一个字节都没有」。
// 一轮正常结束时终态事件应在数秒内出现；90 秒只覆盖首 token 和工具返回后的空档。
const OutputIdleTimeout = 90 * time.Second

// OutputToolHardTimeout 是单次工具执行的总时限，从工具调用开始计算。
// 编译、安装可以长时间不打印；只要子进程还在就继续等，到点才终止。
const OutputToolHardTimeout = 30 * time.Minute

const outputIdleCheckInterval = 5 * time.Second

// OutputIdleStopMessage 是 stdout 静默超时后终止进程的原因。
const OutputIdleStopMessage = "CLI 长时间没有新输出，已终止挂起进程"

// OutputToolHardStopMessage 是工具执行达到总时限后终止进程的原因。
const OutputToolHardStopMessage = "工具执行超过 30 分钟，已终止"

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
	mu              sync.Mutex
	cmd             *exec.Cmd
	cancel          context.CancelFunc
	pid             int
	stopped         bool
	stopErr         error
	scanErr         error
	lines           chan string
	stderrTail      *DiagnosticTail
	stderrDone      chan struct{}
	stdinErr        chan error
	promptPath      string
	waitOnce        sync.Once
	result          *RunResult
	stdoutDone      chan struct{}
	waitDone        chan struct{}
	lastStdout      atomic.Int64
	watch           idleWatchState
	stopMu          sync.Mutex
	idleStopMessage string
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
	// 上下文取消也走进程树终止，不能只杀根进程留下编译器或包管理器。
	cmd.Cancel = func() error { return TerminateProcessTree(cmd.Process.Pid) }
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
		stdoutDone: make(chan struct{}),
		waitDone:   make(chan struct{}),
		stdinErr:   stdinErrCh,
		promptPath: promptPath,
	}
	proc.lastStdout.Store(time.Now().UnixMilli())

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

	// stderr 只保留尾部，进程非零退出时作为错误详情回传。
	// 必须一直读到 EOF：中途停下会让 CLI 堵在 stderr 管道上，进程无法退出。
	go func() {
		defer close(proc.stderrDone)
		buf := make([]byte, 32*1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				proc.stderrTail.Append(buf[:n])
			}
			if err != nil {
				if err != io.EOF {
					proc.stderrTail.AppendLine("读取 stderr 失败: " + err.Error())
				}
				return
			}
		}
	}()

	go func() {
		defer close(proc.stdoutDone)
		defer close(proc.lines)
		reader := &stdoutActivityReader{r: stdout, touch: func() {
			proc.lastStdout.Store(time.Now().UnixMilli())
		}}
		if err := readStdoutLines(reader, maxStdoutLineBytes, proc.lines); err != nil {
			proc.mu.Lock()
			proc.scanErr = err
			proc.mu.Unlock()
		}
	}()
	go proc.watchStdoutIdle()

	return proc, nil
}

// stdoutActivityReader 在每次真正读到字节时刷新静默计时。
type stdoutActivityReader struct {
	r     io.Reader
	touch func()
}

func (r *stdoutActivityReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 && r.touch != nil {
		r.touch()
	}
	return n, err
}

// ObserveEvent 在解码器出口更新工具状态，不依赖 WebSocket 或数据库消费速度。
func (p *Process) ObserveEvent(event ExecutorEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return
	}
	now := time.Now()
	if p.watch.observe(event, now) {
		p.lastStdout.Store(now.UnixMilli())
	}
}

func (p *Process) IdleStopped() bool { return p.IdleStopMessage() != "" }

// IdleStopMessage 只在成功终止进程树后提供失败原因，不把终止请求当作成功。
func (p *Process) IdleStopMessage() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.idleStopMessage
}

func (p *Process) watchStdoutIdle() {
	ticker := time.NewTicker(outputIdleCheckInterval)
	defer ticker.Stop()
	stopFailures := 0
	for {
		select {
		case <-p.waitDone:
			return
		case <-ticker.C:
			if p.Stopped() {
				return
			}
			if p.idleStopReason(time.Now()) == "" {
				continue
			}
			if err := p.stopProcess(true); err != nil {
				stopFailures++
				applog.Warn("[Executor] 挂起进程终止失败，未标记会话完成", "pid", p.Pid(), "attempt", stopFailures, "error", err)
				// 不取消根进程、不提前路由；最多重试三次，之后允许用户手动重试停止。
				if stopFailures >= 3 {
					return
				}
				continue
			}
			if p.Stopped() {
				return
			}
		}
	}
}

func (p *Process) idleStopReason(now time.Time) string {
	p.mu.Lock()
	if p.stopped || p.watch.paused() {
		p.mu.Unlock()
		return ""
	}
	version, pid := p.watch.version, p.pid
	probe := p.watch.hasSubprocessTool() && !p.watch.hardExpired(now)
	p.mu.Unlock()
	childAlive := probe && hasLiveDescendant(pid)
	p.mu.Lock()
	defer p.mu.Unlock()
	// 慢进程探测期间可能已经收到结果或授权请求，不能用旧快照终止。
	if p.stopped || p.watch.version != version {
		return ""
	}
	return p.watch.reason(now, time.UnixMilli(p.lastStdout.Load()), childAlive)
}

// hasLiveDescendant 报告 root 下面是否还有活着的子进程。
// 查不到进程表时按「还在」处理，避免把正在编译的命令误判成挂起。
func hasLiveDescendant(root int) bool {
	if root <= 0 {
		return false
	}
	parents, err := snapshotProcessParents()
	if err != nil {
		return true
	}
	return hasDescendant(parents, root)
}

func hasDescendant(parents map[int]int, root int) bool {
	if root <= 0 || len(parents) == 0 {
		return false
	}
	for pid, ppid := range parents {
		if pid != root && ppid == root {
			return true
		}
	}
	return false
}

// readStdoutLines 按 LF 读取 stdout，并一直读到 EOF。
// 超长行会被跳过而不是让整个读取失败退出，这样后续的终态事件和进程退出都还能被观察到。
func readStdoutLines(r io.Reader, maxLine int, lines chan<- string) error {
	reader := bufio.NewReaderSize(r, 64*1024)
	var acc []byte
	skipping := false
	var skipErr error
	for {
		part, err := reader.ReadSlice('\n')
		if len(part) > 0 {
			if skipping {
				// 继续丢掉当前超长行，直到遇到换行。
			} else if maxLine > 0 && len(acc)+len(part) > maxLine {
				skipping = true
				acc = nil
				if skipErr == nil {
					skipErr = fmt.Errorf("单行输出超过 %d 字节，已跳过并继续读取", maxLine)
				}
			} else {
				acc = append(acc, part...)
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if !skipping && len(acc) > 0 {
			lines <- strings.TrimRight(string(acc), "\r\n")
		}
		acc = acc[:0]
		skipping = false
		if err == io.EOF {
			return skipErr
		}
		if err != nil {
			if skipErr != nil {
				return skipErr
			}
			return err
		}
	}
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
func (p *Process) Stop() error { return p.stopProcess(false) }

func (p *Process) stopProcess(watchdog bool) error {
	return p.stopUsing(watchdog, TerminateProcessTree)
}

func (p *Process) stopUsing(watchdog bool, terminate func(int) error) error {
	p.stopMu.Lock()
	defer p.stopMu.Unlock()
	select {
	case <-p.waitDone:
		return nil
	default:
	}
	if p.Stopped() {
		return nil
	}
	reason := ""
	if watchdog {
		reason = p.idleStopReason(time.Now())
		if reason == "" {
			return nil
		}
	}
	pid := p.Pid()
	err := terminate(pid)
	p.mu.Lock()
	p.stopErr = err
	if err == nil {
		p.stopped = true
		p.idleStopMessage = reason
	}
	p.mu.Unlock()
	if err == nil {
		p.cancel()
	}
	return err
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

		// 与停止请求同步，确保终止原因先于最终事件可见。
		p.stopMu.Lock()
		defer p.stopMu.Unlock()
		defer close(p.waitDone)
		defer p.cancel()
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
