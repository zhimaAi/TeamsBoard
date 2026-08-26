package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// nowMillis returns the current Unix millisecond timestamp
func nowMillis() int64 {
	return time.Now().UnixMilli()
}

// sha256Hex returns the lowercase hex SHA-256 digest of the input string.
func sha256Hex(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}
