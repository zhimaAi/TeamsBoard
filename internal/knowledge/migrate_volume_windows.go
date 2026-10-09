//go:build windows

package knowledge

import "path/filepath"

// volumeID 返回路径所在卷的标识（Windows：盘符，如 "C:"）。
func volumeID(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	volume := filepath.VolumeName(path)
	if volume == "" {
		return "", false
	}
	return volume, true
}
