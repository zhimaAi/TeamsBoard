package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// EmitFunc 是解码器的事件出口。解码器只负责翻译协议，不关心事件如何被消费。
type EmitFunc func(ExecutorEvent)

// Outcome 汇总解码过程中收集到的协议侧执行结果。
type Outcome struct {
	SessionID     string
	InputTokens   int
	OutputTokens  int
	UsageSeen     bool
	FailureReason string   // 非空表示协议层判定本次执行失败
	ExtraDetails  []string // 协议特有诊断信息（如未解析输出），拼进失败原因
}

// Decoder 把一个 CLI 的原始输出翻译成统一事件流。
// 实现者只处理协议差异：进程生命周期、stderr、退出码、错误归并都由运行器与适配器骨架负责。
type Decoder interface {
	// Decode 处理一段原始输出（通常是 stdout 的一行）。
	Decode(raw string, emit EmitFunc)
	// Done 在输出读取结束后调用一次，用于缓冲型协议（单信封、纯文本）的收尾解析。
	Done(emit EmitFunc)
	// Outcome 返回协议侧执行结果。
	Outcome() Outcome
}

// ContentBlock 是 stream-json 类 CLI 通用的内容块结构。
// claude / codebuddy / qoder / qwen 属于同一协议族，共用这一份定义与解码逻辑。
type ContentBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Content  json.RawMessage `json:"content"`
	Result   json.RawMessage `json:"result"`
	// 工具调用 ID：tool_use 块自己带 id，tool_result 块用 tool_use_id 指回所属调用。
	ID        string `json:"id"`
	ToolUseID string `json:"tool_use_id"`
}

// BaseDecoder 提供各 CLI 解码器共用的状态收集与事件发射逻辑。
// 各解码器内嵌它，只实现协议特有的分支判断。
type BaseDecoder struct {
	DisplayName string

	sessionID     string
	inputTokens   int
	outputTokens  int
	usageSeen     bool
	terminalSeen  bool
	failureReason string
	details       []string
}

// NewBaseDecoder 创建基础解码器。
func NewBaseDecoder(displayName string) BaseDecoder {
	return BaseDecoder{DisplayName: displayName}
}

// SessionID 返回已识别的外部会话 ID。
func (b *BaseDecoder) SessionID() string {
	return b.sessionID
}

// SetSession 记录外部会话 ID，空值不覆盖已有值。
func (b *BaseDecoder) SetSession(id string) {
	if id != "" {
		b.sessionID = id
	}
}

// SetUsage 覆盖本次执行的 token 用量（终态事件携带的是总量）。
func (b *BaseDecoder) SetUsage(input, output int) {
	if input > 0 {
		b.inputTokens = input
	}
	if output > 0 {
		b.outputTokens = output
	}
	if input > 0 || output > 0 {
		b.usageSeen = true
	}
}

// AccumulateUsage 累加过程中的 token 用量（仅作为终态缺失时的兜底）。
func (b *BaseDecoder) AccumulateUsage(input, output int) {
	b.inputTokens += input
	b.outputTokens += output
}

// TerminalSeen 报告是否已收到协议终态事件。
func (b *BaseDecoder) TerminalSeen() bool {
	return b.terminalSeen
}

// MarkTerminal 标记已收到协议终态事件。
func (b *BaseDecoder) MarkTerminal() {
	b.terminalSeen = true
}

// Fail 记录协议层失败原因，首个原因生效。
func (b *BaseDecoder) Fail(reason string) {
	if b.failureReason == "" && reason != "" {
		b.failureReason = reason
	}
}

// FailMissingTerminal 在未收到终态事件时填充标准失败原因。
func (b *BaseDecoder) FailMissingTerminal(terminalName string) {
	if b.terminalSeen || b.failureReason != "" {
		return
	}
	b.failureReason = fmt.Sprintf("%s 未输出有效的 %s 结束事件", b.DisplayName, terminalName)
}

// AddDetail 追加协议特有诊断信息，仅在判定失败时拼进错误详情。
func (b *BaseDecoder) AddDetail(detail string) {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return
	}
	b.details = append(b.details, detail)
}

// Outcome 返回协议侧执行结果。
func (b *BaseDecoder) Outcome() Outcome {
	return Outcome{
		SessionID:     b.sessionID,
		InputTokens:   b.inputTokens,
		OutputTokens:  b.outputTokens,
		UsageSeen:     b.usageSeen,
		FailureReason: b.failureReason,
		ExtraDetails:  b.details,
	}
}

// Message 发射一条面向用户的输出事件，空内容自动跳过。
func (b *BaseDecoder) Message(content string, emit EmitFunc) {
	b.emitContent(EventMessage, content, emit)
}

// Thinking 发射一条思考事件，空内容自动跳过。
func (b *BaseDecoder) Thinking(content string, emit EmitFunc) {
	b.emitContent(EventThinking, content, emit)
}

// ToolResult 发射一条工具结果事件，空内容自动跳过。
// toolUseID 用于把结果归属到对应调用；协议未提供时传空字符串。
func (b *BaseDecoder) ToolResult(toolUseID, content string, emit EmitFunc) {
	b.emitContentWithID(EventToolResult, toolUseID, content, emit)
}

// ToolCall 发射一条工具调用事件。内容统一为「工具名 参数摘要」，与前端解析约定一致。
// toolUseID 是协议侧的工具调用 ID，协议未提供时传空字符串。
func (b *BaseDecoder) ToolCall(toolUseID, name, args string, emit EmitFunc) {
	name = strings.TrimSpace(name)
	args = OneLine(args)
	content := name
	if args != "" {
		if content == "" {
			content = args
		} else {
			content = content + " " + args
		}
	}
	b.emitContentWithID(EventToolCall, toolUseID, content, emit)
}

// Permission 发射权限请求事件。
func (b *BaseDecoder) Permission(content string, requests []PermissionRequest, emit EmitFunc) {
	if content == "" && len(requests) == 0 {
		return
	}
	emit(ExecutorEvent{
		Type:               EventPermission,
		Content:            content,
		SessionID:          b.sessionID,
		PermissionRequests: requests,
		Timestamp:          NowMillis(),
	})
}

// Usage 发射 token 用量事件。
func (b *BaseDecoder) Usage(input, output int, emit EmitFunc) {
	emit(ExecutorEvent{
		Type:         EventUsage,
		SessionID:    b.sessionID,
		InputTokens:  input,
		OutputTokens: output,
		Timestamp:    NowMillis(),
	})
}

func (b *BaseDecoder) emitContent(eventType, content string, emit EmitFunc) {
	b.emitContentWithID(eventType, "", content, emit)
}

// emitContentWithID 与 emitContent 一致，额外携带工具调用 ID。
func (b *BaseDecoder) emitContentWithID(eventType, toolUseID, content string, emit EmitFunc) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	emit(ExecutorEvent{
		Type:      eventType,
		Content:   content,
		SessionID: b.sessionID,
		ToolUseID: strings.TrimSpace(toolUseID),
		Timestamp: NowMillis(),
	})
}

// DecodeContentBlocks 把 stream-json 内容块翻译为统一事件。
// claude / codebuddy / qoder / qwen 共用：这些 CLI 只在内容块类型的支持范围上有差异。
func DecodeContentBlocks(blocks []ContentBlock, base *BaseDecoder, emit EmitFunc) {
	for _, block := range blocks {
		switch block.Type {
		case "thinking", "reasoning":
			base.Thinking(block.Thinking, emit)
		case "text":
			base.Message(block.Text, emit)
		case "tool_use", "tool_call":
			base.ToolCall(block.ID, block.Name, FormatToolArgs(block.Input), emit)
		case "tool_result":
			content := RawText(block.Content)
			if content == "" {
				content = RawText(block.Result)
			}
			base.ToolResult(block.ToolUseID, content, emit)
		}
	}
}

// FormatToolArgs 把工具参数压成单行摘要。
// 字符串参数原样返回，对象/数组返回紧凑 JSON 单行。
func FormatToolArgs(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	text := RawText(raw)
	if text == "" {
		return ""
	}
	return OneLine(text)
}

// Invocation 描述一次 CLI 调用的命令行与 prompt 投递方式。
// 由各 CLI 自行决定 prompt 走标准输入、命令行参数还是临时文件。
type Invocation struct {
	Args       []string
	StdinText  string
	PromptFile *PromptFileSpec
}

// AdapterSpec 声明一个 CLI 适配器：如何构造命令行、如何解码输出。
// 新增 CLI 只需要提供这两项，进程管理与错误归并复用 StreamAdapter。
type AdapterSpec struct {
	DisplayName string
	// BuildInvocation 根据执行选项构造命令行；resumeSession 非空表示续聊既有会话。
	BuildInvocation func(opts RunOptions, resumeSession string) Invocation
	// NewDecoder 构造本次执行的协议解码器。
	NewDecoder func(displayName string) Decoder
}

// StreamAdapter 是用 AdapterSpec 驱动的通用适配器实现。
type StreamAdapter struct {
	spec AdapterSpec

	mu   sync.Mutex
	proc *Process
}

// NewStreamAdapter 创建通用适配器。
func NewStreamAdapter(spec AdapterSpec) Adapter {
	return &StreamAdapter{spec: spec}
}

// NewConversation 开始新会话。
func (a *StreamAdapter) NewConversation(ctx context.Context, opts RunOptions) (<-chan ExecutorEvent, error) {
	return a.run(ctx, opts, "")
}

// ResumeConversation 续聊既有会话。
func (a *StreamAdapter) ResumeConversation(ctx context.Context, opts ResumeOptions) (<-chan ExecutorEvent, error) {
	return a.run(ctx, opts.RunOptions, opts.ExternalSessionID)
}

// Stop 终止当前进程树。
func (a *StreamAdapter) Stop() error {
	a.mu.Lock()
	proc := a.proc
	a.mu.Unlock()
	if proc == nil {
		return nil
	}
	return proc.Stop()
}

func (a *StreamAdapter) run(ctx context.Context, opts RunOptions, resumeSession string) (<-chan ExecutorEvent, error) {
	displayName := a.displayName()
	invocation := a.spec.BuildInvocation(opts, resumeSession)
	decoder := a.spec.NewDecoder(displayName)

	proc, err := StartProcess(ctx, RunSpec{
		ExecPath:   opts.ExecPath,
		Args:       invocation.Args,
		WorkDir:    opts.WorkDir,
		EnvVars:    opts.EnvVars,
		StdinText:  invocation.StdinText,
		PromptFile: invocation.PromptFile,
	})
	if err != nil {
		return nil, fmt.Errorf("启动 %s 失败: %w", displayName, err)
	}

	a.mu.Lock()
	a.proc = proc
	a.mu.Unlock()

	eventCh := make(chan ExecutorEvent, 100)
	go func() {
		defer close(eventCh)
		defer a.clearProcess(proc)

		emit := func(event ExecutorEvent) {
			if event.Timestamp == 0 {
				event.Timestamp = NowMillis()
			}
			eventCh <- event
		}

		emit(NewEvent(EventStart))
		for line := range proc.Lines() {
			decoder.Decode(line, emit)
		}
		decoder.Done(emit)

		result := proc.WaitFor()
		outcome := decoder.Outcome()
		if outcome.FailureReason != "" || result.ScanErr != nil || result.WaitErr != nil || result.StdinErr != nil {
			reason := outcome.FailureReason
			if reason == "" && result.StdinErr != nil {
				reason = "写入 " + displayName + " Prompt 失败: " + result.StdinErr.Error()
			}
			emit(ExecutorEvent{
				Type:      EventError,
				Error:     FormatExecutionError(displayName, reason, result.Stderr, outcome.ExtraDetails, result.ScanErr, result.WaitErr),
				SessionID: outcome.SessionID,
				Timestamp: NowMillis(),
			})
			return
		}
		emit(NewCompleteEvent(outcome.SessionID, outcome.InputTokens, outcome.OutputTokens))
	}()

	return eventCh, nil
}

func (a *StreamAdapter) displayName() string {
	name := strings.TrimSpace(a.spec.DisplayName)
	if name == "" {
		return "CLI"
	}
	return name
}

func (a *StreamAdapter) clearProcess(proc *Process) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.proc != proc {
		return
	}
	a.proc = nil
}
