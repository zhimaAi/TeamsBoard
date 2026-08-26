//go:build windows

package bootstrap

import (
	"context"

	"golang.org/x/sys/windows"
)

func waitForParentExit(ctx context.Context, parentPID int) bool {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(parentPID))
	if err != nil {
		return true
	}
	defer windows.CloseHandle(handle)

	for {
		result, waitErr := windows.WaitForSingleObject(handle, 1_000)
		if result == windows.WAIT_OBJECT_0 {
			return true
		}
		if waitErr != nil && result != uint32(windows.WAIT_TIMEOUT) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		default:
		}
	}
}
