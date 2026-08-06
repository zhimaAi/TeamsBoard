package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var deprecatedBuiltinNames = []string{
	"goteams-git",
	"goteams-docker",
	"goteams-know",
	"goteams-common",
}

// deprecatedCleanupVersion is the cleanup batch number for deprecated built-in Skills.
// Incremented each time a new directory is appended to deprecatedBuiltinNames, triggering a new cleanup round.
const deprecatedCleanupVersion = "1"

// cleanupStateFile records completed cleanup batches to avoid unconditionally deleting directories on every startup.
const cleanupStateFile = ".builtin-cleanup"

// InstallBuiltin syncs built-in Skills from the compile-time embedded filesystem to the user directory.
// sourceFS should be the embedded skills directory (e.g. fs.Sub(assets.SkillsFS, "skills")).
// Only directories declared in BuiltinNames are synced, to avoid overwriting user-created Skills.
func InstallBuiltin(sourceFS fs.FS, targetRoot string) error {
	if err := cleanupDeprecatedBuiltins(targetRoot); err != nil {
		return err
	}

	for _, name := range BuiltinNames() {
		if info, err := fs.Stat(sourceFS, name); err != nil {
			return fmt.Errorf("内置 Skill 缺失: %s", name)
		} else if !info.IsDir() {
			return fmt.Errorf("内置 Skill 不是目录: %s", name)
		}

		err := fs.WalkDir(sourceFS, name, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(name, path)
			if err != nil {
				return err
			}
			target := filepath.Join(targetRoot, name, rel)
			if entry.IsDir() {
				return os.MkdirAll(target, 0700)
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			data, err := fs.ReadFile(sourceFS, path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, 0600)
		})
		if err != nil {
			return fmt.Errorf("安装内置 Skill %s 失败: %w", name, err)
		}
	}
	return nil
}

// cleanupDeprecatedBuiltins removes built-in Skills installed by old versions, executed once per batch number.
// Directory names are fixed and targets are confined to the skills root; after cleanup a state file is written,
// so subsequent startups skip it, avoiding an unconditional RemoveAll on the user directory every time.
func cleanupDeprecatedBuiltins(targetRoot string) error {
	statePath := filepath.Join(targetRoot, cleanupStateFile)
	if data, err := os.ReadFile(statePath); err == nil &&
		strings.TrimSpace(string(data)) == deprecatedCleanupVersion {
		return nil
	}

	for _, name := range deprecatedBuiltinNames {
		if err := os.RemoveAll(filepath.Join(targetRoot, name)); err != nil {
			return fmt.Errorf("清理废弃内置 Skill %s 失败: %w", name, err)
		}
	}

	if err := os.MkdirAll(targetRoot, 0700); err != nil {
		return fmt.Errorf("创建 Skill 根目录失败: %w", err)
	}
	if err := os.WriteFile(statePath, []byte(deprecatedCleanupVersion), 0600); err != nil {
		return fmt.Errorf("记录内置 Skill 清理状态失败: %w", err)
	}
	return nil
}
