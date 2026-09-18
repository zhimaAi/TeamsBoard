package workflow

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

// BuildTaskPrompt builds the complete prompt for one CLI execution round.
// Product history deliberately stays in the database and is not
// injected; the CLI receives only the immutable step instruction, the current
// question, and the task's directory contract.
func BuildTaskPrompt(ctx context.Context, db *sql.DB, taskUUID, stepUUID, question string) (string, error) {
	var title, taskDir, taskMDPath, currentName, currentPrompt, currentStepDir string
	err := db.QueryRowContext(ctx, `SELECT t.title, t.task_dir, t.task_md_path, s.name, s.prompt_snapshot, s.step_dir
		FROM gt_tasks t JOIN gt_task_steps s ON s.task_uuid = t.uuid
		WHERE t.uuid = ? AND s.uuid = ?`, taskUUID, stepUUID).Scan(
		&title, &taskDir, &taskMDPath, &currentName, &currentPrompt, &currentStepDir)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("任务或 Agent 编排不存在")
	}
	if err != nil {
		return "", fmt.Errorf("读取任务执行上下文失败: %w", err)
	}

	workRows, err := db.QueryContext(ctx, `SELECT path FROM gt_task_work_dirs WHERE task_uuid = ? ORDER BY sort_order, id`, taskUUID)
	if err != nil {
		return "", fmt.Errorf("读取项目目录失败: %w", err)
	}
	defer workRows.Close()
	workDirs := make([]string, 0)
	for workRows.Next() {
		var path string
		if err := workRows.Scan(&path); err != nil {
			return "", err
		}
		workDirs = append(workDirs, path)
	}
	if err := workRows.Err(); err != nil {
		return "", err
	}

	stepRows, err := db.QueryContext(ctx, `SELECT uuid, name, step_dir FROM gt_task_steps WHERE task_uuid = ? ORDER BY step_order, rowid`, taskUUID)
	if err != nil {
		return "", fmt.Errorf("读取 Agent 编排目录失败: %w", err)
	}
	defer stepRows.Close()
	type stepDirItem struct{ id, name, path string }
	stepDirs := make([]stepDirItem, 0)
	for stepRows.Next() {
		var item stepDirItem
		if err := stepRows.Scan(&item.id, &item.name, &item.path); err != nil {
			return "", err
		}
		stepDirs = append(stepDirs, item)
	}
	if err := stepRows.Err(); err != nil {
		return "", err
	}

	beforeDirs := make([]stepDirItem, 0)
	afterDirs := make([]stepDirItem, 0)
	currentFound := false
	for _, item := range stepDirs {
		if item.id == stepUUID {
			currentFound = true
			continue
		}
		if currentFound {
			afterDirs = append(afterDirs, item)
		} else {
			beforeDirs = append(beforeDirs, item)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "你正在执行本地任务「%s」的 Agent 编排步骤「%s」。\n\n", title, currentName)
	b.WriteString("请先读取文档，以及前面步骤的产出列表后再开始本次任务！\n\n")

	b.WriteString("## 执行前必读（必须按顺序执行）\n\n")
	fmt.Fprintf(&b, "1. 首先读取任务原始需求文件：%s。\n", taskMDPath)
	if len(beforeDirs) > 0 {
		b.WriteString("2. 然后按执行顺序，逐一读取此前已执行步骤目录中的全部产出文档（如需求设计文档、方案文档、分析结论等）。这些文档是本步骤开发的权威需求与设计来源，必须完整阅读、不得遗漏，不得用「未提供具体需求」等理由拒绝执行或反问。\n")
		b.WriteString("   此前步骤目录（已执行，必读其全部产出）：\n")
		for _, item := range beforeDirs {
			fmt.Fprintf(&b, "   - %s：%s\n", item.name, item.path)
		}
	} else {
		b.WriteString("2. 本步骤是该任务的第一个执行步骤，除 task.md 外没有此前步骤产出。\n")
	}
	b.WriteString("3. 阅读完上述文档后再开始执行本步骤。\n\n")

	b.WriteString("## 本步骤要求\n\n")
	b.WriteString(strings.TrimSpace(currentPrompt))
	b.WriteString("\n\n## 任务数据目录\n\n")
	fmt.Fprintf(&b, "- 任务专属目录：%s\n- 需求文件：%s\n- 用户附件目录：%s\n", taskDir, taskMDPath, filepath.Join(taskDir, "attachments"))
	b.WriteString("- task.md 中的 attachments/... 是相对任务专属目录的附件引用；请按它在需求中出现的顺序和上下文读取。\n")
	b.WriteString("- Agent 编排目录（按执行顺序）：\n")
	for _, item := range stepDirs {
		class := "后续步骤（尚未执行）"
		if item.id == stepUUID {
			class = "当前步骤"
		} else {
			for _, bd := range beforeDirs {
				if bd.id == item.id {
					class = "此前步骤（已执行）"
					break
				}
			}
		}
		fmt.Fprintf(&b, "  - %s（%s）：%s\n", item.name, class, item.path)
	}
	b.WriteString("\n## 项目目录\n\n")
	for i, path := range workDirs {
		label := "子项目目录"
		if i == 0 {
			label = "主项目目录"
		}
		fmt.Fprintf(&b, "- %s：%s\n", label, path)
	}
	fmt.Fprintf(&b, "\n所有新产生的中间产物只能写入当前步骤专属目录：%s。此前步骤目录只读（必须读取、不得修改）；后续步骤目录尚未执行，禁止读取或写入。\n", currentStepDir)
	if strings.TrimSpace(question) != "" {
		b.WriteString("\n## 用户本次问题\n\n")
		b.WriteString(strings.TrimSpace(question))
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// BuildRoundPrompt selects the prompt builder for the task's execution mode.
// Pipeline and Vibe Coding tasks carry Agent orchestration steps; expert group
// tasks carry a frozen roster; CLI direct execution carries neither.
func BuildRoundPrompt(ctx context.Context, db *sql.DB, executionMode, taskUUID, stepUUID, question string) (string, error) {
	switch executionMode {
	case "expert_group":
		return BuildExpertTaskPrompt(ctx, db, taskUUID, stepUUID, question)
	case "cli":
		return BuildCLIPrompt(ctx, db, taskUUID, question)
	default:
		return BuildTaskPrompt(ctx, db, taskUUID, stepUUID, question)
	}
}

// BuildCLIPrompt builds the prompt for one direct-CLI execution round. The mode
// skips both the pipeline and the Agent orchestration, so the CLI receives only
// the task document contract, the project directories and the user's own
// instruction — without step instructions, previous step outputs or step output
// directories.
func BuildCLIPrompt(ctx context.Context, db *sql.DB, taskUUID, question string) (string, error) {
	var title, taskDir, taskMDPath string
	err := db.QueryRowContext(ctx, `SELECT title, task_dir, task_md_path FROM gt_tasks WHERE uuid = ?`,
		taskUUID).Scan(&title, &taskDir, &taskMDPath)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("任务不存在")
	}
	if err != nil {
		return "", fmt.Errorf("读取任务执行上下文失败: %w", err)
	}

	workRows, err := db.QueryContext(ctx, `SELECT path FROM gt_task_work_dirs WHERE task_uuid = ? ORDER BY sort_order, id`, taskUUID)
	if err != nil {
		return "", fmt.Errorf("读取项目目录失败: %w", err)
	}
	defer workRows.Close()
	workDirs := make([]string, 0)
	for workRows.Next() {
		var path string
		if err := workRows.Scan(&path); err != nil {
			return "", err
		}
		workDirs = append(workDirs, path)
	}
	if err := workRows.Err(); err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "你正在直接执行本地任务「%s」。\n\n", title)
	b.WriteString("## 任务数据目录\n\n")
	fmt.Fprintf(&b, "- 任务专属目录：%s\n- 需求文件：%s\n- 用户附件目录：%s\n", taskDir, taskMDPath, filepath.Join(taskDir, "attachments"))
	b.WriteString("- task.md 中的 attachments/... 是相对任务专属目录的附件引用；请按它在需求中出现的顺序和上下文读取。\n")
	b.WriteString("\n## 项目目录\n\n")
	for i, path := range workDirs {
		label := "子项目目录"
		if i == 0 {
			label = "主项目目录"
		}
		fmt.Fprintf(&b, "- %s：%s\n", label, path)
	}
	if strings.TrimSpace(question) != "" {
		b.WriteString("\n## 用户本次指令\n\n")
		b.WriteString(strings.TrimSpace(question))
		b.WriteByte('\n')
	}
	return b.String(), nil
}
