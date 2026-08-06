//go:build windows

package executor

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const createNewProcessGroup = 0x00000200
const processExitVerificationTimeout = 2 * time.Second

// CreateProcessGroup sets process group attributes
func CreateProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup,
	}
}

// TerminateProcessTree terminates the process tree
func TerminateProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}

	// taskkill may return a non-zero exit code when some descendants have already exited or when another termination is not supported,
	// Even if the entire target process tree is actually cleaned. Record the process tree before calling, and use the actual survival after calling
	// PID shall prevail to avoid misreporting "part of the operation failed but finally exited" as a termination failure.
	targetPIDs, snapshotErr := collectProcessTreePIDs(pid)

	// taskkill /T will wait for and forcefully terminate the root process and all its descendants.
	killCmd := exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprintf("%d", pid))
	output, err := killCmd.CombinedOutput()
	if snapshotErr == nil {
		survivors, verifyErr := waitForProcessesToExit(targetPIDs, processExitVerificationTimeout)
		if verifyErr == nil {
			if len(survivors) == 0 {
				return nil
			}
			detail := strings.TrimSpace(string(output))
			if detail == "" {
				detail = "taskkill 未提供错误详情"
			}
			return fmt.Errorf(
				"taskkill 未完全终止进程树 %d，仍存活 PID: %v: %s",
				pid, survivors, detail,
			)
		}
	}

	// Retain the original semantics of taskkill when a complete snapshot cannot be obtained; a successful return can still be directly regarded as a success.
	if err == nil {
		return nil
	}

	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return fmt.Errorf("taskkill 终止进程树 %d 失败: %w", pid, err)
	}
	return fmt.Errorf("taskkill 终止进程树 %d 失败: %w: %s", pid, err, detail)
}

func collectProcessTreePIDs(rootPID int) ([]int, error) {
	parents, err := snapshotProcessParents()
	if err != nil {
		return nil, err
	}

	children := make(map[int][]int)
	for processID, parentID := range parents {
		children[parentID] = append(children[parentID], processID)
	}

	seen := map[int]bool{rootPID: true}
	queue := []int{rootPID}
	for len(queue) > 0 {
		parentID := queue[0]
		queue = queue[1:]
		for _, childPID := range children[parentID] {
			if seen[childPID] {
				continue
			}
			seen[childPID] = true
			queue = append(queue, childPID)
		}
	}

	processIDs := make([]int, 0, len(seen))
	for processID := range seen {
		processIDs = append(processIDs, processID)
	}
	sort.Ints(processIDs)
	return processIDs, nil
}

func waitForProcessesToExit(processIDs []int, timeout time.Duration) ([]int, error) {
	targets := make(map[int]bool, len(processIDs))
	for _, processID := range processIDs {
		targets[processID] = true
	}

	deadline := time.Now().Add(timeout)
	for {
		parents, err := snapshotProcessParents()
		if err != nil {
			return nil, err
		}

		// There may still be descendants just forked during taskkill execution; as long as their parent PID belongs to the target tree,
		// Continue to include the verification to avoid missing newly generated orphan processes after the root process exits.
		for {
			added := false
			for processID, parentID := range parents {
				if targets[parentID] && !targets[processID] {
					targets[processID] = true
					added = true
				}
			}
			if !added {
				break
			}
		}

		survivors := make([]int, 0)
		for processID := range targets {
			if _, running := parents[processID]; running {
				survivors = append(survivors, processID)
			}
		}
		sort.Ints(survivors)
		if len(survivors) == 0 || !time.Now().Before(deadline) {
			return survivors, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func snapshotProcessParents() (map[int]int, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		if err == windows.ERROR_NO_MORE_FILES {
			return map[int]int{}, nil
		}
		return nil, err
	}

	parents := make(map[int]int)
	for {
		parents[int(entry.ProcessID)] = int(entry.ParentProcessID)
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				return parents, nil
			}
			return nil, err
		}
	}
}
