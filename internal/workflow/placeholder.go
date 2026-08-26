package workflow

import (
	"fmt"
	"strings"
)

const workDirContextMarker = "[GoTeams 工作目录上下文]"

// InjectWorkDirPromptContext writes the task's primary and related directories into each step's prompt snapshot.
// The CLI process uses the first directory as its current directory; the rest are used via absolute paths for cross-repo tasks.
func InjectWorkDirPromptContext(prompt string, workDirs []string) string {
	if strings.Contains(prompt, workDirContextMarker) {
		return prompt
	}

	normalized := make([]string, 0, len(workDirs))
	seen := make(map[string]struct{})
	for _, workDir := range workDirs {
		workDir = strings.TrimSpace(workDir)
		if workDir == "" {
			continue
		}
		if _, exists := seen[workDir]; exists {
			continue
		}
		seen[workDir] = struct{}{}
		normalized = append(normalized, workDir)
	}
	if len(normalized) == 0 {
		return prompt
	}

	lines := []string{
		workDirContextMarker,
		"以下目录均属于本任务；CLI 当前目录为主目录，涉及其他代码库时请使用对应绝对路径。",
		"主目录（CLI 当前目录）: " + normalized[0],
	}
	for index, workDir := range normalized[1:] {
		lines = append(lines, fmt.Sprintf("关联目录 %d: %s", index+1, workDir))
	}
	return strings.Join(lines, "\n") + "\n\n" + prompt
}
