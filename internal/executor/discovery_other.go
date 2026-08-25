//go:build !windows

package executor

// persistedPathDirs returns directories persisted in the environment PATH
// outside the current process. On non-Windows platforms the process inherits
// the full environment from its parent, so there is no separate persisted PATH
// to consult.
func persistedPathDirs() []string {
	return nil
}
