package ipamgap

import (
	"errors"
	"math/rand"
	"sort"
	"testing"
)

// v4 builds a valid IPv4 Address encoding a value 0..2^32-1 with the first
// twelve bytes zero and the last four big-endian.
func v4(v uint32) Address {
	var a Address
	a[12] = byte(v >> 24)
	a[13] = byte(v >> 16)
	a[14] = byte(v >> 8)
	a[15] = byte(v)
	return a
}

// bruteFirst is an independent reference for a small IPv4 interval space.
func bruteFirst(scopeA, scopeB uint32, used []Interval) (Address, error) {
	for x := scopeA; ; x++ {
		covered := false
		for _, r := range used {
			if compare(v4(x), r.First) >= 0 && compare(v4(x), r.Last) <= 0 {
				covered = true
				break
			}
		}
		if !covered {
			return v4(x), nil
		}
		if x == scopeB {
			return Address{}, ErrFull
		}
	}
}

// TestFirstMatchesBruteForce checks First against an independent brute-force
// reference over many random small ranges with overlapping/adjacent intervals.
func TestFirstMatchesBruteForce(t *testing.T) {
	r := rand.New(rand.NewSource(20261108))
	const base = uint32(0x0A000000) // 10.0.0.0
	for iter := 0; iter < 5000; iter++ {
		a := base + uint32(r.Intn(240))
		b := a + uint32(r.Intn(16))
		scope := Interval{First: v4(a), Last: v4(b)}
		n := r.Intn(6)
		var used []Interval
		for i := 0; i < n; i++ {
			f := a + uint32(r.Intn(int(b-a)+1))
			l := f + uint32(r.Intn(int(b-f)+1))
			used = append(used, Interval{First: v4(f), Last: v4(l)})
		}
		sort.Slice(used, func(i, j int) bool { return compare(used[i].First, used[j].First) < 0 })

		want, wantErr := bruteFirst(a, b, used)
		got, gotErr := First(4, scope, used, len(used)+1)
		if errors.Is(wantErr, ErrFull) != errors.Is(gotErr, ErrFull) {
			t.Fatalf("iter %d scope[%d,%d] used=%v: got %v,%v want %v,%v", iter, a, b, used, got, gotErr, want, wantErr)
		}
		if wantErr == nil {
			if gotErr != nil {
				t.Fatalf("iter %d scope[%d,%d] used=%v: unexpected err %v", iter, a, b, used, gotErr)
			}
			if got != want {
				t.Fatalf("iter %d scope[%d,%d] used=%v: got %v want %v", iter, a, b, used, got, want)
			}
		}
	}
}

// TestFirstBudgetIsStrict checks that an exhausted budget returns ErrBudget
// rather than a possibly-wrong answer, and that a sufficient budget succeeds.
func TestFirstBudgetIsStrict(t *testing.T) {
	scope := Interval{First: v4(0), Last: v4(9)}
	used := []Interval{{First: v4(0), Last: v4(0)}, {First: v4(1), Last: v4(1)}, {First: v4(2), Last: v4(2)}}
	if _, err := First(4, scope, used, 2); !errors.Is(err, ErrBudget) {
		t.Fatalf("budget 2 over 3 intervals: err=%v want ErrBudget", err)
	}
	got, err := First(4, scope, used, 3)
	if err != nil || got != v4(3) {
		t.Fatalf("budget 3: got %v,%v want v4(3)", got, err)
	}
}
