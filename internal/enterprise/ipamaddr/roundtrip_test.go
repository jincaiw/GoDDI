package ipamaddr

import (
	"math/rand"
	"net/netip"
	"testing"
)

func randAddr(r *rand.Rand, v4 bool) netip.Addr {
	if v4 {
		var b [4]byte
		r.Read(b[:])
		return netip.AddrFrom4(b)
	}
	var b [16]byte
	r.Read(b[:])
	return netip.AddrFrom16(b)
}

// TestAddrEncodeDecodeRoundTrip verifies Decode(Encode(a)) == a for many random
// canonical addresses. A mismatch here would corrupt stored addresses on write
// or read.
func TestAddrEncodeDecodeRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(20261108))
	for i := 0; i < 40000; i++ {
		a := randAddr(r, i%2 == 0)
		if !a.IsValid() || a.Is4In6() {
			continue // validateAddr rejects mapped IPv6; not a canonical input
		}
		f, enc, err := Encode(a)
		if err != nil {
			t.Fatalf("encode %v: %v", a, err)
		}
		got, err := Decode(f, enc[:])
		if err != nil || got != a {
			t.Fatalf("round-trip %v -> %v (err=%v)", a, got, err)
		}
	}
}

// TestPrefixEncodeDecodeRoundTrip verifies DecodePrefix(EncodePrefix(p)) == p and
// Bounds consistency for many random canonical prefixes.
func TestPrefixEncodeDecodeRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(20261109))
	for i := 0; i < 40000; i++ {
		v4 := i%2 == 0
		base := randAddr(r, v4)
		if !base.IsValid() || base.Is4In6() {
			continue
		}
		maxBits := 128
		if v4 {
			maxBits = 32
		}
		p, perr := base.Prefix(r.Intn(maxBits + 1))
		if perr != nil {
			continue
		}
		p = p.Masked()
		if err := validateAddr(p.Addr()); err != nil {
			continue
		}
		f, first, last, err := EncodePrefix(p)
		if err != nil {
			t.Fatalf("encode prefix %v: %v", p, err)
		}
		got, err := DecodePrefix(f, first[:], last[:], p.Bits())
		if err != nil || got != p {
			t.Fatalf("prefix round-trip %v -> %v (err=%v)", p, got, err)
		}
		bf, _, err := Bounds(p)
		if err != nil || bf != p.Addr() {
			t.Fatalf("bounds first for %v: %v (err=%v)", p, bf, err)
		}
		if dfirst, err := Decode(f, first[:]); err != nil || dfirst != p.Addr() {
			t.Fatalf("first endpoint decode %v -> %v (err=%v)", p, dfirst, err)
		}
	}
}

// TestDecodeRejectsMalformed confirms the storage decoder rejects bad padding,
// wrong length and unknown families rather than silently accepting them.
func TestDecodeRejectsMalformed(t *testing.T) {
	var bad4 [16]byte
	bad4[0] = 1 // IPv4 requires twelve zero padding bytes
	if _, err := Decode(IPv4, bad4[:]); err == nil {
		t.Fatal("IPv4 nonzero padding accepted")
	}
	if _, err := Decode(IPv4, make([]byte, 15)); err == nil {
		t.Fatal("short address accepted")
	}
	if _, err := Decode(Family(7), make([]byte, 16)); err == nil {
		t.Fatal("unknown family accepted")
	}
}
