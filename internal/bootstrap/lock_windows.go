//go:build windows

package bootstrap

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutex = kernel32.NewProc("CreateMutexW")
	procCloseHandle = kernel32.NewProc("CloseHandle")
)

const ERROR_ALREADY_EXISTS = syscall.Errno(183)

// windowsLock Windows named Mutex single instance lock
type windowsLock struct {
	handle uintptr
}

// acquireLockImpl acquires a single instance lock (Windows named Mutex)
func (a *App) acquireLockImpl() error {
	mutexName, _ := syscall.UTF16PtrFromString("Global\\GoTeams_Client_SingleInstance")

	ret, _, lastErr := procCreateMutex.Call(
		0, //Default security properties
		0, // Not initially owned
		uintptr(unsafe.Pointer(mutexName)),
	)

	// ret == 0 means the call failed
	if ret == 0 {
		return fmt.Errorf("创建 Mutex 失败: %w", lastErr)
	}

	// Check if it already exists (ERROR_ALREADY_EXISTS)
	if errno, ok := lastErr.(syscall.Errno); ok && errno == ERROR_ALREADY_EXISTS {
		procCloseHandle.Call(ret)
		return fmt.Errorf("GoTeams 客户端已在运行")
	}

	a.lock = &windowsLock{handle: ret}
	return nil
}

// releaseLockImpl releases the single instance lock
func (a *App) releaseLockImpl() {
	if wl, ok := a.lock.(*windowsLock); ok && wl.handle != 0 {
		procCloseHandle.Call(wl.handle)
		wl.handle = 0
	}
}
