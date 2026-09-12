package ipam

// The 360° address view had no tests, and that is how its DNS half went dead
// unnoticed: `ipam_dns_links` has no production writer, so the view's answer to
// "which names publish this address" was always the empty list, and every
// allocated address was reported as conflicting with "allocated address has no
// DNS name". Nothing exercised the view, so nothing contradicted it.
//
// The cases below are written against that specific failure first: the pair in
// TestOnlyTheAddressWithNoNameIsReportedAsNameless fails the moment the DNS
// half goes back to reading a table nobody writes.

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jasonwa/goddi"
	"github.com/jasonwa/goddi/internal/ipam/address"
	"github.com/jasonwa/goddi/pkg/dnsutil"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Production caps the pool at one connection; keeping that here means an
	// unclosed cursor fails the test instead of hanging only in production.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	goose.SetBaseFS(goddiassets.Migrations())
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}

func seedSpaceAndSubnet(t *testing.T, db *sql.DB, spaceID, subnetID, cidr string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT OR IGNORE INTO ipam_spaces (id, name, description) VALUES (?, ?, '')`,
		spaceID, "space-"+spaceID); err != nil {
		t.Fatalf("seed space: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO ipam_subnets (id, space_id, name, cidr) VALUES (?, ?, ?, ?)`,
		subnetID, spaceID, "subnet-"+subnetID, cidr); err != nil {
		t.Fatalf("seed subnet: %v", err)
	}
}

func seedAddress(t *testing.T, db *sql.DB, id, spaceID, subnetID, ip string, status address.Status) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO ipam_addresses (id, subnet_id, space_id, ip_address, status, observed_state)
		VALUES (?, ?, ?, ?, ?, 'unknown')`, id, subnetID, spaceID, ip, string(status)); err != nil {
		t.Fatalf("seed address %s: %v", ip, err)
	}
}

// seedZone inserts the minimum `dns_zones` row, which needs the SOA fields and
// a serial: a partial insert would fail on a NOT NULL rather than on anything
// the test is about.
func seedZone(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO dns_zones (id, name, type, enabled, soa_mname, soa_rname, serial)
		VALUES (?, ?, 'master', 1, 'ns1.example.test.', 'hostmaster.example.test.', 1)`,
		id, name); err != nil {
		t.Fatalf("seed zone %s: %v", name, err)
	}
}

func seedRecord(t *testing.T, db *sql.DB, id, zoneID, name, rtype, value string, enabled bool) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, enabled)
		VALUES (?, ?, ?, ?, ?, ?)`, id, zoneID, name, rtype, value, enabled); err != nil {
		t.Fatalf("seed record %s %s: %v", name, rtype, err)
	}
}

func seedScope(t *testing.T, db *sql.DB, id, name, subnet, startIP, endIP, router string, enabled bool) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip, router, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, id, name, subnet, startIP, endIP, router, enabled); err != nil {
		t.Fatalf("seed scope %s: %v", name, err)
	}
}

func hasConflict(conflicts []string, substr string) bool {
	for _, c := range conflicts {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

// TestTheViewListsTheNamesThatPublishAnAddress covers both directions. The two
// are separate queries because a name holds the address in two different
// places: an A record in its value, the PTR that reverses it in its name. A
// lookup written as a single `WHERE value = ?` would pass the forward half of
// this test and silently drop the reverse one.
func TestTheViewListsTheNamesThatPublishAnAddress(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedAddress(t, db, "ad1", "sp1", "sn1", "192.0.2.10", address.StatusStatic)
	seedZone(t, db, "z1", "example.test")
	seedZone(t, db, "z2", "2.0.192.in-addr.arpa")
	seedRecord(t, db, "r1", "z1", "host10.example.test.", "A", "192.0.2.10", true)
	seedRecord(t, db, "r2", "z1", "alias.example.test.", "A", "192.0.2.10", true)
	seedRecord(t, db, "r3", "z2", dnsutil.ReverseIP("192.0.2.10"), "PTR", "host10.example.test.", true)

	view, err := NewLinkage(db).ViewAddress("sp1", "192.0.2.10")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(view.DNSRecords) != 3 {
		t.Fatalf("found %d records, want 3: %+v", len(view.DNSRecords), view.DNSRecords)
	}

	byName := make(map[string]PublishingRecord, len(view.DNSRecords))
	for _, p := range view.DNSRecords {
		byName[p.Name] = p
	}

	if got := byName["host10.example.test."]; got.Direction != publishingDirectionForward {
		t.Errorf("host10 direction = %q, want %q", got.Direction, publishingDirectionForward)
	}
	if got := byName[dnsutil.ReverseIP("192.0.2.10")]; got.Direction != publishingDirectionReverse {
		t.Errorf("PTR direction = %q, want %q", got.Direction, publishingDirectionReverse)
	}
	if got := byName["host10.example.test."]; got.ZoneID != "z1" {
		t.Errorf("zone id = %q, want z1 (the view is useless without knowing which zone)", got.ZoneID)
	}
}

// TestPublishingRecordsForAnAddressThatDoesNotExistIsAnError is the "extract
// nothing" guard: a caller that gets an empty list back cannot tell "this
// address publishes no names" from "that address id is not mine".
func TestPublishingRecordsForAnAddressThatDoesNotExistIsAnError(t *testing.T) {
	db := newTestDB(t)
	if _, err := NewLinkage(db).PublishingRecordsForAddressID("nosuchaddress"); err == nil {
		t.Error("looking up an unknown address id returned no error")
	}
}

// TestOnlyTheAddressWithNoNameIsReportedAsNameless is the case that would have
// caught the dead half.
//
// It asserts both directions of the same rule, because only the pair is
// meaningful: an implementation that reports "no DNS name" unconditionally
// passes the second assertion, and one that never reports it passes the first.
func TestOnlyTheAddressWithNoNameIsReportedAsNameless(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedZone(t, db, "z1", "example.test")

	// 192.0.2.10 is allocated and has a name.
	seedAddress(t, db, "ad1", "sp1", "sn1", "192.0.2.10", address.StatusStatic)
	seedRecord(t, db, "r1", "z1", "host10.example.test.", "A", "192.0.2.10", true)

	// 192.0.2.11 is allocated and has none.
	seedAddress(t, db, "ad2", "sp1", "sn1", "192.0.2.11", address.StatusStatic)

	linkage := NewLinkage(db)

	named, err := linkage.ViewAddress("sp1", "192.0.2.10")
	if err != nil {
		t.Fatalf("view named: %v", err)
	}
	if hasConflict(named.Conflicts, "no DNS name") {
		t.Errorf("an address with an A record was reported as having no name: %v", named.Conflicts)
	}

	nameless, err := linkage.ViewAddress("sp1", "192.0.2.11")
	if err != nil {
		t.Fatalf("view nameless: %v", err)
	}
	if !hasConflict(nameless.Conflicts, "no DNS name") {
		t.Errorf("an allocated address with no record was not reported: %v", nameless.Conflicts)
	}
}

// TestADisabledRecordIsNotAName. A record that is switched off is not being
// answered, so it must not suppress the finding -- otherwise an operator who
// disables the last A record sees the address as named right up until the
// moment they need the name to resolve.
func TestADisabledRecordIsNotAName(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedZone(t, db, "z1", "example.test")
	seedAddress(t, db, "ad1", "sp1", "sn1", "192.0.2.12", address.StatusStatic)
	seedRecord(t, db, "r1", "z1", "host12.example.test.", "A", "192.0.2.12", false)

	view, err := NewLinkage(db).ViewAddress("sp1", "192.0.2.12")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(view.DNSRecords) != 1 {
		t.Fatalf("the disabled record should still be listed, so the operator can see it: %+v", view.DNSRecords)
	}
	if !hasConflict(view.Conflicts, "no DNS name") {
		t.Errorf("a disabled record was counted as a name: %v", view.Conflicts)
	}
}

// TestANonCanonicalValueIsReportedAndNotSilentlyUnfindable documents a limit of
// the derivation and the check that guards it.
//
// The view matches a record's value as a string, so an AAAA record written in
// its long form is published and invisible at the same time. Integrity reports
// it rather than the view pretending to have found it.
func TestANonCanonicalValueIsReportedAndNotSilentlyUnfindable(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "2001:db8::/32")
	seedAddress(t, db, "ad1", "sp1", "sn1", "2001:db8::1", address.StatusStatic)
	seedZone(t, db, "z1", "example.test")

	const longForm = "2001:0db8:0000:0000:0000:0000:0000:0001"
	seedRecord(t, db, "r1", "z1", "host1.example.test.", "AAAA", longForm, true)

	linkage := NewLinkage(db)

	view, err := linkage.ViewAddress("sp1", "2001:db8::1")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(view.DNSRecords) != 0 {
		t.Fatalf("the lookup is a string comparison; finding this would mean it is not: %+v", view.DNSRecords)
	}

	found, err := linkage.Addresses().NonCanonicalPublishingRecords(200)
	if err != nil {
		t.Fatalf("integrity: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("integrity found %d non-canonical publishers, want 1: %+v", len(found), found)
	}
	if found[0].Canonical != "2001:db8::1" {
		t.Errorf("canonical = %q, want 2001:db8::1", found[0].Canonical)
	}
	if found[0].Name != "host1.example.test." {
		t.Errorf("name = %q; the report is only actionable if it names the record", found[0].Name)
	}

	// A canonical record must not be reported, or the report is noise.
	seedRecord(t, db, "r2", "z1", "host2.example.test.", "AAAA", "2001:db8::2", true)
	found, err = linkage.Addresses().NonCanonicalPublishingRecords(200)
	if err != nil {
		t.Fatalf("integrity: %v", err)
	}
	if len(found) != 1 {
		t.Errorf("a canonical value was reported as non-canonical: %+v", found)
	}
}

// TestTheVestigialLinkTableIsNotWhatAnswers. The table is still there and still
// writable through the manager; writing to it must change nothing about the
// view, because the view does not read it. If this ever fails, someone has
// wired the reader back to the table.
func TestTheVestigialLinkTableIsNotWhatAnswers(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedAddress(t, db, "ad1", "sp1", "sn1", "192.0.2.13", address.StatusStatic)
	seedZone(t, db, "z1", "example.test")
	seedRecord(t, db, "r1", "z1", "host13.example.test.", "A", "192.0.2.13", true)

	linkage := NewLinkage(db)
	addrMgr := linkage.Addresses()

	// A record linked in the vestigial table alone, with no dns_records row
	// behind it: the derived answer must not pick it up.
	if err := addrMgr.LinkDNSRecord(address.DNSLink{
		AddressID:  "ad1",
		RecordID:   "ghost",
		RecordName: "ghost.example.test.",
		RecordType: "A",
		Value:      "192.0.2.13",
	}); err != nil {
		t.Fatalf("link: %v", err)
	}

	view, err := linkage.ViewAddress("sp1", "192.0.2.13")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(view.DNSRecords) != 1 || view.DNSRecords[0].Name != "host13.example.test." {
		t.Fatalf("the view answered from the link table instead of dns_records: %+v", view.DNSRecords)
	}
}

// --- DHCP scope membership -------------------------------------------------

// TestPoolMembershipIsNumericNotLexicographic is the case a string comparison
// gets backwards.
//
// `192.0.2.10` sorts *before* `192.0.2.9` as text ('1' < '9'), so a
// `BETWEEN start_ip AND end_ip` on the text columns would put an address that
// is squarely inside the pool outside it. The bounds are stored as text and
// SQLite has no address type, so this is the mistake the Go comparison exists
// to avoid.
func TestPoolMembershipIsNumericNotLexicographic(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedAddress(t, db, "ad1", "sp1", "sn1", "192.0.2.10", address.StatusDHCP)
	seedScope(t, db, "sc1", "office", "192.0.2.0/24", "192.0.2.9", "192.0.2.11", "192.0.2.1", true)

	view, err := NewLinkage(db).ViewAddress("sp1", "192.0.2.10")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(view.DHCPScopes) != 1 {
		t.Fatalf("scopes = %+v, want the one that contains this address", view.DHCPScopes)
	}
	if !view.DHCPScopes[0].InPool {
		t.Errorf("InPool = false for 192.0.2.10 in [192.0.2.9, 192.0.2.11]; " +
			"a lexicographic comparison produces exactly this")
	}
}

// TestAScopeIsListedWhenOnlyTheSubnetMatches. InPool and InSubnet answer
// different questions, and a scope that claims the address in one sense but not
// the other has to be visible: requiring both would hide a pool that reaches
// outside its own subnet, which is the configuration that needs looking at.
func TestAScopeIsListedWhenOnlyTheSubnetMatches(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedAddress(t, db, "ad1", "sp1", "sn1", "192.0.2.50", address.StatusDHCP)
	seedScope(t, db, "sc1", "office", "192.0.2.0/24", "192.0.2.100", "192.0.2.200", "192.0.2.1", true)
	seedScope(t, db, "sc2", "elsewhere", "198.51.100.0/24", "198.51.100.10", "198.51.100.20", "", true)

	view, err := NewLinkage(db).ViewAddress("sp1", "192.0.2.50")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(view.DHCPScopes) != 1 || view.DHCPScopes[0].ID != "sc1" {
		t.Fatalf("scopes = %+v, want only sc1", view.DHCPScopes)
	}
	if view.DHCPScopes[0].InSubnet != true || view.DHCPScopes[0].InPool != false {
		t.Errorf("in_subnet/in_pool = %v/%v, want true/false",
			view.DHCPScopes[0].InSubnet, view.DHCPScopes[0].InPool)
	}
}

// TestAnAddressFamilyIsNotComparedAgainstAnother. An IPv4 scope's bounds parse
// into the 4-in-6 form; an IPv6 address does not. Byte-comparing them yields an
// answer and the answer means nothing, so the comparison refuses instead.
func TestAnAddressFamilyIsNotComparedAgainstAnother(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "2001:db8::/64")
	seedAddress(t, db, "ad1", "sp1", "sn1", "2001:db8::5", address.StatusDHCP)
	// Bounds that an unguarded byte comparison would place on either side of
	// the address by accident.
	seedScope(t, db, "sc1", "v4", "192.0.2.0/24", "0.0.0.0", "255.255.255.255", "", true)

	view, err := NewLinkage(db).ViewAddress("sp1", "2001:db8::5")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(view.DHCPScopes) != 0 {
		t.Fatalf("an IPv4 scope claimed an IPv6 address: %+v", view.DHCPScopes)
	}
}

// TestUnparseableBoundsContainNothing. An empty or malformed bound is not
// "everything": treating it as unbounded would let a half-configured scope
// claim every address in the deployment.
func TestUnparseableBoundsContainNothing(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedAddress(t, db, "ad1", "sp1", "sn1", "192.0.2.30", address.StatusDHCP)
	seedScope(t, db, "sc1", "broken", "198.51.100.0/24", "", "", "", true)

	view, err := NewLinkage(db).ViewAddress("sp1", "192.0.2.30")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(view.DHCPScopes) != 0 {
		t.Fatalf("unparseable bounds were read as unbounded: %+v", view.DHCPScopes)
	}
}

// TestGatewayIsNamedRatherThanInferred. The scope's router is one column, not a
// guess from "the first usable address", so the view reports the comparison
// instead of reconstructing it.
func TestGatewayIsNamedRatherThanInferred(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedAddress(t, db, "ad1", "sp1", "sn1", "192.0.2.254", address.StatusGateway)
	// A pool that does not reach the gateway, and a router that is somewhere
	// else: the address is claimed only because it *is* the router.
	seedScope(t, db, "sc1", "office", "192.0.2.0/24", "192.0.2.10", "192.0.2.20", "192.0.2.254", true)

	view, err := NewLinkage(db).ViewAddress("sp1", "192.0.2.254")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(view.DHCPScopes) != 1 {
		t.Fatalf("scopes = %+v, want the scope whose router this is", view.DHCPScopes)
	}
	if !view.DHCPScopes[0].IsGateway || view.DHCPScopes[0].InPool {
		t.Errorf("is_gateway/in_pool = %v/%v, want true/false",
			view.DHCPScopes[0].IsGateway, view.DHCPScopes[0].InPool)
	}
}

// TestTheViewBoundIsReportedNotSilent. The scope list is capped, so an operator
// reading "no scope claims this address" off a truncated list would be reading
// a limit as a fact. The flag is the difference between the two.
func TestTheViewBoundIsReportedNotSilent(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedAddress(t, db, "ad1", "sp1", "sn1", "192.0.2.30", address.StatusDHCP)

	// One more than the view reads, and none of them claims the address.
	for i := 0; i < maxScopesInView+1; i++ {
		seedScope(t, db, fmt.Sprintf("sc%03d", i), fmt.Sprintf("scope-%03d", i),
			"198.51.100.0/24", "198.51.100.10", "198.51.100.20", "", true)
	}

	view, err := NewLinkage(db).ViewAddress("sp1", "192.0.2.30")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(view.DHCPScopes) != 0 {
		t.Fatalf("scopes = %+v, want none: they are all in another subnet", view.DHCPScopes)
	}
	if !view.ScopesTruncated {
		t.Error("the list was truncated and the view did not say so; " +
			"a caller cannot tell a bound from an absence")
	}
}

// TestAPoolThatWouldHandOutAFencedAddressIsAConflict.
//
// The allocator reads dhcp_leases and dhcp_reservations and nothing else -- a
// scope has no exclusion list and the query never looks at ipam_addresses. So
// an address IPAM has fenced off, sitting inside the range, is one the DHCP
// side will hand to a client. The pairing matters: a disabled scope is allowed
// to disagree, because it hands out nothing.
func TestAPoolThatWouldHandOutAFencedAddressIsAConflict(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedAddress(t, db, "ad1", "sp1", "sn1", "192.0.2.15", address.StatusGateway)
	seedScope(t, db, "sc1", "live", "192.0.2.0/24", "192.0.2.10", "192.0.2.20", "", true)
	seedScope(t, db, "sc2", "off", "192.0.2.0/24", "192.0.2.10", "192.0.2.20", "", false)

	view, err := NewLinkage(db).ViewAddress("sp1", "192.0.2.15")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if !hasConflict(view.Conflicts, `scope "live"`) {
		t.Errorf("no conflict for a gateway inside a live pool: %v", view.Conflicts)
	}
	if hasConflict(view.Conflicts, `scope "off"`) {
		t.Errorf("a disabled scope was reported as handing the address out: %v", view.Conflicts)
	}
}
