package server

import (
	"net"
	"testing"

	"github.com/jincaiw/GoDDI/internal/dhcp/scope"
)

func TestIsInfrastructureAddress(t *testing.T) {
	s := &Server{serverIPs: map[string]net.IP{"utun7": net.ParseIP("198.18.0.1")}}
	sc := &scope.Scope{Router: "192.168.1.1"}
	cases := []struct {
		ip   string
		want bool
	}{
		{"198.18.0.1", true},  // the DHCP server's own served address
		{"192.168.1.1", true}, // the scope gateway
		{"198.18.0.100", false},
		{"192.168.1.50", false},
	}
	for _, c := range cases {
		got, why := s.isInfrastructureAddress(sc, net.ParseIP(c.ip))
		if got != c.want {
			t.Fatalf("%s: got %v (%q), want %v", c.ip, got, why, c.want)
		}
	}
	// A nil scope must not panic; only the server's own IPs are infrastructure.
	if got, _ := s.isInfrastructureAddress(nil, net.ParseIP("198.18.0.1")); !got {
		t.Fatal("nil scope: server IP not flagged")
	}
	if got, _ := s.isInfrastructureAddress(nil, net.ParseIP("192.168.1.1")); got {
		t.Fatal("nil scope: a gateway with no scope must not be flagged")
	}
}
