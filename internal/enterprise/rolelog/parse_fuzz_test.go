package rolelog

import (
	"errors"
	"testing"
)

// TestParseValidSeed keeps the fuzz seed accepted so a format change fails
// without an actual fuzzing run.
func TestParseValidSeed(t *testing.T) {
	if _, err := Parse([]byte(valid)); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
}

// FuzzParse feeds arbitrary bytes to the untrusted local role/epoch record
// parser. It must never panic, must only reject with the typed ErrInvalid, and
// must accept only bytes that re-parse (a successful Parse is stable).
func FuzzParse(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("{}"))
	f.Add([]byte(`{"version":"1"}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(valid))

	f.Fuzz(func(t *testing.T, raw []byte) {
		if _, err := Parse(raw); err != nil {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Parse returned untyped error %v for %q", err, raw)
			}
			return
		}
		if _, err := Parse(raw); err != nil {
			t.Fatalf("accepted record not idempotent: %v", err)
		}
	})
}
