package bootstrap

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"goteams-client"
	"goteams-client/internal/applog"
	"goteams-client/internal/localserver"
	builtinskills "goteams-client/internal/skills"
	"goteams-client/internal/taskruntime"
)

func (a *App) installBuiltinSkills() (bool, string, []localserver.VibeSkillStatus) {
	sourceFS, err := fs.Sub(assets.SkillsFS, "skills")
	if err != nil {
		applog.Error("打开内置 Skill 资源失败，Vibe Coding 功能不可用", "error", err)
		return false, builtinskills.CapabilityReasonInstallationFail, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		applog.Error("读取用户目录失败，Vibe Coding Skill 未安装", "error", err)
		return false, builtinskills.CapabilityReasonInstallationFail, nil
	}
	installed := map[string]struct {
		ready  bool
		reason string
	}{}
	statuses := make([]localserver.VibeSkillStatus, 0, len(taskruntime.VibeCodingTools()))
	for _, tool := range taskruntime.VibeCodingTools() {
		root := taskruntime.VibeSkillRoot(home, tool)
		result, ok := installed[root]
		if !ok {
			result.ready, result.reason = installSkillRoot(sourceFS, root)
			installed[root] = result
		}
		statuses = append(statuses, localserver.VibeSkillStatus{
			Tool: tool, Root: root, Ready: result.ready, Reason: result.reason,
		})
	}
	codexReady, codexReason := false, builtinskills.CapabilityReasonInstallationFail
	for _, status := range statuses {
		if status.Tool == taskruntime.ExecutionToolCodex {
			codexReady, codexReason = status.Ready, status.Reason
			break
		}
	}
	return codexReady, codexReason, statuses
}

func installSkillRoot(sourceFS fs.FS, root string) (bool, string) {
	if err := builtinskills.InstallBuiltin(sourceFS, root); err != nil {
		if errors.Is(err, builtinskills.ErrManagedUserChanged) {
			applog.Warn("跳过内置 Skill 更新：用户已修改 TeamsBoard 托管内容",
				"skill", builtinskills.TaskSkillName,
				"path", filepath.Join(root, builtinskills.TaskSkillName),
			)
			return true, ""
		}
		if errors.Is(err, builtinskills.ErrTargetNotManaged) {
			applog.Warn("跳过内置 Skill 安装：同名目录不由 TeamsBoard 管理",
				"skill", builtinskills.TaskSkillName,
				"path", filepath.Join(root, builtinskills.TaskSkillName),
			)
			return false, builtinskills.CapabilityReasonNotManaged
		}
		applog.Error("安装内置 Skill 失败",
			"skill", builtinskills.TaskSkillName,
			"path", filepath.Join(root, builtinskills.TaskSkillName),
			"error", err,
		)
		return false, builtinskills.CapabilityReasonInstallationFail
	}
	return true, ""
}
