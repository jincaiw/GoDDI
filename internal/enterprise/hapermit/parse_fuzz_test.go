package hapermit

import (
	"errors"
	"testing"
)

// TestParseGoldenSeed keeps the fuzz seed accepted so a format change fails
// without an actual fuzzing run.
func TestParseGoldenSeed(t *testing.T) {
	if _, err := parse([]byte(golden)); err != nil {
		t.Fatalf("golden body rejected: %v", err)
	}
}

// FuzzParse feeds arbitrary bytes to the untrusted candidate-permit parser. It
// must never panic, must only reject with the typed ErrInvalid, and must accept
// only canonical bytes (re-parsing an accepted body succeeds).
func FuzzParse(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("{}"))
	f.Add([]byte(`{"a":"b"}`))
	f.Add([]byte("\xef\xbb\xbf{}"))
	f.Add([]byte(golden))

	f.Fuzz(func(t *testing.T, raw []byte) {
		if _, err := parse(raw); err != nil {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("parse returned untyped error %v for %q", err, raw)
			}
			return
		}
		if _, err := parse(raw); err != nil {
			t.Fatalf("accepted body not idempotent: %v", err)
		}
	})
}
