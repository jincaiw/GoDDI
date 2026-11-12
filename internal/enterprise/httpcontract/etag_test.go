package httpcontract

import (
	"errors"
	"math"
	"net/http"
	"strings"
	"testing"
)

func TestParseIfMatch(t *testing.T) {
	for _, tt := range []struct {
		name   string
		values []string
		want   int64
		err    error
	}{
		{"missing nil", nil, 0, ErrIfMatchMissing},
		{"missing empty slice", []string{}, 0, ErrIfMatchMissing},
		{"present empty", []string{""}, 0, ErrIfMatchInvalid},
		{"one", []string{`"v1"`}, 1, nil},
		{"max", []string{`"v9223372036854775807"`}, math.MaxInt64, nil},
		{"overflow", []string{`"v9223372036854775808"`}, 0, ErrIfMatchInvalid},
		{"very long", []string{`"v` + strings.Repeat("9", 10000) + `"`}, 0, ErrIfMatchInvalid},
		{"zero", []string{`"v0"`}, 0, ErrIfMatchInvalid},
		{"leading zero", []string{`"v01"`}, 0, ErrIfMatchInvalid},
		{"weak", []string{`W/"v1"`}, 0, ErrIfMatchInvalid},
		{"multiple lines", []string{`"v1"`, `"v1"`}, 0, ErrIfMatchInvalid},
		{"one empty line", []string{`"v1"`, ""}, 0, ErrIfMatchInvalid},
		{"combined", []string{`"v1", "v2"`}, 0, ErrIfMatchInvalid},
		{"wildcard", []string{"*"}, 0, ErrIfMatchInvalid},
		{"leading space", []string{` "v1"`}, 0, ErrIfMatchInvalid},
		{"trailing tab", []string{`"v1"` + "\t"}, 0, ErrIfMatchInvalid},
		{"inner space", []string{`"v 1"`}, 0, ErrIfMatchInvalid},
		{"no quotes", []string{"v1"}, 0, ErrIfMatchInvalid},
		{"negative", []string{`"v-1"`}, 0, ErrIfMatchInvalid},
		{"plus", []string{`"v+1"`}, 0, ErrIfMatchInvalid},
		{"unicode digit", []string{`"v１"`}, 0, ErrIfMatchInvalid},
		{"invalid UTF8", []string{string([]byte{'"', 'v', 0xff, '"'})}, 0, ErrIfMatchInvalid},
		{"newlines", []string{"\"v1\"\r\n"}, 0, ErrIfMatchInvalid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIfMatch(tt.values)
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Fatalf("got %d, %v; want %d, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestCheckAndFormat(t *testing.T) {
	for _, tt := range []struct {
		values  []string
		current int64
		status  int
		err     error
	}{
		{nil, 1, http.StatusPreconditionRequired, ErrIfMatchMissing},
		{[]string{""}, 1, http.StatusBadRequest, ErrIfMatchInvalid},
		{[]string{`"v1"`}, 2, http.StatusPreconditionFailed, ErrRevisionMismatch},
		{[]string{`"v1"`}, 1, 0, nil},
		{[]string{`"v1"`}, 0, http.StatusInternalServerError, ErrInvalidRevision},
	} {
		status, err := CheckIfMatch(tt.values, tt.current)
		if status != tt.status || !errors.Is(err, tt.err) {
			t.Fatalf("got %d, %v", status, err)
		}
	}
	for _, n := range []int64{1, 7, math.MaxInt64} {
		tag, err := FormatETag(n)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ParseIfMatch([]string{tag})
		if err != nil || got != n {
			t.Fatalf("roundtrip %d: %q %d %v", n, tag, got, err)
		}
	}
	for _, n := range []int64{0, -1, math.MinInt64} {
		if _, err := FormatETag(n); !errors.Is(err, ErrInvalidRevision) {
			t.Fatalf("revision %d: %v", n, err)
		}
	}
}

func TestMultipleHeaderLines(t *testing.T) {
	h := http.Header{}
	h.Add("If-Match", `"v1"`)
	h.Add("If-Match", `"v1"`)
	if _, err := ParseIfMatch(h.Values("If-Match")); !errors.Is(err, ErrIfMatchInvalid) {
		t.Fatalf("multiple lines: %v", err)
	}
}

func FuzzParseIfMatch(f *testing.F) {
	for _, seed := range []string{`"v1"`, `"v9223372036854775807"`, "", `W/"v1"`, `"v01"`, `"v1","v2"`, string([]byte{0xff})} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		n, err := ParseIfMatch([]string{input})
		if err != nil {
			if !errors.Is(err, ErrIfMatchInvalid) {
				t.Fatalf("unexpected error %v", err)
			}
			return
		}
		formatted, err := FormatETag(n)
		if err != nil || formatted != input {
			t.Fatalf("not canonical: %q => %d => %q (%v)", input, n, formatted, err)
		}
	})
}
