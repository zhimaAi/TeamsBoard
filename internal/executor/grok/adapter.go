// Package grok provides Grok CLI execution adaptation.
//
// TODO(grok-acp): 当前实现适配的是社区版 `@vibe-kit/grok-cli`
// （npm i -g @vibe-kit/grok-cli，bin 名同为 `grok`），命令行形态为
// `grok --format json --no-sandbox [-s <session>] -p <prompt>`。
//
// xAI 官方 Grok Build（`curl -fsSL https://x.ai/cli/install.sh | bash`，
// 当前稳定版 1.0.34）**不兼容这套参数**，其真实能力为：
//   - 顶层 `grok --output-format plain|json|streaming-json|streaming-messages-json`，
//     但顶层命令强制要求 TTY，PTY 下会进入 TUI 渲染、输出格式不生效；
//   - 程序化集成的官方路径是 `grok agent stdio`：标准 ACP（Agent Client Protocol）
//     over stdio 的 JSON-RPC 2.0 + NDJSON，无需 TTY。握手流程为
//     initialize → session/new → session/prompt，事件通过 session/update 通知下发
//     （AgentMessageChunk / AgentThoughtChunk / ToolCall{toolCallId} / Plan），
//     权限请求走 session/request_permission（需要客户端应答），取消走 session/cancel。
//
// 因此官方 Grok Build 需要一套**双向 JSON-RPC 的 ACP 客户端适配器**，
// 与现有「启动进程 + 逐行解析 stdout」的 StreamAdapter 模型不同，
// 需要单独实现（含权限应答与取消处理），故当前未启用其发现项
// （见 discovery.go 中 CLITypeGrok 的注释），待 ACP 适配落地后再放开。
//
// 另注：官方 grok 安装器会把 `~/.grok/bin/agent` 软链到 `~/.local/bin/agent`，
// 与 Cursor Agent CLI 的 `agent` 命令名冲突（Cursor 侧改用 `cursor-agent` 规避）。
package grok

import (
	"encoding/json"
	"strings"

	"goteams-client/internal/executor"
)

const defaultDisplayName = "Grok CLI"

// NewAdapter creates the Grok adapter.
//
// The Grok CLI is run headless with `-p <prompt>` and `--format json`: it emits
// either a single terminal JSON object at the end of the run or a JSONL stream
// mixing terminal metadata with streaming event lines (type=thought/text/
// tool_call). The streaming lines are now surfaced as thinking/message/tool_call
// events; the terminal object is parsed in Done for the final message and session.
// New turns use a fresh process; later turns resume the saved session with
// `-s <session-id>`. The prompt is passed as the `-p` argument (not stdin).
func NewAdapter() executor.Adapter {
	return executor.NewStreamAdapter(executor.AdapterSpec{
		DisplayName:     defaultDisplayName,
		BuildInvocation: buildInvocation,
		NewDecoder: func(name string) executor.Decoder {
			return NewDecoder(name)
		},
	})
}

// buildInvocation 构造 grok 命令行。
//
//	新会话: grok --format json --no-sandbox [-m <model>] [extraArgs] -p <prompt>
//	续聊:   grok --format json --no-sandbox -s <session-id> [-m <model>] [extraArgs] -p <prompt>
//
// prompt 作为 -p 参数传递，不占用 stdin（Invocation.StdinText 留空）。
func buildInvocation(opts executor.RunOptions, resumeSession string) executor.Invocation {
	args := []string{"--format", "json", "--no-sandbox"}
	if resumeSession != "" {
		args = append(args, "-s", resumeSession)
	}
	if opts.ModelProfile != "" {
		args = append(args, "-m", opts.ModelProfile)
	}
	args = append(args, opts.ExtraArgs...)
	args = append(args, "-p", opts.Prompt)
	return executor.Invocation{Args: args}
}

// Decoder 解码 Grok CLI 的单信封 + 流式行混合 JSON 输出。
type Decoder struct {
	executor.BaseDecoder

	// rawLines 缓存所有成功解析为 JSON 的行，供 Done 执行终态解析。
	rawLines []string
	// unparsed 收集无法解析为 JSON 的行，作为失败诊断细节。
	unparsed executor.DiagnosticTail
}

// NewDecoder 创建 Grok 解码器。
func NewDecoder(name string) executor.Decoder {
	if strings.TrimSpace(name) == "" {
		name = defaultDisplayName
	}
	return &Decoder{BaseDecoder: executor.NewBaseDecoder(name)}
}

// grokStreamEvent 是流式行与终态对象的并集结构，用于按 type 分派。
// 文本 / 思考 / 工具参数各字段均宽容声明，保证不同 CLI 版本都能解码。
type grokStreamEvent struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thought  string          `json:"thought"`
	Content  string          `json:"content"`
	Message  string          `json:"message"`
	Data     string          `json:"data"`
	ToolName string          `json:"toolName"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Args     json.RawMessage `json:"args"`
}

// Decode 解析一行输出：流式行即时分派为事件，其余行缓存待 Done 终态解析。
func (d *Decoder) Decode(raw string, emit executor.EmitFunc) {
	line := strings.TrimSpace(raw)
	if line == "" {
		return
	}
	var event grokStreamEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		d.unparsed.AppendLine(line)
		return
	}
	// 原 sawEvent 兜底：只要解析到任意事件即视为已输出（等价 FailMissingTerminal 语义）。
	d.MarkTerminal()
	d.rawLines = append(d.rawLines, line)

	switch event.Type {
	case "thought":
		// 思考行：取 thought / text / content / message / data 中第一个非空值。
		d.Thinking(extractGrokThought(event), emit)

	case "text":
		// 流式文本行。
		d.Message(extractGrokText(event), emit)

	case "tool_call":
		name := strings.TrimSpace(firstNonEmpty(event.ToolName, event.Name))
		args := executor.FormatToolArgs(event.Input)
		if args == "" {
			args = executor.FormatToolArgs(event.Args)
		}
		// grok 的 tool_call 流式行不带工具调用 ID，由前端在唯一候选时归属
		d.ToolCall("", name, args, emit)
	}
}

// Done 在输出结束后执行终态解析并校验终态事件。
func (d *Decoder) Done(emit executor.EmitFunc) {
	if terminal := d.parseTerminal(); terminal != nil {
		if terminal.Text != "" {
			d.Message(terminal.Text, emit)
		}
		d.SetSession(terminal.SessionID)
		d.SetUsage(terminal.InputTokens, terminal.OutputTokens)
		if terminal.Error != "" {
			d.Fail(terminal.Error)
		}
	} else if output := d.unparsed.String(); output != "" {
		// 解析失败时按原逻辑把整段原始文本作为 message 输出。
		d.Message(output, emit)
	}
	if output := d.unparsed.String(); output != "" {
		d.AddDetail("未解析的 " + d.DisplayName + " 输出:\n" + output)
	}
	d.FailMissingTerminal("结束")
}

// parseTerminal 复用原 parseGrokJSONObject 逻辑，从缓存行中找出最后一个终态对象。
// 流式事件（text/thought/tool_call）已由 Decode 显式分派，不再作为终态消息，避免重复输出。
func (d *Decoder) parseTerminal() *grokResult {
	var last *grokResult
	for _, line := range d.rawLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var t struct {
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(trimmed), &t) == nil {
			switch t.Type {
			case "text", "thought", "tool_call":
				continue
			}
		}
		if obj := parseGrokJSONObject(trimmed); obj != nil {
			last = obj
		}
	}
	return last
}

// grokResult is the parsed terminal JSON object produced by
// `grok -p ... --format json`.
type grokResult struct {
	Text         string
	SessionID    string
	InputTokens  int
	OutputTokens int
	Error        string
}

// parseGrokJSONObject unmarshals one line/object into a grokResult. Unknown
// field names are tolerated so newer CLI output shapes keep working.
func parseGrokJSONObject(raw string) *grokResult {
	var v struct {
		Text        string `json:"text"`
		SessionID   string `json:"sessionId"`
		SessionID2  string `json:"session_id"`
		StopReason  string `json:"stopReason"`
		Error       string `json:"error"`
		Message     string `json:"message"`
		Result      string `json:"result"`
		Type        string `json:"type"`
		Data        string `json:"data"`
		InputUsage  int    `json:"inputTokens"`
		OutputUsage int    `json:"outputTokens"`
		Usage       struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil
	}
	if v.Type != "" && v.Type != "result" && v.Type != "end" && v.Type != "message" {
		// streaming event lines (text/thought/tool_call/...) are not a terminal result
		if v.Text == "" && v.Message == "" && v.Result == "" {
			return nil
		}
	}
	res := &grokResult{}
	res.Text = strings.TrimSpace(firstNonEmpty(v.Text, v.Message, v.Result, v.Data))
	res.SessionID = firstNonEmpty(v.SessionID, v.SessionID2)
	res.InputTokens = v.InputUsage
	res.OutputTokens = v.OutputUsage
	if v.Usage.InputTokens > 0 {
		res.InputTokens = v.Usage.InputTokens
	}
	if v.Usage.OutputTokens > 0 {
		res.OutputTokens = v.Usage.OutputTokens
	}
	if msg := strings.TrimSpace(firstNonEmpty(v.Error, v.Message)); msg != "" && res.Text == "" {
		res.Error = msg
	}
	// Errors reported through a streaming "error" event.
	if v.Type == "error" {
		msg := strings.TrimSpace(firstNonEmpty(v.Message, v.Data, v.Error))
		if msg == "" {
			msg = "Grok 运行失败"
		}
		res.Error = msg
	}
	if res.Text == "" && res.SessionID == "" && res.Error == "" && res.InputTokens == 0 && res.OutputTokens == 0 {
		return nil
	}
	return res
}

// extractGrokText 从流式文本行提取文本，兼容 text / content / message / data 字段名。
func extractGrokText(event grokStreamEvent) string {
	return strings.TrimSpace(firstNonEmpty(event.Text, event.Content, event.Message, event.Data))
}

// extractGrokThought 从流式思考行提取思考文本，兼容 thought / text / content / message / data 字段名。
func extractGrokThought(event grokStreamEvent) string {
	return strings.TrimSpace(firstNonEmpty(event.Thought, event.Text, event.Content, event.Message, event.Data))
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
