package server

import (
	"net"
	"testing"
)

func TestIPInRange_NormalRange(t *testing.T) {
	start := net.ParseIP("192.168.1.1")
	end := net.ParseIP("192.168.1.254")

	tests := []struct {
		name    string
		ip      string
		inRange bool
	}{
		{"IP at start boundary", "192.168.1.1", true},
		{"IP at end boundary", "192.168.1.254", true},
		{"IP in middle", "192.168.1.100", true},
		{"IP before range", "192.168.1.0", false},
		{"IP after range", "192.168.1.255", false},
		{"IP different subnet", "192.168.2.1", false},
		{"IP completely different", "10.0.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if got := ipInRange(ip, start, end); got != tt.inRange {
				t.Errorf("ipInRange(%s, %s, %s) = %v, want %v", tt.ip, start, end, got, tt.inRange)
			}
		})
	}
}

func TestIPInRange_CrossByteBoundary(t *testing.T) {
	// Range crosses a byte boundary: 192.168.0.250 -> 192.168.1.10
	start := net.ParseIP("192.168.0.250")
	end := net.ParseIP("192.168.1.10")

	tests := []struct {
		name    string
		ip      string
		inRange bool
	}{
		{"IP at start", "192.168.0.250", true},
		{"IP at end", "192.168.1.10", true},
		{"IP at boundary 192.168.0.255", "192.168.0.255", true},
		{"IP at boundary 192.168.1.0", "192.168.1.0", true},
		{"IP just before range", "192.168.0.249", false},
		{"IP just after range", "192.168.1.11", false},
		{"IP in different third octet", "192.168.2.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if got := ipInRange(ip, start, end); got != tt.inRange {
				t.Errorf("ipInRange(%s, %s, %s) = %v, want %v", tt.ip, start, end, got, tt.inRange)
			}
		})
	}
}

func TestIPInRange_InvalidIPs(t *testing.T) {
	start := net.ParseIP("192.168.1.1")
	end := net.ParseIP("192.168.1.254")

	// nil IP should return false
	if ipInRange(nil, start, end) {
		t.Error("ipInRange(nil, ...) should return false")
	}

	// IPv6 IP with IPv4 range should return false
	ip6 := net.ParseIP("::1")
	if ipInRange(ip6, start, end) {
		t.Error("ipInRange(IPv6, IPv4 range) should return false")
	}

	// nil start should return false
	ip := net.ParseIP("192.168.1.100")
	if ipInRange(ip, nil, end) {
		t.Error("ipInRange(ip, nil, end) should return false")
	}

	// nil end should return false
	if ipInRange(ip, start, nil) {
		t.Error("ipInRange(ip, start, nil) should return false")
	}
}

func TestIPInRange_SingleIPRange(t *testing.T) {
	// Range of a single IP
	start := net.ParseIP("10.0.0.1")
	end := net.ParseIP("10.0.0.1")

	if !ipInRange(net.ParseIP("10.0.0.1"), start, end) {
		t.Error("ipInRange should return true for single-IP range")
	}
	if ipInRange(net.ParseIP("10.0.0.2"), start, end) {
		t.Error("ipInRange should return false for IP outside single-IP range")
	}
}

func TestBytesCompare(t *testing.T) {
	tests := []struct {
		a, b     net.IP
		expected int
	}{
		{net.ParseIP("192.168.1.1").To4(), net.ParseIP("192.168.1.1").To4(), 0},
		{net.ParseIP("192.168.1.1").To4(), net.ParseIP("192.168.1.2").To4(), -1},
		{net.ParseIP("192.168.1.2").To4(), net.ParseIP("192.168.1.1").To4(), 1},
		{net.ParseIP("10.0.0.1").To4(), net.ParseIP("192.168.1.1").To4(), -1},
		{net.ParseIP("192.168.1.255").To4(), net.ParseIP("192.168.2.0").To4(), -1},
	}

	for _, tt := range tests {
		got := bytesCompare(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("bytesCompare(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.expected)
		}
	}
}
