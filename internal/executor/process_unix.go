//go:build !windows

package executor

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// snapshotProcessParents 读取当前进程表，返回 pid 到父 pid 的映射。
// 读不到时由调用方按「子进程还在」处理，避免误杀正在编译的工具。
func snapshotProcessParents() (map[int]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,stat=").Output()
	if err != nil {
		return nil, err
	}
	parents := make(map[int]int)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || strings.HasPrefix(fields[2], "Z") {
			continue
		}
		pid, errPID := strconv.Atoi(fields[0])
		ppid, errPPID := strconv.Atoi(fields[1])
		if errPID != nil || errPPID != nil {
			continue
		}
		parents[pid] = ppid
	}
	return parents, nil
}

// CreateProcessGroup sets process group attributes
func CreateProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// The maximum time to wait for the process to exit itself after terminateGracePeriod SIGTERM.
const terminateGracePeriod = 3 * time.Second

// terminatePollInterval The interval for polling whether the process group has exited.
const terminatePollInterval = 50 * time.Millisecond

// TerminateProcessTree terminates the process tree.
//
// First SIGTERM gives the process a chance to clean up by itself, and then polls for detection; once the process group exits, it will return immediately.
// No longer unconditionally sleep for 3 seconds - most CLIs will exit within tens of milliseconds.
// Fixed waiting will cause the "stop execution" operation to be obviously stuck.
func TerminateProcessTree(pgid int) error {
	//Send SIGTERM first
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		// The process may have exited
		if err == syscall.ESRCH {
			return nil
		}
	}

	deadline := time.Now().Add(terminateGracePeriod)
	for time.Now().Before(deadline) {
		// Signal 0 only does existence detection and does not actually deliver the signal.
		if err := syscall.Kill(-pgid, 0); err == syscall.ESRCH {
			return nil
		}
		time.Sleep(terminatePollInterval)
	}

	// If no exit occurs within the grace period, force SIGKILL
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		if err == syscall.ESRCH {
			return nil
		}
		return fmt.Errorf("SIGKILL 失败: %w", err)
	}
	return nil
}
