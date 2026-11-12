package timewater

import (
	"errors"
	"math/rand"
	"testing"
)

// TestAdvanceMonotonicProperty drives random Advance calls and checks the
// watermark invariants: Read never decreases, a lower Advance is refused with
// ErrRollback and leaves the value unchanged, a higher/equal Advance is stored,
// and the value survives reopening the store from its directory.
func TestAdvanceMonotonicProperty(t *testing.T) {
	dir := privateDir(t)
	s := NewStore(dir)
	if err := s.Bootstrap(1); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	r := rand.New(rand.NewSource(94))
	want := int64(1)
	for i := 0; i < 150; i++ {
		n := int64(r.Intn(100000)) + 1
		err := s.Advance(n)
		if n < want {
			if !errors.Is(err, ErrRollback) {
				t.Fatalf("Advance(%d) with watermark %d: got %v, want ErrRollback", n, want, err)
			}
		} else {
			if err != nil {
				t.Fatalf("Advance(%d) >= %d: unexpected error %v", n, want, err)
			}
			want = n
		}
		got, e := s.Read()
		if e != nil || got != want {
			t.Fatalf("after Advance(%d): Read = %d (err %v), want %d", n, got, e, want)
		}
	}

	// Durability: a fresh Store over the same directory sees the same value.
	s2 := NewStore(dir)
	if got, e := s2.Read(); e != nil || got != want {
		t.Fatalf("reopened Read = %d (err %v), want %d", got, e, want)
	}

	// A strictly lower value is always refused after reopen.
	if err := s2.Advance(want - 1); !errors.Is(err, ErrRollback) {
		t.Fatalf("Advance(want-1) after reopen: got %v, want ErrRollback", err)
	}
}
