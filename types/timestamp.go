package types

import (
	"time"
)

// MaxTimestampDrift is the maximum allowed deviation from current time
const MaxTimestampDrift = 5 * time.Second

// ValidateTimestamp checks if a Unix timestamp is within ±5s of current time
func ValidateTimestamp(ts uint64) error {
	now := uint64(time.Now().Unix())
	drift := int64(now) - int64(ts)
	if drift < -5 || drift > 5 {
		return ErrInvalidTimestamp
	}
	return nil
}
