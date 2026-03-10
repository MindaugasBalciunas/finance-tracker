package timeutil

import "time"

// ParseDate parses a YYYY-MM-DD string into a time.Time.
func ParseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}
