package timeutil

import (
	"fmt"
	"strconv"
	"time"
)

// FormatDuration formats a duration in a human-readable format.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd%dh", days, hours)
}

// ParseTTL parses a TTL value that can be either an integer (seconds) or a duration string.
func ParseTTL(s string) (int, error) {
	// Try parsing as integer seconds first.
	if v, err := strconv.Atoi(s); err == nil {
		return v, nil
	}

	// Try parsing as duration string.
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid TTL value %q: %w", s, err)
	}

	return int(d.Seconds()), nil
}

// Now returns the current time in UTC.
func Now() time.Time {
	return time.Now().UTC()
}

// FormatDateTime formats a time as ISO 8601 datetime string.
func FormatDateTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// ParseDateTime parses an ISO 8601 datetime string.
func ParseDateTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// DaysAgo returns a time that is n days before now.
func DaysAgo(n int) time.Time {
	return Now().AddDate(0, 0, -n)
}

// IsExpired checks if a time is before now.
func IsExpired(t time.Time) bool {
	return t.Before(Now())
}
