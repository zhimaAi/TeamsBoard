package workflow

import "strings"

// composeSessionFinalResult 组合会话的最终文本。
// 失败时保留可见回复，避免专家团交回团长时丢掉成员的实际交付内容。
func composeSessionFinalResult(status, responseContent, errorMessage string) string {
	responseContent = strings.TrimSpace(responseContent)
	errorMessage = strings.TrimSpace(errorMessage)
	if status == SessionStatusSuccess {
		return responseContent
	}
	switch {
	case errorMessage == "" && responseContent == "":
		return "CLI 执行" + status
	case errorMessage == "":
		return responseContent
	case responseContent == "" || responseContent == errorMessage:
		return errorMessage
	default:
		return errorMessage + "\n\n" + responseContent
	}
}
