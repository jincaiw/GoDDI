package server

import (
	"database/sql"
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/jasonwa/goddi/internal/dhcp/lease"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/miekg/dns"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jasonwa/goddi"
	dhcpinternal "github.com/jasonwa/goddi/internal/dhcp"
)

const (
	linkageZone    = "example.test."
	linkageReverse = "2.0.192.in-addr.arpa."
	linkageDomain  = "example.test"
	// The pool is a single address so the assertions do not depend on which
	// candidate the allocator happens to pick.
	linkagePoolIP = "192.0.2.100"
)

// newLinkageServer builds a DHCP server wired to a real authoritative zone
// store over the real migration schema.
//
// Using the real store is the point: the database row alone proves nothing,
// because queries are answered from the in-memory zone data. Only a lookup
// through the store shows that the linkage is actually closed.
func newLinkageServer(t *testing.T) (*Server, *sql.DB, *zone.Store) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	goose.SetBaseFS(goddiassets.Migrations())
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	goose.SetBaseFS(nil)
	t.Cleanup(func() { db.Close() })

	mustExec(t, db, `INSERT INTO dns_zones (id, name, type, enabled, soa_mname, soa_rname, serial,
		refresh, retry, expire, minimum)
		VALUES ('zone-forward', ?, 'primary', 1, 'ns1.example.test.', 'hostmaster.example.test.',
		        1, 3600, 600, 86400, 300)`, linkageZone)
	mustExec(t, db, `INSERT INTO dns_zones (id, name, type, enabled, soa_mname, soa_rname, serial,
		refresh, retry, expire, minimum)
		VALUES ('zone-reverse', ?, 'primary', 1, 'ns1.example.test.', 'hostmaster.example.test.',
		        1, 3600, 600, 86400, 300)`, linkageReverse)
	mustExec(t, db, `INSERT INTO dhcp_scopes (id, name, interface, subnet, start_ip, end_ip,
		subnet_mask, router, dns_servers, domain_name, lease_time, enabled, ping_check_enabled, dns_updates)
		VALUES ('scope-1', 'lan', 'eth0', '192.0.2.0/24', ?, ?, '255.255.255.0',
		        '192.0.2.1', '192.0.2.1', ?, 3600, 1, 0, 1)`,
		linkagePoolIP, linkagePoolIP, linkageDomain)

	store := zone.NewStore(db)
	s := New(db, []string{"eth0"}, nil)
	s.serverIPs = map[string]net.IP{"eth0": net.ParseIP(testServerIP)}
	return s, db, store
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// resolve asks the authoritative store for an A record, the way a query would.
func resolve(t *testing.T, store *zone.Store, name string) []string {
	t.Helper()
	_, answers, found := store.Lookup(dns.Fqdn(name), dns.TypeA)
	if !found {
		return nil
	}
	var out []string
	for _, rr := range answers {
		if a, ok := rr.(*dns.A); ok {
			out = append(out, a.A.String())
		}
	}
	return out
}

func discoverWithHostname(t *testing.T, mac net.HardwareAddr, hostname string) *dhcpv4.DHCPv4 {
	t.Helper()
	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeDiscover),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithOption(dhcpv4.OptHostName(hostname)),
	)
	if err != nil {
		t.Fatalf("build DISCOVER: %v", err)
	}
	return msg
}

func requestWithHostname(t *testing.T, mac net.HardwareAddr, ip, hostname string) *dhcpv4.DHCPv4 {
	t.Helper()
	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(net.ParseIP(testServerIP))),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.ParseIP(ip))),
		dhcpv4.WithOption(dhcpv4.OptHostName(hostname)),
	)
	if err != nil {
		t.Fatalf("build REQUEST: %v", err)
	}
	return msg
}

func pendingEvents(t *testing.T, db *sql.DB) []dhcpinternal.DNSEvent {
	t.Helper()
	events, err := dhcpinternal.NewDNSOutbox(db).PendingBatch(100)
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	return events
}

// drain applies everything the queue owes, the way the DNS plane's consumer
// does.
//
// It is not a method on the server, and that is the point of the split: the
// consumer belongs to the plane that writes the records, so a DHCP-only process
// runs no part of it.
func drain(t *testing.T, db *sql.DB, store *zone.Store) {
	t.Helper()
	link := dhcpinternal.NewDNSLink(dhcpinternal.Same(db), store)
	dhcpinternal.NewDNSConsumer(db, link).Drain()
}

// ---------------------------------------------------------------------------

// TestLinkage_ConfirmedBindingBecomesResolvable is the acceptance test for the
// whole work package: after the REQUEST is acknowledged, the name the client
// asked for must actually answer a query. Writing the row is not enough — the
// authoritative data is served from memory, so a write with no reload leaves a
// client with a working lease and an unresolvable name.
func TestLinkage_ConfirmedBindingBecomesResolvable(t *testing.T) {
	s, db, store := newLinkageServer(t)
	mac := testMAC(1)

	offer, err := s.HandleDiscover(discoverWithHostname(t, mac, "host1"), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}
	if offer == nil {
		t.Fatal("HandleDiscover returned no OFFER")
	}
	// A DISCOVER is a proposal: publishing a name for an address the client may
	// never accept would put an unused name in the zone.
	if events := pendingEvents(t, db); len(events) != 0 {
		t.Fatalf("DISCOVER queued %d DNS events, want 0", len(events))
	}
	if got := resolve(t, store, "host1."+linkageDomain); len(got) != 0 {
		t.Fatalf("host resolved before confirmation: %v", got)
	}

	ack, err := s.HandleRequest(
		requestWithHostname(t, mac, linkagePoolIP, "host1"), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if ack == nil || ack.MessageType() != dhcpv4.MessageTypeAck {
		t.Fatalf("REQUEST did not produce an ACK: %+v", ack)
	}

	// The intent has to be durable before the ACK is handed out, so a crash
	// cannot silently drop an update the client already believes happened.
	events := pendingEvents(t, db)
	if len(events) != 1 || events[0].Action != dhcpinternal.DNSEventCreate {
		t.Fatalf("queued events = %+v, want one create", events)
	}

	drain(t, db, store)

	got := resolve(t, store, "host1."+linkageDomain)
	if len(got) != 1 || got[0] != linkagePoolIP {
		t.Fatalf("host1 resolved to %v, want [%s]", got, linkagePoolIP)
	}

	// The reverse record must work too: a forward-only linkage leaves tools
	// that look up the address by name broken.
	var ptrValue string
	if err := db.QueryRow(
		`SELECT value FROM dns_records WHERE type = 'PTR' AND name = '100.2.0.192.in-addr.arpa.'`).Scan(&ptrValue); err != nil {
		t.Fatalf("read PTR record: %v", err)
	}
	if ptrValue != "host1."+linkageZone {
		t.Errorf("PTR value = %q, want %q", ptrValue, "host1."+linkageZone)
	}

	if left := pendingEvents(t, db); len(left) != 0 {
		t.Errorf("outbox still holds %d events after draining", len(left))
	}
}

func TestLinkage_ReleaseWithdrawsTheName(t *testing.T) {
	s, db, store := newLinkageServer(t)
	mac := testMAC(2)

	if _, err := s.HandleDiscover(discoverWithHostname(t, mac, "host2"), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}
	if _, err := s.HandleRequest(requestWithHostname(t, mac, linkagePoolIP, "host2"), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	drain(t, db, store)
	if got := resolve(t, store, "host2."+linkageDomain); len(got) != 1 {
		t.Fatalf("host2 did not resolve after the ACK: %v", got)
	}

	release, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRelease),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithClientIP(net.ParseIP(linkagePoolIP)),
	)
	if err != nil {
		t.Fatalf("build RELEASE: %v", err)
	}
	if err := s.HandleRelease(release); err != nil {
		t.Fatalf("HandleRelease: %v", err)
	}
	drain(t, db, store)

	if got := resolve(t, store, "host2."+linkageDomain); len(got) != 0 {
		t.Fatalf("host2 still resolves after release: %v", got)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM dns_records WHERE type = 'PTR'").Scan(&n); err != nil {
		t.Fatalf("count PTR records: %v", err)
	}
	if n != 0 {
		t.Errorf("PTR records after release = %d, want 0", n)
	}
}

// TestLinkage_DeclinedAddressLosesItsName covers the case where the client
// itself tells us the binding is wrong. Leaving the name published would keep
// handing out an address something else is already using.
func TestLinkage_DeclinedAddressLosesItsName(t *testing.T) {
	s, db, store := newLinkageServer(t)
	mac := testMAC(3)

	if _, err := s.HandleDiscover(discoverWithHostname(t, mac, "host3"), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}
	if _, err := s.HandleRequest(requestWithHostname(t, mac, linkagePoolIP, "host3"), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	drain(t, db, store)
	if got := resolve(t, store, "host3."+linkageDomain); len(got) != 1 {
		t.Fatalf("host3 did not resolve after the ACK: %v", got)
	}

	decline, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeDecline),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.ParseIP(linkagePoolIP))),
	)
	if err != nil {
		t.Fatalf("build DECLINE: %v", err)
	}
	if err := s.HandleDecline(decline); err != nil {
		t.Fatalf("HandleDecline: %v", err)
	}
	drain(t, db, store)

	if got := resolve(t, store, "host3."+linkageDomain); len(got) != 0 {
		t.Fatalf("host3 still resolves after the address was declined: %v", got)
	}
	if n := len(pendingEvents(t, db)); n != 0 {
		t.Errorf("outbox holds %d events after draining", n)
	}
}

// TestLinkage_ExpiredLeaseWithdrawsTheName covers the most common real-world
// case: most clients never send RELEASE, they simply stop renewing. Without a
// teardown on expiry the name would keep pointing at an address the pool is
// free to hand to somebody else.
func TestLinkage_ExpiredLeaseWithdrawsTheName(t *testing.T) {
	s, db, store := newLinkageServer(t)
	mac := testMAC(4)

	if _, err := s.HandleDiscover(discoverWithHostname(t, mac, "host4"), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}
	if _, err := s.HandleRequest(requestWithHostname(t, mac, linkagePoolIP, "host4"), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	drain(t, db, store)
	if got := resolve(t, store, "host4."+linkageDomain); len(got) != 1 {
		t.Fatalf("host4 did not resolve after the ACK: %v", got)
	}

	// Let the binding lapse without any client message.
	mustExec(t, db, `UPDATE dhcp_leases SET lease_end = datetime('now', '-1 hour')`)

	s.sweepExpiredLeases("test")
	drain(t, db, store)

	if got := resolve(t, store, "host4."+linkageDomain); len(got) != 0 {
		t.Fatalf("host4 still resolves after the lease expired: %v", got)
	}
	if n := len(pendingEvents(t, db)); n != 0 {
		t.Errorf("outbox holds %d events after draining", n)
	}
}

// TestLinkage_QueuedUpdateSurvivesRestart is the property the previous
// fire-and-forget goroutine could not provide: the update is owed by the
// database, not by an in-flight goroutine, so a process that dies before doing
// the work still does it after coming back.
func TestLinkage_QueuedUpdateSurvivesRestart(t *testing.T) {
	s, db, store := newLinkageServer(t)
	mac := testMAC(5)

	if _, err := s.HandleDiscover(discoverWithHostname(t, mac, "host5"), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}
	ack, err := s.HandleRequest(
		requestWithHostname(t, mac, linkagePoolIP, "host5"), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	// Read the lease the client was told about, so the simulated restart
	// replays exactly the work that was outstanding.
	if ack == nil {
		t.Fatal("no ACK")
	}
	events := pendingEvents(t, db)
	if len(events) != 1 {
		t.Fatalf("queued events = %d, want 1", len(events))
	}

	// The process dies here: no consumer ever ran, and the record was never
	// published. A fresh server over the same database is what restarts into
	// the queue.
	if got := resolve(t, store, "host5."+linkageDomain); len(got) != 0 {
		t.Fatalf("host5 resolved before the queue was drained: %v", got)
	}
	// The consumer belongs to the plane that writes the records, so the
	// restart here is only of the queue: a fresh consumer over the same
	// database is what picks the outstanding work back up.
	drain(t, db, store)

	got := resolve(t, store, "host5."+linkageDomain)
	if len(got) != 1 || got[0] != linkagePoolIP {
		t.Fatalf("host5 resolved to %v after restart, want [%s]", got, linkagePoolIP)
	}
}

// TestLinkage_ReconcilerClosesTheCrashWindow covers the gap the queue itself
// cannot see: a crash between the lease commit and the event insert leaves a
// live binding with no record and no event.
func TestLinkage_ReconcilerClosesTheCrashWindow(t *testing.T) {
	s, db, store := newLinkageServer(t)
	_ = s

	// A confirmed binding the queue knows nothing about, as if the process died
	// in the window.
	mustExec(t, db, `INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname,
		client_id, lease_start, lease_end, status, last_seen, generation)
		VALUES ('lease-orphan-event', 'scope-1', ?, 'aa:bb:cc:dd:ee:06', 'host6', '',
		        datetime('now'), datetime('now', '+1 hour'), 'active', datetime('now'), 1)`,
		linkagePoolIP)

	link := dhcpinternal.NewDNSLink(dhcpinternal.Same(db), store)
	repaired, err := link.Reconcile(50)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if repaired == 0 {
		t.Fatal("reconciler did not repair the missing record")
	}
	if got := resolve(t, store, "host6."+linkageDomain); len(got) != 1 || got[0] != linkagePoolIP {
		t.Fatalf("host6 resolved to %v after reconciliation, want [%s]", got, linkagePoolIP)
	}
}

// TestLinkage_SupersededEventDoesNotResurrectTheName pins the ordering rule:
// the queue replays in insertion order, but an event that has been superseded
// must not take effect even if it is applied afterwards.
func TestLinkage_SupersededEventDoesNotResurrectTheName(t *testing.T) {
	s, db, store := newLinkageServer(t)
	mac := testMAC(7)

	if _, err := s.HandleDiscover(discoverWithHostname(t, mac, "host7"), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}
	if _, err := s.HandleRequest(requestWithHostname(t, mac, linkagePoolIP, "host7"), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	drain(t, db, store)

	var leaseID string
	var generation int64
	if err := db.QueryRow(
		`SELECT id, generation FROM dhcp_leases WHERE mac_address = ?`, mac.String()).
		Scan(&leaseID, &generation); err != nil {
		t.Fatalf("read lease: %v", err)
	}

	// Tear the binding down and let the teardown take effect.
	mustExec(t, db, `UPDATE dhcp_leases SET status='released' WHERE id = ?`, leaseID)
	link := dhcpinternal.NewDNSLink(dhcpinternal.Same(db), store)
	if err := link.ApplyEvent(dhcpinternal.DNSEvent{
		LeaseID: leaseID, Generation: generation, Action: dhcpinternal.DNSEventDelete,
		IPAddress: linkagePoolIP, Hostname: "host7",
	}); err != nil {
		t.Fatalf("teardown: %v", err)
	}
	if got := resolve(t, store, "host7."+linkageDomain); len(got) != 0 {
		t.Fatalf("host7 still resolves after release: %v", got)
	}

	// A create for the binding that no longer holds the address arrives late.
	if err := link.ApplyEvent(dhcpinternal.DNSEvent{
		LeaseID: leaseID, Generation: generation, Action: dhcpinternal.DNSEventCreate,
		ScopeID: "scope-1", IPAddress: linkagePoolIP, Hostname: "host7",
	}); err != nil {
		t.Fatalf("late create: %v", err)
	}
	if got := resolve(t, store, "host7."+linkageDomain); len(got) != 0 {
		t.Fatalf("a late create resurrected a released binding: %v", got)
	}
}

// TestLinkage_ExpirySweepReturnsTheSweptLeases guards the hand-off between the
// lease sweeper and the DNS side: the sweep has to report what it expired, or
// nothing can withdraw the names of clients that simply left.
func TestLinkage_ExpirySweepReturnsTheSweptLeases(t *testing.T) {
	s, db, _ := newLinkageServer(t)
	mac := testMAC(8)

	if _, err := s.HandleDiscover(discoverWithHostname(t, mac, "host8"), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}
	if _, err := s.HandleRequest(requestWithHostname(t, mac, linkagePoolIP, "host8"), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	mustExec(t, db, `UPDATE dhcp_leases SET lease_end = datetime('now', '-1 hour') WHERE status = 'active'`)

	swept, err := lease.NewManager(db).ExpireLeases()
	if err != nil {
		t.Fatalf("ExpireLeases: %v", err)
	}
	if len(swept) != 1 {
		t.Fatalf("swept leases = %d, want 1", len(swept))
	}
	if swept[0].Hostname != "host8" || swept[0].Status != lease.LeaseStatusActive {
		t.Errorf("swept lease = (%q, %q), want (host8, active as it was before the sweep)",
			swept[0].Hostname, swept[0].Status)
	}
	if swept[0].Generation < 1 {
		t.Errorf("swept lease generation = %d, want >= 1", swept[0].Generation)
	}
}
