package server

import (
	"database/sql"
	"net"
	"path/filepath"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dataplane"
)

// These tests exercise the split the whole W07 batch exists for: the DHCP
// server reads and writes a data-plane store it owns, while the DNS rows it
// eventually feeds live in a different database. The store is the real one --
// dataplane.Open, with the real migrations -- rather than a hand-written schema,
// because a test that passes against a schema nobody ships proves nothing.
//
// The lease store carries one scope with DNS updates on and a pool of one
// address, so every path the request can take is reachable.

const (
	splitPoolIP   = "192.0.2.100"
	splitSubnet   = "192.0.2.0/24"
	splitPoolFrom = "192.0.2.100"
	splitPoolTo   = "192.0.2.100"
)

// newLeaseStore opens the lease store the product ships, for a test.
//
// Every fixture in this package goes through it. The lease manager records each
// state change in an audit table, and a fixture that is a table behind does not
// fail -- the write is logged and dropped, which is exactly what production
// does when the database is unhappy. Sharing one opener means a new migration
// reaches every fixture at once instead of one at a time.
func newLeaseStore(t *testing.T) *dataplane.Store {
	t.Helper()

	store, err := dataplane.Open(config.DataPlaneLease, filepath.Join(t.TempDir(), "leases.db"))
	if err != nil {
		t.Fatalf("open lease store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// newSplitServer returns a server, its lease store and its DNS store, all
// separate. The DNS store is deliberately a different file: if the reply path
// ever reads it, these tests fail rather than quietly reading the same rows.
func newSplitServer(t *testing.T) (*Server, *sql.DB, *sql.DB) {
	t.Helper()

	leaseStore := newLeaseStore(t)
	dnsStore, err := dataplane.Open(config.DataPlaneZone,
		filepath.Join(t.TempDir(), "dnsdata.db"))
	if err != nil {
		t.Fatalf("open dns store: %v", err)
	}
	t.Cleanup(func() { dnsStore.Close() })

	if _, err := leaseStore.Exec(`
		INSERT INTO dhcp_scopes (id, name, interface, subnet, start_ip, end_ip,
			subnet_mask, router, dns_servers, domain_name, lease_time, enabled,
			ping_check_enabled, dns_updates)
		VALUES ('scope-1', 'lan', 'eth0', ?, ?, ?, '255.255.255.0',
			'192.0.2.1', '192.0.2.1', 'example.test', 3600, 1, 0, 1)`,
		splitSubnet, splitPoolFrom, splitPoolTo); err != nil {
		t.Fatalf("insert scope into the lease store: %v", err)
	}

	// The server is given one database -- its own -- and no zone store at all.
	// A DHCP process with no DNS role has no business holding a handle to a
	// zone table it must not write to, and this is where that would show up.
	s := New(leaseStore.DB, []string{"eth0"}, nil)
	// New() derives server IPs from the host's interfaces; pin them so the test
	// does not depend on the machine it runs on.
	s.serverIPs = map[string]net.IP{"eth0": net.ParseIP(testServerIP)}
	return s, leaseStore.DB, dnsStore.DB
}

func splitCount(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count with %q: %v", query, err)
	}
	return n
}

// TestTheReplyDependsOnlyOnTheLeaseStore is the property the split is for.
//
// The DNS store is closed before the exchange. Nothing on the path from
// DISCOVER to ACK may need it: a client asking for an address must not be
// affected by the DNS plane being restarted, upgraded or gone.
func TestTheReplyDependsOnlyOnTheLeaseStore(t *testing.T) {
	s, lease, dns := newSplitServer(t)

	// Closed, not merely empty: any read or write here fails outright.
	if err := dns.Close(); err != nil {
		t.Fatalf("close the DNS store: %v", err)
	}

	mac := testMAC(0x71)
	offer, err := s.HandleDiscover(discoverMsg(t, mac), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleDiscover with the DNS store gone: %v", err)
	}
	if offer == nil {
		t.Fatal("no OFFER was built; the discover path needed something it should not")
	}

	ip := offer.YourIPAddr.String()
	ack, err := s.HandleRequest(selectingRequest(t, mac, ip), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest with the DNS store gone: %v", err)
	}
	if ack == nil {
		t.Fatal("no ACK was built; the request path needed something it should not")
	}

	var status string
	if err := lease.QueryRow(
		`SELECT status FROM dhcp_leases WHERE ip_address = ?`, ip).Scan(&status); err != nil {
		t.Fatalf("read the lease back from the lease store: %v", err)
	}
	if status != "active" {
		t.Errorf("lease status = %q, want active", status)
	}
}

// TestNoAckWhenTheLeaseCannotBeWritten is the acceptance criterion of the
// batch, stated as a test: if the binding cannot be committed, the client is
// not told it may use the address.
//
// The failure is injected with a trigger rather than by closing the store,
// because closing it would break the scope lookup first and the reply would
// never reach the write at all -- which is a different test wearing the same
// name.
func TestNoAckWhenTheLeaseCannotBeWritten(t *testing.T) {
	s, lease, _ := newSplitServer(t)

	mac := testMAC(0x72)
	// Go through the DISCOVER first: the address has to be reserved while the
	// store still works, otherwise the REQUEST would be refused for a reason
	// that has nothing to do with the write failing.
	offer, err := s.HandleDiscover(discoverMsg(t, mac), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}
	if offer == nil {
		t.Fatal("HandleDiscover returned no OFFER")
	}
	ip := offer.YourIPAddr.String()

	for _, stmt := range []string{
		`CREATE TRIGGER refuse_new_lease BEFORE INSERT ON dhcp_leases BEGIN
			SELECT RAISE(ABORT, 'simulated lease store write failure'); END;`,
		`CREATE TRIGGER refuse_lease_change BEFORE UPDATE ON dhcp_leases BEGIN
			SELECT RAISE(ABORT, 'simulated lease store write failure'); END;`,
	} {
		if _, err := lease.Exec(stmt); err != nil {
			t.Fatalf("install the failing trigger: %v", err)
		}
	}

	ack, err := s.HandleRequest(selectingRequest(t, mac, ip), "eth0", net.ParseIP(testServerIP))
	if err == nil {
		t.Fatal("HandleRequest reported success although the binding could not be committed")
	}
	if ack != nil {
		t.Fatal("an ACK was built for a binding that was never committed")
	}

	// And the same exchange does produce an ACK once the store works again, so
	// the assertion above is about the write failing and not about the
	// exchange being unable to produce an ACK at all.
	for _, name := range []string{"refuse_new_lease", "refuse_lease_change"} {
		if _, err := lease.Exec(`DROP TRIGGER ` + name); err != nil {
			t.Fatalf("drop trigger %s: %v", name, err)
		}
	}
	ack, err = s.HandleRequest(selectingRequest(t, mac, ip), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest after the store recovered: %v", err)
	}
	if ack == nil {
		t.Fatal("no ACK after the store recovered; the earlier refusal was not about the write")
	}
}

// TestTheDNSOutboxLivesWithTheLease pins where the DDNS work is recorded: in
// the data plane's own store, next to the binding that produced it.
//
// The DNS plane consumes this queue in the next step of the batch, so its
// location is not an implementation detail -- it is the interface between the
// two planes.
func TestTheReleasePathAlsoStaysOnTheLeaseStore(t *testing.T) {
	s, lease, dns := newSplitServer(t)
	if err := dns.Close(); err != nil {
		t.Fatalf("close the DNS store: %v", err)
	}

	mac := testMAC(0x74)
	offer, err := s.HandleDiscover(discoverMsg(t, mac), "eth0", net.ParseIP(testServerIP))
	if err != nil || offer == nil {
		t.Fatalf("HandleDiscover = (%v, %v)", offer, err)
	}
	ip := offer.YourIPAddr.String()
	if _, err := s.HandleRequest(selectingRequest(t, mac, ip), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	release, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRelease),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithClientIP(net.ParseIP(ip)),
	)
	if err != nil {
		t.Fatalf("build RELEASE: %v", err)
	}
	if err := s.HandleRelease(release); err != nil {
		t.Fatalf("HandleRelease with the DNS store gone: %v", err)
	}

	if status := leaseStatusIn(t, lease, ip); status != "released" {
		t.Errorf("lease status after RELEASE = %q, want released", status)
	}
}

func leaseStatusIn(t *testing.T, db *sql.DB, ip string) string {
	t.Helper()
	var status string
	if err := db.QueryRow(
		`SELECT status FROM dhcp_leases WHERE ip_address = ?
		 ORDER BY lease_end DESC LIMIT 1`, ip).Scan(&status); err != nil {
		t.Fatalf("read lease status for %s: %v", ip, err)
	}
	return status
}

// TestTheOutboxCrossesToTheDNSPlaneAndPublishesWithoutTheLeaseReplica covers
// the two stores meeting, which is the seam the split actually turns on.
//
// Three things have to hold at once, and each of them is a way the wiring could
// look right and be wrong:
//
//   - The DHCP exchange writes the queue in the lease store and no record
//     anywhere. Publishing is the DNS plane's job.
//   - What crosses the boundary is the payload, and only the payload. The
//     consumer's own retry state is per-consumer; copying one process's attempt
//     counter over another's is how a retry silently resets.
//   - A create is published even though the DNS store's lease replica has not
//     caught up. In a split deployment the replica arrives by replication and a
//     binding confirmed a second ago may not be in it. Dropping the create
//     would leave the name unpublished until the reconciler made its slow pass,
//     which is the failure the tolerance in applyCreate exists to prevent.
//
// The record published that way carries no expiry, because the lease's end was
// not visible. The last part of the test is what keeps that from being an
// unbounded name: once the replica arrives, the sweep treats an unknown expiry
// as unfinished work and fills it in.
