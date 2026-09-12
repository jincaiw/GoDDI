package server

import (
	"database/sql"
	"net"
	"path/filepath"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dataplane"
	dhcpinternal "github.com/jasonwa/goddi/internal/dhcp"
	"github.com/jasonwa/goddi/internal/dns/zone"
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
func TestTheDNSOutboxLivesWithTheLease(t *testing.T) {
	s, lease, dns := newSplitServer(t)

	mac := testMAC(0x73)
	offer, err := s.HandleDiscover(discoverMsg(t, mac), "eth0", net.ParseIP(testServerIP))
	if err != nil || offer == nil {
		t.Fatalf("HandleDiscover = (%v, %v)", offer, err)
	}
	if _, err := s.HandleRequest(selectingRequest(t, mac, offer.YourIPAddr.String()),
		"eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	pending := splitCount(t, lease,
		`SELECT COUNT(*) FROM dhcp_dns_events WHERE status = 'pending'`)
	if pending == 0 {
		t.Error("no DNS event was queued in the lease store")
	}
	// The event is owed, not published: publishing is the DNS plane's job and
	// nothing here may have written a record anywhere.
	if leftover := splitCount(t, dns, `SELECT COUNT(*) FROM dns_records`); leftover != 0 {
		t.Errorf("dns_records in the DNS store = %d, want 0 before the consumer runs", leftover)
	}
	remote := splitCount(t, dns, `SELECT COUNT(*) FROM dhcp_leases`)
	if remote != 0 {
		t.Errorf("dhcp_leases in the DNS store = %d, want 0: leases belong to the lease store", remote)
	}
}

// TestTheReleasePathAlsoStaysOnTheLeaseStore covers the other two handlers a
// client drives, so the isolation claim is not limited to DISCOVER/REQUEST.
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
func TestTheOutboxCrossesToTheDNSPlaneAndPublishesWithoutTheLeaseReplica(t *testing.T) {
	s, lease, dns := newSplitServer(t)

	for _, zone := range []struct{ id, name string }{
		{"z1", "example.test"},
		{"z2", "2.0.192.in-addr.arpa"},
	} {
		if _, err := dns.Exec(`
			INSERT INTO dns_zones (id, name, type, enabled, soa_mname, soa_rname, serial)
			VALUES (?, ?, 'master', 1, 'ns1.example.test', 'hostmaster.example.test', 1)`,
			zone.id, zone.name); err != nil {
			t.Fatalf("insert zone %s: %v", zone.name, err)
		}
	}

	mac := testMAC(0x75)
	offer, err := s.HandleDiscover(discoverWithHostname(t, mac, "host7"), "eth0", net.ParseIP(testServerIP))
	if err != nil || offer == nil {
		t.Fatalf("HandleDiscover = (%v, %v)", offer, err)
	}
	ip := offer.YourIPAddr.String()
	if _, err := s.HandleRequest(
		requestWithHostname(t, mac, ip, "host7"), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	// The queue is owed, not delivered: nothing has been published and the DNS
	// plane cannot see the entry yet.
	if n := splitCount(t, dns, `SELECT COUNT(*) FROM dns_records`); n != 0 {
		t.Fatalf("dns_records in the DNS store = %d, want 0 before replication", n)
	}
	if n := splitCount(t, dns, `SELECT COUNT(*) FROM dhcp_dns_events`); n != 0 {
		t.Fatalf("the DNS store already holds %d outbox entries; nothing should have carried them yet", n)
	}

	// Replication, as the data plane performs it: the payload columns, and only
	// those. status, attempts and next_attempt_at are left to their defaults,
	// which is what gives the consumer its own state to work from.
	rows, err := lease.Query(`
		SELECT id, lease_id, generation, action, scope_id, ip_address, mac_address,
		       hostname, created_at
		FROM dhcp_dns_events ORDER BY id`)
	if err != nil {
		t.Fatalf("read the lease store's outbox: %v", err)
	}
	type queued struct {
		id         int64
		generation int64
		leaseID    string
		action     string
		scopeID    string
		ip         string
		macAddr    string
		hostname   string
		when       string
	}
	var carried []queued
	for rows.Next() {
		var q queued
		if err := rows.Scan(&q.id, &q.leaseID, &q.generation, &q.action, &q.scopeID,
			&q.ip, &q.macAddr, &q.hostname, &q.when); err != nil {
			rows.Close()
			t.Fatalf("scan queued entry: %v", err)
		}
		carried = append(carried, q)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate the lease store's outbox: %v", err)
	}
	if len(carried) == 0 {
		t.Fatal("the DHCP exchange queued no DNS work")
	}
	for _, q := range carried {
		if _, err := dns.Exec(`
			INSERT INTO dhcp_dns_events (id, lease_id, generation, action, scope_id,
			                             ip_address, mac_address, hostname, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			q.id, q.leaseID, q.generation, q.action, q.scopeID,
			q.ip, q.macAddr, q.hostname, q.when); err != nil {
			t.Fatalf("carry outbox entry %d across: %v", q.id, err)
		}
	}

	// The scope is configuration, so it travels with the configuration sync and
	// is already here; only the lease is behind.
	if _, err := dns.Exec(`
		INSERT INTO dhcp_scopes (id, name, interface, subnet, start_ip, end_ip,
			subnet_mask, router, dns_servers, domain_name, lease_time, enabled,
			ping_check_enabled, dns_updates)
		VALUES ('scope-1', 'lan', 'eth0', ?, ?, ?, '255.255.255.0',
			'192.0.2.1', '192.0.2.1', 'example.test', 3600, 1, 0, 1)`,
		splitSubnet, splitPoolFrom, splitPoolTo); err != nil {
		t.Fatalf("carry the scope across: %v", err)
	}

	link := dhcpinternal.NewDNSLink(dhcpinternal.Same(dns), zone.NewStore(dns))
	dhcpinternal.NewDNSConsumer(dns, link).Drain()

	// The lease exists only in the lease store: replication has not run for
	// leases at all in this test. The record must exist anyway.
	if n := splitCount(t, dns, `SELECT COUNT(*) FROM dhcp_leases`); n != 0 {
		t.Fatalf("dhcp_leases in the DNS store = %d, want 0", n)
	}
	if n := splitCount(t, dns,
		`SELECT COUNT(*) FROM dns_records WHERE name = 'host7.example.test.' AND type = 'A'`); n != 1 {
		t.Errorf("forward records in the DNS store = %d, want 1", n)
	}
	if n := splitCount(t, dns,
		`SELECT COUNT(*) FROM dns_records WHERE type = 'PTR'`); n != 1 {
		t.Errorf("reverse records in the DNS store = %d, want 1", n)
	}
	// No expiry was knowable, so none was invented.
	if n := splitCount(t, dns,
		`SELECT COUNT(*) FROM dns_records WHERE owner_ref IS NOT NULL AND expires_at IS NULL`); n == 0 {
		t.Error("an expiry was written for a lease the DNS store cannot see")
	}
	// And nothing was written back into the lease store's DNS tables.
	if n := splitCount(t, lease, `SELECT COUNT(*) FROM dns_records`); n != 0 {
		t.Errorf("dns_records in the lease store = %d, want 0", n)
	}

	var leaseID string
	if err := lease.QueryRow(`SELECT id FROM dhcp_leases LIMIT 1`).Scan(&leaseID); err != nil {
		t.Fatalf("read the lease id: %v", err)
	}
	var (
		scopeID, ipAddr, macAddr, hostname, clientID string
		leaseStart, leaseEnd, status, lastSeen       string
		generation                                   int64
	)
	if err := lease.QueryRow(`
		SELECT scope_id, ip_address, mac_address, hostname, client_id,
		       lease_start, lease_end, status, last_seen, generation
		FROM dhcp_leases WHERE id = ?`, leaseID).Scan(
		&scopeID, &ipAddr, &macAddr, &hostname, &clientID,
		&leaseStart, &leaseEnd, &status, &lastSeen, &generation); err != nil {
		t.Fatalf("read the lease for the replica: %v", err)
	}
	if _, err := dns.Exec(`
		INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname,
			client_id, lease_start, lease_end, status, last_seen, generation)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		leaseID, scopeID, ipAddr, macAddr, hostname, clientID,
		leaseStart, leaseEnd, status, lastSeen, generation); err != nil {
		t.Fatalf("carry the lease across: %v", err)
	}

	dhcpinternal.NewDNSConsumer(dns, link).Sweep()

	if n := splitCount(t, dns,
		`SELECT COUNT(*) FROM dns_records WHERE owner_ref IS NOT NULL AND expires_at IS NULL`); n != 0 {
		t.Errorf("%d records still have no expiry after the sweep saw the lease", n)
	}
}
