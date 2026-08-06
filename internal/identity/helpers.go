package identity

import "time"

// timeNowMillis returns the current Unix millisecond timestamp
func timeNowMillis() int64 {
	return time.Now().UnixMilli()
}
