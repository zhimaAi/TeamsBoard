package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// maxSearchFileBytes 超过此大小的真实文件在索引/搜索时只记录路径、不预读内容（DA-05 验收③）
const maxSearchFileBytes int64 = 5 * 1024 * 1024

// Store 真实目录映射存储层。
// KR（知识库根目录）为磁盘真实目录：文档正文位于 <KR>/<file_path>，
// file_path 为相对 KR 的真实相对路径（含文件名与扩展名，如 子文件夹/我的文档.md）。
// 回收站真实文件落在 <KR>/.trash，历史版本落在 <KR>/.history。
type Store struct {
	krDir string
}

// NewStore 创建文件存储实例并确保 KR 及 .trash/.history 目录存在。
func NewStore(krDir string) (*Store, error) {
	if err := os.MkdirAll(krDir, 0755); err != nil {
		return nil, fmt.Errorf("创建知识库根目录失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(krDir, ".trash"), 0755); err != nil {
		return nil, fmt.Errorf("创建回收站目录失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(krDir, ".history"), 0755); err != nil {
		return nil, fmt.Errorf("创建历史目录失败: %w", err)
	}
	return &Store{krDir: krDir}, nil
}

// KRDir 返回知识库根目录。
func (s *Store) KRDir() string {
	return s.krDir
}

// DocPath 把相对 KR 的路径安全地拼接在 KR 之内（防止 ../ 越界）。
func (s *Store) DocPath(relPath string) (string, error) {
	// S-DA-02: 显式拒绝包含 .. 的相对片段，避免任何上溯逃逸
	if strings.Contains(relPath, "..") {
		return "", fmt.Errorf("路径包含非法相对片段: %s", relPath)
	}
	clean := filepath.Clean("/" + relPath) // 规范化，去掉 ../ 并去掉首尾分隔符
	full := filepath.Join(s.krDir, clean)
	if full != s.krDir && !strings.HasPrefix(full, s.krDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径越界: %s", relPath)
	}
	return full, nil
}

// WriteFile 写 <KR>/<relPath>。
func (s *Store) WriteFile(relPath, content string) error {
	full, err := s.DocPath(relPath)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(full); dir != s.krDir {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建父目录失败: %w", err)
		}
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}
	return nil
}

// ReadFile 读 <KR>/<relPath>。
func (s *Store) ReadFile(relPath string) (string, error) {
	full, err := s.DocPath(relPath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w 链接: %s", err, full)
	}
	return string(data), nil
}

// FileSize 返回 <KR>/<relPath> 的字节大小；文件不存在返回错误。
func (s *Store) FileSize(relPath string) (int64, error) {
	full, err := s.DocPath(relPath)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// DeleteFile 删除 <KR>/<relPath>；文件不存在时忽略。
func (s *Store) DeleteFile(relPath string) error {
	full, err := s.DocPath(relPath)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除文件失败: %w", err)
	}
	return nil
}

// MoveFile 把真实文件从 srcRel 重命名/移动到 dstRel（DA-04 恢复/重命名）。
func (s *Store) MoveFile(srcRel, dstRel string) error {
	src, err := s.DocPath(srcRel)
	if err != nil {
		return err
	}
	dst, err := s.DocPath(dstRel)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(dst); dir != s.krDir {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建父目录失败: %w", err)
		}
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("移动文件失败: %w", err)
	}
	return nil
}

// MoveToTrash 把 <KR>/<relPath> 移入回收站，返回回收站内相对路径（DA-04）。
// 回收站真实文件名形如 <时间戳>_<原名>。
func (s *Store) MoveToTrash(relPath string) (string, error) {
	name := filepath.Base(relPath)
	trashRel := fmt.Sprintf(".trash/%d_%s", time.Now().UnixNano(), name)
	if err := s.MoveFile(relPath, trashRel); err != nil {
		return "", err
	}
	return trashRel, nil
}

// RestoreFromTrash 把回收站文件移回原 <KR>/<relPath>（文件夹缺失则重建目录，DA-04）。
func (s *Store) RestoreFromTrash(trashRel, relPath string) error {
	return s.MoveFile(trashRel, relPath)
}

// FindInTrash 在回收站中按原名查找最新（时间戳最大）的匹配项，返回其相对路径。
// 用于「文档已软删、真实文件已在 .trash」的恢复/永久删除场景。
func (s *Store) FindInTrash(name string) (string, error) {
	trashDir := filepath.Join(s.krDir, ".trash")
	entries, err := os.ReadDir(trashDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("读取回收站失败: %w", err)
	}
	suffix := "_" + name
	var best string
	var bestTS int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, suffix) {
			continue
		}
		// 文件名形如 <ts>_<name>，提取前导时间戳进行比较
		tsPart := strings.TrimSuffix(n[:len(n)-len(suffix)], "_")
		var ts int64
		if v, cerr := strconv.ParseInt(tsPart, 10, 64); cerr == nil {
			ts = v
		}
		if ts >= bestTS {
			bestTS = ts
			best = ".trash/" + n
		}
	}
	return best, nil
}

// HistoryPath 返回历史版本相对路径（<KR>/.history/<uuid>_<id>.md）。
func (s *Store) HistoryPath(uuid string, historyID int64) string {
	return fmt.Sprintf(".history/%s_%d.md", uuid, historyID)
}

// WriteHistoryFile 写历史版本文件。
func (s *Store) WriteHistoryFile(uuid string, historyID int64, content string) error {
	return s.WriteFile(s.HistoryPath(uuid, historyID), content)
}

// ReadHistoryFile 读历史版本文件。
func (s *Store) ReadHistoryFile(uuid string, historyID int64) (string, error) {
	return s.ReadFile(s.HistoryPath(uuid, historyID))
}

// DeleteHistoryFiles 删除某文档的全部历史版本文件。
func (s *Store) DeleteHistoryFiles(uuid string) error {
	historyDir := filepath.Join(s.krDir, ".history")
	entries, err := os.ReadDir(historyDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取历史目录失败: %w", err)
	}
	prefix := uuid + "_"
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(historyDir, entry.Name())
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除历史文件失败: %w", err)
		}
	}
	return nil
}

// SanitizeFileName 把标题安全地转义为文件名（BE-02 验收②）。
// 非法字符（/ \ : * ? " < > | 及控制字符、首尾空白/点）被替换为下划线。
func SanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		if unicode.IsControl(r) ||
			r == '/' || r == '\\' || r == ':' || r == '*' ||
			r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			b.WriteRune('_')
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	out = strings.Trim(out, ". ")
	if out == "" {
		out = "未命名"
	}
	return out
}
