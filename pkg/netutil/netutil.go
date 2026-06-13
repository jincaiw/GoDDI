package netutil

import (
	"fmt"
	"net"
	"strings"
)

// IsPrivateIP reports whether the given IP is in a private/loopback/link-local range.
func IsPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	return false
}

// IsZeroIP reports whether ip is the zero address (0.0.0.0 or ::).
func IsZeroIP(ip net.IP) bool {
	return ip.IsUnspecified()
}

// NormalizeAddress trims whitespace from an address string.
func NormalizeAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	return addr
}

// ParseCIDR parses a CIDR notation string and returns the network.
func ParseCIDR(cidr string) (*net.IPNet, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	return ipNet, nil
}

// CIDRContainsIP checks if a CIDR range contains the given IP.
func CIDRContainsIP(cidr string, ip net.IP) (bool, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, err
	}
	return network.Contains(ip), nil
}

// DefaultMaxExpandIPs is the default maximum number of IPs ExpandCIDR will return.
const DefaultMaxExpandIPs = 65536

// ExpandCIDR returns all IP addresses in a CIDR range.
// maxIPs limits the number of IP addresses expanded; if the subnet contains more
// addresses than maxIPs, an error is returned. Use 0 for the default limit.
func ExpandCIDR(cidr string, maxIPs int) ([]net.IP, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	if maxIPs <= 0 {
		maxIPs = DefaultMaxExpandIPs
	}

	// Estimate the number of IPs to prevent OOM.
	ones, bits := ipNet.Mask.Size()
	hostBits := bits - ones
	if hostBits >= 64 {
		return nil, fmt.Errorf("subnet %s is too large to expand (/%d has too many addresses)", cidr, ones)
	}
	estimatedSize := uint64(1) << uint(hostBits)
	if estimatedSize > uint64(maxIPs) {
		return nil, fmt.Errorf("subnet %s has %d addresses, exceeding max expand limit of %d", cidr, estimatedSize, maxIPs)
	}

	// Make a full copy of the IP to avoid modifying ipNet's internal state.
	current := make(net.IP, len(ip))
	copy(current, ip)
	current = current.Mask(ipNet.Mask)

	var ips []net.IP
	for ; ipNet.Contains(current); incrementIP(current) {
		ipCopy := make(net.IP, len(current))
		copy(ipCopy, current)
		ips = append(ips, ipCopy)
	}

	// Remove network and broadcast addresses for IPv4. We use To4() to
	// detect the address family instead of substring-matching the input
	// string: a colon in the CIDR text would falsely match for some IPv6
	// netip styles, and an IPv4-mapped IPv6 like ::ffff:192.0.2.0/120
	// must be treated as IPv4.
	if len(ips) > 2 && current.To4() != nil {
		ips = ips[1 : len(ips)-1]
	}

	return ips, nil
}

// incrementIP increments an IP address by one in place.
func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
