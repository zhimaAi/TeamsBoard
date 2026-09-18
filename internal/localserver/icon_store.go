package localserver

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"goteams-client/internal/i18n"
)

const (
	MaxIconFileSize      int64 = 2 * 1024 * 1024
	ManagedIconURLPrefix       = "/api/local/assets/icons/"
)

var (
	ErrIconTooLarge         = errors.New("图片大小不能超过 2MB")
	ErrUnsupportedIconType  = errors.New("仅支持 PNG、JPEG、WebP 图片")
	managedIconFilenameExpr = regexp.MustCompile(`^[0-9a-f-]{36}\.(png|jpg|webp)$`)
)

// IconStore owns user-selected icons in one managed directory. Business handlers
// only persist the returned URL and can reuse the same lifecycle operations.
type IconStore struct {
	root string
}

func NewIconStore(dataDir string) *IconStore {
	return &IconStore{root: filepath.Join(dataDir, "icon-assets")}
}

func (s *IconStore) Save(header *multipart.FileHeader) (string, error) {
	if header == nil {
		return "", ErrUnsupportedIconType
	}
	if header.Size > MaxIconFileSize {
		return "", ErrIconTooLarge
	}

	source, err := header.Open()
	if err != nil {
		return "", fmt.Errorf("读取图片失败: %w", err)
	}
	defer source.Close()

	data, err := io.ReadAll(io.LimitReader(source, MaxIconFileSize+1))
	if err != nil {
		return "", fmt.Errorf("读取图片失败: %w", err)
	}
	if int64(len(data)) > MaxIconFileSize {
		return "", ErrIconTooLarge
	}

	extension, ok := iconExtension(http.DetectContentType(data))
	if !ok {
		return "", ErrUnsupportedIconType
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return "", fmt.Errorf("创建图标目录失败: %w", err)
	}

	filename := uuid.NewString() + extension
	target := filepath.Join(s.root, filename)
	temporary, err := os.CreateTemp(s.root, ".icon-upload-*")
	if err != nil {
		return "", fmt.Errorf("创建图标临时文件失败: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return "", fmt.Errorf("设置图标文件权限失败: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return "", fmt.Errorf("保存图标失败: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("保存图标失败: %w", err)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return "", fmt.Errorf("提交图标文件失败: %w", err)
	}
	return ManagedIconURLPrefix + filename, nil
}

func (s *IconStore) Remove(rawURL string) error {
	filename, ok := managedIconFilename(rawURL)
	if !ok {
		return nil
	}
	if err := os.Remove(filepath.Join(s.root, filename)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("删除受管图标失败: %w", err)
	}
	return nil
}

func (s *IconStore) Serve(c *gin.Context) {
	filename := c.Param("filename")
	if !managedIconFilenameExpr.MatchString(filename) {
		i18n.Error(c, http.StatusNotFound, "localserver_icon_not_found", "icon_not_found")
		return
	}
	path := filepath.Join(s.root, filename)
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		i18n.Error(c, http.StatusNotFound, "localserver_icon_not_found", "icon_not_found")
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(path)
}

func iconExtension(contentType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "image/png":
		return ".png", true
	case "image/jpeg":
		return ".jpg", true
	case "image/webp":
		return ".webp", true
	default:
		return "", false
	}
}

func managedIconFilename(rawURL string) (string, bool) {
	if !strings.HasPrefix(rawURL, ManagedIconURLPrefix) {
		return "", false
	}
	filename := strings.TrimPrefix(rawURL, ManagedIconURLPrefix)
	if !managedIconFilenameExpr.MatchString(filename) {
		return "", false
	}
	return filename, true
}
