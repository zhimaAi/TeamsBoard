package bootstrap

import (
	"fmt"
	"io/fs"
	"path/filepath"

	"goteams-client"
	builtinskills "goteams-client/internal/skills"
)

func (a *App) installBuiltinSkills() error {
	// Built-in Skills are compiled into embedded binaries and read from the embedded file system at runtime.
	sourceFS, err := fs.Sub(assets.SkillsFS, "skills")
	if err != nil {
		return fmt.Errorf("打开嵌入的内置 skills 失败: %w", err)
	}
	targetRoot := filepath.Join(filepath.Dir(a.dataDir), "skills")
	return builtinskills.InstallBuiltin(sourceFS, targetRoot)
}
