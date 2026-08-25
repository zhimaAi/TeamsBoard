package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound = errors.New("项目不存在")
	ErrInUse    = errors.New("项目已被任务使用")
)

type Project struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	IconType  string `json:"icon_type"`
	IconURL   string `json:"icon_url"`
	MainDir   string `json:"main_dir"`
	LocalDir  string `json:"local_dir"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type Input struct {
	Name     string `json:"name"`
	Icon     string `json:"icon"`
	IconType string `json:"icon_type"`
	IconURL  string `json:"icon_url"`
	MainDir  string `json:"main_dir"`
	LocalDir string `json:"local_dir"`
}

type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }

func (s *Service) List(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT uuid, name, icon, icon_type, icon_url, main_dir, main_dir, created_at, updated_at FROM gt_projects ORDER BY created_at, rowid`)
	if err != nil {
		return nil, fmt.Errorf("查询项目失败: %w", err)
	}
	defer rows.Close()
	items := make([]Project, 0)
	for rows.Next() {
		var item Project
		if err := rows.Scan(&item.UUID, &item.Name, &item.Icon, &item.IconType, &item.IconURL, &item.MainDir, &item.LocalDir, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Get(ctx context.Context, id string) (*Project, error) {
	var item Project
	err := s.db.QueryRowContext(ctx, `SELECT uuid, name, icon, icon_type, icon_url, main_dir, main_dir, created_at, updated_at FROM gt_projects WHERE uuid = ?`, id).
		Scan(&item.UUID, &item.Name, &item.Icon, &item.IconType, &item.IconURL, &item.MainDir, &item.LocalDir, &item.CreatedAt, &item.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) Create(ctx context.Context, input Input) (*Project, error) {
	name, mainDir, err := validateInput(input)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	now := time.Now().UnixMilli()
	iconType := strings.TrimSpace(input.IconType)
	if iconType == "" {
		iconType = strings.TrimSpace(input.Icon)
	}
	if iconType == "" {
		iconType = "folder"
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO gt_projects(uuid, name, icon, icon_type, icon_url, main_dir, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, iconType, iconType, strings.TrimSpace(input.IconURL), mainDir, now, now); err != nil {
		return nil, fmt.Errorf("创建项目失败: %w", err)
	}
	return s.Get(ctx, id)
}

func (s *Service) Update(ctx context.Context, id string, input Input) (*Project, error) {
	name, mainDir, err := validateInput(input)
	if err != nil {
		return nil, err
	}
	iconType := strings.TrimSpace(input.IconType)
	if iconType == "" {
		iconType = strings.TrimSpace(input.Icon)
	}
	if iconType == "" {
		iconType = "folder"
	}
	result, err := s.db.ExecContext(ctx, `UPDATE gt_projects SET name = ?, icon = ?, icon_type = ?, icon_url = ?, main_dir = ?, updated_at = ? WHERE uuid = ?`,
		name, iconType, iconType, strings.TrimSpace(input.IconURL), mainDir, time.Now().UnixMilli(), id)
	if err != nil {
		return nil, fmt.Errorf("更新项目失败: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return nil, ErrNotFound
	}
	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	var links int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gt_task_project_links WHERE project_uuid = ?`, id).Scan(&links); err != nil {
		return err
	}
	if links > 0 {
		return ErrInUse
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM gt_projects WHERE uuid = ?`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func validateInput(input Input) (string, string, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return "", "", fmt.Errorf("项目名称不能为空")
	}
	raw := strings.TrimSpace(input.MainDir)
	if raw == "" {
		raw = strings.TrimSpace(input.LocalDir)
	}
	if raw == "" {
		return "", "", fmt.Errorf("项目主目录不能为空")
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", "", fmt.Errorf("项目主目录无效: %w", err)
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", "", fmt.Errorf("项目主目录不存在或不是目录: %s", abs)
	}
	if runtime.GOOS == "windows" {
		// Store one canonical spelling so the unique index also catches common
		// case-only duplicates on Windows.
		abs = strings.ToLower(abs)
	}
	return name, abs, nil
}
