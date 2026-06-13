package iputil

import (
	"net"
	"testing"
)

func TestParseIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		isNil bool
	}{
		{"valid IPv4", "192.168.1.1", false},
		{"valid IPv6", "::1", false},
		{"valid IPv6 full", "2001:db8::1", false},
		{"invalid IP", "not-an-ip", true},
		{"empty string", "", true},
		{"partial IP", "192.168", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ParseIP(tt.input)
			if (result == nil) != tt.isNil {
				t.Errorf("ParseIP(%q) nil = %v, want %v", tt.input, result == nil, tt.isNil)
			}
		})
	}
}

func TestIPToUint32(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected uint32
	}{
		{"0.0.0.0", "0.0.0.0", 0},
		{"0.0.0.1", "0.0.0.1", 1},
		{"192.168.1.1", "192.168.1.1", 0xC0A80101},
		{"255.255.255.255", "255.255.255.255", 0xFFFFFFFF},
		{"127.0.0.1", "127.0.0.1", 0x7F000001},
		{"10.0.0.1", "10.0.0.1", 0x0A000001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.input)
			result := IPToUint32(ip)
			if result != tt.expected {
				t.Errorf("IPToUint32(%q) = 0x%08X, want 0x%08X", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIPToUint32_IPv6(t *testing.T) {
	t.Parallel()

	// IPv6 addresses should return 0 since they can't be represented as uint32
	ip := net.ParseIP("::1")
	result := IPToUint32(ip)
	if result != 0 {
		t.Errorf("IPToUint32(::1) = %d, want 0 for IPv6", result)
	}
}

func TestUint32ToIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    uint32
		expected string
	}{
		{"zero", 0, "0.0.0.0"},
		{"one", 1, "0.0.0.1"},
		{"192.168.1.1", 0xC0A80101, "192.168.1.1"},
		{"255.255.255.255", 0xFFFFFFFF, "255.255.255.255"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := Uint32ToIP(tt.input)
			if result.String() != tt.expected {
				t.Errorf("Uint32ToIP(0x%08X) = %s, want %s", tt.input, result.String(), tt.expected)
			}
		})
	}
}

func TestIPToUint32AndBack(t *testing.T) {
	t.Parallel()

	ips := []string{"0.0.0.0", "0.0.0.1", "192.168.1.1", "10.0.0.1", "255.255.255.255"}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		n := IPToUint32(ip)
		result := Uint32ToIP(n)
		if !result.Equal(ip.To4()) {
			t.Errorf("Uint32ToIP(IPToUint32(%q)) = %s, want %s", ipStr, result.String(), ipStr)
		}
	}
}

func TestNextIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple increment", "192.168.1.1", "192.168.1.2"},
		{"carry over", "192.168.1.255", "192.168.2.0"},
		{"double carry", "192.168.255.255", "192.169.0.0"},
		{"zero address", "0.0.0.0", "0.0.0.1"},
		{"IPv6 increment", "::1", "::2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.input)
			result := NextIP(ip)
			if result.String() != tt.expected {
				t.Errorf("NextIP(%q) = %s, want %s", tt.input, result.String(), tt.expected)
			}
		})
	}
}

func TestPrevIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple decrement", "192.168.1.2", "192.168.1.1"},
		{"borrow", "192.168.2.0", "192.168.1.255"},
		{"double borrow", "192.169.0.0", "192.168.255.255"},
		{"one address", "0.0.0.1", "0.0.0.0"},
		{"IPv6 decrement", "::2", "::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.input)
			result := PrevIP(ip)
			if result.String() != tt.expected {
				t.Errorf("PrevIP(%q) = %s, want %s", tt.input, result.String(), tt.expected)
			}
		})
	}
}

func TestNextIPAndPrevIP_RoundTrip(t *testing.T) {
	t.Parallel()

	ips := []string{"192.168.1.1", "10.0.0.1", "172.16.0.100"}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		next := NextIP(ip)
		prev := PrevIP(next)
		if !prev.Equal(ip) {
			t.Errorf("PrevIP(NextIP(%q)) = %s, want %s", ipStr, prev.String(), ipStr)
		}
	}
}

func TestSubnetSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cidr     string
		expected uint64
		wantErr  bool
	}{
		{"/24", "192.168.1.0/24", 254, false},
		{"/16", "10.0.0.0/16", 65534, false},
		{"/8", "10.0.0.0/8", 16777214, false},
		{"/32", "192.168.1.1/32", 1, false},
		{"/31", "192.168.1.0/31", 2, false},
		{"/30", "192.168.1.0/30", 2, false},
		{"invalid CIDR", "invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := SubnetSize(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("SubnetSize(%q) error = %v, wantErr %v", tt.cidr, err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("SubnetSize(%q) = %d, want %d", tt.cidr, result, tt.expected)
			}
		})
	}
}

func TestIPRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cidr    string
		firstIP string
		lastIP  string
		wantErr bool
	}{
		{"/24", "192.168.1.0/24", "192.168.1.1", "192.168.1.254", false},
		{"/16", "10.0.0.0/16", "10.0.0.1", "10.0.255.254", false},
		{"/30", "192.168.1.0/30", "192.168.1.1", "192.168.1.2", false},
		{"invalid CIDR", "invalid", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			first, last, err := IPRange(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("IPRange(%q) error = %v, wantErr %v", tt.cidr, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if first.String() != tt.firstIP {
					t.Errorf("IPRange(%q) first = %s, want %s", tt.cidr, first.String(), tt.firstIP)
				}
				if last.String() != tt.lastIP {
					t.Errorf("IPRange(%q) last = %s, want %s", tt.cidr, last.String(), tt.lastIP)
				}
			}
		})
	}
}

func TestIPRange_Slash24(t *testing.T) {
	t.Parallel()

	first, last, err := IPRange("192.168.1.0/24")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.String() != "192.168.1.1" {
		t.Errorf("first IP = %s, want 192.168.1.1", first.String())
	}
	if last.String() != "192.168.1.254" {
		t.Errorf("last IP = %s, want 192.168.1.254", last.String())
	}
}

func TestContainsIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cidr     string
		ip       string
		expected bool
		wantErr  bool
	}{
		{"IP in range", "192.168.1.0/24", "192.168.1.100", true, false},
		{"IP at start", "192.168.1.0/24", "192.168.1.0", true, false},
		{"IP at end", "192.168.1.0/24", "192.168.1.255", true, false},
		{"IP outside range", "192.168.1.0/24", "192.168.2.1", false, false},
		{"IP in 10.0.0.0/8", "10.0.0.0/8", "10.255.255.255", true, false},
		{"invalid CIDR", "invalid", "192.168.1.1", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.ip)
			result, err := ContainsIP(tt.cidr, ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("ContainsIP(%q, %q) error = %v, wantErr %v", tt.cidr, tt.ip, err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("ContainsIP(%q, %q) = %v, want %v", tt.cidr, tt.ip, result, tt.expected)
			}
		})
	}
}

func TestMaskToPrefixLen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mask     string
		expected int
	}{
		{"/24 mask", "255.255.255.0", 24},
		{"/16 mask", "255.255.0.0", 16},
		{"/8 mask", "255.0.0.0", 8},
		{"/32 mask", "255.255.255.255", 32},
		{"/0 mask", "0.0.0.0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mask := net.IPMask(net.ParseIP(tt.mask).To4())
			result := MaskToPrefixLen(mask)
			if result != tt.expected {
				t.Errorf("MaskToPrefixLen(%q) = %d, want %d", tt.mask, result, tt.expected)
			}
		})
	}
}
