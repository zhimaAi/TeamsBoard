package knowledge

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"goteams-client/internal/applog"
)

// ==================== KR 迁移执行器（S-BE-07 ~ S-BE-14 / S-DA-06 ~ S-DA-09） ====================
//
// 迁移语义：把旧 KR（知识库根目录）下的全部顶层项移动到新 KR，包括正文文件夹与 md/txt 文档、
// 以及回收站 .trash 与历史版本 .history。
//
// 设计约束：
//   - 不做任何 DB 写入：gt_knowledge_documents.file_path 是相对 KR 的路径，迁移后语义不变（S-DA-06）。
//   - .trash / .history 作为普通顶层项整体移动，不解析其内部结构，命名规则保持不变（S-BE-11 / S-DA-09）。
//   - 移动采用「同卷 os.Rename，跨卷复制 + 删源」，全过程中不得覆盖目标目录既有内容（S-DA-07 / CF-7）。
//   - 任一顶层项失败即返回错误与已完成的 journal，由调用方逆序回滚（S-BE-09）。

const (
	// migrateMode 是 PUT /knowledge/root 当前唯一支持的模式（S-IN-06）。
	migrateMode = "migrate"

	migrateMethodRename     = "rename"
	migrateMethodCopyDelete = "copy-delete"

	conflictTypeFile = "file"
	conflictTypeDir  = "dir"
)

// migrateConflict 迁移前预检发现的同名冲突项。
// CF-3：只需枚举旧 KR 顶层项即可完备——目标目录存在深层同名项时，其顶层祖先必然同名。
type migrateConflict struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

// migrateStats 迁移统计（S-IN-06 成功响应 migrated 字段）。
type migrateStats struct {
	Entries   int `json:"entries"`
	Folders   int `json:"folders"`
	Documents int `json:"documents"`
}

// migratePlan 迁移计划（预检产物，含待迁移的顶层项名与统计）。
type migratePlan struct {
	oldRoot string
	newRoot string
	entries []string
	stats   migrateStats
}

// migrateJournalEntry 已成功移动的顶层项记录，用于失败时逆序回滚（S-BE-09）。
type migrateJournalEntry struct {
	Name   string
	Src    string
	Dst    string
	Method string
}

// migrateRollbackResult 回滚结果（S-BE-09 失败响应 rollback 字段）。
type migrateRollbackResult struct {
	Completed bool     `json:"completed"`
	Failed    []string `json:"failed"`
}

// sameVolumeFn 同卷判定入口，可在测试中注入以模拟跨卷迁移（CF-7 / R-2）。
var sameVolumeFn = sameVolume

// migrateEntryMover 单个顶层项的移动实现，可在测试中注入以模拟迁移中途失败（S-BE-09 验收方式）。
var migrateEntryMover = moveEntry

// sameVolume 默认同卷判定（CF-7）：比较各自的卷标识。
// 无法取得卷标识时保守视为同卷——os.Rename 失败仍会自动降级为「复制 + 删源」，不会丢数据。
func sameVolume(a, b string) bool {
	idA, okA := volumeID(cleanAbsPath(a))
	idB, okB := volumeID(cleanAbsPath(b))
	if !okA || !okB {
		return true
	}
	return strings.EqualFold(idA, idB)
}

// ==================== 路径与冲突判定 ====================

// isSameOrSubPath 判断 target 是否等于 base 或位于 base 之下（S-BE-10）。
// Windows 路径大小写不敏感，比较前统一小写。
func isSameOrSubPath(base, target string) bool {
	basePath := cleanAbsPath(base)
	targetPath := cleanAbsPath(target)
	if basePath == "" || targetPath == "" {
		return false
	}
	if runtime.GOOS == "windows" {
		basePath = strings.ToLower(basePath)
		targetPath = strings.ToLower(targetPath)
	}
	if basePath == targetPath {
		return true
	}
	return strings.HasPrefix(targetPath, basePath+string(os.PathSeparator))
}

// cleanAbsPath 归一化为绝对路径；失败时退化为 Clean 结果。
func cleanAbsPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

// pathExists 判断路径（含目录）是否存在，不跟随符号链接。
func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// isDocumentFileName 判断文件名是否为知识库索引的文档类型（md / txt）。
func isDocumentFileName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".txt":
		return true
	default:
		return false
	}
}

// ==================== 预检（S-BE-08 / S-IN-07） ====================

// buildMigratePlan 读取旧 KR 顶层项、检出与新 KR 的同名冲突，并统计迁移规模。
// 冲突即中止（不覆盖、不自动改名），因此本函数只做只读扫描，不产生任何副作用。
func buildMigratePlan(oldRoot, newRoot string) (migratePlan, []migrateConflict, error) {
	plan := migratePlan{oldRoot: oldRoot, newRoot: newRoot, entries: []string{}}
	conflicts := []migrateConflict{}

	entries, err := os.ReadDir(oldRoot)
	if err != nil {
		if os.IsNotExist(err) {
			// 旧 KR 尚未建立：无内容需要迁移，视为空计划
			return plan, conflicts, nil
		}
		return plan, conflicts, fmt.Errorf("读取旧知识库目录失败: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		plan.entries = append(plan.entries, name)
		if info, statErr := os.Lstat(filepath.Join(newRoot, name)); statErr == nil {
			kind := conflictTypeFile
			if info.IsDir() {
				kind = conflictTypeDir
			}
			conflicts = append(conflicts, migrateConflict{Path: name, Type: kind})
		}
	}
	plan.stats = preScanStats(oldRoot, entries)
	return plan, conflicts, nil
}

// preScanStats 递归统计迁移规模：顶层项数 + 文件夹数 + md/txt 文档数。
// 统计口径与 runScan 的索引口径一致：跳过隐藏目录（含 .trash / .history）及其内容。
func preScanStats(oldRoot string, entries []fs.DirEntry) migrateStats {
	stats := migrateStats{Entries: len(entries)}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(oldRoot, name)
		if !entry.IsDir() {
			if isDocumentFileName(name) {
				stats.Documents++
			}
			continue
		}
		stats.Folders++
		_ = filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// 统计失败不影响迁移主流程，交由迁移阶段的真实错误上报
				return nil
			}
			if d.IsDir() {
				if path == full {
					return nil
				}
				if strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				stats.Folders++
				return nil
			}
			if isDocumentFileName(d.Name()) {
				stats.Documents++
			}
			return nil
		})
	}
	return stats
}

// ==================== 迁移与回滚（S-BE-07 / S-BE-09 / S-BE-11） ====================

// executeMigrate 逐顶层项执行移动并记录 journal。
// 任一项失败立即返回错误与已完成的 journal；调用方据此回滚（S-BE-09）。
func executeMigrate(plan migratePlan) ([]migrateJournalEntry, error) {
	journal := make([]migrateJournalEntry, 0, len(plan.entries))
	for _, name := range plan.entries {
		src := filepath.Join(plan.oldRoot, name)
		dst := filepath.Join(plan.newRoot, name)
		// 预检已确认目标顶层项原本不存在；此处再记录一次，
		// 以便失败时仅清理「本次创建」的半成品路径（禁止无前提的 RemoveAll）
		dstExistedBefore := pathExists(dst)

		method, err := migrateEntryMover(src, dst)
		if err != nil {
			if !dstExistedBefore && pathExists(dst) {
				if rmErr := os.RemoveAll(dst); rmErr != nil {
					applog.Warn("清理迁移失败产生的半成品路径失败", "path", dst, "error", rmErr)
				}
			}
			return journal, fmt.Errorf("迁移 %q 失败: %w", name, err)
		}
		journal = append(journal, migrateJournalEntry{Name: name, Src: src, Dst: dst, Method: method})
	}
	return journal, nil
}

// rollbackMigrate 逆序回滚 journal 中已成功移动的顶层项（S-BE-09）。
// 仅当全部项都回滚成功才返回 completed=true；失败项按原始名称列出，供前端提示人工处理。
func rollbackMigrate(journal []migrateJournalEntry) migrateRollbackResult {
	result := migrateRollbackResult{Completed: true, Failed: []string{}}
	for i := len(journal) - 1; i >= 0; i-- {
		entry := journal[i]
		var err error
		if entry.Method == migrateMethodRename {
			if rerr := os.Rename(entry.Dst, entry.Src); rerr == nil {
				continue
			}
			// rename 回滚失败时降级为复制回原位置，尽量恢复迁移前状态
			err = copyTreeThenRemoveSource(entry.Dst, entry.Src)
		} else {
			err = copyTreeThenRemoveSource(entry.Dst, entry.Src)
		}
		if err != nil {
			result.Completed = false
			result.Failed = append(result.Failed, entry.Name)
			applog.Warn("知识库目录迁移回滚失败", "name", entry.Name, "error", err)
		}
	}
	return result
}

// pruneEmptyDir 在「确认为空」的前提下移除已迁空的旧 KR 顶层目录，返回是否真的移除了（E18）。
//
// 严格语义（AGENTS.md：删除不得依赖未经确认的空参数或过宽条件）：
//   - 读取失败：一律保留并返回 false（目录已不存在视为无需清理，属幂等成功路径）；
//   - 仍有任何顶层项：保留并返回 false（非空即保留，绝不递归删除）；
//   - 确认为空：仅用 os.Remove 移除该空目录本身，禁用 os.RemoveAll。
//
// 清理只是迁移收尾语义，失败不影响迁移与配置写入的成功判定，故仅 Warn 不上抛。
func pruneEmptyDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			applog.Warn("检查旧知识库目录是否为空失败，保留目录", "dir", dir, "error", err)
		}
		return false
	}
	if len(entries) > 0 {
		applog.Info("旧知识库目录仍有残留内容，保留目录", "dir", dir, "entries", len(entries))
		return false
	}
	// 已确认空目录：只移除目录本身，不递归（os.Remove 对非空目录会失败，天然兜底）
	if err := os.Remove(dir); err != nil {
		applog.Warn("移除已迁空的旧知识库目录失败", "dir", dir, "error", err)
		return false
	}
	applog.Info("已移除迁移后空的旧知识库目录", "dir", dir)
	return true
}

// moveEntry 把单个顶层项从 src 移动到 dst，返回实际使用的方式。
// 同卷优先 os.Rename（原子快路径）；判定为跨卷或 rename 失败时降级为「复制 + 删源」（CF-7）。
func moveEntry(src, dst string) (string, error) {
	var renameErr error
	if sameVolumeFn(src, dst) {
		if err := os.Rename(src, dst); err == nil {
			return migrateMethodRename, nil
		} else {
			renameErr = err
		}
	}
	if err := copyTreeThenRemoveSource(src, dst); err != nil {
		if renameErr != nil {
			return "", fmt.Errorf("rename 失败(%v)；复制迁移失败: %w", renameErr, err)
		}
		return "", err
	}
	return migrateMethodCopyDelete, nil
}

// copyTreeThenRemoveSource 递归复制后删除源目录/文件（跨卷降级路径）。
// 复制阶段通过 O_EXCL 与「目标已存在即拒绝」保证绝不覆盖既有数据。
func copyTreeThenRemoveSource(src, dst string) error {
	if err := copyPath(src, dst); err != nil {
		return err
	}
	if err := os.RemoveAll(src); err != nil {
		return fmt.Errorf("删除迁移源路径失败: %w", err)
	}
	return nil
}

// copyPath 递归复制文件或目录；目标路径若已存在则拒绝，符号链接一律拒绝复制。
func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("拒绝迁移符号链接: %s", src)
	}
	if pathExists(dst) {
		return fmt.Errorf("目标路径已存在，拒绝覆盖: %s", dst)
	}
	if !info.IsDir() {
		return copyFileExclusive(src, dst, info.Mode().Perm())
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyPath(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// copyFileExclusive 以 O_EXCL 创建目标文件后复制内容，任何情况下都不覆盖既有文件。
func copyFileExclusive(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
