package dnsanswer

import (
	"strings"
	"testing"
)

// TestSimpleNameBoundaries locks in the byte-length and label limits of the
// wire-name validator. These are security-relevant: an off-by-one at the 63-byte
// label or 254-byte name boundary, or acceptance of a missing trailing dot, an
// empty label, or a non-ASCII byte, could admit malformed owner names.
func TestSimpleNameBoundaries(t *testing.T) {
	build := func(labels ...int) string {
		parts := make([]string, len(labels))
		for i, n := range labels {
			parts[i] = strings.Repeat("a", n)
		}
		return strings.Join(parts, ".") + "."
	}

	cases := []struct {
		name string
		want bool
	}{
		{".", true},                            // root
		{build(1), true},                       // single 1-byte label, len 2
		{strings.Repeat("a", 63) + ".", true},  // label 63 is allowed
		{strings.Repeat("a", 64) + ".", false}, // label 64 exceeds limit
		{build(63, 63, 62, 62), true},          // total name length exactly 254
		{build(63, 63, 62, 63), false},         // total name length 255 (too long)
		{"a..b.", false},                       // empty label
		{"ab", false},                          // missing trailing dot
		{"a b.", false},                        // space is not allowed
		{"a/b.", false},                        // slash is not allowed
		{"\u00e9.", false},                     // non-ASCII byte rejected
		{build(3), true},                       // ordinary label
	}
	for _, tc := range cases {
		if got := simpleName(tc.name); got != tc.want {
			t.Errorf("simpleName(%q)=%v want %v (len=%d)", tc.name, got, tc.want, len(tc.name))
		}
	}
}
