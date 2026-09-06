package forwarder

import (
	"log/slog"
	"net"

	"github.com/miekg/dns"
)

// ECSMode controls how EDNS Client Subnet (RFC 7871) data is handled when
// forwarding queries upstream.
//
//   - ECSStrip (default): client-supplied ECS options are removed before the
//     query leaves the server. This protects client privacy and keeps the
//     cache key independent of client topology.
//   - ECSPassthrough: the client's ECS option is forwarded unchanged.
//   - ECSAdd: any client-supplied ECS is replaced with one derived from the
//     client's IP address, truncated to the configured prefix lengths.
type ECSMode string

const (
	ECSStrip       ECSMode = "strip"
	ECSPassthrough ECSMode = "passthrough"
	ECSAdd         ECSMode = "add"
)

// DefaultECS prefix lengths. /24 for IPv4 and /56 for IPv6 follow the RFC
// 7871 recommendations for recursive resolvers.
const (
	DefaultECSIPv4Prefix = 24
	DefaultECSIPv6Prefix = 56
)

// ParseECSMode normalizes a raw setting value into an ECSMode. Unknown or
// empty values fall back to the privacy-preserving default (strip).
func ParseECSMode(v string) ECSMode {
	switch ECSMode(v) {
	case ECSPassthrough:
		return ECSPassthrough
	case ECSAdd:
		return ECSAdd
	default:
		return ECSStrip
	}
}

// StripECS removes all EDNS Client Subnet options from the message's OPT
// record. Messages without EDNS0 are returned unchanged.
func StripECS(m *dns.Msg) {
	opt := m.IsEdns0()
	if opt == nil {
		return
	}
	kept := opt.Option[:0]
	for _, o := range opt.Option {
		if o.Option() == dns.EDNS0SUBNET {
			continue
		}
		kept = append(kept, o)
	}
	opt.Option = kept
}

// HasECS reports whether the message carries an EDNS Client Subnet option.
func HasECS(m *dns.Msg) bool {
	opt := m.IsEdns0()
	if opt == nil {
		return false
	}
	for _, o := range opt.Option {
		if o.Option() == dns.EDNS0SUBNET {
			return true
		}
	}
	return false
}

// InjectECS replaces (or adds) the message's EDNS Client Subnet option with
// one derived from clientIP, truncated to the given prefix length. An
// unparseable or unspecified client IP results in a /0 ECS entry, which
// semantically equals "no subnet information" per RFC 7871 §7.2.1.
func InjectECS(m *dns.Msg, clientIP net.IP, v4Prefix, v6Prefix int) {
	ip := clientIP
	if ip == nil || ip.IsUnspecified() {
		ip = net.IPv4zero
	}

	family := uint16(1)
	prefix := v4Prefix
	if ip.To4() == nil {
		family = 2
		prefix = v6Prefix
		if prefix < 0 || prefix > 128 {
			prefix = DefaultECSIPv6Prefix
		}
	} else if prefix < 0 || prefix > 32 {
		prefix = DefaultECSIPv4Prefix
	}

	truncated := truncateIP(ip, prefix)

	// Ensure an OPT record exists (settle on a sane 1232-byte payload).
	opt := m.IsEdns0()
	if opt == nil {
		m.SetEdns0(1232, false)
		opt = m.IsEdns0()
	}

	// Remove any existing ECS option before appending the new one.
	kept := opt.Option[:0]
	for _, o := range opt.Option {
		if o.Option() == dns.EDNS0SUBNET {
			continue
		}
		kept = append(kept, o)
	}
	opt.Option = append(kept, &dns.EDNS0_SUBNET{
		Code:          dns.EDNS0SUBNET,
		Family:        family,
		SourceNetmask: uint8(prefix),
		SourceScope:   0,
		Address:       truncated,
	})
}

// truncateIP zeroes the host bits of ip beyond prefix. It mirrors the
// normalization the RFC requires before placing an address on the wire.
func truncateIP(ip net.IP, prefix int) net.IP {
	bits := 32
	ip4 := ip.To4()
	if ip4 == nil {
		bits = 128
		ip4 = ip.To16()
	}
	if prefix >= bits {
		return ip4
	}
	if prefix <= 0 {
		if bits == 32 {
			return net.IPv4zero.To4()
		}
		return net.IPv6zero.To16()
	}
	mask := net.CIDRMask(prefix, bits)
	masked := ip4.Mask(mask)
	if masked == nil {
		slog.Debug("ecs: failed to truncate client address", "prefix", prefix)
		return ip4
	}
	return masked
}
