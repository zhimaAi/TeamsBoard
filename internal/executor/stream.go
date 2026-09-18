package executor

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxDiagnosticBytes 是单个诊断缓冲保留的最大字节数。
// CLI 异常输出可能无上限，只保留尾部即可定位错误，避免占用无界内存。
const maxDiagnosticBytes = 64 * 1024

// DiagnosticTail 只保留诊断输出的尾部内容。
// 各 CLI 适配器共用这一份实现，避免逐包复制。
type DiagnosticTail struct {
	data []byte
}

// AppendLine 追加一行诊断输出。
func (b *DiagnosticTail) AppendLine(line string) {
	if len(b.data) > 0 {
		b.appendChunk([]byte{'\n'})
	}
	b.appendChunk([]byte(line))
}

// Append 追加一段原始输出。
func (b *DiagnosticTail) Append(chunk []byte) {
	b.appendChunk(chunk)
}

// String 返回去除首尾空白后的诊断内容。
func (b *DiagnosticTail) String() string {
	return strings.TrimSpace(string(b.data))
}

func (b *DiagnosticTail) appendChunk(chunk []byte) {
	if len(chunk) >= maxDiagnosticBytes {
		b.data = append(b.data[:0], chunk[len(chunk)-maxDiagnosticBytes:]...)
		return
	}

	overflow := len(b.data) + len(chunk) - maxDiagnosticBytes
	if overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
	}
	b.data = append(b.data, chunk...)
}

// NowMillis 返回当前毫秒时间戳。
func NowMillis() int64 {
	return time.Now().UnixMilli()
}

// RawText 把 JSON 原始值转成便于展示的文本：
// 字符串直接返回；对象/数组返回紧凑 JSON；空值返回空串。
func RawText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" || text == "{}" || text == "[]" {
		return ""
	}

	var str string
	if json.Unmarshal(raw, &str) == nil {
		return strings.TrimSpace(str)
	}

	var value interface{}
	if json.Unmarshal(raw, &value) != nil {
		return text
	}
	return TextFromValue(value)
}

// TextFromValue 把任意 JSON 值归一为展示文本。
// CLI 的工具结果形态不统一（字符串 / 内容块数组 / {content}/{text}/{output} 包装对象），
// 这里做一次统一收敛，避免每个适配器各写一套。
func TextFromValue(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		return strings.TrimRight(strings.TrimRight(formatFloat(typed), "0"), ".")
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := TextFromValue(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]interface{}:
		return textFromMap(typed)
	default:
		if encoded, err := json.Marshal(typed); err == nil {
			return strings.TrimSpace(string(encoded))
		}
		return ""
	}
}

// textFromMap 按常见字段优先级从对象中取文本。
func textFromMap(fields map[string]interface{}) string {
	for _, key := range []string{"text", "output", "result", "content", "message", "delta", "thinking", "value"} {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		if text := TextFromValue(raw); text != "" {
			return text
		}
	}
	if len(fields) == 0 {
		return ""
	}
	// 未命中已知字段时给出稳定排序的紧凑 JSON，保证可读且可复现
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(map[string]interface{}, len(fields))
	for _, key := range keys {
		ordered[key] = fields[key]
	}
	if encoded, err := json.Marshal(ordered); err == nil {
		return strings.TrimSpace(string(encoded))
	}
	return ""
}

func formatFloat(value float64) string {
	if value == math.Trunc(value) && math.Abs(value) < 1e15 {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// Truncate 按最大字符数截断文本，用于事件落库前的长度控制。
func Truncate(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	// 按 rune 截断，避免把多字节字符切成乱码
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}

// OneLine 把多行文本压成单行，用于摘要类展示。
func OneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
