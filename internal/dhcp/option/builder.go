package option

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/rfc1035label"
)

// BuildOptions builds DHCPv4 options for a response based on scope and reservation.
// Only includes options from the requested options list (Parameter Request List).
func BuildOptions(scopeID, reservationID string, requestedOptions []dhcpv4.OptionCode, dbOptionMap map[int]string) []dhcpv4.Option {
	var opts []dhcpv4.Option

	// If no specific options requested, include common defaults.
	requestedSet := make(map[uint8]bool)
	if len(requestedOptions) > 0 {
		for _, o := range requestedOptions {
			requestedSet[o.Code()] = true
		}
	}

	for code, value := range dbOptionMap {
		// If client requested specific options, only include those.
		if len(requestedSet) > 0 && !requestedSet[uint8(code)] {
			continue
		}

		opt, err := encodeOption(uint8(code), value)
		if err != nil {
			slog.Warn("dhcp_option: failed to encode option, dropping", "code", code, "value", value, "error", err)
			continue
		}
		if opt != nil {
			opts = append(opts, *opt)
		}
	}

	return opts
}

// encodeOption encodes a DHCP option value from its string representation
// using the proper dhcpv4 library types. It returns an error (instead of
// silently dropping the option) when the value cannot be parsed.
func encodeOption(code uint8, value string) (*dhcpv4.Option, error) {
	switch code {
	case 1: // Subnet Mask
		ip := net.ParseIP(value)
		if ip == nil {
			return nil, fmt.Errorf("invalid subnet mask: %s", value)
		}
		ip = ip.To4()
		if ip == nil {
			return nil, fmt.Errorf("subnet mask is not IPv4: %s", value)
		}
		opt := dhcpv4.OptSubnetMask(net.IPv4Mask(ip[0], ip[1], ip[2], ip[3]))
		return &opt, nil

	case 3: // Router
		ips, err := parseIPList(value)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("router list is empty")
		}
		opt := dhcpv4.OptRouter(ips...)
		return &opt, nil

	case 6: // DNS Servers
		ips, err := parseIPList(value)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("DNS server list is empty")
		}
		opt := dhcpv4.OptDNS(ips...)
		return &opt, nil

	case 12: // Host Name
		opt := dhcpv4.OptHostName(value)
		return &opt, nil

	case 15: // Domain Name
		opt := dhcpv4.OptDomainName(value)
		return &opt, nil

	case 42: // NTP Servers
		ips, err := parseIPList(value)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("NTP server list is empty")
		}
		opt := dhcpv4.OptNTPServers(ips...)
		return &opt, nil

	case 44: // WINS (NetBIOS Name Servers)
		ips, err := parseIPList(value)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("WINS server list is empty")
		}
		opt := dhcpv4.OptNetBIOSNameServers(ips...)
		return &opt, nil

	case 51: // Lease Time
		secs, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid lease time %q: %w", value, err)
		}
		opt := dhcpv4.OptIPAddressLeaseTime(time.Duration(secs) * time.Second)
		return &opt, nil

	case 54: // Server ID
		ip := net.ParseIP(value)
		if ip == nil {
			return nil, fmt.Errorf("invalid server ID: %s", value)
		}
		ip = ip.To4()
		if ip == nil {
			return nil, fmt.Errorf("server ID is not IPv4: %s", value)
		}
		opt := dhcpv4.OptServerIdentifier(ip)
		return &opt, nil

	case 58: // Renewal Time (T1)
		secs, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid renewal time %q: %w", value, err)
		}
		opt := dhcpv4.OptRenewTimeValue(time.Duration(secs) * time.Second)
		return &opt, nil

	case 59: // Rebinding Time (T2)
		secs, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid rebinding time %q: %w", value, err)
		}
		opt := dhcpv4.OptRebindingTimeValue(time.Duration(secs) * time.Second)
		return &opt, nil

	case 66: // TFTP Server Name
		opt := dhcpv4.OptTFTPServerName(value)
		return &opt, nil

	case 67: // Bootfile Name
		opt := dhcpv4.OptBootFileName(value)
		return &opt, nil

	case 138: // CAPWAP AC Address - encode as 4-byte IP address
		ip := net.ParseIP(value)
		if ip == nil {
			return nil, fmt.Errorf("invalid CAPWAP AC address: %s", value)
		}
		ip4 := ip.To4()
		if ip4 == nil {
			return nil, fmt.Errorf("CAPWAP AC address is not IPv4: %s", value)
		}
		opt := dhcpv4.OptGeneric(dhcpv4.GenericOptionCode(code), ip4)
		return &opt, nil

	case 150: // TFTP Server Address - encode as 4-byte IP address
		ip := net.ParseIP(value)
		if ip == nil {
			return nil, fmt.Errorf("invalid TFTP server address: %s", value)
		}
		ip4 := ip.To4()
		if ip4 == nil {
			return nil, fmt.Errorf("TFTP server address is not IPv4: %s", value)
		}
		opt := dhcpv4.OptGeneric(dhcpv4.GenericOptionCode(code), ip4)
		return &opt, nil

	case 119: // Domain Search List
		labels := &rfc1035label.Labels{}
		parts := strings.Split(value, ",")
		for _, domain := range parts {
			domain = strings.TrimSpace(domain)
			if domain != "" {
				labels.Labels = append(labels.Labels, domain)
			}
		}
		opt := dhcpv4.OptDomainSearch(labels)
		return &opt, nil

	case 121: // Classless Static Route
		routes, err := parseClasslessRoutes(value)
		if err != nil {
			return nil, err
		}
		if len(routes) == 0 {
			return nil, fmt.Errorf("classless static route list is empty")
		}
		opt := dhcpv4.OptClasslessStaticRoute(routes...)
		return &opt, nil

	default:
		// Generic option
		opt := dhcpv4.OptGeneric(dhcpv4.GenericOptionCode(code), []byte(value))
		return &opt, nil
	}
}

// parseIPList parses a comma-separated list of IPv4 addresses. It returns an
// error if any component cannot be parsed as an IPv4 address.
func parseIPList(s string) ([]net.IP, error) {
	var ips []net.IP
	parts := strings.Split(s, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		ip := net.ParseIP(p)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address in list: %q", p)
		}
		if ip4 := ip.To4(); ip4 != nil {
			ips = append(ips, ip4)
		} else {
			return nil, fmt.Errorf("non-IPv4 address in list: %q", p)
		}
	}
	return ips, nil
}

// parseClasslessRoutes parses classless static routes from string format.
// Format: "dest_cidr,gateway;dest_cidr,gateway"
func parseClasslessRoutes(routes string) ([]*dhcpv4.Route, error) {
	var result []*dhcpv4.Route
	parts := strings.Split(routes, ";")
	for _, route := range parts {
		route = strings.TrimSpace(route)
		if route == "" {
			continue
		}
		components := strings.Split(route, ",")
		if len(components) != 2 {
			return nil, fmt.Errorf("classless route %q must be dest_cidr,gateway", route)
		}

		_, ipNet, err := net.ParseCIDR(strings.TrimSpace(components[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", components[0], err)
		}
		gw := net.ParseIP(strings.TrimSpace(components[1]))
		if gw == nil {
			return nil, fmt.Errorf("invalid gateway IP %q", components[1])
		}

		result = append(result, &dhcpv4.Route{
			Dest:   ipNet,
			Router: gw,
		})
	}
	return result, nil
}

// FormatOptionForDisplay returns a human-readable description of a DHCP option.
func FormatOptionForDisplay(code uint8, value []byte) string {
	switch code {
	case 1:
		if len(value) == 4 {
			return fmt.Sprintf("Subnet Mask: %d.%d.%d.%d", value[0], value[1], value[2], value[3])
		}
	case 3, 6, 42, 44:
		var ips []string
		for i := 0; i+3 < len(value); i += 4 {
			ips = append(ips, fmt.Sprintf("%d.%d.%d.%d", value[i], value[i+1], value[i+2], value[i+3]))
		}
		name := "IP List"
		switch code {
		case 3:
			name = "Router"
		case 6:
			name = "DNS Servers"
		case 42:
			name = "NTP Servers"
		case 44:
			name = "WINS Servers"
		}
		return fmt.Sprintf("%s: %s", name, strings.Join(ips, ", "))
	case 12:
		return fmt.Sprintf("Host Name: %s", string(value))
	case 15:
		return fmt.Sprintf("Domain Name: %s", string(value))
	}
	return fmt.Sprintf("Option %d: %v", code, value)
}
