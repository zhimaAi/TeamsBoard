package cloud

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"goteams-client/internal/protocol"
)

// maxErrorBodyLength caps the raw cloud response body retained for diagnostics
// so a verbose error page (e.g. a proxy HTML 502) cannot blow up the log line.
const maxErrorBodyLength = 1024

// APIError Error returned by cloud API
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Retryable  bool
	// Body is the raw cloud response body (truncated to maxErrorBodyLength) kept
	// for diagnostics. Some endpoints return the real failure detail in a field
	// other than message/error, or only inside the raw body, so it is surfaced
	// through Error() instead of being silently dropped.
	Body string
}

func (e *APIError) Error() string {
	var msg string
	if e.Code != "" {
		msg = fmt.Sprintf("cloud api error [%d] %s: %s", e.StatusCode, e.Code, e.Message)
	} else {
		msg = fmt.Sprintf("cloud api error [%d]: %s", e.StatusCode, e.Message)
	}
	if e.Body != "" {
		msg += fmt.Sprintf(" (response body: %s)", e.Body)
	}
	return msg
}

// truncateBody caps a raw response body for storage in APIError.Body, keeping
// single-line output for slog. Empty bodies stay empty so callers can tell
// "no body" apart from "empty string".
func truncateBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if len(body) > maxErrorBodyLength {
		return string(body[:maxErrorBodyLength]) + "...(truncated)"
	}
	return string(body)
}

// IsRetryable returns whether the error can be retried
func (e *APIError) IsRetryable() bool {
	return e.Retryable
}

// parseAPIError Parse error from HTTP response
func parseAPIError(resp *http.Response) *APIError {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("读取错误响应失败: %v", err),
		}
	}

	truncated := truncateBody(body)

	var errBody struct {
		// code may be a string or a number, use interface{} to be compatible with both situations
		Code    interface{} `json:"code"`
		Message string      `json:"message"`
		Error   string      `json:"error"`
	}
	if jsonErr := json.Unmarshal(body, &errBody); jsonErr != nil {
		// The body is not JSON (e.g. a proxy HTML error page): surface it directly.
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    truncated,
			Body:       truncated,
		}
	}

	msg := errBody.Message
	if msg == "" {
		msg = errBody.Error
	}
	if msg == "" {
		msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	code := ""
	if errBody.Code != nil {
		code = fmt.Sprint(errBody.Code)
	}

	retryable := false
	if code != "" {
		retryable = protocol.IsRetryable(code)
	}
	// 5xx can be retried by default
	if resp.StatusCode >= 500 {
		retryable = true
	}

	return &APIError{
		StatusCode: resp.StatusCode,
		Code:       code,
		Message:    msg,
		Retryable:  retryable,
		Body:       truncated,
	}
}
