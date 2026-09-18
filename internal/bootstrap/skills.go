package bootstrap

import (
	"errors"
	"io/fs"
	"path/filepath"

	"goteams-client"
	"goteams-client/internal/applog"
	builtinskills "goteams-client/internal/skills"
)

func (a *App) installBuiltinSkills() (bool, string) {
	sourceFS, err := fs.Sub(assets.SkillsFS, "skills")
	if err != nil {
		applog.Error("打开内置 Skill 资源失败，Codex 功能不可用", "error", err)
		return false, builtinskills.CapabilityReasonInstallationFail
	}
	if err := builtinskills.InstallBuiltin(sourceFS, a.skillsDir); err != nil {
		if errors.Is(err, builtinskills.ErrManagedUserChanged) {
			applog.Warn("跳过内置 Skill 更新：用户已修改 TeamsBoard 托管内容",
				"skill", builtinskills.TaskSkillName,
				"path", filepath.Join(a.skillsDir, builtinskills.TaskSkillName),
			)
			return true, ""
		}
		if errors.Is(err, builtinskills.ErrTargetNotManaged) {
			applog.Warn("跳过内置 Skill 安装：同名目录不由 TeamsBoard 管理",
				"skill", builtinskills.TaskSkillName,
				"path", filepath.Join(a.skillsDir, builtinskills.TaskSkillName),
			)
			return false, builtinskills.CapabilityReasonNotManaged
		}
		applog.Error("安装内置 Skill 失败，Codex 功能不可用",
			"skill", builtinskills.TaskSkillName,
			"path", filepath.Join(a.skillsDir, builtinskills.TaskSkillName),
			"error", err,
		)
		return false, builtinskills.CapabilityReasonInstallationFail
	}
	return true, ""
}
