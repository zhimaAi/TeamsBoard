//go:build !windows

package bootstrap

import (
	"os"
	"syscall"
)

// tryFlock tries to acquire a non-blocking exclusive lock
func tryFlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// unlockFile releases the file lock
func unlockFile(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
