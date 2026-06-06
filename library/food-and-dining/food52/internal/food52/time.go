package food52

import "time"

// unixMilliToRFC3339 formats a millisecond-precision Unix timestamp as
// RFC3339 in UTC. Returns the empty string for non-positive input.
func unixMilliToRFC3339(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}
