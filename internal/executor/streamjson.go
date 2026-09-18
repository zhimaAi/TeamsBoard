package executor

import (
	"encoding/json"
	"strings"
)

// StreamJSONEvent 是 stream-json 协议族 CLI 的通用事件结构。
// claude、codebuddy、qoder、qwen 属于同一协议族：事件以 type 区分，
// assistant 消息的内容放在 message.content 内容块数组里。
// 字段按各 CLI 的并集声明，缺失字段留空即可，不存在的分支自然不产出事件。
type StreamJSONEvent struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	Content   json.RawMessage `json:"content"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	SessionID string          `json:"session_id"`
	ToolUseID string          `json:"tool_use_id"`
	Result    string          `json:"result"`
	// Errors 是失败原因数组。claude 系把原因放在 result 字符串，
	// codebuddy 系则只给 errors 数组（如鉴权失败），两者都要能取到。
	Errors            []string          `json:"errors"`
	IsError           bool              `json:"is_error"`
	PermissionDenials []PermissionDeny  `json:"permission_denials"`
	Message           StreamJSONMessage `json:"message"`
	Usage             StreamJSONUsage   `json:"usage"`
}

// StreamJSONMessage 是消息信封，承载内容块数组。
type StreamJSONMessage struct {
	Content []ContentBlock `json:"content"`
}

// PermissionDeny 是被拒绝的工具授权请求。
type PermissionDeny struct {
	ToolName  string          `json:"tool_name"`
	ToolUseID string          `json:"tool_use_id"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// StreamJSONUsage 是 token 用量。
type StreamJSONUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StreamJSONDecoder 解码 stream-json 协议族的输出。
// 各 CLI 的差异（是否有思考块、是否有权限拒绝）由数据决定，无需分别实现。
type StreamJSONDecoder struct {
	BaseDecoder
}

// NewStreamJSONDecoder 创建 stream-json 协议解码器。
func NewStreamJSONDecoder(displayName string) Decoder {
	return &StreamJSONDecoder{BaseDecoder: NewBaseDecoder(displayName)}
}

// Decode 解析一行原始输出。
func (d *StreamJSONDecoder) Decode(raw string, emit EmitFunc) {
	line := strings.TrimSpace(raw)
	if line == "" {
		return
	}
	var event StreamJSONEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		// 非 JSON 行是 CLI 的诊断输出，交由 stderr/退出码路径处理
		return
	}
	d.SetSession(event.SessionID)

	switch event.Type {
	case "assistant":
		DecodeContentBlocks(event.Message.Content, &d.BaseDecoder, emit)
		// 部分版本把最终文本直接放在顶层 content
		d.Message(RawText(event.Content), emit)

	case "user":
		// 工具结果在 user 消息的内容块里回传，必须一并解析，否则工具调用只有请求没有结果
		for _, block := range event.Message.Content {
			if block.Type == "tool_result" {
				content := RawText(block.Content)
				if content == "" {
					content = RawText(block.Result)
				}
				d.ToolResult(block.ToolUseID, content, emit)
			}
		}

	case "text", "message":
		d.Message(RawText(event.Content), emit)

	case "thinking", "reasoning":
		d.Thinking(RawText(event.Content), emit)

	case "tool_use", "tool_call":
		// 顶层形态不带可信的工具调用 ID（同层的 id 是 message id），故不传，避免误配对。
		d.ToolCall("", event.Name, FormatToolArgs(event.Input), emit)

	case "tool_result":
		d.ToolResult(event.ToolUseID, RawText(event.Content), emit)

	case "result":
		d.handleResult(event, emit)
	}
}

// Done 在输出结束后校验终态事件。
func (d *StreamJSONDecoder) Done(emit EmitFunc) {
	d.FailMissingTerminal("result")
}

func (d *StreamJSONDecoder) handleResult(event StreamJSONEvent, emit EmitFunc) {
	d.MarkTerminal()
	d.SetSession(event.SessionID)
	d.SetUsage(event.Usage.InputTokens, event.Usage.OutputTokens)

	if len(event.PermissionDenials) > 0 {
		requests := make([]PermissionRequest, 0, len(event.PermissionDenials))
		for _, denial := range event.PermissionDenials {
			requests = append(requests, PermissionRequest{
				ToolName:  denial.ToolName,
				ToolUseID: denial.ToolUseID,
				ToolInput: RawText(denial.ToolInput),
			})
		}
		d.Permission(SummarizePermissionRequests(d.DisplayName, requests), requests, emit)
	}

	if event.IsError {
		reason := strings.TrimSpace(event.Result)
		if reason == "" {
			// codebuddy 等只给 errors 数组，取非空项拼成原因，避免丢失鉴权失败等真实原因
			parts := make([]string, 0, len(event.Errors))
			for _, item := range event.Errors {
				if trimmed := strings.TrimSpace(item); trimmed != "" {
					parts = append(parts, trimmed)
				}
			}
			reason = strings.TrimSpace(strings.Join(parts, "\n"))
		}
		if reason == "" {
			reason = d.DisplayName + " 返回错误结果，但未提供错误详情"
		}
		d.Fail(reason)
	}
}

// SummarizePermissionRequests 生成权限请求提示文案。
func SummarizePermissionRequests(displayName string, requests []PermissionRequest) string {
	names := make([]string, 0, len(requests))
	seen := make(map[string]struct{}, len(requests))
	for _, request := range requests {
		name := strings.TrimSpace(request.ToolName)
		if name == "" {
			name = "未知工具"
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return displayName + " 请求授权使用：" + strings.Join(names, "、")
}
