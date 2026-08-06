package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// sha256Hex computes SHA-256 and returns the hex string
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// nowMillis returns the current Unix millisecond timestampond timestamp
func nowMillis() int64 {
	return time.Now().UnixMilli()
}
