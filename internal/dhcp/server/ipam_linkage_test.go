package server

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	dhcpinternal "github.com/jasonwa/goddi/internal/dhcp"
	"github.com/jasonwa/goddi/internal/dhcp/lease"
	"github.com/jasonwa/goddi/internal/facts"
	"github.com/jasonwa/goddi/internal/ipam"
	"github.com/jasonwa/goddi/internal/ipam/address"
	"github.com/jasonwa/goddi/internal/ipam/space"
	"github.com/jasonwa/goddi/internal/ipam/subnet"
)

// These tests close the DHCP -> IPAM half of the linkage over the real
// migration schema and the real ipam.Linkage, driven by real DHCP messages.
//
// The DHCP->DNS linkage (ddns_linkage_test.go) proves a name resolves. This
// file proves the *address* is accounted for: that an allocation made in IPAM
// records what the lease says, that a release stops reporting the address as
// in use without giving it away, and above all that an administrative decision
// recorded in IPAM is not silently overwritten by the next renewal.
//
// Assertions read the IPAM tables directly rather than through a manager
// helper where the helper could hide a discrepancy; where a row has to be
// interpreted, the column is named explicitly.

const ipamLinkageIP = linkagePoolIP

// newIPAMLinkedServer wires the real ipam.Linkage into a DHCP server over the
// real schema, with an IPAM space and subnet covering the DHCP scope.
func newIPAMLinkedServer(t *testing.T) (*Server, *sql.DB, *ipam.Linkage, string) {
	t.Helper()

	s, db, _ := newLinkageServer(t)
	// dhcp_leases -> dhcp_scopes and ipam_addresses -> ipam_subnets are
	// foreign keys; without this any accidental orphan would pass unnoticed.
	mustExec(t, db, "PRAGMA foreign_keys = ON")

	sp, err := space.NewManager(db).CreateSpace("corp", "test space")
	if err != nil {
		t.Fatalf("create space: %v", err)
	}
	if _, err := subnet.NewManager(db).CreateSubnet(sp.ID, "lan", "192.0.2.0/24", subnet.SubnetOptions{}); err != nil {
		t.Fatalf("create subnet: %v", err)
	}

	ensureReleaseFactsSchema(t, db)

	link := ipam.NewLinkage(db)
	s.SetLeaseObserver(link)
	return s, db, link, sp.ID
}

// ensureReleaseFactsSchema installs the migration-stage facts tables in this
// combined control/lease fixture. Production keeps these tables in the lease
// data-plane migration set; this fixture intentionally shares one in-memory
// database because the server's IPAM linkage tests exercise both sides through
// one SQL handle.
func ensureReleaseFactsSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec(t, db, `
		CREATE TABLE IF NOT EXISTS dhcp_ipam_observation_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id TEXT NOT NULL UNIQUE,
			version INTEGER NOT NULL CHECK (version > 0),
			entity TEXT NOT NULL,
			action TEXT NOT NULL,
			generation INTEGER NOT NULL CHECK (generation >= 0),
			sequence INTEGER NOT NULL UNIQUE CHECK (sequence > 0),
			source TEXT NOT NULL,
			occurred_at DATETIME NOT NULL,
			payload_version INTEGER NOT NULL CHECK (payload_version > 0),
			payload TEXT NOT NULL CHECK (json_valid(payload)),
			attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
			next_attempt_at DATETIME NOT NULL DEFAULT (datetime('now')),
			last_error TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending'
				CHECK (status IN ('pending', 'done', 'failed')),
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`)
	mustExec(t, db, `
		CREATE TABLE IF NOT EXISTS facts_projection_watermark (
			domain TEXT PRIMARY KEY,
			applied_seq INTEGER NOT NULL DEFAULT 0 CHECK (applied_seq >= 0)
		)`)
	mustExec(t, db, `
		CREATE TABLE IF NOT EXISTS facts_sequence_allocator (
			domain TEXT PRIMARY KEY,
			last_sequence INTEGER NOT NULL DEFAULT 0 CHECK (last_sequence >= 0)
		)`)
}

// ipamAddress reads the address row the operator would see.
func ipamAddress(t *testing.T, link *ipam.Linkage, spaceID, ip string) *address.Address {
	t.Helper()
	a, err := link.Addresses().GetAddressBySpaceIP(spaceID, ip)
	if err != nil {
		t.Fatalf("read ipam address %s: %v", ip, err)
	}
	if a == nil {
		t.Fatalf("no ipam address row for %s", ip)
	}
	return a
}

// countHistory counts history rows for an address, optionally filtered by the
// status the row moved to.
func countHistory(t *testing.T, db *sql.DB, ip, newStatus string) int {
	t.Helper()
	var n int
	query := `SELECT COUNT(*) FROM ipam_history WHERE ip_address = ?`
	args := []interface{}{ip}
	if newStatus != "" {
		query += ` AND new_status = ?`
		args = append(args, newStatus)
	}
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count ipam history: %v", err)
	}
	return n
}

func dhcpExchange(t *testing.T, s *Server, mac net.HardwareAddr, hostname string) {
	t.Helper()
	if _, err := s.HandleDiscover(
		discoverWithHostname(t, mac, hostname), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}
	ack, err := s.HandleRequest(
		requestWithHostname(t, mac, ipamLinkageIP, hostname), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if ack == nil || ack.MessageType() != dhcpv4.MessageTypeAck {
		t.Fatalf("REQUEST did not produce an ACK: %+v", ack)
	}
}

func sendRelease(t *testing.T, s *Server, mac net.HardwareAddr) {
	t.Helper()
	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRelease),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithClientIP(net.ParseIP(ipamLinkageIP)),
	)
	if err != nil {
		t.Fatalf("build RELEASE: %v", err)
	}
	if err := s.HandleRelease(msg); err != nil {
		t.Fatalf("HandleRelease: %v", err)
	}
}

type releaseFactsCallerProbe struct {
	writer    *lease.FactsMutationWriter
	calls     int
	eventIDs  []string
	wakeCalls int
}

func (p *releaseFactsCallerProbe) ReleaseLease(ctx context.Context, eventID, source, spaceID, id string) (*lease.Lease, error) {
	p.calls++
	p.eventIDs = append(p.eventIDs, eventID)
	return p.writer.ReleaseLease(ctx, eventID, source, spaceID, id)
}

type releaseFactsDNSSinkFailure struct{}

func (releaseFactsDNSSinkFailure) EnqueueTx(*sql.Tx, *lease.Lease, lease.DNSMutationAction) error {
	return errors.New("legacy DNS sink unavailable")
}

type releaseFactsObserverProbe struct {
	db        *sql.DB
	calls     int
	status    string
	factCount int
}

func (p *releaseFactsObserverProbe) ObserveLease(string, string, string, string, string, string) error {
	p.calls++
	_ = p.db.QueryRow(`SELECT status FROM dhcp_leases WHERE id = (SELECT id FROM dhcp_leases LIMIT 1)`).Scan(&p.status)
	_ = p.db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events`).Scan(&p.factCount)
	return nil
}

type releaseFactsReplicatorProbe struct {
	db        *sql.DB
	calls     int
	status    string
	factCount int
}

func (p *releaseFactsReplicatorProbe) MayBind() bool { return true }

func (p *releaseFactsReplicatorProbe) Confirm(context.Context, *lease.Lease) error { return nil }

func (p *releaseFactsReplicatorProbe) Replicate(l *lease.Lease) {
	p.calls++
	if l != nil {
		p.status = string(l.Status)
	}
	_ = p.db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events`).Scan(&p.factCount)
}

func newReleaseFactsServer(t *testing.T) (*Server, *sql.DB, *releaseFactsCallerProbe, *ipam.ScopeIdentityResolver, string) {
	t.Helper()
	s, db, _, spaceID := newIPAMLinkedServer(t)
	allocator, err := facts.NewSequenceAllocator(db)
	if err != nil {
		t.Fatalf("create facts sequence allocator: %v", err)
	}
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatalf("create facts outbox: %v", err)
	}
	writer, err := lease.NewFactsMutationWriter(lease.NewManager(db), allocator, outbox)
	if err != nil {
		t.Fatalf("create facts mutation writer: %v", err)
	}
	resolver, err := ipam.NewScopeIdentityResolver(db)
	if err != nil {
		t.Fatalf("create scope identity resolver: %v", err)
	}
	probe := &releaseFactsCallerProbe{writer: writer}
	writer.WithPostCommitWake(func() { probe.wakeCalls++ })
	s.SetReleaseFactsMutation(&ReleaseFactsMutationConfig{
		Caller: probe,
		Source: "dhcp-node-a",
		ResolveSpaceID: func(scopeID, ip string) (string, error) {
			identity, err := resolver.Resolve(scopeID, ip)
			return identity.SpaceID, err
		},
	})
	return s, db, probe, resolver, spaceID
}

func TestDefaultLeaseFactsWriterCoversRequestAndReleaseAtomically(t *testing.T) {
	s, db, _, _ := newIPAMLinkedServer(t)
	allocator, err := facts.NewSequenceAllocator(db)
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := lease.NewFactsMutationWriter(lease.NewManager(db), allocator, outbox)
	if err != nil {
		t.Fatal(err)
	}
	writer.WithDNSSink(dhcpinternal.NewScopeAwareDNSMutationSink(db))
	resolver, err := ipam.NewScopeIdentityResolver(db)
	if err != nil {
		t.Fatal(err)
	}
	s.SetLeaseFactsMutation(&LeaseFactsMutationConfig{
		Caller: writer, Source: "dhcp-node-a", DNSOutboxAtomic: true,
		ResolveSpaceID: func(scopeID, ip string) (string, error) {
			identity, err := resolver.Resolve(scopeID, ip)
			return identity.SpaceID, err
		},
	})

	mac := testMAC(24)
	dhcpExchange(t, s, mac, "host24")
	var factsCount, dnsCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events`).Scan(&factsCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_dns_events`).Scan(&dnsCount); err != nil {
		t.Fatal(err)
	}
	if factsCount != 1 || dnsCount != 1 {
		t.Fatalf("after REQUEST: facts=%d DNS events=%d, want 1/1", factsCount, dnsCount)
	}

	release, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRelease),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithClientIP(net.ParseIP(ipamLinkageIP)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.HandleRelease(release); err != nil {
		t.Fatalf("HandleRelease: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events`).Scan(&factsCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_dns_events`).Scan(&dnsCount); err != nil {
		t.Fatal(err)
	}
	if factsCount != 2 || dnsCount != 2 {
		t.Fatalf("after RELEASE: facts=%d DNS events=%d, want 2/2", factsCount, dnsCount)
	}
}

func TestHandleReleaseFactsOptInCommitsLeaseAndFactBeforePostCommitSideEffects(t *testing.T) {
	s, db, caller, _, _ := newReleaseFactsServer(t)
	mac := testMAC(20)
	dhcpExchange(t, s, mac, "host20")

	observer := &releaseFactsObserverProbe{db: db}
	replicator := &releaseFactsReplicatorProbe{db: db}
	s.SetLeaseObserver(observer)
	s.SetLeaseReplicator(replicator)

	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRelease),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithClientIP(net.ParseIP(ipamLinkageIP)),
	)
	if err != nil {
		t.Fatalf("build RELEASE: %v", err)
	}
	if err := s.HandleRelease(msg); err != nil {
		t.Fatalf("HandleRelease: %v", err)
	}

	if caller.calls != 1 {
		t.Fatalf("facts caller calls = %d, want 1", caller.calls)
	}
	if len(caller.eventIDs) != 1 || caller.eventIDs[0] != "" {
		t.Fatalf("caller event IDs = %#v, want one omitted event ID", caller.eventIDs)
	}
	if got := leaseStatusFor(t, db, ipamLinkageIP); got != string(lease.LeaseStatusReleased) {
		t.Fatalf("lease status = %q, want released", got)
	}
	var action string
	if err := db.QueryRow(`SELECT action FROM dhcp_ipam_observation_events`).Scan(&action); err != nil {
		t.Fatalf("read release fact: %v", err)
	}
	if action != string(lease.MutationRelease) {
		t.Fatalf("fact action = %q, want %q", action, lease.MutationRelease)
	}
	if caller.wakeCalls != 1 {
		t.Fatalf("post-commit wake calls = %d, want 1", caller.wakeCalls)
	}
	if observer.calls != 1 || observer.status != string(lease.LeaseStatusReleased) || observer.factCount != 1 {
		t.Fatalf("observer calls=%d status=%q facts=%d, want committed release and one fact", observer.calls, observer.status, observer.factCount)
	}
	if replicator.calls != 1 || replicator.status != string(lease.LeaseStatusReleased) || replicator.factCount != 1 {
		t.Fatalf("replicator calls=%d status=%q facts=%d, want committed release and one fact", replicator.calls, replicator.status, replicator.factCount)
	}
}

func TestHandleReleaseFactsFailureDoesNotFallbackOrTriggerPostCommitSideEffects(t *testing.T) {
	s, db, caller, _, _ := newReleaseFactsServer(t)
	caller.writer.WithDNSSink(releaseFactsDNSSinkFailure{})
	mac := testMAC(21)
	dhcpExchange(t, s, mac, "host21")

	observer := &releaseFactsObserverProbe{db: db}
	replicator := &releaseFactsReplicatorProbe{db: db}
	s.SetLeaseObserver(observer)
	s.SetLeaseReplicator(replicator)

	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRelease),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithClientIP(net.ParseIP(ipamLinkageIP)),
	)
	if err != nil {
		t.Fatalf("build RELEASE: %v", err)
	}
	if err := s.HandleRelease(msg); err == nil {
		t.Fatal("facts failure unexpectedly succeeded")
	}

	if caller.calls != 1 {
		t.Fatalf("facts caller calls = %d, want 1", caller.calls)
	}
	if got := leaseStatusFor(t, db, ipamLinkageIP); got != string(lease.LeaseStatusActive) {
		t.Fatalf("lease status after facts failure = %q, want active; legacy fallback or partial commit occurred", got)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events`).Scan(&count); err != nil {
		t.Fatalf("count facts after failure: %v", err)
	}
	if count != 0 {
		t.Fatalf("facts after failed release = %d, want 0", count)
	}
	if caller.wakeCalls != 0 {
		t.Fatalf("post-commit wake calls after failed release = %d, want 0", caller.wakeCalls)
	}
	if observer.calls != 0 {
		t.Fatalf("observer calls after failed release = %d, want 0", observer.calls)
	}
	if replicator.calls != 0 {
		t.Fatalf("replicator calls after failed release = %d, want 0", replicator.calls)
	}
}

func TestHandleReleaseFactsIdentityMissingFailsClosed(t *testing.T) {
	s, db, caller, resolver, _ := newReleaseFactsServer(t)
	mac := testMAC(22)
	dhcpExchange(t, s, mac, "host22")
	s.SetReleaseFactsMutation(&ReleaseFactsMutationConfig{
		Caller: caller,
		Source: "",
		ResolveSpaceID: func(scopeID, ip string) (string, error) {
			identity, err := resolver.Resolve(scopeID, ip)
			return identity.SpaceID, err
		},
	})

	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRelease),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithClientIP(net.ParseIP(ipamLinkageIP)),
	)
	if err != nil {
		t.Fatalf("build RELEASE: %v", err)
	}
	if err := s.HandleRelease(msg); err == nil {
		t.Fatal("missing facts identity unexpectedly succeeded")
	}
	if caller.calls != 0 {
		t.Fatalf("facts caller calls after missing identity = %d, want 0", caller.calls)
	}
	if got := leaseStatusFor(t, db, ipamLinkageIP); got != string(lease.LeaseStatusActive) {
		t.Fatalf("lease status after missing identity = %q, want active", got)
	}
}

func TestHandleReleaseDefaultsToLegacySQLiteMutation(t *testing.T) {
	s, db := newDHCPTestServer(t)
	if s.releaseFacts != nil {
		t.Fatal("default server unexpectedly enabled RELEASE facts mutation")
	}
	mac := testMAC(23)
	if _, err := s.HandleDiscover(discoverMsg(t, mac), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}
	if _, err := s.HandleRequest(selectingRequest(t, mac, "192.0.2.100"), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRelease),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithClientIP(net.ParseIP("192.0.2.100")),
	)
	if err != nil {
		t.Fatalf("build RELEASE: %v", err)
	}
	if err := s.HandleRelease(msg); err != nil {
		t.Fatalf("legacy HandleRelease: %v", err)
	}
	if got := leaseStatusFor(t, db, "192.0.2.100"); got != string(lease.LeaseStatusReleased) {
		t.Fatalf("legacy lease status = %q, want released", got)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events`).Scan(&count); err != nil {
		t.Fatalf("count default facts: %v", err)
	}
	if count != 0 {
		t.Fatalf("default legacy release wrote %d facts", count)
	}
}

func sendDecline(t *testing.T, s *Server, mac net.HardwareAddr) {
	t.Helper()
	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeDecline),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.ParseIP(ipamLinkageIP))),
	)
	if err != nil {
		t.Fatalf("build DECLINE: %v", err)
	}
	if err := s.HandleDecline(msg); err != nil {
		t.Fatalf("HandleDecline: %v", err)
	}
}

// ---------------------------------------------------------------------------

// TestIPAMLinkage_OfferIsNotAnObservation pins the rule that an OFFER is a
// proposal. Recording it as in-use would make a scanner walking the range —
// or any client that never sends REQUEST — mark the whole pool as taken in
// IPAM while the addresses stay free.
func TestIPAMLinkage_OfferIsNotAnObservation(t *testing.T) {
	s, _, link, spaceID := newIPAMLinkedServer(t)
	mac := testMAC(11)

	if _, err := s.HandleDiscover(
		discoverWithHostname(t, mac, "host11"), "eth0", net.ParseIP(testServerIP)); err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}

	a := ipamAddress(t, link, spaceID, ipamLinkageIP)
	if a.Status != address.StatusAvailable {
		t.Errorf("status after DISCOVER = %q, want %q (an offer allocates nothing)", a.Status, address.StatusAvailable)
	}
	if a.ObservedState != address.ObservedUnknown {
		t.Errorf("observed_state after DISCOVER = %q, want %q", a.ObservedState, address.ObservedUnknown)
	}
	if a.DHCPLeaseID != "" {
		t.Errorf("dhcp_lease_id = %q after DISCOVER, want empty", a.DHCPLeaseID)
	}
}

// TestIPAMLinkage_AckAllocatesTheAddress is the acceptance test for the DHCP
// half: after the ACK the address must read as allocated to this client, with
// the client's identity attached, and the change must be in the audit trail.
func TestIPAMLinkage_AckAllocatesTheAddress(t *testing.T) {
	s, db, link, spaceID := newIPAMLinkedServer(t)
	mac := testMAC(12)

	dhcpExchange(t, s, mac, "host12")

	a := ipamAddress(t, link, spaceID, ipamLinkageIP)
	if a.Status != address.StatusDHCP {
		t.Fatalf("status after ACK = %q, want %q", a.Status, address.StatusDHCP)
	}
	if a.ObservedState != address.ObservedInUse {
		t.Errorf("observed_state = %q, want %q", a.ObservedState, address.ObservedInUse)
	}
	if a.ObservedSource != address.SourceDHCP {
		t.Errorf("observed_source = %q, want %q", a.ObservedSource, address.SourceDHCP)
	}
	if a.ObservedMAC != mac.String() {
		t.Errorf("observed_mac = %q, want %q", a.ObservedMAC, mac.String())
	}
	if a.ObservedHostname != "host12" {
		t.Errorf("observed_hostname = %q, want %q", a.ObservedHostname, "host12")
	}
	if a.DHCPLeaseID == "" {
		t.Error("dhcp_lease_id is empty after a confirmed binding")
	}

	// Without the audit row a state change is indistinguishable from a bug.
	if n := countHistory(t, db, ipamLinkageIP, string(address.StatusDHCP)); n != 1 {
		t.Errorf("history rows recording the move to dhcp = %d, want 1", n)
	}
	var actor, reason string
	if err := db.QueryRow(
		`SELECT changed_by, reason FROM ipam_history WHERE ip_address = ? AND new_status = ?`,
		ipamLinkageIP, string(address.StatusDHCP)).Scan(&actor, &reason); err != nil {
		t.Fatalf("read history: %v", err)
	}
	if actor != "dhcp" {
		t.Errorf("history changed_by = %q, want %q", actor, "dhcp")
	}
	if reason == "" {
		t.Error("history reason is empty; an unexplained status change cannot be reviewed")
	}
}

// TestIPAMLinkage_RenewDoesNotDuplicateTheAllocation covers the ordinary case
// of a client that keeps renewing. Every renewal is an observation, but only
// the first one is an allocation: writing a status change and a history row
// per renewal would bury the one row an operator needs under thousands.
func TestIPAMLinkage_RenewDoesNotDuplicateTheAllocation(t *testing.T) {
	s, db, link, spaceID := newIPAMLinkedServer(t)
	mac := testMAC(13)

	dhcpExchange(t, s, mac, "host13")

	renew, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithOption(dhcpv4.OptHostName("host13")),
	)
	if err != nil {
		t.Fatalf("build renewing REQUEST: %v", err)
	}
	// RENEWING carries ciaddr and no server identifier (RFC 2131 §4.3.2).
	renew.ClientIPAddr = net.ParseIP(ipamLinkageIP)
	ack, err := s.HandleRequest(renew, "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("renewing REQUEST: %v", err)
	}
	if ack == nil || ack.MessageType() != dhcpv4.MessageTypeAck {
		t.Fatalf("renewal did not produce an ACK: %+v", ack)
	}

	a := ipamAddress(t, link, spaceID, ipamLinkageIP)
	if a.Status != address.StatusDHCP {
		t.Errorf("status after renewal = %q, want %q", a.Status, address.StatusDHCP)
	}
	if n := countHistory(t, db, ipamLinkageIP, string(address.StatusDHCP)); n != 1 {
		t.Errorf("history rows for the allocation = %d, want 1 after a renewal", n)
	}
}

// TestIPAMLinkage_ReleaseFreesTheObservationButKeepsTheAllocation is the
// distinction the split between status and observed_state exists for. The
// client stopped using the address, so IPAM must stop saying it is in use; it
// does not follow that an administrator's decision recorded as 'dhcp'
// allocation should be erased.
func TestIPAMLinkage_ReleaseFreesTheObservationButKeepsTheAllocation(t *testing.T) {
	s, _, link, spaceID := newIPAMLinkedServer(t)
	mac := testMAC(14)

	dhcpExchange(t, s, mac, "host14")
	sendRelease(t, s, mac)

	a := ipamAddress(t, link, spaceID, ipamLinkageIP)
	if a.ObservedState != address.ObservedFree {
		t.Errorf("observed_state after RELEASE = %q, want %q", a.ObservedState, address.ObservedFree)
	}
	if a.Status != address.StatusDHCP {
		t.Errorf("status after RELEASE = %q, want %q (a release is not a deallocation)",
			a.Status, address.StatusDHCP)
	}
	if a.ObservedSource != address.SourceDHCP {
		t.Errorf("observed_source = %q, want %q", a.ObservedSource, address.SourceDHCP)
	}
}

// TestIPAMLinkage_ExpirySweepFreesTheObservation covers the common real case:
// most clients never send RELEASE, they simply stop renewing. If the sweep
// did not report back, the address would read as in use forever and the IPAM
// view would drift further from the pool with every lapsed client.
func TestIPAMLinkage_ExpirySweepFreesTheObservation(t *testing.T) {
	s, db, link, spaceID := newIPAMLinkedServer(t)
	mac := testMAC(15)

	dhcpExchange(t, s, mac, "host15")
	mustExec(t, db, `UPDATE dhcp_leases SET lease_end = datetime('now', '-1 hour') WHERE status = 'active'`)

	s.sweepExpiredLeases("test")

	a := ipamAddress(t, link, spaceID, ipamLinkageIP)
	if a.ObservedState != address.ObservedFree {
		t.Errorf("observed_state after the lease expired = %q, want %q", a.ObservedState, address.ObservedFree)
	}
}

// TestIPAMLinkage_DeclineMarksTheAddressContested pins the DECLINE semantics.
// The client is telling us the address is already used by another device, so
// it must not be filed as free: doing so would hand a known-conflicting
// address straight to the next client and reproduce the decline loop the
// quarantine exists to stop.
func TestIPAMLinkage_DeclineMarksTheAddressContested(t *testing.T) {
	s, _, link, spaceID := newIPAMLinkedServer(t)
	mac := testMAC(16)

	dhcpExchange(t, s, mac, "host16")
	sendDecline(t, s, mac)

	a := ipamAddress(t, link, spaceID, ipamLinkageIP)
	if a.ObservedState != address.ObservedContested {
		t.Errorf("observed_state after DECLINE = %q, want %q", a.ObservedState, address.ObservedContested)
	}
	if a.Status != address.StatusConflict {
		t.Errorf("status after DECLINE = %q, want %q", a.Status, address.StatusConflict)
	}
}

// TestIPAMLinkage_DeclineWithoutALeaseIsStillRecorded covers the report that
// has no lease row behind it — a statically configured host, or a conflict
// reported by a peer. Dropping it would leave the address marked free.
func TestIPAMLinkage_DeclineWithoutALeaseIsStillRecorded(t *testing.T) {
	s, _, link, spaceID := newIPAMLinkedServer(t)

	// No DISCOVER, no REQUEST: this client never got an address from us.
	sendDecline(t, s, testMAC(17))

	a := ipamAddress(t, link, spaceID, ipamLinkageIP)
	if a.ObservedState != address.ObservedContested {
		t.Errorf("observed_state = %q, want %q", a.ObservedState, address.ObservedContested)
	}
	if a.Status != address.StatusConflict {
		t.Errorf("status = %q, want %q", a.Status, address.StatusConflict)
	}
}

// TestIPAMLinkage_AdministrativeDecisionSurvivesADHCPRenewal is the regression
// this work package exists for. The address was fenced off by an operator; a
// DHCP server that hands it out anyway is a real conflict that must be raised,
// not a status update that quietly replaces the operator's decision.
func TestIPAMLinkage_AdministrativeDecisionSurvivesADHCPRenewal(t *testing.T) {
	s, _, link, spaceID := newIPAMLinkedServer(t)
	mac := testMAC(18)

	subs, _, err := link.Subnets().ListSubnets(subnet.SubnetFilter{SpaceID: spaceID})
	if err != nil {
		t.Fatalf("list subnets: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("subnets = %d, want 1", len(subs))
	}
	if _, err := link.Addresses().AllocateIP(address.AllocateRequest{
		SubnetID:  subs[0].ID,
		IPAddress: ipamLinkageIP,
		Status:    address.StatusReserved,
		Owner:     "ops",
		Actor:     "ops",
		Reason:    "fenced off for the lab switch",
	}); err != nil {
		t.Fatalf("reserve address: %v", err)
	}

	dhcpExchange(t, s, mac, "host18")

	a := ipamAddress(t, link, spaceID, ipamLinkageIP)
	if a.Status != address.StatusConflict {
		t.Fatalf("status = %q after DHCP used a reserved address, want %q",
			a.Status, address.StatusConflict)
	}
	if a.ObservedState != address.ObservedInUse {
		t.Errorf("observed_state = %q, want %q (the observation is still a fact)",
			a.ObservedState, address.ObservedInUse)
	}
	// The operator's own record of why must not be replaced by the observation.
	if a.Owner != "ops" {
		t.Errorf("owner = %q, want %q", a.Owner, "ops")
	}
}

// TestIPAMLinkage_ReportOutsideEverySubnetIsNotAFailure pins the direction of
// the dependency. A DHCP scope with no matching IPAM subnet is a
// configuration gap: the lease stands, the client keeps its address, and the
// report is dropped with a log rather than failing the data plane.
func TestIPAMLinkage_ReportOutsideEverySubnetIsNotAFailure(t *testing.T) {
	_, db, link, _ := newIPAMLinkedServer(t)

	// A scope whose address is outside every IPAM subnet.
	mustExec(t, db, `INSERT INTO dhcp_scopes (id, name, interface, subnet, start_ip, end_ip,
		subnet_mask, router, dns_servers, domain_name, lease_time, enabled, ping_check_enabled, dns_updates)
		VALUES ('scope-outside', 'other', 'eth0', '198.51.100.0/24', '198.51.100.10', '198.51.100.10',
		        '255.255.255.0', '198.51.100.1', '198.51.100.1', 'example.test', 3600, 1, 0, 0)`)

	if err := link.ObserveLease(ipam.LeaseActionBind, "lease-x", "scope-outside",
		"198.51.100.10", "aa:bb:cc:dd:ee:18", "outside"); err != nil {
		t.Fatalf("ObserveLease must not fail when no subnet covers the scope: %v", err)
	}

	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM ipam_addresses WHERE ip_address = '198.51.100.10'`).Scan(&n); err != nil {
		t.Fatalf("count addresses: %v", err)
	}
	if n != 0 {
		t.Errorf("addresses created outside every subnet = %d, want 0 (the DHCP side must not invent allocations)", n)
	}
}

// TestIPAMLinkage_UnknownActionIsRejected keeps the interface honest: the
// observer is told the vocabulary, and an action it does not understand is an
// error rather than a silent no-op that leaves the address unaccounted for.
func TestIPAMLinkage_UnknownActionIsRejected(t *testing.T) {
	_, _, link, _ := newIPAMLinkedServer(t)

	if err := link.ObserveLease("teleport", "lease-y", "scope-1", ipamLinkageIP, "aa:bb:cc:dd:ee:19", "h"); err == nil {
		t.Fatal("ObserveLease accepted an unknown action")
	}
}
