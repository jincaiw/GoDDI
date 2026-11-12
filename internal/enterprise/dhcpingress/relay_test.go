package dhcpingress

import (
	"bytes"
	"errors"
	"sync"
	"testing"
)

func relayPacket(raw []byte) Packet {
	return Packet{Interface: "eth0", Destination: ip("192.0.2.10"), Source: ip("192.0.2.20"), GIAddr: ip("198.51.100.1"), Option82: raw}
}
func TestRelayExactConditionsAndCopies(t *testing.T) {
	first := rules()[1]
	first.CircuitID = []byte{1, 2}
	first.RemoteID = []byte{0xff, 3}
	second := first
	second.CircuitID = []byte{1, 2, 3}
	second.Target.Scope = "other"
	m, err := Compile([]Rule{first, second})
	if err != nil {
		t.Fatal(err)
	}
	first.CircuitID[0] = 9
	first.RemoteID[0] = 9
	second.CircuitID[0] = 9
	raw := []byte{2, 2, 0xff, 3, 1, 2, 1, 2}
	r, err := m.Match(relayPacket(raw))
	if err != nil || r.Target.Scope != "relay" || !bytes.Equal(r.EchoOption82(), raw) {
		t.Fatalf("match=%+v err=%v", r, err)
	}
	raw[2] = 0
	echo := r.EchoOption82()
	if echo[2] != 0xff {
		t.Fatal("retained input")
	}
	echo[2] = 0
	if r.EchoOption82()[2] != 0xff {
		t.Fatal("mutable output")
	}
	raw2 := []byte{1, 3, 1, 2, 3, 2, 2, 0xff, 3}
	r, err = m.Match(relayPacket(raw2))
	if err != nil || r.Target.Scope != "other" {
		t.Fatalf("exact prefix: %+v %v", r, err)
	}
	for _, bad := range [][]byte{nil, {}, {1, 2, 1, 2}, {1, 1, 1, 2, 2, 0xff, 3}, {1, 2, 1, 2, 2, 1, 0xff}, {1, 2, 1, 2, 2, 2, 0xff, 3, 1, 1, 4}, {1, 2, 1, 2, 3, 0}, {1, 2, 1, 2, 3, 2, 1}, {1, 0}, {1}, make([]byte, 256)} {
		if _, err = m.Match(relayPacket(bad)); !errors.Is(err, ErrRejected) {
			t.Fatalf("bad option %v: %v", bad, err)
		}
	}
}
func TestUnknownRelaySuboptionsPreservedOnlyOnMatch(t *testing.T) {
	rule := rules()[1]
	rule.CircuitID = []byte{42}
	m, err := Compile([]Rule{rule})
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte{99, 2, 5, 6, 1, 1, 42, 99, 1, 7, 203, 1, 8}
	result, err := m.Match(relayPacket(raw))
	if err != nil || result.Target.Scope != "relay" || !bytes.Equal(result.EchoOption82(), raw) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	packet := relayPacket(raw)
	packet.Source = ip("192.0.2.21")
	denied, err := m.Match(packet)
	if !errors.Is(err, ErrRejected) || denied.EchoOption82() != nil || denied.Target != (Target{}) {
		t.Fatalf("unmatched relay got echo: %+v %v", denied, err)
	}
	packet = relayPacket([]byte{99, 1, 7, 1, 1, 43})
	denied, err = m.Match(packet)
	if !errors.Is(err, ErrRejected) || denied.EchoOption82() != nil {
		t.Fatalf("mismatched condition got echo: %+v %v", denied, err)
	}
}

func TestConditionOverlapRejects(t *testing.T) {
	relay := rules()[1]
	for _, pair := range [][2]Rule{
		{relay, relay},
		{func() Rule { r := relay; r.CircuitID = []byte{1}; return r }(), relay},
		{func() Rule { r := relay; r.CircuitID = []byte{1}; return r }(), func() Rule { r := relay; r.RemoteID = []byte{2}; return r }()},
		{func() Rule { r := relay; r.CircuitID = []byte{1}; return r }(), func() Rule { r := relay; r.CircuitID = []byte{1}; r.RemoteID = []byte{2}; return r }()},
	} {
		if _, err := Compile(pair[:]); !errors.Is(err, ErrInvalid) {
			t.Fatal("overlap accepted")
		}
	}
	for _, bad := range [][]byte{{}, make([]byte, 254)} {
		r := relay
		r.CircuitID = bad
		if _, err := Compile([]Rule{r}); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid condition accepted")
		}
	}
	direct := rules()[0]
	direct.CircuitID = []byte{1}
	if _, err := Compile([]Rule{direct}); !errors.Is(err, ErrInvalid) {
		t.Fatal("direct condition accepted")
	}
}
func TestRelayTupleAndDirectDowngrade(t *testing.T) {
	r := rules()[1]
	r.CircuitID = []byte{42}
	m, err := Compile([]Rule{r})
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte{1, 1, 42}
	p := relayPacket(raw)
	tests := []Packet{p, p, p, p, p}
	tests[0].Source = ip("192.0.2.21")
	tests[1].GIAddr = ip("198.51.100.2")
	tests[2].Destination = ip("192.0.2.11")
	tests[3].Interface = "eth1"
	tests[4].GIAddr = ip("0.0.0.0")
	for _, p := range tests {
		if _, err := m.Match(p); !errors.Is(err, ErrRejected) {
			t.Fatal("wrong tuple accepted")
		}
	}
}
func TestConcurrentMatchIndependentResults(t *testing.T) {
	r := rules()[1]
	r.CircuitID = []byte{42}
	m, err := Compile([]Rule{r})
	if err != nil {
		t.Fatal(err)
	}
	p := relayPacket([]byte{1, 1, 42})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				r, e := m.Match(p)
				if e != nil || !bytes.Equal(r.EchoOption82(), p.Option82) {
					t.Error("match failed")
					return
				}
				r.EchoOption82()[2] = 0
			}
		}()
	}
	wg.Wait()
}
