package snapshotrows

import (
	"errors"
	"testing"
)

func newFuzzValidator(tb testing.TB) *Validator {
	tb.Helper()
	v, err := New(
		[]Table{{Name: "t", Count: 1, ValidateRow: func([]byte) error { return nil }}},
		Limits{LineBytes: 4096, TotalBytes: 4096, Rows: 1, Tables: 1},
	)
	if err != nil {
		tb.Fatalf("construct validator: %v", err)
	}
	return v
}

// FuzzFeed feeds arbitrary bytes to the peer snapshot NDJSON stream parser and
// asserts two safety properties: it never panics, and once it reports an error
// it is permanently invalidated, so a later Feed returns ErrDirty rather than
// resuming on a corrupt stream.
func FuzzFeed(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("\n"))
	f.Add([]byte(`{"table":"t","row":{}}` + "\n"))
	f.Add([]byte(`{"table":"t","row":{"a":1}}` + "\n"))
	f.Add([]byte(`{"table":"t","row":`))
	f.Add([]byte("\xef\xbb\xbf{}\n"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		v := newFuzzValidator(t)
		if err := v.Feed(raw); err != nil {
			if next := v.Feed([]byte("x\n")); !errors.Is(next, ErrDirty) {
				t.Fatalf("validator not permanently dirty after error %v (next=%v)", err, next)
			}
			return
		}
		// A successful feed may still be mid-stream; Finish must not panic.
		_, _ = v.Finish()
	})
}
