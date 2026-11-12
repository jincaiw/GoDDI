package dhcpdeadline

import (
	"errors"
	"math"
	"math/big"
	"math/rand"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/trustedtime"
)

func TestBoundaries(t *testing.T) {
	const s int64 = 1_000_000_000
	for _, tc := range []struct {
		name            string
		expiry          int64
		b               trustedtime.Bounds
		source          error
		clock, delivery int64
		want            uint32
		ok              bool
	}{
		{"exact second", 2*s + 1, trustedtime.Bounds{Lower: 1, Upper: s + 1}, nil, 0, 0, 1, true},
		{"fraction rounds down", 3 * s, trustedtime.Bounds{Lower: 1, Upper: s}, nil, 1, 0, 1, true},
		{"margin both", 5 * s, trustedtime.Bounds{Lower: s, Upper: 2 * s}, nil, s / 2, s / 2, 2, true},
		{"subsecond", 2*s - 1, trustedtime.Bounds{Lower: 1, Upper: s}, nil, 0, 0, 0, false},
		{"expired", s, trustedtime.Bounds{Lower: s, Upper: s}, nil, 0, 0, 0, false},
		{"bad bounds", 3 * s, trustedtime.Bounds{Lower: 2 * s, Upper: s}, nil, 0, 0, 0, false},
		{"untrusted", 3 * s, trustedtime.Bounds{Lower: 1, Upper: s}, errors.New("clock"), 0, 0, 0, false},
		{"missing", 3 * s, trustedtime.Bounds{}, nil, 0, 0, 0, false},
		{"negative now", 3 * s, trustedtime.Bounds{Lower: math.MinInt64, Upper: 1}, nil, 0, 0, 0, false},
		{"negative expiry", math.MinInt64, trustedtime.Bounds{Lower: 1, Upper: 1}, nil, 0, 0, 0, false},
		{"negative margin", 3 * s, trustedtime.Bounds{Lower: 1, Upper: s}, nil, -1, 0, 0, false},
		{"huge margin", math.MaxInt64, trustedtime.Bounds{Lower: 1, Upper: 1}, nil, math.MaxInt64, math.MaxInt64, 0, false},
		{"max finite", int64(math.MaxUint32-1)*s + 1, trustedtime.Bounds{Lower: 1, Upper: 1}, nil, 0, 0, math.MaxUint32 - 1, true},
		{"infinite rejected", int64(math.MaxUint32)*s + 1, trustedtime.Bounds{Lower: 1, Upper: 1}, nil, 0, 0, 0, false},
		{"int64 maximum duration", math.MaxInt64, trustedtime.Bounds{Lower: 1, Upper: 1}, nil, 0, 0, 0, false},
		{"one before max", math.MaxInt64, trustedtime.Bounds{Lower: math.MaxInt64 - s, Upper: math.MaxInt64 - s}, nil, 0, 0, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Seconds(tc.expiry, tc.b, tc.source, tc.clock, tc.delivery)
			if (err == nil) != tc.ok || got != tc.want {
				t.Fatalf("got %d %v", got, err)
			}
		})
	}
}
func TestRandomAgainstBigInteger(t *testing.T) {
	r := rand.New(rand.NewSource(12345))
	billion := big.NewInt(nanosPerSecond)
	limit := big.NewInt(math.MaxUint32)
	for i := 0; i < 20000; i++ {
		lower := r.Int63()
		upper := r.Int63()
		expiry := r.Int63()
		cm := r.Int63()
		dm := r.Int63()
		if i%2 == 0 {
			lower = r.Int63n(1_000_000)
			upper = lower + r.Int63n(1_000_000)
			expiry = upper + r.Int63n(8_000_000_000)
			cm = r.Int63n(1_000_000_000)
			dm = r.Int63n(1_000_000_000)
		}
		got, err := Seconds(expiry, trustedtime.Bounds{Lower: lower, Upper: upper}, nil, cm, dm)
		x := new(big.Int).SetInt64(expiry)
		x.Sub(x, big.NewInt(upper))
		x.Sub(x, big.NewInt(cm))
		x.Sub(x, big.NewInt(dm))
		x.Quo(x, billion)
		valid := lower > 0 && upper >= lower && x.Sign() > 0 && x.Cmp(limit) < 0
		if valid {
			if err != nil || int64(got) != x.Int64() {
				t.Fatalf("%d got %d %v want %s", i, got, err, x)
			}
		} else if err == nil || got != 0 {
			t.Fatalf("%d unexpected %d %v", i, got, err)
		}
	}
}
