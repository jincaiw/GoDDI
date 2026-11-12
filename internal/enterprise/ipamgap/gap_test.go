package ipamgap

import (
	"errors"
	"math/rand"
	"net/netip"
	"sort"
	"testing"
)

func addr(s string) Address {
	a := netip.MustParseAddr(s)
	if a.Is4() {
		b := a.As4()
		var r Address
		copy(r[12:], b[:])
		return r
	}
	return Address(a.As16())
}
func span(a, b string) Interval { return Interval{addr(a), addr(b)} }
func small(n int) Address       { var a Address; a[14] = byte(n >> 8); a[15] = byte(n); return a }

func TestFirst(t *testing.T) {
	cases := []struct {
		name   string
		family int
		scope  Interval
		used   []Interval
		budget int
		want   Address
		err    error
	}{
		{"empty", 4, span("0.0.0.0", "0.0.0.2"), nil, 0, addr("0.0.0.0"), nil},
		{"merge overlap adjacent", 4, span("10.0.0.0", "10.0.0.9"), []Interval{span("10.0.0.0", "10.0.0.2"), span("10.0.0.1", "10.0.0.4"), span("10.0.0.5", "10.0.0.6")}, 3, addr("10.0.0.7"), nil},
		{"clipped", 4, span("10.0.0.2", "10.0.0.9"), []Interval{span("10.0.0.0", "10.0.0.4")}, 1, addr("10.0.0.5"), nil},
		{"ipv4 maximum full", 4, span("255.255.255.254", "255.255.255.255"), []Interval{span("255.255.255.254", "255.255.255.255")}, 1, Address{}, ErrFull},
		{"ipv4 maximum gap", 4, span("255.255.255.254", "255.255.255.255"), []Interval{span("255.255.255.254", "255.255.255.254")}, 1, addr("255.255.255.255"), nil},
		{"cross 64", 6, span("2001:db8::ffff:ffff:ffff:fffe", "2001:db8:0:1::2"), []Interval{span("2001:db8::ffff:ffff:ffff:fffe", "2001:db8::ffff:ffff:ffff:ffff")}, 1, addr("2001:db8:0:1::"), nil},
		{"ipv6 maximum full", 6, span("ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"), []Interval{span("ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")}, 1, Address{}, ErrFull},
		{"budget exhausted", 4, span("0.0.0.0", "0.0.0.9"), []Interval{span("0.0.0.0", "0.0.0.0"), span("0.0.0.1", "0.0.0.1")}, 1, Address{}, ErrBudget},
		{"zero budget", 4, span("0.0.0.0", "0.0.0.9"), []Interval{span("0.0.0.1", "0.0.0.1")}, 0, Address{}, ErrBudget},
		{"invalid family", 5, span("0.0.0.0", "0.0.0.1"), nil, 1, Address{}, ErrInvalid},
		{"invalid v4", 4, span("::1:0:0", "::1:0:1"), nil, 1, Address{}, ErrInvalid},
		{"reverse", 4, span("0.0.0.1", "0.0.0.0"), nil, 1, Address{}, ErrInvalid},
		{"outside", 4, span("0.0.0.0", "0.0.0.1"), []Interval{span("0.0.0.2", "0.0.0.3")}, 1, Address{}, ErrInvalid},
		{"unsorted", 4, span("0.0.0.0", "0.0.0.9"), []Interval{span("0.0.0.0", "0.0.0.2"), span("0.0.0.0", "0.0.0.0"), span("0.0.0.0", "0.0.0.1")}, 3, addr("0.0.0.3"), nil},
		{"descending", 4, span("0.0.0.0", "0.0.0.9"), []Interval{span("0.0.0.0", "0.0.0.3"), span("0.0.0.1", "0.0.0.2"), span("0.0.0.0", "0.0.0.1")}, 3, Address{}, ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := First(tc.family, tc.scope, tc.used, tc.budget)
			if !errors.Is(err, tc.err) || got != tc.want {
				t.Fatalf("got %v, %v; want %v, %v", got, err, tc.want, tc.err)
			}
		})
	}
}

func TestRandomAgainstNaive(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for iteration := 0; iteration < 5000; iteration++ {
		n := 1 + rng.Intn(256)
		count := rng.Intn(80)
		used := make([]Interval, 0, count)
		occupied := make([]bool, n)
		for j := 0; j < count; j++ {
			a, b := rng.Intn(n), rng.Intn(n)
			if a > b {
				a, b = b, a
			}
			used = append(used, Interval{small(a), small(b)})
			for k := a; k <= b; k++ {
				occupied[k] = true
			}
		}
		sort.Slice(used, func(i, j int) bool { return compare(used[i].First, used[j].First) < 0 })
		want := -1
		for i, v := range occupied {
			if !v {
				want = i
				break
			}
		}
		for _, family := range []int{4, 6} {
			got, err := First(family, Interval{small(0), small(n - 1)}, used, len(used))
			if want < 0 {
				if !errors.Is(err, ErrFull) {
					t.Fatalf("iteration %d: expected full, got %v", iteration, err)
				}
			} else if err != nil || got != small(want) {
				t.Fatalf("iteration %d: got %v %v, want %d", iteration, got, err, want)
			}
		}
	}
}
