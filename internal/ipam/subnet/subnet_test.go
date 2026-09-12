package subnet

import (
	"database/sql"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jasonwa/goddi"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
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

func seedSpace(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT OR IGNORE INTO ipam_spaces (id, name) VALUES (?, ?)`, id, "space-"+id); err != nil {
		t.Fatalf("seed space: %v", err)
	}
}

func seedScope(t *testing.T, db *sql.DB, id, name, cidr, startIP, endIP string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip)
		VALUES (?, ?, ?, ?, ?)`, id, name, cidr, startIP, endIP); err != nil {
		t.Fatalf("seed scope: %v", err)
	}
}

func seedZone(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	// dns_zones requires SOA fields and a serial, so a partial insert would
	// fail on a NOT NULL rather than on anything to do with the test.
	if _, err := db.Exec(`
		INSERT INTO dns_zones (id, name, type, enabled, soa_mname, soa_rname, serial)
		VALUES (?, ?, 'master', 1, 'ns1.example.test.', 'hostmaster.example.test.', 1)`,
		id, name); err != nil {
		t.Fatalf("seed zone: %v", err)
	}
}

// --- CIDR canonicalisation -------------------------------------------------

func TestCreateSubnet_CanonicalisesCIDR(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	m := NewManager(db)

	// A CIDR whose host bits are set must be stored as the network it denotes.
	// Storing the caller's string made an equality comparison against a
	// generated scope's subnet fail, which is how a DHCP scope ended up not
	// depending on the subnet it was created from.
	s, err := m.CreateSubnet("sp1", "office", "10.0.0.5/24", SubnetOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if s.CIDR != "10.0.0.0/24" {
		t.Fatalf("stored CIDR = %q, want 10.0.0.0/24", s.CIDR)
	}
	if _, err := m.CreateSubnet("sp1", "bad", "not-a-cidr", SubnetOptions{}); err == nil {
		t.Fatal("an invalid CIDR was accepted")
	}
	if _, err := m.CreateSubnet("missing", "orphan", "10.9.0.0/24", SubnetOptions{}); err == nil {
		t.Fatal("a subnet was created in a space that does not exist")
	}
}

// TestCreateSubnet_OverlapIsScopedToItsSpace pins where the overlap rule
// applies. Overlap inside one space is a real error: two subnets claiming the
// same address make every allocation ambiguous. Across spaces it is the normal
// case -- two sites both use 10.0.0.0/24, and that is the whole reason a space
// exists. A global overlap check made the second site impossible to create.
func TestCreateSubnet_OverlapIsScopedToItsSpace(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	seedSpace(t, db, "sp2")
	m := NewManager(db)

	if _, err := m.CreateSubnet("sp1", "site-a", "10.0.0.0/24", SubnetOptions{}); err != nil {
		t.Fatalf("create the first subnet: %v", err)
	}

	// Same space, overlapping range: refused.
	if _, err := m.CreateSubnet("sp1", "site-a-tunnel", "10.0.0.0/30", SubnetOptions{}); err == nil {
		t.Fatal("an overlapping subnet was accepted inside one space")
	}

	// Another space, identical range: accepted.
	if _, err := m.CreateSubnet("sp2", "site-b", "10.0.0.0/24", SubnetOptions{}); err != nil {
		t.Fatalf("the same CIDR in another space was refused: %v", err)
	}

	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM ipam_subnets WHERE cidr = '10.0.0.0/24'`).Scan(&n); err != nil {
		t.Fatalf("count subnets: %v", err)
	}
	if n != 2 {
		t.Errorf("subnets holding 10.0.0.0/24 = %d, want 2 (one per space)", n)
	}
}

// TestCreateSubnet_IPv4AndIPv6DoNotOverlap pins the address-family guard in
// the overlap comparison. The ranges are compared as byte strings, and a byte
// comparison between a 4-byte and a 16-byte address compares only the first
// four bytes -- which can report two unrelated subnets as overlapping and make
// one of them impossible to create. Both orders are checked because the
// comparison is not symmetric in the operands.
func TestCreateSubnet_IPv4AndIPv6DoNotOverlap(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	seedSpace(t, db, "sp2")
	m := NewManager(db)

	if _, err := m.CreateSubnet("sp1", "v6-first", "2001:db8::/64", SubnetOptions{}); err != nil {
		t.Fatalf("create the IPv6 subnet: %v", err)
	}
	if _, err := m.CreateSubnet("sp1", "v4-second", "192.0.2.0/24", SubnetOptions{}); err != nil {
		t.Fatalf("an IPv4 subnet was judged to overlap an IPv6 subnet: %v", err)
	}

	if _, err := m.CreateSubnet("sp2", "v4-first", "192.0.2.0/24", SubnetOptions{}); err != nil {
		t.Fatalf("create the IPv4 subnet: %v", err)
	}
	if _, err := m.CreateSubnet("sp2", "v6-second", "2001:db8::/64", SubnetOptions{}); err != nil {
		t.Fatalf("an IPv6 subnet was judged to overlap an IPv4 subnet: %v", err)
	}
}

func TestMaterializationPolicy(t *testing.T) {
	cases := []struct {
		cidr string
		want bool
	}{
		{"10.0.0.0/24", true},    // 256
		{"10.0.0.0/16", true},    // 65536, exactly at the threshold
		{"10.0.0.0/15", false},   // 131072, above it
		{"10.0.0.0/8", false},    // 16M
		{"10.0.0.0/30", true},    // 4
		{"10.0.0.0/32", true},    // 1
		{"2001:db8::/120", true}, // 256 IPv6 addresses; a prefix-length rule
		//                            that assumed 4 chars per reverse label, or
		//                            that measured the IPv6 prefix against an
		//                            IPv4 threshold, would get this wrong
		{"2001:db8::/64", false}, // 2^64
		{"2001:db8::/128", true}, // 1
		{"::/0", false},          // 2^128: the shift must not wrap
	}
	for _, c := range cases {
		_, ipNet, err := net.ParseCIDR(c.cidr)
		if err != nil {
			t.Fatalf("parse %s: %v", c.cidr, err)
		}
		if got := materialize(ipNet); got != c.want {
			t.Errorf("materialize(%s) = %v, want %v", c.cidr, got, c.want)
		}
	}
}

// --- dependencies ----------------------------------------------------------

func TestCheckDependencies_FindsOverlappingScope(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	m := NewManager(db)
	s, err := m.CreateSubnet("sp1", "office", "10.0.0.0/24", SubnetOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	deps, err := m.CheckDependencies(s.ID)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(deps) != 0 {
		t.Fatalf("a fresh subnet reported dependencies: %+v", deps)
	}

	// The scope's declared subnet is a /25 inside the /24. The old check
	// compared CIDR strings for equality, so this scope was invisible.
	seedScope(t, db, "sc1", "half", "10.0.0.0/25", "10.0.0.10", "10.0.0.100")

	deps, err = m.CheckDependencies(s.ID)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(deps) == 0 {
		t.Fatal("an overlapping DHCP scope was not reported as a dependency")
	}
	found := false
	for _, d := range deps {
		if d.Kind == "dhcp_scope" && d.ID == "sc1" {
			found = true
			if !strings.Contains(d.Detail, "overlap") {
				t.Errorf("dependency detail does not explain itself: %q", d.Detail)
			}
		}
	}
	if !found {
		t.Fatalf("dependencies = %+v, want the overlapping scope", deps)
	}

	if err := m.DeleteSubnet(s.ID); !errors.Is(err, ErrSubnetHasDependencies) {
		t.Fatalf("delete err = %v, want ErrSubnetHasDependencies", err)
	}
	var depErr *DependencyError
	if err := m.DeleteSubnet(s.ID); !errors.As(err, &depErr) {
		t.Fatalf("delete err = %v, want *DependencyError", err)
	}
	if !strings.Contains(depErr.Error(), "sc1") {
		t.Errorf("refusal does not name the blocking scope: %v", depErr)
	}
	if depErr.CIDR != "10.0.0.0/24" {
		t.Errorf("refusal CIDR = %q", depErr.CIDR)
	}
}

func TestCheckDependencies_FindsAllocatedAddresses(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	m := NewManager(db)
	s, err := m.CreateSubnet("sp1", "office", "192.0.2.0/28", SubnetOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	cases := []struct {
		status string
		want   bool
	}{
		{"available", false},
		{"used", true},
		{"reserved", true},
		{"dhcp", true},
		{"conflict", true},
		{"gateway", true},
		{"excluded", true},
		{"unknown", true},
	}
	for i, c := range cases {
		ip := "192.0.2." + itoa(i+1)
		if _, err := db.Exec(`
			UPDATE ipam_addresses SET status = ? WHERE subnet_id = ? AND ip_address = ?`,
			string(c.status), s.ID, ip); err != nil {
			t.Fatalf("set status: %v", err)
		}
		deps, err := m.CheckDependencies(s.ID)
		if err != nil {
			t.Fatalf("check: %v", err)
		}
		hasDetail := false
		for _, d := range deps {
			if d.Kind == "address_detail" && d.Name == ip {
				hasDetail = true
			}
		}
		if hasDetail != c.want {
			t.Errorf("status %s: listed as a blocker = %v, want %v (deps %+v)",
				c.status, hasDetail, c.want, deps)
		}
		// Reset so the next case starts from the same state.
		if _, err := db.Exec(`
			UPDATE ipam_addresses SET status = 'available' WHERE subnet_id = ? AND ip_address = ?`,
			s.ID, ip); err != nil {
			t.Fatalf("reset status: %v", err)
		}
	}
}

func TestCheckDependencies_FindsEnclosingReverseZone(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	m := NewManager(db)
	s, err := m.CreateSubnet("sp1", "office", "192.0.2.0/24", SubnetOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// A /16 reverse zone also serves the /24 subnet. Only checking the exact
	// prefix length missed it.
	seedZone(t, db, "z1", "2.0.192.in-addr.arpa")
	seedZone(t, db, "z2", "0.192.in-addr.arpa")

	deps, err := m.CheckDependencies(s.ID)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	zones := map[string]bool{}
	for _, d := range deps {
		if d.Kind == "dns_zone" {
			zones[d.Name] = true
		}
	}
	if !zones["2.0.192.in-addr.arpa"] || !zones["0.192.in-addr.arpa"] {
		t.Fatalf("zones found = %v, want both the exact and the enclosing zone", zones)
	}
}

func TestDeleteSubnet_SucceedsWhenClean(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	m := NewManager(db)
	s, err := m.CreateSubnet("sp1", "office", "192.0.2.0/29", SubnetOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	var before int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ipam_addresses WHERE subnet_id = ?`, s.ID).Scan(&before); err != nil {
		t.Fatalf("count: %v", err)
	}
	if before == 0 {
		t.Fatal("the subnet was created without materialising its addresses")
	}

	if err := m.DeleteSubnet(s.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := m.GetSubnet(s.ID); err == nil {
		t.Fatal("the subnet still exists after deletion")
	}
	var after int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ipam_addresses WHERE subnet_id = ?`, s.ID).Scan(&after); err != nil {
		t.Fatalf("count: %v", err)
	}
	if after != 0 {
		t.Fatalf("%d address rows survived the subnet", after)
	}
}

func TestDeleteSubnet_RefusesWhileAnAddressIsAllocated(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	m := NewManager(db)
	s, err := m.CreateSubnet("sp1", "office", "192.0.2.0/29", SubnetOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Exec(`
		UPDATE ipam_addresses SET status = 'dhcp' WHERE subnet_id = ? AND ip_address = '192.0.2.1'`,
		s.ID); err != nil {
		t.Fatalf("allocate: %v", err)
	}

	err = m.DeleteSubnet(s.ID)
	if !errors.Is(err, ErrSubnetHasDependencies) {
		t.Fatalf("delete err = %v, want ErrSubnetHasDependencies", err)
	}
	if !strings.Contains(err.Error(), "192.0.2.1") {
		t.Errorf("refusal does not name the address at stake: %v", err)
	}
	if _, err := m.GetSubnet(s.ID); err != nil {
		t.Fatal("the subnet was deleted despite the refusal")
	}
}

// --- resize ----------------------------------------------------------------

func TestUpdateSubnet_CIDRChange(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	m := NewManager(db)
	s, err := m.CreateSubnet("sp1", "office", "192.0.2.0/29", SubnetOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Widening a subnet whose addresses are all free rebuilds the rows.
	updated, err := m.UpdateSubnet(s.ID, SubnetOptions{CIDR: "192.0.2.0/28"})
	if err != nil {
		t.Fatalf("widen: %v", err)
	}
	if updated.CIDR != "192.0.2.0/28" {
		t.Fatalf("CIDR = %s", updated.CIDR)
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ipam_addresses WHERE subnet_id = ?`, s.ID).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	// /28 has 16 addresses; .0 network and .15 broadcast are excluded.
	if rows != 14 {
		t.Fatalf("address rows after widening = %d, want 14", rows)
	}

	// Once an address is allocated, changing the prefix would delete the row
	// that records who holds it.
	if _, err := db.Exec(`
		UPDATE ipam_addresses SET status = 'static' WHERE subnet_id = ? AND ip_address = '192.0.2.1'`,
		s.ID); err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if _, err := m.UpdateSubnet(s.ID, SubnetOptions{CIDR: "192.0.2.0/27"}); err == nil {
		t.Fatal("the CIDR was changed while an address was allocated")
	}
	got, err := m.GetSubnet(s.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CIDR != "192.0.2.0/28" {
		t.Fatalf("CIDR = %s, want it unchanged", got.CIDR)
	}
}

// --- usage -----------------------------------------------------------------

func TestGetUsageStats_SparseSubnetDerivesAvailability(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	m := NewManager(db)

	small, err := m.CreateSubnet("sp1", "small", "192.0.2.0/28", SubnetOptions{})
	if err != nil {
		t.Fatalf("create small: %v", err)
	}
	stats, err := m.GetUsageStats(small.ID)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if !stats.Materialized {
		t.Error("/28 should be materialised")
	}
	if stats.AvailableDerived {
		t.Error("a materialised subnet counts its free addresses; it does not derive them")
	}
	if stats.Total != 14 || stats.Available != 14 {
		t.Fatalf("stats = %+v, want 14/14", stats)
	}

	large, err := m.CreateSubnet("sp1", "large", "10.0.0.0/8", SubnetOptions{})
	if err != nil {
		t.Fatalf("create large: %v", err)
	}
	stats, err = m.GetUsageStats(large.ID)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Materialized {
		t.Error("/8 must not be materialised")
	}
	if !stats.AvailableDerived {
		t.Error("a sparse subnet must say that its free count is derived")
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ipam_addresses WHERE subnet_id = ?`, large.ID).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 0 {
		t.Fatalf("a /8 materialised %d rows", rows)
	}
	if stats.Available != stats.Total {
		t.Fatalf("available = %d of total %d; an untouched /8 is entirely free",
			stats.Available, stats.Total)
	}
}

// --- scope / zone generation ----------------------------------------------

func TestGenerateDHCPScope(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	m := NewManager(db)

	cases := []struct {
		cidr  string
		start string
		end   string
	}{
		{"192.0.2.0/24", "192.0.2.1", "192.0.2.254"},
		{"198.51.100.0/30", "198.51.100.1", "198.51.100.2"},
		{"203.0.113.0/31", "203.0.113.1", "203.0.113.1"}, // both usable; start is the second address
		{"203.0.113.7/32", "203.0.113.7", "203.0.113.7"},
	}
	for i, c := range cases {
		s, err := m.CreateSubnet("sp1", "s"+itoa(i), c.cidr, SubnetOptions{})
		if err != nil {
			t.Fatalf("create %s: %v", c.cidr, err)
		}
		opts, err := m.GenerateDHCPScope(s.ID)
		if err != nil {
			t.Fatalf("generate for %s: %v", c.cidr, err)
		}
		if opts.StartIP != c.start || opts.EndIP != c.end {
			t.Errorf("%s range = %s..%s, want %s..%s", c.cidr, opts.StartIP, opts.EndIP, c.start, c.end)
		}
		if opts.Subnet != c.cidr {
			t.Errorf("%s scope subnet = %s", c.cidr, opts.Subnet)
		}
	}

	// IPv6 must be refused, not silently mangled: the old code incremented
	// byte 3 of the address, which on a 16-byte IPv6 network produces a range
	// unrelated to the subnet while reporting success.
	v6, err := m.CreateSubnet("sp1", "v6", "2001:db8::/64", SubnetOptions{})
	if err != nil {
		t.Fatalf("create v6: %v", err)
	}
	if _, err := m.GenerateDHCPScope(v6.ID); err == nil {
		t.Fatal("an IPv6 subnet produced an IPv4 DHCP scope")
	} else if !strings.Contains(err.Error(), "DHCPv6") {
		t.Errorf("refusal does not explain itself: %v", err)
	}
}

func TestGenerateReverseZone(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	m := NewManager(db)

	// Reverse delegation happens on octet boundaries, so the zone that owns a
	// subnet is the longest octet-aligned prefix that fits inside it. A /12 is
	// not octet aligned and spans 172.16-172.31, so no single /16 zone can own
	// it and the /8 zone is the answer.
	cases := map[string]string{
		"192.0.2.0/24":  "2.0.192.in-addr.arpa",
		"198.51.0.0/16": "51.198.in-addr.arpa",
		"10.0.0.0/8":    "10.in-addr.arpa",
		"172.16.0.0/12": "172.in-addr.arpa",
	}
	i := 0
	for cidr, want := range cases {
		s, err := m.CreateSubnet("sp1", "r"+itoa(i), cidr, SubnetOptions{})
		if err != nil {
			t.Fatalf("create %s: %v", cidr, err)
		}
		got, err := m.GenerateReverseZone(s.ID)
		if err != nil {
			t.Fatalf("reverse zone for %s: %v", cidr, err)
		}
		if got != want {
			t.Errorf("GenerateReverseZone(%s) = %q, want %q", cidr, got, want)
		}
		i++
	}
}

func TestReverseZoneCandidates_IPv6(t *testing.T) {
	got := ReverseZoneCandidates("2001:db8::/32")
	if len(got) < 2 {
		t.Fatalf("candidates = %v", got)
	}
	if got[0] != "8.b.d.0.1.0.0.2.ip6.arpa" {
		t.Fatalf("most specific candidate = %q", got[0])
	}
	if got[len(got)-1] != "ip6.arpa" {
		t.Fatalf("least specific candidate = %q, want ip6.arpa", got[len(got)-1])
	}
	if ReverseZoneCandidates("not-a-cidr") != nil {
		t.Error("a non-CIDR produced candidates")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
