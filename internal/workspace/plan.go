package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"goteams-client/internal/applog"
)

// IsDesktopRuntime 判断当前进程是否运行在 Electron 桌面宿主内。
// 判定顺序：
//  1. GOTEAMS_DESKTOP_TOKEN 环境变量（打包桌面端总会注入）；
//  2. 向上走父进程链，命中 electron 进程名（开发模式 npm start 形态）。
//
// 浏览器模式（go run ./cmd/client）两者都不命中，返回 false。
func IsDesktopRuntime() bool {
	if strings.TrimSpace(os.Getenv("GOTEAMS_DESKTOP_TOKEN")) != "" {
		return true
	}
	pid := os.Getpid()
	for i := 0; i < 6; i++ {
		ppid := parentPIDOf(pid)
		if ppid <= 0 || ppid == pid {
			return false
		}
		if isElectronProcessName(processName(ppid)) {
			return true
		}
		pid = ppid
	}
	return false
}

// WorkspacePlan 描述一次工作空间重定位的预览信息，供前端确认提示。
type WorkspacePlan struct {
	CurrentRoot   string `json:"current_root"`   // 当前生效数据根
	TargetRoot    string `json:"target_root"`    // 搬迁后的数据根（<parent>/.goteams）
	TargetParent  string `json:"target_parent"`  // 用户选择的父目录
	CrossVolume   bool   `json:"cross_volume"`   // 是否跨卷（跨卷搬迁更耗时，前端需强提示）
	RelocatedFrom string `json:"relocated_from"` // 当前已处于重定位状态时的原父目录（未重定位为空）
}

// PlanWorkspace 校验并生成搬迁计划；所有校验失败都以明确错误返回。
//
// 校验规则（与产品确认）：
//   - 目标父目录必须存在且为空（除 .goteams 外没有任何条目，且 .goteams 不允许存在）；
//   - 目标父目录不得是当前数据根或其子目录（防止把数据搬进自己里面）；
//   - 目标父目录不得是系统关键目录（Windows: 盘符根目录、Windows、Program Files 等）；
//   - 目标父目录必须可写；
//   - 不能与当前数据根相同。
func PlanWorkspace(targetParent string) (*WorkspacePlan, error) {
	targetParent = strings.TrimSpace(targetParent)
	if targetParent == "" {
		return nil, fmt.Errorf("目标目录不能为空")
	}
	abs, err := filepath.Abs(targetParent)
	if err != nil {
		return nil, fmt.Errorf("目标目录无效: %w", err)
	}
	targetParent = filepath.Clean(abs)

	currentRoot, err := ResolveDataRoot()
	if err != nil {
		return nil, err
	}
	defaultRoot := DefaultDataRoot()

	info, err := os.Stat(targetParent)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("目标目录 %s 不存在，请先创建", targetParent)
		}
		return nil, fmt.Errorf("读取目标目录失败: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("目标路径 %s 不是目录", targetParent)
	}

	// 目标父目录下不允许已经存在 .goteams（产品确认：禁止合并，避免覆盖旧数据）。
	newRoot := filepath.Join(targetParent, dataRootDirName)
	if _, err := os.Stat(newRoot); err == nil {
		return nil, ErrWorkspaceConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("检查目标目录失败: %w", err)
	}

	// 目标父目录必须是空目录（产品确认：禁止选择非空目标，保证搬迁边界清晰）。
	if n, err := countEntries(targetParent); err != nil {
		return nil, fmt.Errorf("读取目标目录失败: %w", err)
	} else if n > 0 {
		return nil, fmt.Errorf("目标目录 %s 不为空，请选择空目录", targetParent)
	}

	// 不能把数据搬进自己里面，也不能把目标指向当前根的祖先（搬迁会中断自身）。
	if samePath(targetParent, currentRoot) || samePath(newRoot, currentRoot) {
		return nil, fmt.Errorf("目标目录与当前工作空间相同，无需搬迁")
	}
	if isPathWithin(targetParent, currentRoot) {
		return nil, fmt.Errorf("目标目录不能位于当前工作空间之内")
	}

	if err := validateNotSystemPath(targetParent); err != nil {
		return nil, err
	}

	// 可写性探测：创建并删除一个临时文件。
	probe, err := os.CreateTemp(targetParent, ".goteams-probe-*")
	if err != nil {
		return nil, fmt.Errorf("目标目录不可写: %w", err)
	}
	probeName := probe.Name()
	probe.Close()
	_ = os.Remove(probeName)

	return &WorkspacePlan{
		CurrentRoot:   currentRoot,
		TargetRoot:    newRoot,
		TargetParent:  targetParent,
		CrossVolume:   !sameVol(currentRoot, targetParent),
		RelocatedFrom: relocatedFrom(currentRoot, defaultRoot),
	}, nil
}

// ApplyWorkspace 执行搬迁：把当前数据根搬到计划目标，并立即在 HOME 下写入
// 指针文件。指针写入成功后本次搬迁即生效（下次启动解析到新根）；之后的
// 默认根收尾（重建最小结构 + 结构标记）失败只记日志，不影响搬迁结果。
func ApplyWorkspace(plan *WorkspacePlan) (string, error) {
	if plan == nil {
		return "", fmt.Errorf("搬迁计划为空")
	}
	// 执行前重新校验目标仍为空（从计划生成到执行之间用户可能放入了文件）。
	if _, err := os.Stat(plan.TargetRoot); err == nil {
		return "", ErrWorkspaceConflict
	}
	if n, err := countEntries(plan.TargetParent); err != nil {
		return "", fmt.Errorf("读取目标目录失败: %w", err)
	} else if n > 0 {
		return "", fmt.Errorf("目标目录 %s 在执行前被写入了内容，搬迁中止", plan.TargetParent)
	}

	newRoot, err := relocateDataRoot(plan.CurrentRoot, plan.TargetParent)
	if err != nil {
		return "", err
	}

	// 结构标记优先于指针：保证指针一旦写成功，ResolveDataRoot 的完整性校验
	// 必然通过。源根本身带标记，rename/复制会把它带过来，这里是双保险。
	if err := MarkDataRootStructure(newRoot); err != nil {
		return "", err
	}

	// 立即写指针。搬迁完成与指针写入之间是唯一的极端丢失窗口：
	// 此时数据完整保留在新目录，用户可按启动日志指引手工重建指针。
	if err := os.WriteFile(LinkFilePath(), []byte(newRoot), 0o600); err != nil {
		applog.Error("工作空间已搬迁但写入指针失败，请手工在指针文件中记录新目录",
			"new_root", newRoot, "link", LinkFilePath(), "error", err.Error())
		return "", fmt.Errorf("写入工作空间指针失败: %w", err)
	}

	// 收尾：重建默认根最小结构，作为「该用户目录曾使用过 GoTeams」的锚点，
	// 避免换盘符后 ResolveDataRoot 把空壳默认根误判为可回退的完整数据。
	defaultRoot := DefaultDataRoot()
	if err := os.MkdirAll(defaultRoot, 0o755); err != nil {
		applog.Warn("重建默认工作空间目录失败（不影响搬迁）", "dir", defaultRoot, "error", err.Error())
	} else if err := MarkDataRootStructure(defaultRoot); err != nil {
		applog.Warn("写入默认工作空间结构标记失败（不影响搬迁）", "dir", defaultRoot, "error", err.Error())
	}

	return newRoot, nil
}

// isPathWithin 判断 p 是否位于 ancestor 之内（不含相等）。
func isPathWithin(p, ancestor string) bool {
	p = filepath.Clean(p)
	ancestor = filepath.Clean(ancestor)
	if samePath(p, ancestor) {
		return false
	}
	rel, err := filepath.Rel(ancestor, p)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// relocatedFrom 返回当前重定位状态下"原父目录"（用于前端展示从哪搬走）。
// 未重定位时返回空串。
func relocatedFrom(currentRoot, defaultRoot string) string {
	if samePath(currentRoot, defaultRoot) {
		return ""
	}
	return filepath.Dir(currentRoot)
}

// validateNotSystemPath 拒绝把数据根搬到系统关键位置。
func validateNotSystemPath(p string) error {
	// 盘符根目录（C:\、D:\）过浅，不允许。
	if pathDepth(p) <= 1 {
		return fmt.Errorf("目标目录不能是磁盘根目录，请新建一个子目录")
	}
	upper := strings.ToUpper(p)
	forbidden := []string{`\WINDOWS`, `\PROGRAM FILES`, `\PROGRAM FILES (X86)`, `\PROGRAMDATA`}
	for _, f := range forbidden {
		if strings.Contains(upper, f+string(filepath.Separator)) || strings.HasSuffix(upper, f) {
			return fmt.Errorf("目标目录不能位于系统目录 %s 之下", f)
		}
	}
	return nil
}
