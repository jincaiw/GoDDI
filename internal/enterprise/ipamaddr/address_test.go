package ipamaddr

import (
	"encoding/hex"
	"errors"
	"net/netip"
	"testing"
)

func TestAddressEncoding(t *testing.T) {
	tests := []struct {
		input   string
		family  Family
		encoded string
	}{
		{"192.0.2.10", IPv4, "000000000000000000000000c000020a"},
		{"0.0.0.0", IPv4, "00000000000000000000000000000000"},
		{"255.255.255.255", IPv4, "000000000000000000000000ffffffff"},
		{"2001:db8::1", IPv6, "20010db8000000000000000000000001"},
		{"::", IPv6, "00000000000000000000000000000000"},
		{"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", IPv6, "ffffffffffffffffffffffffffffffff"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			a, err := ParseAddr(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			f, b, err := Encode(a)
			if err != nil || f != tt.family || hex.EncodeToString(b[:]) != tt.encoded {
				t.Fatalf("Encode: %v %x %v", f, b, err)
			}
			decoded, err := Decode(f, b[:])
			if err != nil || decoded != a {
				t.Fatalf("Decode: %v %v", decoded, err)
			}
		})
	}
}

func TestInvalidAddresses(t *testing.T) {
	for _, s := range []string{"127.1", "192.168.001.1", "::ffff:192.0.2.1", "fe80::1%eth0", "", "192.0.2.256"} {
		if _, err := ParseAddr(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
	for _, a := range []netip.Addr{netip.Addr{}, netip.MustParseAddr("::ffff:192.0.2.1"), netip.MustParseAddr("fe80::1%eth0")} {
		if _, _, err := Encode(a); err == nil {
			t.Errorf("encoded %v", a)
		}
	}
	if _, err := Decode(IPv4, make([]byte, 15)); err == nil {
		t.Error("accepted wrong length")
	}
	b := make([]byte, 16)
	b[0] = 1
	if _, err := Decode(IPv4, b); err == nil {
		t.Error("accepted nonzero IPv4 padding")
	}
	if _, err := Decode(7, b); err == nil {
		t.Error("accepted invalid family")
	}
	mapped := netip.MustParseAddr("::ffff:192.0.2.1").As16()
	if _, err := Decode(IPv6, mapped[:]); err == nil {
		t.Error("accepted mapped IPv6 blob")
	}
}

func TestPrefixBoundsAndDecode(t *testing.T) {
	cases := []struct{ prefix, last string }{
		{"0.0.0.0/0", "255.255.255.255"}, {"192.0.2.0/25", "192.0.2.127"},
		{"192.0.2.2/31", "192.0.2.3"}, {"192.0.2.1/32", "192.0.2.1"},
		{"::/0", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"},
		{"2001:db8::/65", "2001:db8::7fff:ffff:ffff:ffff"},
		{"2001:db8::2/127", "2001:db8::3"}, {"2001:db8::1/128", "2001:db8::1"},
	}
	for _, tt := range cases {
		t.Run(tt.prefix, func(t *testing.T) {
			p, err := ParsePrefix(tt.prefix)
			if err != nil {
				t.Fatal(err)
			}
			first, last, err := Bounds(p)
			if err != nil || first != p.Addr() || last.String() != tt.last {
				t.Fatalf("bounds %v %v %v", first, last, err)
			}
			f, fb, _ := Encode(first)
			_, lb, _ := Encode(last)
			got, err := DecodePrefix(f, fb[:], lb[:], p.Bits())
			if err != nil || got != p {
				t.Fatalf("DecodePrefix %v %v", got, err)
			}
			lb[15] ^= 1
			if _, err = DecodePrefix(f, fb[:], lb[:], p.Bits()); err == nil {
				t.Error("accepted inconsistent endpoint")
			}
		})
	}
}

func TestPrefixWithMappedEndpoint(t *testing.T) {
	for _, cidr := range []string{"::fffe:0:0/95", "::fffc:0:0/94"} {
		t.Run(cidr, func(t *testing.T) {
			p, err := ParsePrefix(cidr)
			if err != nil {
				t.Fatal(err)
			}
			first, last, err := Bounds(p)
			if err != nil || !last.Is4In6() {
				t.Fatalf("bounds: %v %v %v", first, last, err)
			}
			fb, lb := first.As16(), last.As16()
			got, err := DecodePrefix(IPv6, fb[:], lb[:], p.Bits())
			if err != nil || got != p {
				t.Fatalf("decode: %v %v", got, err)
			}
			if _, err := Decode(IPv6, lb[:]); err == nil {
				t.Fatal("mapped host accepted")
			}
			lb[15] ^= 1
			if _, err := DecodePrefix(IPv6, fb[:], lb[:], p.Bits()); err == nil {
				t.Fatal("bad endpoint accepted")
			}
		})
	}
}

func TestEncodePrefixForStorage(t *testing.T) {
	for _, tc := range []struct {
		cidr, first, last string
		family            Family
	}{
		{"::fffe:0:0/95", "00000000000000000000fffe00000000", "00000000000000000000ffffffffffff", IPv6},
		{"192.0.2.0/25", "000000000000000000000000c0000200", "000000000000000000000000c000027f", IPv4},
		{"::/0", "00000000000000000000000000000000", "ffffffffffffffffffffffffffffffff", IPv6},
	} {
		t.Run(tc.cidr, func(t *testing.T) {
			p, err := ParsePrefix(tc.cidr)
			if err != nil {
				t.Fatal(err)
			}
			family, first, last, err := EncodePrefix(p)
			if err != nil || family != tc.family || hex.EncodeToString(first[:]) != tc.first || hex.EncodeToString(last[:]) != tc.last {
				t.Fatalf("encode: family=%d first=%x last=%x err=%v", family, first, last, err)
			}
			got, err := DecodePrefix(family, first[:], last[:], p.Bits())
			if err != nil || got != p {
				t.Fatalf("round trip: %v %v", got, err)
			}
		})
	}
	for _, p := range []netip.Prefix{netip.Prefix{}, netip.MustParsePrefix("192.0.2.1/24"), netip.MustParsePrefix("::ffff:192.0.2.0/120")} {
		if _, _, _, err := EncodePrefix(p); err == nil {
			t.Errorf("accepted invalid prefix %v", p)
		}
	}
	if _, _, err := Encode(netip.MustParseAddr("::ffff:192.0.2.1")); err == nil {
		t.Fatal("accepted mapped host")
	}
}

func TestInvalidPrefixes(t *testing.T) {
	for _, s := range []string{"192.0.2.1", "192.0.2.1/24", "2001:db8::1/64", "::ffff:192.0.2.0/120", "fe80::1%eth0/64", "192.0.2.0/33", "::/129"} {
		if _, err := ParsePrefix(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
	_, err := ParsePrefix("192.0.2.5/24")
	var noncanonical *NonCanonicalPrefixError
	if !errors.As(err, &noncanonical) || noncanonical.Suggested.String() != "192.0.2.0/24" {
		t.Fatalf("missing correction: %v", err)
	}
	_, b, _ := Encode(netip.MustParseAddr("192.0.2.1"))
	if _, err := DecodePrefix(IPv4, b[:], b[:], 33); err == nil {
		t.Error("accepted invalid prefix length")
	}
	if _, err := DecodePrefix(IPv4, b[:], b[:], 24); !errors.As(err, &noncanonical) {
		t.Errorf("accepted host bits: %v", err)
	}
}
