package snapshotrows

import (
	"errors"
	"strings"
	"testing"
)

func limits() Limits { return Limits{LineBytes: 1024, TotalBytes: 4096, Rows: 20, Tables: 4} }
func makeValidator(t *testing.T, tables []Table) *Validator {
	t.Helper()
	v, e := New(tables, limits())
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestChunksUTF8AndCallbackOwnership(t *testing.T) {
	var kept [][]byte
	fn := func(row []byte) error { kept = append(kept, row); return nil }
	v := makeValidator(t, []Table{{"a", 2, fn}, {"b", 1, fn}})
	raw := []byte("{\"table\":\"a\",\"row\":{\"name\":\"中文\"}}\n{\"table\":\"a\",\"row\":{\"n\":2}}\n{\"table\":\"b\",\"row\":{}}\n")
	for i := range raw {
		if e := v.Feed(raw[i : i+1]); e != nil {
			t.Fatal(i, e)
		}
	}
	result, e := v.Finish()
	if e != nil || !result.StructureAndCountsValid || !result.AllRowCallbacksPassed || result.Rows != 3 || result.Bytes != uint64(len(raw)) {
		t.Fatal(result, e)
	}
	if string(kept[0]) != "{\"name\":\"中文\"}" || string(kept[1]) != "{\"n\":2}" || string(kept[2]) != "{}" {
		t.Fatalf("callbacks alias: %q", kept)
	}
	kept[0][0] = 'x'
	if string(kept[1]) != "{\"n\":2}" {
		t.Fatal("shared callback bytes")
	}
	result.Tables[0].Name = "modified"
	if e = v.Feed(nil); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if _, e = v.Finish(); !errors.Is(e, ErrDirty) {
		t.Fatal(e)
	}
}
func TestInvalidStreamsArePermanentlyDirty(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		finish     bool
	}{
		{"duplicate root", `{"table":"a","table":"a","row":{}}` + "\n", false},
		{"nested duplicate", `{"table":"a","row":{"x":[{"key":1,"key":2}]}}` + "\n", false},
		{"escaped duplicate", `{"table":"a","row":{"x":1,"\u0078":2}}` + "\n", false},
		{"root array", `[]` + "\n", false}, {"row array", `{"table":"a","row":[]}` + "\n", false},
		{"unknown field", `{"table":"a","row":{},"extra":1}` + "\n", false},
		{"missing row", `{"table":"a"}` + "\n", false},
		{"unknown table", `{"table":"other","row":{}}` + "\n", false},
		{"trailing json", `{"table":"a","row":{}} {}` + "\n", false},
		{"malformed", `{"table":"a","row":{` + "\n", false},
		{"truncated no lf", `{"table":"a","row":{}`, true},
		{"missing lf", `{"table":"a","row":{}}`, true},
		{"bom", "\xef\xbb\xbf" + `{"table":"a","row":{}}` + "\n", false},
		{"invalid utf8", "{\"table\":\"a\",\"row\":{\"x\":\"\xff\"}}\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := makeValidator(t, []Table{{Name: "a", Count: 1}})
			e := v.Feed([]byte(tc.data))
			if tc.finish {
				if e != nil {
					t.Fatal(e)
				}
				_, e = v.Finish()
			}
			if e == nil {
				t.Fatal("accepted invalid")
			}
			if e = v.Feed(nil); !errors.Is(e, ErrDirty) {
				t.Fatalf("not dirty: %v", e)
			}
			if _, e = v.Finish(); !errors.Is(e, ErrDirty) {
				t.Fatal(e)
			}
		})
	}
}
func TestUnicodeScalarEscapesAcrossChunks(t *testing.T) {
	for _, tc := range []struct {
		name, row string
		valid     bool
	}{
		{"lone high nested value", `{"nested":[{"x":"\uD800"}]}`, false},
		{"lone low nested key", `{"nested":{"\uDC00":1}}`, false},
		{"two highs", `{"x":"\uD800\uD800"}`, false},
		{"high then ordinary", `{"x":"\uD800\u0041"}`, false},
		{"pair value", `{"x":"\uD83D\uDE00"}`, true},
		{"pair nested key", `{"nested":{"\uD800\uDC00":1}}`, true},
		{"literal replacement", `{"x":"�"}`, true},
		{"escaped replacement", `{"x":"\uFFFD"}`, true},
		{"escaped slash", `{"x":"\\uD800"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := makeValidator(t, []Table{{Name: "a", Count: 1}})
			data := []byte(`{"table":"a","row":` + tc.row + "}\n")
			var err error
			for i := range data {
				if err = v.Feed(data[i : i+1]); err != nil {
					break
				}
			}
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
				if _, err = v.Finish(); err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil {
					t.Fatal("accepted invalid surrogate")
				}
				if _, err = v.Finish(); !errors.Is(err, ErrDirty) {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestOrderAndExactCounts(t *testing.T) {
	a := "{\"table\":\"a\",\"row\":{}}\n"
	b := "{\"table\":\"b\",\"row\":{}}\n"
	for _, data := range []string{b + a, a + a, a} {
		v := makeValidator(t, []Table{{Name: "a", Count: 1}, {Name: "b", Count: 1}})
		e := v.Feed([]byte(data))
		if e == nil {
			_, e = v.Finish()
		}
		if e == nil {
			t.Fatalf("accepted %q", data)
		}
	}
	v := makeValidator(t, []Table{{Name: "a", Count: 1}, {Name: "b", Count: 1}})
	if e := v.Feed([]byte(a + b)); e != nil {
		t.Fatal(e)
	}
	r, e := v.Finish()
	if e != nil || r.AllRowCallbacksPassed || !r.StructureAndCountsValid {
		t.Fatal(r, e)
	}
}
func TestLimitsAndCallbackFailure(t *testing.T) {
	l := limits()
	l.LineBytes = 10
	v, e := New([]Table{{Name: "a", Count: 1}}, l)
	if e != nil {
		t.Fatal(e)
	}
	if e = v.Feed([]byte(strings.Repeat("x", 11))); e == nil {
		t.Fatal("line limit")
	}
	l = limits()
	l.TotalBytes = 5
	v, e = New([]Table{{Name: "a", Count: 1}}, l)
	if e != nil {
		t.Fatal(e)
	}
	if e = v.Feed([]byte("123456")); e == nil {
		t.Fatal("total limit")
	}
	v = makeValidator(t, []Table{{Name: "a", Count: 1, ValidateRow: func([]byte) error { return errors.New("bad row") }}})
	if e = v.Feed([]byte("{\"table\":\"a\",\"row\":{}}\n")); e == nil {
		t.Fatal("callback error")
	}
	if _, e = v.Finish(); !errors.Is(e, ErrDirty) {
		t.Fatal(e)
	}
	for _, bad := range []Limits{{}, {LineBytes: MaxLineBytes + 1, TotalBytes: 1, Rows: 1, Tables: 1}, {LineBytes: 1, TotalBytes: MaxTotalBytes + 1, Rows: 1, Tables: 1}, {LineBytes: 1, TotalBytes: 1, Rows: MaxRows + 1, Tables: 1}, {LineBytes: 1, TotalBytes: 1, Rows: 1, Tables: MaxTables + 1}} {
		if _, e = New([]Table{{Name: "a"}}, bad); e == nil {
			t.Fatal("bad bounds", bad)
		}
	}
	if _, e = New([]Table{{Name: "a"}, {Name: "a"}}, limits()); e == nil {
		t.Fatal("duplicate table")
	}
}

func TestCallbackPanicPoisonsStandaloneValidator(t *testing.T) {
	sentinel := &struct{ message string }{"callback panic"}
	var effects int
	v := makeValidator(t, []Table{{Name: "a", Count: 1, ValidateRow: func([]byte) error { effects++; panic(sentinel) }}})
	func() {
		defer func() {
			if got := recover(); got != sentinel {
				t.Fatalf("panic changed: %v", got)
			}
		}()
		_ = v.Feed([]byte("{\"table\":\"a\",\"row\":{}}\n"))
		t.Fatal("callback did not panic")
	}()
	if effects != 1 {
		t.Fatalf("callback calls %d", effects)
	}
	if err := v.Feed([]byte("\n")); !errors.Is(err, ErrDirty) {
		t.Fatalf("Feed after panic: %v", err)
	}
	if _, err := v.Finish(); !errors.Is(err, ErrDirty) {
		t.Fatalf("Finish after panic: %v", err)
	}
	if effects != 1 {
		t.Fatalf("callback reran: %d", effects)
	}
}
