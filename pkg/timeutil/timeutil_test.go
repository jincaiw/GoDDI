package timeutil

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    time.Duration
		expected string
	}{
		{"30 seconds", 30 * time.Second, "30s"},
		{"0 seconds", 0, "0s"},
		{"1 second", 1 * time.Second, "1s"},
		{"1 minute", 1 * time.Minute, "1m0s"},
		{"90 seconds", 90 * time.Second, "1m30s"},
		{"1 hour", 1 * time.Hour, "1h0m"},
		{"2 hours 30 minutes", 150 * time.Minute, "2h30m"},
		{"1 day", 24 * time.Hour, "1d0h"},
		{"1 day 12 hours", 36 * time.Hour, "1d12h"},
		{"7 days", 168 * time.Hour, "7d0h"},
		{"2 days 6 hours", 54 * time.Hour, "2d6h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := FormatDuration(tt.input)
			if result != tt.expected {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected int
		wantErr  bool
	}{
		{"integer seconds", "3600", 3600, false},
		{"zero", "0", 0, false},
		{"small value", "60", 60, false},
		{"duration minutes", "5m", 300, false},
		{"duration hours", "1h", 3600, false},
		{"duration seconds", "30s", 30, false},
		{"duration days", "24h", 86400, false},
		{"duration mixed", "1h30m", 5400, false},
		{"invalid string", "invalid", 0, true},
		{"empty string", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := ParseTTL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTTL(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("ParseTTL(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNow(t *testing.T) {
	result := Now()
	if result.Location() != time.UTC {
		t.Errorf("Now() location = %v, want UTC", result.Location())
	}

	// Should be close to current time
	now := time.Now().UTC()
	diff := now.Sub(result)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Second {
		t.Errorf("Now() differs from time.Now().UTC() by %v", diff)
	}
}

func TestFormatDateTime(t *testing.T) {
	t.Parallel()

	// Create a known time in UTC
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	result := FormatDateTime(ts)
	expected := "2024-01-15T10:30:00Z"
	if result != expected {
		t.Errorf("FormatDateTime() = %q, want %q", result, expected)
	}
}

func TestParseDateTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid RFC3339", "2024-01-15T10:30:00Z", false},
		{"valid with timezone", "2024-01-15T10:30:00+08:00", false},
		{"invalid format", "2024-01-15", true},
		{"empty string", "", true},
		{"invalid date", "not-a-date", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseDateTime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDateTime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestFormatAndParseDateTime_RoundTrip(t *testing.T) {
	t.Parallel()

	original := time.Date(2024, 6, 15, 14, 30, 45, 0, time.UTC)
	formatted := FormatDateTime(original)
	parsed, err := ParseDateTime(formatted)
	if err != nil {
		t.Fatalf("ParseDateTime failed: %v", err)
	}
	if !parsed.Equal(original) {
		t.Errorf("round trip: got %v, want %v", parsed, original)
	}
}

func TestDaysAgo(t *testing.T) {
	result := DaysAgo(7)
	expected := Now().AddDate(0, 0, -7)
	diff := result.Sub(expected)
	if diff < 0 {
		diff = -diff
	}
	// Allow small time difference due to execution time
	if diff > time.Second {
		t.Errorf("DaysAgo(7) = %v, want approximately %v", result, expected)
	}
}

func TestDaysAgo_Zero(t *testing.T) {
	result := DaysAgo(0)
	now := Now()
	diff := result.Sub(now)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Second {
		t.Errorf("DaysAgo(0) should be approximately now, diff = %v", diff)
	}
}

func TestIsExpired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		time     time.Time
		expected bool
	}{
		{"past time", time.Now().Add(-1 * time.Hour), true},
		{"future time", time.Now().Add(1 * time.Hour), false},
		{"far past", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsExpired(tt.time)
			if result != tt.expected {
				t.Errorf("IsExpired(%v) = %v, want %v", tt.time, result, tt.expected)
			}
		})
	}
}
