package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Store file system storage
type Store struct {
	contentDir string
}

// NewStore creates a file storage instance and ensures that the directory exists
func NewStore(contentDir string) (*Store, error) {
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		return nil, fmt.Errorf("创建内容目录失败: %w", err)
	}
	historyDir := filepath.Join(contentDir, "..", "history")
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		return nil, fmt.Errorf("创建历史目录失败: %w", err)
	}
	return &Store{contentDir: contentDir}, nil
}

// contentPath returns the document content file path
func (s *Store) contentPath(uuid string) string {
	return filepath.Join(s.contentDir, uuid+".md")
}

// historyPath returns the historical version file path
func (s *Store) historyPath(uuid string, historyID int64) string {
	historyDir := filepath.Join(s.contentDir, "..", "history")
	return filepath.Join(historyDir, uuid+"_"+strconv.FormatInt(historyID, 10)+".md")
}

// WriteFile writes Markdown file
func (s *Store) WriteFile(uuid, content string) error {
	return os.WriteFile(s.contentPath(uuid), []byte(content), 0644)
}

// ReadFile reads Markdown file
func (s *Store) ReadFile(uuid string) (string, error) {
	data, err := os.ReadFile(s.contentPath(uuid))
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}
	return string(data), nil
}

// DeleteFile deletes the file
func (s *Store) DeleteFile(uuid string) error {
	err := os.Remove(s.contentPath(uuid))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除文件失败: %w", err)
	}
	return nil
}

// DeleteHistoryFiles deletes all historical version files of the document.
func (s *Store) DeleteHistoryFiles(uuid string) error {
	entries, err := os.ReadDir(filepath.Join(s.contentDir, "..", "history"))
	if err != nil {
		return fmt.Errorf("读取历史目录失败: %w", err)
	}
	prefix := uuid + "_"
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(s.contentDir, "..", "history", entry.Name())
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除历史文件失败: %w", err)
		}
	}
	return nil
}

// WriteHistoryFile writes the historical version file
func (s *Store) WriteHistoryFile(uuid string, historyID int64, content string) error {
	return os.WriteFile(s.historyPath(uuid, historyID), []byte(content), 0644)
}

// ReadHistoryFile reads historical version files
func (s *Store) ReadHistoryFile(uuid string, historyID int64) (string, error) {
	data, err := os.ReadFile(s.historyPath(uuid, historyID))
	if err != nil {
		return "", fmt.Errorf("读取历史文件失败: %w", err)
	}
	return string(data), nil
}
