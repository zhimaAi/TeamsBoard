//go:build !windows

package knowledge

import (
	"os"
	"strconv"
	"syscall"
)

// volumeID 返回路径所在卷的标识（非 Windows：设备号），用于判定 os.Rename 是否会跨卷失败。
func volumeID(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false
	}
	return strconv.FormatUint(uint64(stat.Dev), 10), true
}
