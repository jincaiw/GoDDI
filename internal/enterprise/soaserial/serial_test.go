package soaserial

import (
	"errors"
	"math/rand"
	"testing"
)

func TestCompareBoundaries(t *testing.T) {
	for _, tc := range []struct {
		a, b      uint32
		want      int
		ambiguous bool
	}{
		{0, 0, 0, false}, {0, 1, -1, false}, {1, 0, 1, false},
		{^uint32(0), 0, -1, false}, {0, ^uint32(0), 1, false},
		{0, half - 1, -1, false}, {0, half + 1, 1, false},
		{0, half, 0, true}, {half, 0, 0, true}, {1, half + 1, 0, true},
	} {
		got, err := Compare(tc.a, tc.b)
		if got != tc.want || errors.Is(err, ErrAmbiguous) != tc.ambiguous || (err != nil && !tc.ambiguous) {
			t.Errorf("Compare(%d,%d)=%d,%v", tc.a, tc.b, got, err)
		}
	}
}

func TestNextBoundaries(t *testing.T) {
	for _, tc := range []struct {
		start, step, want uint32
		invalid           bool
	}{
		{0, 1, 1, false}, {^uint32(0), 1, 0, false}, {^uint32(0) - 1, 2, 0, false},
		{^uint32(0), half - 1, half - 2, false}, {5, 0, 0, true}, {5, half, 0, true}, {5, ^uint32(0), 0, true},
	} {
		got, err := Next(tc.start, tc.step)
		if got != tc.want || errors.Is(err, ErrIncrement) != tc.invalid || (err != nil && !tc.invalid) {
			t.Errorf("Next(%d,%d)=%d,%v", tc.start, tc.step, got, err)
		}
	}
}

func TestRandomSymmetryAndOrderedAdvances(t *testing.T) {
	r := rand.New(rand.NewSource(1982))
	for i := 0; i < 10000; i++ {
		a, b := r.Uint32(), r.Uint32()
		ab, e1 := Compare(a, b)
		ba, e2 := Compare(b, a)
		if errors.Is(e1, ErrAmbiguous) != errors.Is(e2, ErrAmbiguous) || (e1 == nil && (e2 != nil || ab != -ba)) {
			t.Fatalf("asymmetry %d %d: %d %v / %d %v", a, b, ab, e1, ba, e2)
		}
		step := r.Uint32()%(half-1) + 1
		next, err := Next(a, step)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Compare(a, next)
		if err != nil || got != -1 {
			t.Fatalf("step %d+%d: %d %v", a, step, got, err)
		}
		if next != a+step {
			t.Fatalf("unexpected modulo result")
		}
		// A pair of advances whose total is below half retains strict ordering.
		firstStep := r.Uint32()%(half-2) + 1
		secondStep := r.Uint32()%(half-firstStep-1) + 1
		mid, _ := Next(a, firstStep)
		end, _ := Next(mid, secondStep)
		if ab, _ := Compare(a, mid); ab != -1 {
			t.Fatal("first step not ordered")
		}
		if bc, _ := Compare(mid, end); bc != -1 {
			t.Fatal("second step not ordered")
		}
		if ac, _ := Compare(a, end); ac != -1 {
			t.Fatal("bounded cumulative step not ordered")
		}
	}
}
