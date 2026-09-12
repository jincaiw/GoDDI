package server

import (
	"database/sql"
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
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

	link := ipam.NewLinkage(db)
	s.SetLeaseObserver(link)
	return s, db, link, sp.ID
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
