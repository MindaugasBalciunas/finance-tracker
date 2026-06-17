package timeutil

import "time"

// ParseDate parses a YYYY-MM-DD string into a time.Time.
func ParseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// ParseDatetime parses a datetime string into a time.Time.
// Accepted formats: "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02" (midnight UTC).
func ParseDatetime(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, &time.ParseError{Layout: "2006-01-02[T/ ]15:04", Value: s}
}
