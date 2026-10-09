//go:build windows

package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// realHomeDir 返回当前 Windows 用户主目录。
//
// Windows 换盘符（系统重装、双系统盘符互换等）后，os.UserHomeDir 一定返回
// 当前真实盘符下的 HOME；这里额外把短文件名/8.3 形式归一，避免与搬迁前
// 记录的绝对路径出现 `C:\Users\ADMINI~1` 与 `C:\Users\Administrator` 不一致。
func realHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return home
	}
	if long, err := getLongPathName(filepath.Clean(home)); err == nil && long != "" {
		return long
	}
	return filepath.Clean(home)
}

// getLongPathName 把可能包含 8.3 短文件名的路径归一为长路径。
func getLongPathName(p string) (string, error) {
	pathPtr, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetLongPathName(pathPtr, &buf[0], uint32(len(buf)))
	if err != nil {
		return "", err
	}
	if n == 0 {
		return p, nil
	}
	return windows.UTF16ToString(buf[:n]), nil
}

// volumeSerialNumber 返回路径所在卷的卷序列号。
// 卷序列号在盘符互换后保持不变，是判断"两个路径是否实际在同一物理卷"
// 的可靠依据。路径不存在时逐级向上取最近已存在祖先。
func volumeSerialNumber(p string) (uint32, error) {
	p = existingAncestor(p)
	if p == "" {
		return 0, os.ErrNotExist
	}
	pathPtr, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return 0, err
	}
	// 打开目录本身需要 FILE_FLAG_BACKUP_SEMANTICS。
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(handle)

	var serial uint32
	var maxLen, flags uint32
	volBuf := make([]uint16, windows.MAX_PATH+1)
	fsBuf := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumeInformationByHandle(
		handle,
		&volBuf[0], uint32(len(volBuf)),
		&serial,
		&maxLen, &flags,
		&fsBuf[0], uint32(len(fsBuf)),
	); err != nil {
		return 0, err
	}
	return serial, nil
}

// existingAncestor 返回路径最近已存在的祖先（含自身）；全部不存在时返回空。
func existingAncestor(p string) string {
	p = filepath.Clean(p)
	for {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			return ""
		}
		p = parent
	}
}

// canonicalizeWindowsTarget 在检测到目标路径的盘符与
// 真实卷不一致时，把盘符替换为 HOME 当前所在盘符。
// 返回 (归一后的路径, 是否发生变化)。
//
// 触发条件：指针文件里记录的是搬迁时的盘符（例如 D:），之后系统盘符互换
// （例如系统盘变成 D:、数据盘变成 C:），此时 HOME 的真实盘符才是数据盘
// 当前盘符，目标路径应随之归一。
func canonicalizeWindowsTarget(target string) (string, bool) {
	homeVol := filepath.VolumeName(realHomeDir())
	targetVol := filepath.VolumeName(target)
	if homeVol == "" || targetVol == "" || strings.EqualFold(homeVol, targetVol) {
		return target, false
	}
	// 只有目标卷与 HOME 卷实际是同一物理卷（卷序列号相同）才允许归一；
	// 否则说明数据真的在另一块盘上，不做无谓改写。
	homeSerial, err := volumeSerialNumber(realHomeDir())
	if err != nil {
		return target, false
	}
	targetSerial, err := volumeSerialNumber(target)
	if err != nil || homeSerial != targetSerial {
		return target, false
	}
	rest := strings.TrimPrefix(target, targetVol)
	return homeVol + rest, true
}

// sameVol Windows 下按盘符（VolumeName）判断；盘符不同即跨卷。
// 盘符相同仍可能是挂载点（junction）跨卷，搬迁时 rename 失败会回退到复制，
// 这里保持轻量判断。
func sameVol(a, b string) bool {
	volA := filepath.VolumeName(a)
	volB := filepath.VolumeName(b)
	if volA == "" || volB == "" {
		return true
	}
	return strings.EqualFold(volA, volB)
}

// sameWindowsVolume 供 dataroot.go 使用；与 sameVol 语义一致。
// 独立保留是为了未来需要按卷序列号严格判断时只改这里。
func sameWindowsVolume(a, b string) bool { return sameVol(a, b) }

// isElectronProcessName 命中 Electron 桌面宿主的进程名（开发模式 node 启动
// electron 时父链为 node.exe → electron.exe → 本进程；打包形态由
// GOTEAMS_DESKTOP_TOKEN 环境变量保证，无需进程名兜底）。
func isElectronProcessName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.Contains(name, "electron")
}

// processEntry 是进程快照中的一条记录。
type processEntry struct {
	pid  int
	ppid int
	name string
}

// snapshotProcesses 枚举当前进程快照。
func snapshotProcesses() ([]processEntry, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return nil, err
	}
	var out []processEntry
	for {
		out = append(out, processEntry{
			pid:  int(entry.ProcessID),
			ppid: int(entry.ParentProcessID),
			name: windows.UTF16ToString(entry.ExeFile[:]),
		})
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	return out, nil
}

// processName 返回指定 PID 的进程名；查不到返回空串。
func processName(pid int) string {
	entries, err := snapshotProcesses()
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.pid == pid {
			return e.name
		}
	}
	return ""
}

// parentPIDOf 返回指定 PID 的父进程 ID；查不到返回 0。
func parentPIDOf(pid int) int {
	entries, err := snapshotProcesses()
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if e.pid == pid {
			return e.ppid
		}
	}
	return 0
}
