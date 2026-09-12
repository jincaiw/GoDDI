package server

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

type recordingDurableGate struct {
	err   error
	seen  *lease.Lease
	calls int
}

func (g *recordingDurableGate) Durable(_ context.Context, value *lease.Lease) error {
	g.calls++
	g.seen = value
	return g.err
}

func TestHandleRequestWithDurableGateDoesNotChangeDefaultSQLitePath(t *testing.T) {
	s, db := newDHCPTestServer(t)
	defer db.Close()
	gate := &recordingDurableGate{}
	s.SetDurableLeaseGate(gate)

	mac := testMAC(40)
	offer, err := s.HandleDiscover(discoverMsg(t, mac), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatal(err)
	}
	ip := offer.YourIPAddr.String()
	resp, err := s.HandleRequest(selectingRequest(t, mac, ip), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil || resp.MessageType() != 5 {
		t.Fatalf("response = %v, want DHCP ACK", resp)
	}
	if gate.calls != 1 || gate.seen == nil {
		t.Fatalf("gate calls=%d lease=%v, want one lease check", gate.calls, gate.seen)
	}
}

func TestHandleRequestWithDurableGateWithholdsAckOnFailure(t *testing.T) {
	s, db := newDHCPTestServer(t)
	defer db.Close()
	want := errors.New("wal sync failed")
	gate := &recordingDurableGate{err: want}
	s.SetDurableLeaseGate(gate)

	mac := testMAC(41)
	offer, err := s.HandleDiscover(discoverMsg(t, mac), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatal(err)
	}
	ip := offer.YourIPAddr.String()
	resp, err := s.HandleRequest(selectingRequest(t, mac, ip), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatal(err)
	}
	if resp != nil {
		t.Fatalf("response = %v, want silence when durable gate fails", resp)
	}
	if gate.calls != 1 {
		t.Fatalf("gate calls = %d, want 1", gate.calls)
	}
}
