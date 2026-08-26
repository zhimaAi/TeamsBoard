package bootstrap

import (
	"context"

	"goteams-client/internal/applog"
)

// MonitorParentProcess cancels the Go sidecar when its Electron parent exits
// unexpectedly. Normal shutdown still uses the authenticated HTTP endpoint;
// this monitor covers crashes, task-manager termination and forced updates.
func MonitorParentProcess(ctx context.Context, parentPID int, cancel func()) {
	if parentPID <= 0 || cancel == nil {
		return
	}
	if waitForParentExit(ctx, parentPID) {
		applog.Warn("Electron parent exited; shutting down Go sidecar", "parent_pid", parentPID)
		cancel()
	}
}
