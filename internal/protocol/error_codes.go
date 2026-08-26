package protocol

const (
	ErrCodeRateLimited       = "RATE_LIMITED"
	ErrCodeInternalError     = "INTERNAL_ERROR"
	ErrCodeClientRateLimited = "CLIENT_RATE_LIMITED"
)

var errCodeRetryable = map[string]bool{
	ErrCodeRateLimited:       true,
	ErrCodeInternalError:     true,
	ErrCodeClientRateLimited: true,
}

// IsRetryable reports whether an HTTP API error code is retryable.
func IsRetryable(code string) bool {
	return errCodeRetryable[code]
}
