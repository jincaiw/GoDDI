package server

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

// stubReplicator stands in for the second copy.
//
// It is a stub and not the real thing on purpose: these tests are about what
// the request path does with an answer, and a real peer would make them about
// the peer. The three answers that matter are "there is no second copy", "there
// is one but it did not confirm", and "it confirmed".
type stubReplicator struct {
	bindable   bool
	confirmErr error

	mu         sync.Mutex
	attempts   []string
	confirmed  []string
	replicated []string
}

func (s *stubReplicator) MayBind() bool { return s.bindable }

func (s *stubReplicator) Confirm(_ context.Context, l *lease.Lease) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts = append(s.attempts, l.ID)
	if s.confirmErr != nil {
		return s.confirmErr
	}
	s.confirmed = append(s.confirmed, l.ID)
	return nil
}

func (s *stubReplicator) Replicate(l *lease.Lease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replicated = append(s.replicated, l.ID)
}

// attemptCount counts every call, including the ones that failed. Counting only
// the successes would make "the second copy was never asked" and "the second
// copy was asked and refused" the same number.
func (s *stubReplicator) attemptCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.attempts)
}

func (s *stubReplicator) confirmedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.confirmed)
}

func (s *stubReplicator) replicatedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.replicated)
}

func leaseRowCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_leases`).Scan(&n); err != nil {
		t.Fatalf("counting leases: %v", err)
	}
	return n
}

// offerFor takes a client through DISCOVER and returns the address it was
// offered.
func offerFor(t *testing.T, s *Server, b byte) string {
	t.Helper()
	offer, err := s.HandleDiscover(discoverMsg(t, testMAC(b)), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("DISCOVER: %v", err)
	}
	if offer == nil {
		t.Fatal("no OFFER was sent")
	}
	return offer.YourIPAddr.String()
}

// TestNoSecondCopyMeansNoAcknowledgement is the request-path half of the HA
// contract, and the assertion is the absence of a reply.
//
// A client is not told it holds an address until a second machine has it. When
// there is no second machine and no operator has approved carrying on without
// one, the answer is silence -- not a NAK, which would tell the client to
// abandon an address that is perfectly good, and not an ACK, which would be a
// promise this node cannot keep across a power cut.
func TestNoSecondCopyMeansNoAcknowledgement(t *testing.T) {
	s, db := newDHCPTestServer(t)
	repl := &stubReplicator{bindable: false}
	s.SetLeaseReplicator(repl)

	// An OFFER is not a promise: it expires, and a client that never sees it
	// has lost a round trip rather than an address. A paused node keeps
	// offering, which is what keeps a temporary loss of the second copy from
	// turning into every client losing sight of its server.
	ip := offerFor(t, s, 1)
	if n := leaseRowCount(t, db); n != 1 {
		t.Fatalf("the offer was not written: %d leases", n)
	}

	resp, err := s.HandleRequest(selectingRequest(t, testMAC(1), ip), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("REQUEST returned an error rather than staying silent: %v", err)
	}
	if resp != nil {
		t.Fatalf("a REQUEST was answered (%s) with no second copy of the binding",
			resp.MessageType())
	}
	if repl.confirmedCount() != 0 {
		t.Error("the second copy was asked to confirm a binding this node had decided not to promise")
	}
	// The behavioural assertion above is the one that matters; this one is
	// here to pin the deliberate side effect: the offer stays an offer. A
	// node that promoted the row and then stayed silent would have taken the
	// address out of the pool for the life of a lease nobody holds.
	if got := leaseStatusFor(t, db, ip); got != string(lease.LeaseStatusOffered) {
		t.Errorf("lease status = %q, want %q", got, lease.LeaseStatusOffered)
	}
}

// TestAnUnconfirmedBindingIsAlsoNotAcknowledged covers the other way the second
// copy can fail to be there: the node is willing to promise, and the mirror
// never answers.
//
// The failure mode this rules out is subtle and expensive: a socket that
// accepts a connection and then says nothing looks, from the request path,
// exactly like a healthy pair.
func TestAnUnconfirmedBindingIsAlsoNotAcknowledged(t *testing.T) {
	s, db := newDHCPTestServer(t)
	repl := &stubReplicator{bindable: true, confirmErr: errors.New("no confirmation within 2s")}
	s.SetLeaseReplicator(repl)

	ip := offerFor(t, s, 2)

	resp, err := s.HandleRequest(selectingRequest(t, testMAC(2), ip), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("REQUEST returned an error rather than staying silent: %v", err)
	}
	if resp != nil {
		t.Fatalf("a REQUEST was answered (%s) after the second copy failed to confirm",
			resp.MessageType())
	}
	if repl.attemptCount() != 1 {
		t.Fatalf("the second copy was asked to confirm %d times, want 1", repl.attemptCount())
	}
	if repl.confirmedCount() != 0 {
		t.Error("a confirmation was recorded although the second copy refused")
	}
	// The local row is committed and stays committed. Rolling it back would be
	// worse than leaving it: the mirror may well have applied it before its
	// confirmation was lost, and a row this node deleted but the mirror still
	// holds is a mirror that no longer describes a state this node was ever in.
	// The cost of the other choice is bounded and visible -- a row that expires
	// -- and the cost of this one is a mirror that lies.
	if got := leaseStatusFor(t, db, ip); got != string(lease.LeaseStatusActive) {
		t.Errorf("lease status = %q, want %q: the local commit must not be rolled back",
			got, lease.LeaseStatusActive)
	}
}

// TestAConfirmedBindingIsAcknowledged is the positive control.
//
// Without it, the two tests above would pass for a request path that had
// stopped answering REQUESTs altogether.
func TestAConfirmedBindingIsAcknowledged(t *testing.T) {
	s, db := newDHCPTestServer(t)
	repl := &stubReplicator{bindable: true}
	s.SetLeaseReplicator(repl)

	ip := offerFor(t, s, 3)
	resp, err := s.HandleRequest(selectingRequest(t, testMAC(3), ip), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("REQUEST: %v", err)
	}
	if resp == nil {
		t.Fatal("no ACK was sent although the second copy confirmed")
	}
	if resp.MessageType() != 5 { // DHCPACK
		t.Errorf("message type = %s, want ACK", resp.MessageType())
	}
	if repl.confirmedCount() != 1 {
		t.Errorf("the second copy was asked to confirm %d times, want 1", repl.confirmedCount())
	}
	if got := leaseStatusFor(t, db, ip); got != string(lease.LeaseStatusActive) {
		t.Errorf("lease status = %q, want %q", got, lease.LeaseStatusActive)
	}
}

// TestNoPairMeansTheBehaviourThatWasAlwaysThere is the compatibility control.
//
// A single node has no second copy and never did. Installing no replicator must
// leave the request path exactly as it was before the hook existed -- an
// unacknowledged REQUEST here would be a deployment that stopped handing out
// addresses because a feature it never asked for was added.
func TestNoPairMeansTheBehaviourThatWasAlwaysThere(t *testing.T) {
	s, _ := newDHCPTestServer(t)

	ip := offerFor(t, s, 4)
	resp, err := s.HandleRequest(selectingRequest(t, testMAC(4), ip), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("REQUEST: %v", err)
	}
	if resp == nil {
		t.Fatal("a single-node deployment stopped answering REQUESTs")
	}
}

// TestAnOfferIsReplicatedWithoutWaiting pins the one kind of change that does
// not travel the confirmed path.
//
// An offer replicated synchronously would put a second round of network waits
// in front of every DISCOVER for no durability gain: nothing was promised. It
// is replicated all the same, because a mirror that has never heard of the
// offer would hand the same address to the next client after a takeover.
func TestAnOfferIsReplicatedWithoutWaiting(t *testing.T) {
	s, _ := newDHCPTestServer(t)
	repl := &stubReplicator{bindable: true}
	s.SetLeaseReplicator(repl)

	offerFor(t, s, 5)
	if repl.replicatedCount() == 0 {
		t.Error("an offer was not recorded for the second copy")
	}
	if repl.confirmedCount() != 0 {
		t.Error("an offer waited for the second copy; an offer is not a promise")
	}
}
