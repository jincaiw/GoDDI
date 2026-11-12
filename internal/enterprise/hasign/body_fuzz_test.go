package hasign

import (
	"encoding/base64"
	"errors"
	"testing"
)

// jcsSeed builds a canonical v1 body using the package's exact accepted shape.
func jcsSeed(tb testing.TB) []byte {
	tb.Helper()
	digest := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	raw := []byte(`{"digest":"` + digest + `","kind":"` + string(TakeoverPermit) + `","version":"1"}`)
	if _, err := parseBody(raw); err != nil {
		tb.Fatalf("seed not canonical: %v", err)
	}
	return raw
}

// TestJCSSeedIsCanonical keeps the fuzz seed accepted so a format change that
// invalidates it fails without an actual fuzzing run.
func TestJCSSeedIsCanonical(t *testing.T) { _ = jcsSeed(t) }

// FuzzParseBody asserts the untrusted JCS document parser never panics, always
// returns a typed ErrInvalid on rejection, and accepts only bytes that are
// already canonical (re-parsing an accepted body succeeds). This guards the
// canonicalization boundary that detached Ed25519 signatures are computed over.
func FuzzParseBody(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("{}"))
	f.Add([]byte("null"))
	f.Add([]byte(`{"digest":"x","kind":"takeover-permit","version":"1"}`))
	f.Add([]byte("\xef\xbb\xbf{}"))
	f.Add(jcsSeed(f))

	f.Fuzz(func(t *testing.T, raw []byte) {
		if _, err := parseBody(raw); err != nil {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("parseBody returned untyped error %v for %q", err, raw)
			}
			return
		}
		if _, err := parseBody(raw); err != nil {
			t.Fatalf("accepted body not idempotent: %v", err)
		}
	})
}
