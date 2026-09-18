package claude

import (
	"strings"

	"goteams-client/internal/executor"
)

const fullPermissionArg = "--dangerously-skip-permissions"
const defaultDisplayName = "Claude Code"

// NewAdapter 创建 Claude Code 适配器。
func NewAdapter() executor.Adapter {
	return NewStreamJSONAdapter(defaultDisplayName)
}

// NewStreamJSONAdapter 创建 stream-json 协议族适配器。
// codebuddy 与本 CLI 协议一致，直接复用本函数，只需传入自己的显示名。
func NewStreamJSONAdapter(displayName string) executor.Adapter {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = defaultDisplayName
	}
	return executor.NewStreamAdapter(executor.AdapterSpec{
		DisplayName: displayName,
		BuildInvocation: func(opts executor.RunOptions, resumeSession string) executor.Invocation {
			// Prompt 必须走 stdin：Windows 下 claude 通常由 claude.cmd 包装，
			// 多行 Prompt 放进命令行参数会被 cmd.exe 按换行与特殊字符重新解释。
			args := []string{"-p"}
			if resumeSession != "" {
				args = append(args, "--resume", resumeSession)
			}
			args = append(args, fullPermissionArg, "--verbose", "--output-format", "stream-json")
			if opts.ModelProfile != "" {
				args = append(args, "--model", opts.ModelProfile)
			}
			args = append(args, opts.ExtraArgs...)
			return executor.Invocation{Args: args, StdinText: opts.Prompt}
		},
		NewDecoder: executor.NewStreamJSONDecoder,
	})
}
