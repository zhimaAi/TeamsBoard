package codex

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"goteams-client/internal/executor"
)

const maxDiagnosticBytes = 64 * 1024

// diagnosticTail only retains the end part of the CLI diagnostic output to prevent abnormal output from occupying unlimited memory.
type diagnosticTail struct {
	data []byte
}

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

func formatWaitError(waitErr error) string {
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return fmt.Sprintf("Codex CLI 退出码 %d", exitErr.ExitCode())
	}
	return "Codex CLI 进程退出: " + waitErr.Error()
}

func timeNowMillis() int64 {
	return time.Now().UnixMilli()
}

func environ() []string {
	return os.Environ()
}

func createProcessGroupForCmd(cmd *exec.Cmd) {
	executor.CreateProcessGroup(cmd)
}

func terminateProcessTreeForPID(pid int) error {
	return executor.TerminateProcessTree(pid)
}
