//go:build !windows

package bootstrap

import (
	"context"
	"errors"
	"syscall"
	"time"
)

func waitForParentExit(ctx context.Context, parentPID int) bool {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if err := syscall.Kill(parentPID, 0); errors.Is(err, syscall.ESRCH) {
				return true
			}
		}
	}
}
