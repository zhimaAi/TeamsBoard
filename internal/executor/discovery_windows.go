//go:build windows

package executor

import (
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// persistedPathDirs returns the directories persisted in the Windows user and
// machine PATH (registry). A long-running backend process may have been started
// before new PATH entries were added (or launched through a mechanism that
// bypasses the login-time PATH composition), so its in-memory PATH is stale even
// though the tools resolve fine in an interactive PowerShell. The user PATH takes
// priority over the machine PATH, matching how Windows composes the effective
// PATH. REG_EXPAND_SZ values are expanded before splitting.
func persistedPathDirs() []string {
	var dirs []string
	appendEntry := func(value string) {
		expanded := expandEnvStrings(value)
		for _, dir := range strings.Split(expanded, ";") {
			if dir = strings.TrimSpace(dir); dir != "" {
				dirs = append(dirs, dir)
			}
		}
	}

	if key, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE); err == nil {
		if value, _, err := key.GetStringValue("Path"); err == nil {
			appendEntry(value)
		}
		key.Close()
	}

	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`, registry.QUERY_VALUE); err == nil {
		if value, _, err := key.GetStringValue("Path"); err == nil {
			appendEntry(value)
		}
		key.Close()
	}

	return dirs
}

// expandEnvStrings expands environment variables (%VAR%) in a REG_EXPAND_SZ
// value using the Win32 API, falling back to the raw value on failure.
func expandEnvStrings(value string) string {
	src, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return value
	}
	buf := make([]uint16, 32768)
	n, err := windows.ExpandEnvironmentStrings(src, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		return value
	}
	return windows.UTF16ToString(buf[:n])
}
