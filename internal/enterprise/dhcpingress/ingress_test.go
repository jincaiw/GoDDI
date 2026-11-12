package dhcpingress

import (
	"errors"
	"net/netip"
	"strings"
	"sync"
	"testing"
)

func ip(s string) netip.Addr { return netip.MustParseAddr(s) }
func rules() []Rule {
	return []Rule{
		{Interface: "eth0", Destination: ip("255.255.255.255"), Target: Target{Tenant: "t1", Space: "s1", Scope: "direct"}},
		{Interface: "eth0", Destination: ip("192.0.2.10"), Relay: ip("192.0.2.20"), GIAddr: ip("198.51.100.1"), Target: Target{Tenant: "t2", Space: "s2", Scope: "relay"}},
	}
}
func TestExactMatchingAndBypasses(t *testing.T) {
	m, e := Compile(rules())
	if e != nil {
		t.Fatal(e)
	}
	direct := Packet{Interface: "eth0", Destination: ip("255.255.255.255"), Source: ip("0.0.0.0"), GIAddr: ip("0.0.0.0")}
	relay := Packet{Interface: "eth0", Destination: ip("192.0.2.10"), Source: ip("192.0.2.20"), GIAddr: ip("198.51.100.1")}
	for _, tc := range []struct {
		p     Packet
		scope string
	}{{direct, "direct"}, {relay, "relay"}} {
		got, e := m.Match(tc.p)
		if e != nil || got.Target.Scope != tc.scope {
			t.Fatalf("%+v %v", got, e)
		}
	}
	bad := []Packet{relay, relay, relay, relay, relay, relay, relay, direct, direct}
	bad[0].Source = ip("192.0.2.21")
	bad[1].GIAddr = ip("198.51.100.2")
	bad[2].Interface = "eth1"
	bad[3].Destination = ip("192.0.2.11")
	bad[4].Option82 = []byte{99, 0}
	bad[5].GIAddr = ip("0.0.0.0")
	bad[6].GIAddr = netip.Addr{}
	bad[7].Option82 = []byte{1, 1, 1}
	bad[8].Destination = ip("192.0.2.10")
	for _, p := range bad {
		if got, e := m.Match(p); !errors.Is(e, ErrRejected) || got.Target != (Target{}) {
			t.Fatalf("accepted %+v: %+v %v", p, got, e)
		}
	}
}
func TestCompileRejectsAmbiguityAndInvalid(t *testing.T) {
	duplicate := rules()
	duplicate = append(duplicate, duplicate[0])
	duplicate[2].Target.Scope = "other"
	if m, e := Compile(duplicate); m != nil || !errors.Is(e, ErrInvalid) {
		t.Fatalf("%v %v", m, e)
	}
	for _, mutate := range []func(*Rule){func(r *Rule) { r.Interface = "" }, func(r *Rule) { r.Interface = strings.Repeat("a", MaxInterfaceBytes+1) }, func(r *Rule) { r.Target.Space = "" }, func(r *Rule) { r.Target.Scope = "bad value" }, func(r *Rule) { r.Relay = ip("192.0.2.1") }, func(r *Rule) { r.Destination = ip("::1") }, func(r *Rule) { r.Destination = ip("0.0.0.0") }} {
		r := rules()[0]
		mutate(&r)
		if m, e := Compile([]Rule{r}); m != nil || !errors.Is(e, ErrInvalid) {
			t.Fatalf("accepted %+v", r)
		}
	}
	if _, e := Compile(nil); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := Compile(make([]Rule, MaxEntries+1)); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
func TestDirectAndRelayCannotShareLocation(t *testing.T) {
	base := rules()
	direct, relay := base[0], base[1]
	direct.Destination = relay.Destination
	for _, pair := range [][]Rule{{direct, relay}, {relay, direct}} {
		if m, err := Compile(pair); m != nil || !errors.Is(err, ErrInvalid) {
			t.Fatalf("mixed location accepted: %v %v", m, err)
		}
	}
	secondRelay := relay
	secondRelay.Relay = ip("192.0.2.21")
	for _, allowed := range [][]Rule{{direct}, {relay}, {relay, secondRelay}} {
		if _, err := Compile(allowed); err != nil {
			t.Fatalf("valid location rejected: %v", err)
		}
	}
}

func TestImmutabilityAndConcurrency(t *testing.T) {
	input := rules()
	m, e := Compile(input)
	if e != nil {
		t.Fatal(e)
	}
	input[1].Target.Scope = "changed"
	input[1].Relay = ip("192.0.2.99")
	p := Packet{Interface: "eth0", Destination: ip("192.0.2.10"), Source: ip("192.0.2.20"), GIAddr: ip("198.51.100.1")}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				v, e := m.Match(p)
				if e != nil || v.Target.Scope != "relay" {
					t.Errorf("%+v %v", v, e)
					return
				}
			}
		}()
	}
	wg.Wait()
}
