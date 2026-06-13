package netutil

import (
	"net"
	"testing"
)

func TestParseCIDR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cidr    string
		wantErr bool
		network string
	}{
		{"valid IPv4 CIDR", "192.168.1.0/24", false, "192.168.1.0/24"},
		{"valid IPv4 /16", "10.0.0.0/16", false, "10.0.0.0/16"},
		{"valid IPv6 CIDR", "fd00::/64", false, "fd00::/64"},
		{"invalid CIDR", "invalid", true, ""},
		{"IP without mask", "192.168.1.0", true, ""},
		{"invalid mask", "192.168.1.0/33", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ipNet, err := ParseCIDR(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCIDR(%q) error = %v, wantErr %v", tt.cidr, err, tt.wantErr)
				return
			}
			if !tt.wantErr && ipNet.String() != tt.network {
				t.Errorf("ParseCIDR(%q) network = %s, want %s", tt.cidr, ipNet.String(), tt.network)
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// RFC 1918 private ranges
		{"10.x.x.x", "10.0.0.1", true},
		{"10.255.255.255", "10.255.255.255", true},
		{"172.16.x.x", "172.16.0.1", true},
		{"172.31.x.x", "172.31.255.255", true},
		{"192.168.x.x", "192.168.1.1", true},
		{"192.168.0.0", "192.168.0.0", true},
		// IPv6 private
		{"IPv6 ULA fd00::", "fd00::1", true},
		{"IPv6 ULA fc00::", "fc00::1", true},
		// Public IPs
		{"8.8.8.8", "8.8.8.8", false},
		{"1.1.1.1", "1.1.1.1", false},
		{"172.15.0.1", "172.15.0.1", false},
		{"172.32.0.1", "172.32.0.1", false},
		// IPv6 public
		{"IPv6 public", "2001:db8::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP %q", tt.ip)
			}
			result := IsPrivateIP(ip)
			if result != tt.expected {
				t.Errorf("IsPrivateIP(%q) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestCIDRContainsIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cidr     string
		ip       string
		expected bool
		wantErr  bool
	}{
		{"IP in range", "192.168.1.0/24", "192.168.1.100", true, false},
		{"IP at network", "192.168.1.0/24", "192.168.1.0", true, false},
		{"IP at broadcast", "192.168.1.0/24", "192.168.1.255", true, false},
		{"IP outside", "192.168.1.0/24", "192.168.2.1", false, false},
		{"invalid CIDR", "invalid", "192.168.1.1", false, true},
		{"large range", "10.0.0.0/8", "10.255.255.255", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.ip)
			result, err := CIDRContainsIP(tt.cidr, ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("CIDRContainsIP(%q, %q) error = %v, wantErr %v", tt.cidr, tt.ip, err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("CIDRContainsIP(%q, %q) = %v, want %v", tt.cidr, tt.ip, result, tt.expected)
			}
		})
	}
}

func TestExpandCIDR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cidr        string
		expectedLen int
		firstIP     string
		lastIP      string
		wantErr     bool
	}{
		{"/30 has 2 usable", "192.168.1.0/30", 2, "192.168.1.1", "192.168.1.2", false},
		{"/29 has 6 usable", "192.168.1.0/29", 6, "192.168.1.1", "192.168.1.6", false},
		{"/32 has 1 IP", "192.168.1.1/32", 1, "192.168.1.1", "192.168.1.1", false},
		{"invalid CIDR", "invalid", 0, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ips, err := ExpandCIDR(tt.cidr, 0)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandCIDR(%q) error = %v, wantErr %v", tt.cidr, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(ips) != tt.expectedLen {
					t.Errorf("ExpandCIDR(%q) returned %d IPs, want %d", tt.cidr, len(ips), tt.expectedLen)
				}
				if len(ips) > 0 {
					if ips[0].String() != tt.firstIP {
						t.Errorf("ExpandCIDR(%q) first IP = %s, want %s", tt.cidr, ips[0].String(), tt.firstIP)
					}
					if ips[len(ips)-1].String() != tt.lastIP {
						t.Errorf("ExpandCIDR(%q) last IP = %s, want %s", tt.cidr, ips[len(ips)-1].String(), tt.lastIP)
					}
				}
			}
		})
	}
}

func TestExpandCIDR_Slash24(t *testing.T) {
	t.Parallel()

	ips, err := ExpandCIDR("192.168.1.0/24", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ips) != 254 {
		t.Errorf("ExpandCIDR(/24) returned %d IPs, want 254", len(ips))
	}
	if ips[0].String() != "192.168.1.1" {
		t.Errorf("first IP = %s, want 192.168.1.1", ips[0].String())
	}
	if ips[253].String() != "192.168.1.254" {
		t.Errorf("last IP = %s, want 192.168.1.254", ips[253].String())
	}
}

func TestExpandCIDR_MaxLimit(t *testing.T) {
	t.Parallel()

	// /16 has 65534 usable IPs, which is within the default limit.
	ips, err := ExpandCIDR("10.0.0.0/16", 0)
	if err != nil {
		t.Fatalf("unexpected error for /16: %v", err)
	}
	if len(ips) != 65534 {
		t.Errorf("ExpandCIDR(/16) returned %d IPs, want 65534", len(ips))
	}

	// /8 should exceed the default limit.
	_, err = ExpandCIDR("10.0.0.0/8", 0)
	if err == nil {
		t.Error("expected error for /8 exceeding max limit, got nil")
	}

	// Custom small limit.
	_, err = ExpandCIDR("192.168.1.0/24", 10)
	if err == nil {
		t.Error("expected error for /24 with maxIPs=10, got nil")
	}
}
