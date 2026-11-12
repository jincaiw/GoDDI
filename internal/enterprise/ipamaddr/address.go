// Package ipamaddr implements the IPAM address representation contract.
package ipamaddr

import (
	"fmt"
	"net/netip"
)

// Family is the IP address family stored alongside a 16-byte address.
type Family uint8

const (
	IPv4 Family = 4
	IPv6 Family = 6
)

// NonCanonicalPrefixError provides a correction for a prefix with host bits set.
type NonCanonicalPrefixError struct{ Suggested netip.Prefix }

func (e *NonCanonicalPrefixError) Error() string {
	return fmt.Sprintf("CIDR has host bits set; use %s", e.Suggested)
}

// ParseAddr rejects scoped and IPv4-mapped IPv6 addresses.
func ParseAddr(s string) (netip.Addr, error) {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid IP address: %w", err)
	}
	if err = validateAddr(a); err != nil {
		return netip.Addr{}, err
	}
	return a, nil
}

func validateAddr(a netip.Addr) error {
	if !a.IsValid() || a.Zone() != "" || a.Is4In6() {
		return fmt.Errorf("invalid or unsupported IP address %q", a)
	}
	return nil
}

// ParsePrefix accepts only CIDRs whose host bits are zero.
func ParsePrefix(s string) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("invalid CIDR: %w", err)
	}
	if err = validateAddr(p.Addr()); err != nil {
		return netip.Prefix{}, err
	}
	if p != p.Masked() {
		return netip.Prefix{}, &NonCanonicalPrefixError{Suggested: p.Masked()}
	}
	return p, nil
}

// Encode stores IPv4 as twelve zero bytes followed by its four network bytes.
func Encode(a netip.Addr) (Family, [16]byte, error) {
	var b [16]byte
	if err := validateAddr(a); err != nil {
		return 0, b, err
	}
	if a.Is4() {
		v := a.As4()
		copy(b[12:], v[:])
		return IPv4, b, nil
	}
	return IPv6, a.As16(), nil
}

// Decode validates family, length and canonical IPv4 padding before decoding.
func Decode(f Family, b []byte) (netip.Addr, error) {
	if len(b) != 16 {
		return netip.Addr{}, fmt.Errorf("address must contain 16 bytes")
	}
	switch f {
	case IPv4:
		for _, v := range b[:12] {
			if v != 0 {
				return netip.Addr{}, fmt.Errorf("IPv4 address has nonzero padding")
			}
		}
		var a [4]byte
		copy(a[:], b[12:])
		return netip.AddrFrom4(a), nil
	case IPv6:
		var a [16]byte
		copy(a[:], b)
		addr := netip.AddrFrom16(a)
		if err := validateAddr(addr); err != nil {
			return netip.Addr{}, err
		}
		return addr, nil
	default:
		return netip.Addr{}, fmt.Errorf("unsupported address family %d", f)
	}
}

// Bounds returns the first and last addresses of a canonical CIDR.
func Bounds(p netip.Prefix) (netip.Addr, netip.Addr, error) {
	if !p.IsValid() {
		return netip.Addr{}, netip.Addr{}, fmt.Errorf("invalid prefix")
	}
	if err := validateAddr(p.Addr()); err != nil {
		return netip.Addr{}, netip.Addr{}, err
	}
	if p != p.Masked() {
		return netip.Addr{}, netip.Addr{}, &NonCanonicalPrefixError{Suggested: p.Masked()}
	}
	first := p.Addr()
	f, b, _ := Encode(first)
	bitOffset := 0
	if f == IPv4 {
		bitOffset = 96
	}
	for i := bitOffset + p.Bits(); i < 128; i++ {
		b[i/8] |= byte(1 << (7 - uint(i%8)))
	}
	if f == IPv6 {
		// A valid network may contain mapped addresses even though they are
		// forbidden as standalone hosts. Preserve the raw endpoint bits.
		return first, netip.AddrFrom16(b), nil
	}
	last, err := Decode(f, b[:])
	return first, last, err
}

// EncodePrefix returns the family and the network-order endpoints for storage.
// Unlike Encode, it permits a mapped IPv6 value as the calculated last endpoint
// of a valid IPv6 prefix; mapped addresses remain invalid as prefix inputs.
func EncodePrefix(p netip.Prefix) (Family, [16]byte, [16]byte, error) {
	var zero [16]byte
	first, last, err := Bounds(p)
	if err != nil {
		return 0, zero, zero, err
	}
	family, firstBytes, err := Encode(first)
	if err != nil {
		return 0, zero, zero, err
	}
	if family == IPv6 {
		return family, firstBytes, last.As16(), nil
	}
	_, lastBytes, err := Encode(last)
	if err != nil {
		return 0, zero, zero, err
	}
	return family, firstBytes, lastBytes, nil
}

// DecodePrefix reconstructs a canonical CIDR and validates its stored last address.
func DecodePrefix(f Family, first, last []byte, bits int) (netip.Prefix, error) {
	a, err := Decode(f, first)
	if err != nil {
		return netip.Prefix{}, err
	}
	var end netip.Addr
	if f == IPv6 {
		if len(last) != 16 {
			return netip.Prefix{}, fmt.Errorf("address must contain 16 bytes")
		}
		var raw [16]byte
		copy(raw[:], last)
		end = netip.AddrFrom16(raw)
	} else {
		end, err = Decode(f, last)
		if err != nil {
			return netip.Prefix{}, err
		}
	}
	p, err := a.Prefix(bits)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("invalid prefix length: %w", err)
	}
	if p.Addr() != a {
		return netip.Prefix{}, &NonCanonicalPrefixError{Suggested: p}
	}
	_, expected, err := Bounds(p)
	if err != nil {
		return netip.Prefix{}, err
	}
	if end != expected {
		return netip.Prefix{}, fmt.Errorf("last address does not match CIDR")
	}
	return p, nil
}
