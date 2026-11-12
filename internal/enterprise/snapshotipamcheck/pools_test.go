package snapshotipamcheck

import (
	"context"
	"database/sql"
	"net/netip"
	"testing"
)

func address(t *testing.T, s string) []byte {
	t.Helper()
	a, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 16)
	if a.Is4() {
		v := a.As4()
		copy(b[12:], v[:])
	} else {
		v := a.As16()
		copy(b, v[:])
	}
	return b
}
func insertPool(t *testing.T, db *sql.DB, tenant, space, id, subnet, kind, first, last string, family int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO main.ipam_pools(tenant_id,space_id,id,subnet_id,family,pool_kind,first_ip,last_ip,label,revision) VALUES(?,?,?,?,?,?,?,?,?,1)`, tenant, space, id, subnet, family, kind, address(t, first), address(t, last), id); err != nil {
		t.Fatal(err)
	}
}
func checkPools(t *testing.T, db *sql.DB, counts Counts) (Evidence, error) {
	t.Helper()
	c := conn(t, db, true)
	return Check(context.Background(), c, Limits{Rows: 100, Bytes: 1 << 20, Expected: counts})
}

// Corrupt only the test data and restore the reviewed trigger before Check.
func withoutPoolGuard(t *testing.T, db *sql.DB, fn func()) {
	t.Helper()
	var schema string
	if err := db.QueryRow(`SELECT sql FROM main.sqlite_schema WHERE type='trigger' AND name='ipam_pools_insert_guard'`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TRIGGER main.ipam_pools_insert_guard`); err != nil {
		t.Fatal(err)
	}
	fn()
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
}
func TestPoolValidBoundariesScopesAndKinds(t *testing.T) {
	db := fixture(t)
	insertPool(t, db, "t1", "s", "a", "net", "IPAM", "192.0.2.20", "192.0.2.29", 4)
	insertPool(t, db, "t1", "s", "b", "net", "DHCP", "192.0.2.30", "192.0.2.39", 4)
	insertPool(t, db, "t1", "s", "c", "net", "DHCP", "192.0.2.40", "192.0.2.254", 4)
	insertPool(t, db, "t2", "s", "v6", "net", "DHCP", "2001:db8::2", "2001:db8::ffff:ffff:ffff:ffff", 6)
	if _, err := db.Exec(`INSERT INTO main.ipam_prefixes(tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES('t2','s','v4',4,24,?,?, 'SUBNET','STANDARD')`, address(t, "192.0.2.0"), address(t, "192.0.2.255")); err != nil {
		t.Fatal(err)
	}
	insertPool(t, db, "t2", "s", "different-scope", "v4", "DHCP", "192.0.2.20", "192.0.2.29", 4)
	// IPv4 and IPv6 share the same 16-byte representation here but are
	// separate families and therefore do not conflict.
	if _, err := db.Exec(`INSERT INTO main.ipam_prefixes(tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES('t1','s','v6same',6,120,?,?,'SUBNET','STANDARD')`, address(t, "::c000:200"), address(t, "::c000:2ff")); err != nil {
		t.Fatal(err)
	}
	insertPool(t, db, "t1", "s", "different-family", "v6same", "DHCP", "::c000:214", "::c000:21d", 6)
	counts := baseCounts
	counts.Prefixes += 2
	counts.Pools = 6
	counts.Ledger += 8
	got, err := checkPools(t, db, counts)
	if err != nil || !got.PoolRelations || !got.PoolOverlaps || !got.PoolOccupancyRelations || !got.LedgerRows || got.Counts != counts {
		t.Fatalf("valid pools: %+v %v", got, err)
	}
}
func TestPoolInvalidRelationsDespiteValidSchema(t *testing.T) {
	cases := []struct {
		name, first, last, kind, tenant, subnet string
		family                                  int
		second                                  bool
	}{
		{"v4 network endpoint", "192.0.2.0", "192.0.2.2", "DHCP", "t1", "net", 4, false},
		{"v4 broadcast endpoint", "192.0.2.250", "192.0.2.255", "DHCP", "t1", "net", 4, false},
		{"outside subnet", "192.0.3.1", "192.0.3.2", "IPAM", "t1", "net", 4, false},
		{"family mismatch", "2001:db8::2", "2001:db8::3", "IPAM", "t1", "net", 6, false},
		{"container not subnet", "192.0.1.1", "192.0.1.5", "IPAM", "t1", "parent", 4, false},
		{"v6 network endpoint", "2001:db8::", "2001:db8::2", "DHCP", "t2", "net", 6, false},
		{"touch overlaps", "192.0.2.29", "192.0.2.35", "DHCP", "t1", "net", 4, true},
		{"contained overlap", "192.0.2.21", "192.0.2.25", "DHCP", "t1", "net", 4, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := fixture(t)
			if tc.second {
				insertPool(t, db, "t1", "s", "first", "net", "IPAM", "192.0.2.20", "192.0.2.29", 4)
			}
			withoutPoolGuard(t, db, func() { insertPool(t, db, tc.tenant, "s", "bad", tc.subnet, tc.kind, tc.first, tc.last, tc.family) })
			counts := baseCounts
			counts.Pools = 1
			counts.Ledger++
			if tc.second {
				counts.Pools++
				counts.Ledger++
			}
			got, err := checkPools(t, db, counts)
			if err == nil || got != (Evidence{}) {
				t.Fatalf("accepted %s: %+v %v", tc.name, got, err)
			}
		})
	}
}
func TestPoolsCannotUsePointToPointOrHostSubnets(t *testing.T) {
	for _, tc := range []struct {
		name, ip, last, usage string
		family, bits          int
	}{
		{"v4-p2p", "198.51.100.0", "198.51.100.1", "P2P", 4, 31},
		{"v4-host", "198.51.100.1", "198.51.100.1", "HOST", 4, 32},
		{"v6-p2p", "2001:db9::", "2001:db9::1", "P2P", 6, 127},
		{"v6-host", "2001:db9::1", "2001:db9::1", "HOST", 6, 128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := fixture(t)
			if _, err := db.Exec(`INSERT INTO main.ipam_prefixes(tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES('t1','s','special',?,?,?,?, 'SUBNET',?)`, tc.family, tc.bits, address(t, tc.ip), address(t, tc.last), tc.usage); err != nil {
				t.Fatal(err)
			}
			withoutPoolGuard(t, db, func() { insertPool(t, db, "t1", "s", "invalid", "special", "IPAM", tc.ip, tc.last, tc.family) })
			counts := baseCounts
			counts.Prefixes++
			counts.Pools++
			counts.Ledger += 2
			got, err := checkPools(t, db, counts)
			if err == nil || got != (Evidence{}) {
				t.Fatalf("special subnet accepted: %+v %v", got, err)
			}
		})
	}
}

func TestDHCPPoolRejectsAllOccupancyKinds(t *testing.T) {
	for _, kind := range []string{"HOLD", "STATIC", "RESERVATION", "EXCLUDED"} {
		t.Run(kind, func(t *testing.T) {
			db := fixture(t)
			var expiry any
			if kind == "HOLD" {
				expiry = int64(2)
			}
			if _, err := db.Exec(`INSERT INTO main.ipam_occupancies(tenant_id,space_id,id,subnet_id,family,address,kind,owner_ref,expires_at_ms,created_at_ms) VALUES('t1','s','new','net',4,?,?, 'test',?,1)`, address(t, "192.0.2.11"), kind, expiry); err != nil {
				t.Fatal(err)
			}
			withoutPoolGuard(t, db, func() { insertPool(t, db, "t1", "s", "bad", "net", "DHCP", "192.0.2.11", "192.0.2.11", 4) })
			counts := baseCounts
			counts.Occupancies++
			counts.Pools++
			counts.Ledger += 2
			got, err := checkPools(t, db, counts)
			if err == nil || got != (Evidence{}) {
				t.Fatalf("%s accepted: %+v %v", kind, got, err)
			}
		})
	}
}
func TestIPAMPoolMayContainOccupancy(t *testing.T) {
	db := fixture(t)
	insertPool(t, db, "t1", "s", "ipam", "net", "IPAM", "192.0.2.10", "192.0.2.10", 4)
	counts := baseCounts
	counts.Pools++
	counts.Ledger++
	if got, err := checkPools(t, db, counts); err != nil || !got.PoolOccupancyRelations {
		t.Fatalf("IPAM occupancy: %+v %v", got, err)
	}
}
func TestPoolAndLedgerCountsAndBudgets(t *testing.T) {
	db := fixture(t)
	insertPool(t, db, "t1", "s", "pool", "net", "IPAM", "192.0.2.20", "192.0.2.21", 4)
	c := conn(t, db, true)
	ctx := context.Background()
	counts := baseCounts
	counts.Pools++
	counts.Ledger++
	for _, tc := range []struct {
		name string
		l    Limits
	}{
		{"omitted pool", Limits{Rows: 100, Bytes: 1 << 20, Expected: baseCounts}},
		{"omitted ledger", Limits{Rows: 100, Bytes: 1 << 20, Expected: Counts{Tenants: counts.Tenants, Spaces: counts.Spaces, Prefixes: counts.Prefixes, Pools: counts.Pools, Occupancies: counts.Occupancies, Ledger: counts.Ledger - 1}}},
		{"aggregate rows", Limits{Rows: 19, Bytes: 1 << 20, Expected: counts}},
		{"aggregate bytes", Limits{Rows: 100, Bytes: 8, Expected: counts}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Check(ctx, c, tc.l)
			if err == nil || got != (Evidence{}) {
				t.Fatalf("accepted %s: %+v %v", tc.name, got, err)
			}
		})
	}
}
