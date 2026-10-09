// Package workspace 管理工作空间（数据根）的解析与重定位。
//
// 数据根默认是 <HOME>/.goteams。桌面端用户在配置中心选择新的父目录后，
// 搬迁流程把整个 .goteams 目录搬到 <新父目录>/.goteams，并在 HOME 下长期保留
// .goteams.mlink 指针文件记录真实位置；之后每次启动由 ResolveDataRoot 解析
// 指针回到真实数据根。因为整个数据根（含数据库）一起搬迁，数据库里的绝对路径
// 在新位置保持成立；Windows 换盘符场景在启动时按卷序列号把指针与历史路径
// 归一到真实盘符。
package workspace

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"goteams-client/internal/applog"
)

const (
	// dataRootDirName 是数据根目录名；新父目录下同样以该名字创建。
	dataRootDirName = ".goteams"
	// dataRootLinkFileName 是 HOME 下记录重定位目标的指针文件名。
	// 指针文件搬迁成功后长期保留，是「该用户曾经迁移过工作空间」的唯一事实源：
	// 换盘符、系统重装后只要用户目录还在，启动即可找回真实数据根。
	dataRootLinkFileName = ".goteams.mlink"
	// dataRootSentinelName 是数据根结构标记文件名。只有带此标记的目录才被认为
	// 是完整可用的数据根；它让启动逻辑能区分「完整数据」与「搬迁残留的空壳」，
	// 也能在「原父目录出现同名新用户」时避免误用别人的数据目录。
	dataRootSentinelName = ".goteams-root"
)

// ErrWorkspaceConflict 表示目标父目录下已存在 .goteams 目录（禁止合并）。
var ErrWorkspaceConflict = errors.New("目标父目录下已存在 .goteams 目录")

// DefaultDataRoot 返回当前平台的默认数据根路径（真实 HOME/.goteams）。
func DefaultDataRoot() string {
	return filepath.Join(realHomeDir(), dataRootDirName)
}

// LinkFilePath 返回 HOME 下指针文件的路径（错误提示与排障时使用）。
func LinkFilePath() string {
	return filepath.Join(realHomeDir(), dataRootLinkFileName)
}

// ResolveDataRoot 在启动早期解析数据根：
//  1. HOME 下无指针文件 → 使用并确保默认根存在；
//  2. 有指针文件 → Windows 下先按卷序列号归一盘符（换盘符自愈），再校验目标
//     合法性；目标不可用时，默认根完整则回退默认根，否则返回带排障指引的错误。
func ResolveDataRoot() (string, error) {
	home := realHomeDir()
	defaultRoot := filepath.Join(home, dataRootDirName)
	linkPath := filepath.Join(home, dataRootLinkFileName)

	data, err := os.ReadFile(linkPath)
	if errors.Is(err, os.ErrNotExist) {
		return defaultRoot, os.MkdirAll(defaultRoot, 0o755)
	}
	if err != nil {
		return "", fmt.Errorf("读取工作空间指针文件失败: %w", err)
	}

	target := strings.TrimSpace(string(data))
	if target == "" {
		applog.Warn("工作空间指针文件为空，使用默认数据根", "link", linkPath)
		return defaultRoot, os.MkdirAll(defaultRoot, 0o755)
	}
	target = filepath.Clean(target)

	// Windows 盘符互换后（例如系统盘从 C 变成 D、数据盘变成 C），指针里记录的
	// 仍是搬迁时的盘符。先按卷序列号把目标归一到真实盘符并回写指针，
	// 之后的校验与使用都基于归一后的路径。
	if runtime.GOOS == "windows" {
		if canonical, changed := canonicalizeWindowsTarget(target); changed {
			if dirLooksComplete(canonical) {
				if writeErr := os.WriteFile(linkPath, []byte(canonical), 0o600); writeErr != nil {
					applog.Warn("重写工作空间指针失败", "link", linkPath, "error", writeErr.Error())
				} else {
					applog.Info("检测到系统盘符变化，已校正工作空间指针", "from", target, "to", canonical)
				}
				target = canonical
			} else {
				applog.Warn("盘符变化后的工作空间目录不完整，继续使用旧指针路径",
					"from", target, "to", canonical)
			}
		}
	}

	if err := validateRelocatedTarget(home, defaultRoot, target); err != nil {
		if dirLooksComplete(defaultRoot) {
			applog.Warn("工作空间指针目标不可用，回退默认数据根",
				"target", target, "reason", err.Error())
			return defaultRoot, nil
		}
		return "", fmt.Errorf("工作空间目录不可用：%w（默认目录下没有完整数据，无法回退；请检查磁盘连接后重启，或删除 %s 恢复默认工作空间）", err, linkPath)
	}

	return target, nil
}

// validateRelocatedTarget 校验指针目标目录：
//   - Windows 下目标的卷必须存在且就是 HOME 当前所在卷（换盘符已在调用方归一，
//     走到这里仍不一致说明数据真的在丢失的盘上）；
//   - 目标必须位于该卷的某个用户目录之下（防止指向系统/程序目录）；
//   - 目标必须带结构标记，即看起来是完整的数据根。
func validateRelocatedTarget(home, defaultRoot, target string) error {
	if samePath(target, defaultRoot) || target == filepath.Dir(defaultRoot) {
		return fmt.Errorf("目标 %s 不是有效的重定位目录", target)
	}
	if runtime.GOOS == "windows" {
		homeSerial, err := volumeSerialNumber(home)
		if err != nil {
			return fmt.Errorf("读取用户目录所在卷失败: %w", err)
		}
		targetSerial, err := volumeSerialNumber(target)
		if err != nil {
			return fmt.Errorf("目标目录所在卷不可用: %w", err)
		}
		if homeSerial != targetSerial {
			return fmt.Errorf("目标目录与用户目录不在同一卷，目标 %s", target)
		}
		if !isUnderUserHome(target) {
			return fmt.Errorf("目标目录 %s 不在任何用户目录下", target)
		}
	}
	if !dirLooksComplete(target) {
		return fmt.Errorf("目标目录 %s 缺少完整的工作空间结构", target)
	}
	return nil
}

// dirLooksComplete 判断目录是否是已建立结构的数据根（存在结构标记文件）。
func dirLooksComplete(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, dataRootSentinelName))
	return err == nil && info.Mode().IsRegular()
}

// MarkDataRootStructure 建立数据根结构标记。首次初始化和搬迁后都会调用。
func MarkDataRootStructure(root string) error {
	marker := filepath.Join(root, dataRootSentinelName)
	content := "goteams data root\n" + time.Now().Format(time.RFC3339) + "\n"
	if err := os.WriteFile(marker, []byte(content), 0o600); err != nil {
		return fmt.Errorf("写入工作空间结构标记失败: %w", err)
	}
	return nil
}

// samePath 宽松比较两个路径（清洗 + Windows 忽略大小写）。
func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// isUnderUserHome 判断路径是否位于某用户目录之内（Windows: <卷>:\Users\<name>\...）。
// 用于阻止把工作空间指到系统盘根目录或 Program Files 等位置。
func isUnderUserHome(p string) bool {
	parts := strings.Split(strings.ToUpper(filepath.Clean(p)), string(filepath.Separator))
	// Windows 绝对路径形态: ["", "C:", "USERS", "<name>", ...]
	if len(parts) >= 5 && parts[2] == "USERS" && parts[3] != "" {
		return true
	}
	return false
}

// copyDirTree 递归复制目录；遇到符号链接按链接本身复制（不跟随），
// 避免重定位时把链接指向的外部内容复制进数据根。
func copyDirTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	}
	if !info.IsDir() {
		return copyFile(src, dst, info.Mode())
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyDirTree(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// copyFile 复制单个文件内容并保留权限位。
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// countEntries 统计目录内条目数（含隐藏文件），用于空目录校验。
func countEntries(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

// pathDepth 返回清洗后路径的层级深度，用于禁止磁盘根目录的校验。
func pathDepth(p string) int {
	p = strings.TrimSuffix(filepath.Clean(p), string(filepath.Separator))
	return strings.Count(p, string(filepath.Separator))
}

// relocateDataRoot 把源数据根搬迁到 dstParent/.goteams。
// 调用方需已完成所有前置校验；函数保证：
//  1. 同卷优先 rename（原子），跨卷复制后删除；
//  2. 任何中途失败都尽量回滚，源目录保持可用。
//
// 注意：搬迁成功后调用方必须立刻写指针文件；两条语句之间进程被杀的极端窗口
// 由启动日志与指针校验兜底（数据完整保留在新目录，可手工恢复指针）。
func relocateDataRoot(srcRoot, dstParent string) (newRoot string, err error) {
	newRoot = filepath.Join(dstParent, dataRootDirName)

	moved := false
	defer func() {
		if err != nil {
			// 回滚：目标已部分建立则搬回/删除；源目录已不存在则从目标搬回。
			if _, statErr := os.Stat(srcRoot); errors.Is(statErr, os.ErrNotExist) {
				if _, tgtErr := os.Stat(newRoot); tgtErr == nil {
					if rbErr := os.Rename(newRoot, srcRoot); rbErr != nil {
						applog.Error("搬迁失败且自动回滚失败，请手工把目录移回原位",
							"from", newRoot, "to", srcRoot, "error", rbErr.Error())
					}
				}
			} else if _, tgtErr := os.Stat(newRoot); tgtErr == nil && !moved {
				// 源仍在、目标是复制残留：清理目标避免下次被判为非空冲突。
				if rmErr := os.RemoveAll(newRoot); rmErr != nil {
					applog.Error("清理搬迁残留目标目录失败", "target", newRoot, "error", rmErr.Error())
				}
			}
		}
	}()

	if sameVol(srcRoot, dstParent) {
		if err = os.Rename(srcRoot, newRoot); err != nil {
			err = fmt.Errorf("移动工作空间目录失败: %w", err)
			return "", err
		}
		moved = true
	} else {
		if err = copyDirTree(srcRoot, newRoot); err != nil {
			err = fmt.Errorf("复制工作空间目录失败: %w", err)
			return "", err
		}
		moved = true
		if err = os.RemoveAll(srcRoot); err != nil {
			// 复制完成但源删除失败：目标已完整，保留目标，源残留由用户手工清理。
			applog.Error("工作空间已复制到新目录，但删除旧目录失败，请手工清理",
				"old", srcRoot, "error", err.Error())
			err = nil
		}
	}
	return newRoot, nil
}
