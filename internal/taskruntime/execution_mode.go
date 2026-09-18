package taskruntime

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ExecutionModePipeline    = "pipeline"
	ExecutionModeExpertGroup = "expert_group"
	ExecutionModeVibeCoding  = "vibe_coding"
	ExecutionModeCLI         = "cli"
	ExecutionToolCodex       = "codex"

	// CLIDirectStepKey 是 CLI 直接执行模式的隐式步骤标识。该模式没有 Agent
	// 编排，但仍复用 gt_task_steps 承载用户选定的 CLI 与模型，使会话、动态、
	// 停止和断线恢复沿用同一条链路。
	CLIDirectStepKey = "cli-direct"
	// CLIDirectStepName 是隐式步骤的展示名，前端在 CLI 模式下会用任务实际
	// 选定的 CLI 与模型覆盖它。
	CLIDirectStepName = "CLI 直接执行"
)

var (
	ErrExecutionModeConflict    = errors.New("任务已经指派其他执行方式")
	ErrExecutionModeUnsupported = errors.New("该执行方式暂未支持")
	ErrExpertRoutingPending     = errors.New("专家团自动路由处理中")
)

func NormalizeExecutionMode(mode string) string {
	return strings.ToLower(strings.TrimSpace(mode))
}

func ValidateExecutionAssignment(mode, tool, pipelineUUID string) error {
	return ValidateExecutionTarget(mode, tool, pipelineUUID, "")
}

func ValidateExecutionTarget(mode, tool, pipelineUUID, expertGroupUUID string) error {
	mode = NormalizeExecutionMode(mode)
	tool = strings.ToLower(strings.TrimSpace(tool))
	pipelineUUID = strings.TrimSpace(pipelineUUID)
	expertGroupUUID = strings.TrimSpace(expertGroupUUID)
	switch mode {
	case "":
		if tool != "" || pipelineUUID != "" || expertGroupUUID != "" {
			return fmt.Errorf("执行方式未选择")
		}
		return nil
	case ExecutionModePipeline:
		if tool != "" || expertGroupUUID != "" {
			return fmt.Errorf("流水线执行不能指定编程工具")
		}
		return nil
	case ExecutionModeVibeCoding:
		if pipelineUUID != "" || expertGroupUUID != "" {
			return fmt.Errorf("Vibe Coding 执行不能指定流水线")
		}
		if tool != ExecutionToolCodex {
			return fmt.Errorf("Vibe Coding 当前仅支持 Codex")
		}
		return nil
	case ExecutionModeExpertGroup:
		if tool != "" || pipelineUUID != "" {
			return fmt.Errorf("专家团执行不能指定流水线或编程工具")
		}
		if expertGroupUUID == "" {
			return fmt.Errorf("专家团不能为空")
		}
		return nil
	case ExecutionModeCLI:
		if pipelineUUID != "" || expertGroupUUID != "" {
			return fmt.Errorf("CLI 执行不能指定流水线或专家团")
		}
		if tool == "" {
			return fmt.Errorf("CLI 不能为空")
		}
		return nil
	default:
		return fmt.Errorf("无效任务执行方式")
	}
}

// ValidateCLITarget 校验 CLI 直接执行方式的目标。该模式没有 Agent 编排，
// CLI 与模型都是必填项，缺一不可执行。
func ValidateCLITarget(cliType, modelName string) error {
	if strings.ToLower(strings.TrimSpace(cliType)) == "" {
		return fmt.Errorf("CLI 不能为空")
	}
	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("模型不能为空")
	}
	return nil
}
