package cloud

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"goteams-client/internal/protocol"
)

// APIError Error returned by cloud API
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Retryable  bool
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("cloud api error [%d] %s: %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("cloud api error [%d]: %s", e.StatusCode, e.Message)
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

	var errBody struct {
		// code may be a string or a number, use interface{} to be compatible with both situations
		Code    interface{} `json:"code"`
		Message string      `json:"message"`
		Error   string      `json:"error"`
	}
	if jsonErr := json.Unmarshal(body, &errBody); jsonErr != nil {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
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
	}
}
