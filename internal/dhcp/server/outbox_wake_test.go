package server

import (
	"net"
	"testing"
	"time"
)

// The outbox wake-up, from the side that sends it.
//
// A DHCP reply is built on a goroutine that must not wait for the control
// database. Queuing the DNS change and answering the client is the whole of the
// request path's obligation; what the entry needs after that is to be pushed
// promptly, and the push belongs to a loop this package does not run. So the
// package signals it -- and stops there.

// TestQueueingADNSChangeWakesTheOutboxPush is the near half of the DDNS delay.
//
// Without the signal the row waits for the replication loop's next poll, which
// at the default interval is up to a second on this side and another on the
// other; with it, the wait on this side is the push itself.
func TestQueueingADNSChangeWakesTheOutboxPush(t *testing.T) {
	s, _, _ := newSplitServer(t)

	// Buffered and drained by nobody: the test only asks whether the signal
	// was sent, and a signal that blocks its sender is the failure this
	// package must never have.
	woke := make(chan struct{}, 16)
	s.SetOutboxWake(func() { woke <- struct{}{} })

	mac := testMAC(0x74)

	// A DISCOVER queues nothing, so it must not wake anything. If it did, a
	// client retrying a broadcast would drive a replication pass per retry.
	// The pool this fixture builds holds one address, so the offer made here is
	// the one the REQUEST below takes.
	offer, err := s.HandleDiscover(discoverMsg(t, mac), "eth0", net.ParseIP(testServerIP))
	if err != nil || offer == nil {
		t.Fatalf("HandleDiscover = (%v, %v)", offer, err)
	}
	select {
	case <-woke:
		t.Fatal("a DISCOVER woke the outbox push, but a DISCOVER queues no DNS change")
	case <-time.After(100 * time.Millisecond):
	}

	if _, err := s.HandleRequest(selectingRequest(t, mac, offer.YourIPAddr.String()),
		"eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	select {
	case <-woke:
	case <-time.After(2 * time.Second):
		t.Fatal("a queued DNS change did not wake the outbox push; it will wait for the next poll")
	}
}

// TestTheOutboxWakeIsOptional: a server with no loop to signal must serve
// clients exactly as before. The wake-up is an optimisation of a path that has
// a correct slow version, and a data plane that refused to start without one
// would turn a missing wire into an outage.
func TestTheOutboxWakeIsOptional(t *testing.T) {
	s, _, _ := newSplitServer(t)

	mac := testMAC(0x75)
	offer, err := s.HandleDiscover(discoverMsg(t, mac), "eth0", net.ParseIP(testServerIP))
	if err != nil || offer == nil {
		t.Fatalf("HandleDiscover = (%v, %v)", offer, err)
	}
	if _, err := s.HandleRequest(selectingRequest(t, mac, offer.YourIPAddr.String()),
		"eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleRequest with no wake installed: %v", err)
	}
}
