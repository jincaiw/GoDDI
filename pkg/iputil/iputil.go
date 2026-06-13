package iputil

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"net"
	"strings"
)

// ParseIP parses an IP address string and returns net.IP.
func ParseIP(s string) net.IP {
	return net.ParseIP(s)
}

// IPToUint32 converts an IPv4 address to a uint32.
func IPToUint32(ip net.IP) uint32 {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip4)
}

// Uint32ToIP converts a uint32 to an IPv4 address.
func Uint32ToIP(n uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, n)
	return ip
}

// NextIP returns the next IP address.
func NextIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)
	for j := len(next) - 1; j >= 0; j-- {
		next[j]++
		if next[j] > 0 {
			break
		}
	}
	return next
}

// PrevIP returns the previous IP address.
func PrevIP(ip net.IP) net.IP {
	prev := make(net.IP, len(ip))
	copy(prev, ip)
	for j := len(prev) - 1; j >= 0; j-- {
		prev[j]--
		if prev[j] < 0xFF {
			break
		}
	}
	return prev
}

// SubnetSize returns the number of usable host addresses in a subnet.
// For IPv4 this excludes the network and broadcast addresses; for IPv6
// (or subnets too small for /30) the full address count is returned.
func SubnetSize(cidr string) (uint64, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		// Provide a clearer hint for the most common user mistake: they
		// forgot the prefix length, e.g. typed "192.168.1.0" instead of
		// "192.168.1.0/24". net.ParseCIDR's default error string is
		// technically accurate but unhelpful for an operator typing a
		// subnet by hand.
		if strings.HasPrefix(err.Error(), "invalid CIDR address") {
			return 0, fmt.Errorf("子网格式无效 %q：缺少前缀长度，例如应为 %q", cidr, cidr+"/24")
		}
		return 0, fmt.Errorf("解析子网 %s 失败: %w", cidr, err)
	}

	ones, bits := ipNet.Mask.Size()
	hostBits := bits - ones

	if hostBits >= 64 {
		// For very large subnets, use math/big for precision.
		size := new(big.Int).Lsh(big.NewInt(1), uint(hostBits))
		if bits == 32 && size.Cmp(big.NewInt(2)) > 0 {
			size.Sub(size, big.NewInt(2))
		}
		if !size.IsUint64() {
			return 0, fmt.Errorf("子网 %s 太大，超出 uint64 范围", cidr)
		}
		return size.Uint64(), nil
	}

	size := uint64(1) << uint(hostBits)
	if bits == 32 {
		// IPv4: subtract network and broadcast addresses.
		if size > 2 {
			return size - 2, nil
		}
		return size, nil
	}
	// IPv6.
	return size, nil
}

// IPRange returns the first and last usable IP addresses in a CIDR range.
func IPRange(cidr string) (net.IP, net.IP, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing CIDR %s: %w", cidr, err)
	}

	first := NextIP(ipNet.IP)
	last := make(net.IP, len(ipNet.IP))
	copy(last, ipNet.IP)
	for i := range last {
		last[i] |= ^ipNet.Mask[i]
	}

	// For IPv4, don't include broadcast address.
	if len(last) == 4 || (len(last) == 16 && isIPv4MappedIPv6(last)) {
		last = PrevIP(last)
	}

	return first, last, nil
}

// isIPv4MappedIPv6 checks if an IPv6 address is an IPv4-mapped address.
func isIPv4MappedIPv6(ip net.IP) bool {
	return len(ip) == 16 && ip[10] == 0xff && ip[11] == 0xff
}

// ContainsIP checks if a CIDR range contains the given IP.
func ContainsIP(cidr string, ip net.IP) (bool, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, err
	}
	return ipNet.Contains(ip), nil
}

// MaskToPrefixLen converts a subnet mask to a prefix length.
func MaskToPrefixLen(mask net.IPMask) int {
	ones, _ := mask.Size()
	return ones
}
