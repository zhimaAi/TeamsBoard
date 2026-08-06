//go:build !windows

package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
)

// fileLock universal file lock (macOS flock / Linux flock)
type fileLock struct {
	file *os.File
}

// acquireLockImpl acquires a single instance lock
func (a *App) acquireLockImpl() error {
	lockPath := filepath.Join(a.runtimeDir, "instance.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("创建锁文件失败: %w", err)
	}

	//Use flock exclusive lock (non-blocking)
	if err := tryFlock(f); err != nil {
		f.Close()
		return fmt.Errorf("GoTeams 客户端已在运行")
	}

	a.lock = &fileLock{file: f}
	return nil
}

// releaseLockImpl releases the single instance lock
func (a *App) releaseLockImpl() {
	if fl, ok := a.lock.(*fileLock); ok && fl.file != nil {
		unlockFile(fl.file)
		fl.file.Close()
		fl.file = nil
	}
}
