package knowledge

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"goteams-client/internal/applog"
)

// folderRow 文件夹记录（用于解析真实相对路径）。
type folderRow struct {
	ID       int64
	Name     string
	ParentID int64
}

// MigrateLegacyContent 把旧的 <profileDir>/knowledge/content/<uuid>.md 一次性迁移为 KR 内真实目录结构（DA-03）。
// 行为：按 gt_knowledge_folders 层级把 uuid.md 重写为 <真实相对路径>.<ext>，并更新 file_path / ext。
// 幂等：legacy 不存在、或 KR 已存在真实文件时直接返回；失败单条记录日志、跳过且不删除原文件，可重跑。
func MigrateLegacyContent(krDir, profileDir string, db *sql.DB) error {
	if db == nil {
		return nil
	}
	legacy := filepath.Join(profileDir, "knowledge", "content")
	if info, err := os.Stat(legacy); err != nil || !info.IsDir() {
		// 无旧数据，无需迁移
		return nil
	}
	if dirHasRealFiles(krDir) {
		// KR 已有真实文件，视为已迁移过，跳过避免覆盖
		return nil
	}

	// 加载全部文件夹，构建 id -> folder 映射
	folders, err := loadFolders(db)
	if err != nil {
		return err
	}

	rows, err := db.Query(`SELECT uuid, folder_id, title FROM gt_knowledge_documents WHERE deleted=0`)
	if err != nil {
		return fmt.Errorf("查询旧文档失败: %w", err)
	}
	type legacyDoc struct {
		uuid     string
		folderID int64
		title    string
	}
	var docs []legacyDoc
	for rows.Next() {
		var d legacyDoc
		if err := rows.Scan(&d.uuid, &d.folderID, &d.title); err != nil {
			_ = rows.Close()
			return fmt.Errorf("读取旧文档失败: %w", err)
		}
		docs = append(docs, d)
	}
	_ = rows.Close()

	migrated := 0
	for _, d := range docs {
		relDir := resolveFolderRel(folders, d.folderID)
		fileName := SanitizeFileName(d.title) + ".md"
		relPath := fileName
		if relDir != "" {
			relPath = relDir + "/" + fileName
		}
		// 同名冲突自动加 (n) 后缀，避免覆盖（DA-02 验收③）
		relPath = uniqueRelPath(krDir, relPath)

		oldData, err := os.ReadFile(filepath.Join(legacy, d.uuid+".md"))
		if err != nil {
			applog.Warn("迁移旧文档失败：读取源文件错误，跳过", "uuid", d.uuid, "error", err)
			continue
		}
		if err := writeKRFile(krDir, relPath, oldData); err != nil {
			applog.Warn("迁移旧文档失败：写入目标文件错误，跳过", "uuid", d.uuid, "relPath", relPath, "error", err)
			continue
		}
		if _, err := db.Exec(`UPDATE gt_knowledge_documents SET file_path=?, ext='md' WHERE uuid=?`, relPath, d.uuid); err != nil {
			applog.Warn("迁移旧文档失败：更新索引错误，跳过", "uuid", d.uuid, "relPath", relPath, "error", err)
			continue
		}
		migrated++
	}

	// 迁移完成后将旧 content 目录重命名为 content.bak（标记备份，不删除，DA-03 验收①）
	if migrated > 0 {
		bak := legacy + ".bak"
		if _, err := os.Stat(bak); err != nil {
			if rerr := os.Rename(legacy, bak); rerr != nil {
				applog.Warn("重命名旧 content 目录失败（不影响迁移结果）", "error", rerr)
			}
		}
		applog.Info("知识库旧数据迁移完成", "krDir", krDir, "migrated", migrated)
	}
	return nil
}

// dirHasRealFiles 判断 KR 下是否存在除 .trash / .history 之外的真实文件或目录。
func dirHasRealFiles(krDir string) bool {
	entries, err := os.ReadDir(krDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if name == ".trash" || name == ".history" {
			continue
		}
		return true
	}
	return false
}

// loadFolders 读取全部文件夹记录。
func loadFolders(db *sql.DB) (map[int64]folderRow, error) {
	rows, err := db.Query(`SELECT id, name, parent_id FROM gt_knowledge_folders`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[int64]folderRow)
	for rows.Next() {
		var f folderRow
		if err := rows.Scan(&f.ID, &f.Name, &f.ParentID); err != nil {
			return nil, err
		}
		m[f.ID] = f
	}
	return m, nil
}

// resolveFolderRel 沿 parent_id 链向上回溯，拼出真实相对目录（不含文件名）。
func resolveFolderRel(folders map[int64]folderRow, folderID int64) string {
	if folderID == 0 {
		return ""
	}
	const maxDepth = 256
	var parts []string
	current := folderID
	for step := 0; step < maxDepth; step++ {
		f, ok := folders[current]
		if !ok {
			break
		}
		parts = append([]string{SanitizeFileName(f.Name)}, parts...)
		if f.ParentID == 0 {
			break
		}
		current = f.ParentID
	}
	return strings.Join(parts, "/")
}

// uniqueRelPath 若 KR 内已存在该相对路径，则追加 (n) 直到不冲突。
func uniqueRelPath(krDir, relPath string) string {
	full := filepath.Join(krDir, relPath)
	if _, err := os.Stat(full); os.IsNotExist(err) {
		return relPath
	}
	ext := filepath.Ext(relPath)
	base := strings.TrimSuffix(relPath, ext)
	for n := 1; n < 10000; n++ {
		candidate := fmt.Sprintf("%s(%d)%s", base, n, ext)
		if _, err := os.Stat(filepath.Join(krDir, candidate)); os.IsNotExist(err) {
			return candidate
		}
	}
	return relPath
}

// writeKRFile 在 KR 内写入文件，自动创建父目录。
func writeKRFile(krDir, relPath string, data []byte) error {
	full := filepath.Join(krDir, relPath)
	if dir := filepath.Dir(full); dir != krDir {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(full, data, 0644)
}
