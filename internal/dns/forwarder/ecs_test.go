package forwarder

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func newMsgWithECS(ip net.IP, prefix uint8) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	m.SetEdns0(1232, false)
	opt := m.IsEdns0()
	opt.Option = append(opt.Option, &dns.EDNS0_SUBNET{
		Code:          dns.EDNS0SUBNET,
		Family:        1,
		SourceNetmask: prefix,
		Address:       ip,
	})
	return m
}

func TestStripECSRemovesOption(t *testing.T) {
	m := newMsgWithECS(net.ParseIP("203.0.113.55"), 32)
	if !HasECS(m) {
		t.Fatal("expected ECS to be present before strip")
	}
	StripECS(m)
	if HasECS(m) {
		t.Fatal("ECS option survived StripECS")
	}
	// Other EDNS options must survive.
	opt := m.IsEdns0()
	if opt == nil {
		t.Fatal("OPT record lost after strip")
	}
}

func TestStripECSWithoutEdns(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	StripECS(m) // Must not panic.
	if m.IsEdns0() != nil {
		t.Fatal("unexpected OPT record created")
	}
}

func TestInjectECSIPv4Truncation(t *testing.T) {
	m := newMsgWithECS(net.ParseIP("203.0.113.99"), 32) // Client-supplied ECS must be replaced.
	InjectECS(m, net.ParseIP("198.51.100.77"), 24, 56)

	opt := m.IsEdns0()
	if opt == nil {
		t.Fatal("no OPT record after inject")
	}
	var ecs *dns.EDNS0_SUBNET
	for _, o := range opt.Option {
		if s, ok := o.(*dns.EDNS0_SUBNET); ok {
			ecs = s
		}
	}
	if ecs == nil {
		t.Fatal("no ECS option after inject")
	}
	if ecs.Family != 1 {
		t.Fatalf("family = %d, want 1", ecs.Family)
	}
	if ecs.SourceNetmask != 24 {
		t.Fatalf("source netmask = %d, want 24", ecs.SourceNetmask)
	}
	if got := ecs.Address.String(); got != "198.51.100.0" {
		t.Fatalf("address = %s, want 198.51.100.0 (host bits truncated)", got)
	}
}

func TestInjectECSIPv6(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	InjectECS(m, net.ParseIP("2001:db8:1234:5678::1"), 24, 48)

	opt := m.IsEdns0()
	var ecs *dns.EDNS0_SUBNET
	for _, o := range opt.Option {
		if s, ok := o.(*dns.EDNS0_SUBNET); ok {
			ecs = s
		}
	}
	if ecs == nil {
		t.Fatal("no ECS option after inject")
	}
	if ecs.Family != 2 {
		t.Fatalf("family = %d, want 2", ecs.Family)
	}
	if ecs.SourceNetmask != 48 {
		t.Fatalf("source netmask = %d, want 48", ecs.SourceNetmask)
	}
	if got := ecs.Address.String(); got != "2001:db8:1234::" {
		t.Fatalf("address = %s, want 2001:db8:1234::", got)
	}
}

func TestInjectECSZeroPrefix(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	InjectECS(m, net.ParseIP("198.51.100.77"), 0, 56)

	ecs := m.IsEdns0().Option[0].(*dns.EDNS0_SUBNET)
	if got := ecs.Address.String(); got != "0.0.0.0" {
		t.Fatalf("address = %s, want 0.0.0.0 for /0", got)
	}
}

func TestParseECSMode(t *testing.T) {
	cases := map[string]ECSMode{
		"":            ECSStrip,
		"strip":       ECSStrip,
		"passthrough": ECSPassthrough,
		"add":         ECSAdd,
		"bogus":       ECSStrip,
	}
	for in, want := range cases {
		if got := ParseECSMode(in); got != want {
			t.Errorf("ParseECSMode(%q) = %q, want %q", in, got, want)
		}
	}
}
