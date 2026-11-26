package dhcpholds

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/ipamstore"
)

var ctx = context.Background()

func fixture(t *testing.T) (*sql.DB, *sql.Conn, *sql.Conn) {
	t.Helper()
	db, e := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "test.db")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(1000)")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	for _, path := range ipamstore.MigrationPaths() {
		ddl, e := ipamstore.ContentsOf(path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(ddl); e != nil {
			t.Fatal(e)
		}
	}
	for _, q := range []string{
		`INSERT INTO ipam_tenants(id,slug,display_name) VALUES ('t','t','tenant')`,
		`INSERT INTO ipam_spaces(tenant_id,id,slug,display_name) VALUES ('t','s','s','space'),('t','s2','s2','space2')`,
		`INSERT INTO ipam_prefixes(tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES ('t','s','n',4,24,x'000000000000000000000000C0000200',x'000000000000000000000000C00002FF','SUBNET','STANDARD'),('t','s2','n',4,24,x'000000000000000000000000C0000200',x'000000000000000000000000C00002FF','SUBNET','STANDARD')`,
	} {
		if _, e = db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	a, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { a.Close() })
	b, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { b.Close() })
	return db, a, b
}
func begin(t *testing.T, c *sql.Conn) *sql.Tx {
	t.Helper()
	tx, e := c.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	return tx
}
func install(t *testing.T, c *sql.Conn) {
	t.Helper()
	tx := begin(t, c)
	if e := Install(ctx, tx, []Mapping{{"a", "t", "s", "n"}, {"b", "t", "s", "n"}, {"c", "t", "s2", "n"}}); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(); e != nil {
		t.Fatal(e)
	}
}
func ip(last byte) []byte { b := make([]byte, 16); b[12] = 192; b[14] = 2; b[15] = last; return b }
func static(tx *sql.Tx, space, id string, last byte) error {
	_, e := tx.Exec(`INSERT INTO ipam_occupancies(tenant_id,space_id,id,subnet_id,family,address,kind,owner_ref,created_at_ms) VALUES('t',?,?,'n',4,?,'STATIC','owner',1)`, space, id, ip(last))
	return e
}
func TestInstallUsesV2WithoutDuplicateRowIDGuards(t *testing.T) {
	_, a, _ := fixture(t)
	install(t, a)
	var v2, duplicates int
	if err := a.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type='trigger' AND name LIKE 'ipam_%_rowid_%_guard'`).Scan(&v2); err != nil {
		t.Fatal(err)
	}
	if err := a.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type='trigger' AND name LIKE 'dhcp_rowid_%'`).Scan(&duplicates); err != nil {
		t.Fatal(err)
	}
	if v2 != 18 || duplicates != 0 {
		t.Fatalf("v2=%d duplicate=%d", v2, duplicates)
	}
}
func TestConcurrentStaticVersusHold(t *testing.T) {
	db, a, b := fixture(t)
	install(t, a)
	tx1, tx2 := begin(t, a), begin(t, b)
	start := make(chan struct{})
	results := make(chan error, 2)
	run := func(tx *sql.Tx, write func(*sql.Tx) error) {
		<-start
		err := write(tx)
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
		results <- err
	}
	go run(tx1, func(tx *sql.Tx) error { return static(tx, "s", "racing-static", 20) })
	go run(tx2, func(tx *sql.Tx) error { return Create(ctx, tx, "a", "racing-lease", 4, ip(20), "OFFER") })
	close(start)
	e1, e2 := <-results, <-results
	if (e1 == nil) == (e2 == nil) {
		t.Fatalf("expected exactly one committed writer; errors: %v, %v", e1, e2)
	}
	var n int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM ipam_occupancies WHERE address=x'000000000000000000000000C0000214')+(SELECT count(*) FROM dhcp_address_holds WHERE address=x'000000000000000000000000C0000214')`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("committed occupants=%d error=%v", n, err)
	}
}

func TestHiddenRowIDCannotReplaceHoldOrMapping(t *testing.T) {
	_, a, _ := fixture(t)
	install(t, a)
	tx := begin(t, a)
	if err := Create(ctx, tx, "a", "lease-old", 4, ip(10), "OFFER"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"rowid", "_rowid_", "oid"} {
		tx = begin(t, a)
		q := `INSERT OR REPLACE INTO dhcp_address_holds(` + alias + `,scope,tenant_id,space_id,subnet_id,lease_id,family,address,state,revision) VALUES (1,'a','t','s','n','lease-new',4,?,'OFFER',1)`
		if _, err := tx.Exec(q, ip(11)); err == nil {
			t.Fatalf("hold replaced via %s", alias)
		}
		q = `INSERT OR REPLACE INTO dhcp_scope_map(` + alias + `,scope,tenant_id,space_id,subnet_id) VALUES (1,'z','t','s','n')`
		if _, err := tx.Exec(q); err == nil {
			t.Fatalf("mapping replaced via %s", alias)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
	}
	tx = begin(t, a)
	var count int
	if err := tx.QueryRow(`SELECT count(*) FROM dhcp_address_holds WHERE scope='a' AND lease_id='lease-old' AND address=?`, ip(10)).Scan(&count); err != nil || count != 1 {
		t.Fatalf("old hold count=%d err=%v", count, err)
	}
	if err := static(tx, "s", "static-after-attack", 10); err == nil {
		t.Fatal("IPAM occupied held address")
	}
	_ = tx.Rollback()
}

func TestPinnedIPAMTablesRejectRowIDReplacement(t *testing.T) {
	_, a, _ := fixture(t)
	install(t, a)
	tx := begin(t, a)
	if err := static(tx, "s", "static-original", 30); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO ipam_pools(tenant_id,space_id,id,subnet_id,family,pool_kind,first_ip,last_ip) VALUES('t','s','pool','n',4,'IPAM',?,?)`, ip(100), ip(110)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"ipam_resource_ids", "ipam_tenants", "ipam_spaces", "ipam_prefixes", "ipam_pools", "ipam_occupancies"} {
		t.Run(table, func(t *testing.T) {
			tx := begin(t, a)
			defer tx.Rollback()
			for _, alias := range []string{"rowid", "_rowid_", "oid"} {
				q := `INSERT OR REPLACE INTO ` + table + `(` + alias + `) VALUES ((SELECT rowid FROM ` + table + ` LIMIT 1))`
				if _, err := tx.Exec(q); err == nil || !strings.Contains(err.Error(), "IPAM_ROWID_CONFLICT") {
					t.Fatalf("%s %s rowid guard: %v", table, alias, err)
				}
				q = `UPDATE OR REPLACE ` + table + ` SET ` + alias + `=` + alias + `+100000`
				if _, err := tx.Exec(q); err == nil {
					t.Fatalf("%s accepted %s update", table, alias)
				}
			}
		})
	}
}

var nonPositiveInserts = []struct{ table, sql string }{
	{"ipam_resource_ids", `INSERT INTO ipam_resource_ids(rowid,kind,tenant_id,space_id,id) VALUES(?,'TENANT','','','neg')`},
	{"ipam_tenants", `INSERT INTO ipam_tenants(rowid,id,slug,display_name) VALUES(?,'neg','neg','neg')`},
	{"ipam_spaces", `INSERT INTO ipam_spaces(rowid,tenant_id,id,slug,display_name) VALUES(?,'t','neg','neg','neg')`},
	{"ipam_prefixes", `INSERT INTO ipam_prefixes(rowid,tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES(?,'t','s','neg',4,24,x'000000000000000000000000C6336400',x'000000000000000000000000C63364FF','SUBNET','STANDARD')`},
	{"ipam_pools", `INSERT INTO ipam_pools(rowid,tenant_id,space_id,id,subnet_id,family,pool_kind,first_ip,last_ip) VALUES(?,'t','s','neg','n',4,'IPAM',x'000000000000000000000000C0000264',x'000000000000000000000000C000026E')`},
	{"ipam_occupancies", `INSERT INTO ipam_occupancies(rowid,tenant_id,space_id,id,subnet_id,family,address,kind,owner_ref,created_at_ms) VALUES(?,'t','s','neg','n',4,x'000000000000000000000000C000021E','STATIC','owner',1)`},
}

func TestNonPositiveRowIDRejectedAfterInstall(t *testing.T) {
	_, a, _ := fixture(t)
	install(t, a)
	for _, tc := range nonPositiveInserts {
		t.Run(tc.table, func(t *testing.T) {
			for _, id := range []int64{0, -1, -42} {
				tx := begin(t, a)
				if _, err := tx.Exec(tc.sql, id); err == nil || !strings.Contains(err.Error(), "IPAM_ROWID_NONPOSITIVE") {
					t.Fatalf("rowid %d: %v", id, err)
				}
				var n int
				if err := tx.QueryRow(`SELECT count(*) FROM `+tc.table+` WHERE rowid=?`, id).Scan(&n); err != nil || n != 0 {
					t.Fatalf("failed statement retained row: %d %v", n, err)
				}
				if err := tx.Rollback(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	// ABORT must roll back earlier rows from the same multi-row statement.
	tx := begin(t, a)
	if _, err := tx.Exec(`INSERT INTO ipam_tenants(rowid,id,slug,display_name) VALUES (2000,'partial','partial','partial'),(-1,'bad','bad','bad')`); err == nil {
		t.Fatal("accepted non-positive row in batch")
	}
	var partial int
	if err := tx.QueryRow(`SELECT count(*) FROM ipam_tenants WHERE id='partial'`).Scan(&partial); err != nil || partial != 0 {
		t.Fatalf("partial statement survived: %d %v", partial, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	// Implicit rowids, including multiple rows in one statement, remain usable.
	tx = begin(t, a)
	if _, err := tx.Exec(`INSERT INTO ipam_tenants(id,slug,display_name) VALUES ('ok1','ok1','ok1'),('ok2','ok2','ok2')`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(`SELECT count(*) FROM ipam_tenants WHERE id IN ('ok1','ok2') AND rowid>0`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("implicit count %d %v", count, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestInstallRejectsExistingNonPositiveRowID(t *testing.T) {
	for _, tc := range nonPositiveInserts {
		t.Run(tc.table, func(t *testing.T) {
			_, a, _ := fixture(t)
			// Simulate a tampered v2 database: dropping a required guard must
			// be detected before installing any DHCP objects.
			if _, err := a.ExecContext(ctx, `DROP TRIGGER `+tc.table+`_rowid_positive_guard`); err != nil {
				t.Fatal(err)
			}
			if _, err := a.ExecContext(ctx, tc.sql, -1); err != nil {
				t.Fatalf("fixture: %v", err)
			}
			tx := begin(t, a)
			if err := Install(ctx, tx, []Mapping{{"a", "t", "s", "n"}}); err == nil {
				t.Fatalf("installation: %v", err)
			}
			var n int
			if err := tx.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name LIKE 'dhcp_%'`).Scan(&n); err != nil || n != 0 {
				t.Fatalf("left installation objects=%d err=%v", n, err)
			}
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestIPv6Rejected(t *testing.T) {
	db, a, _ := fixture(t)
	if _, err := db.Exec(`INSERT INTO ipam_prefixes(tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES ('t','s','v6',6,64,x'20010DB8000000000000000000000000',x'20010DB800000000FFFFFFFFFFFFFFFF','SUBNET','STANDARD')`); err != nil {
		t.Fatal(err)
	}
	tx := begin(t, a)
	if err := Install(ctx, tx, []Mapping{{"v6", "t", "s", "v6"}}); err == nil {
		t.Fatal("IPv6 scope accepted")
	}
	_ = tx.Rollback()
	install(t, a)
	tx = begin(t, a)
	if err := Create(ctx, tx, "a", "v6-lease", 6, []byte{0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, "OFFER"); err == nil {
		t.Fatal("IPv6 hold accepted")
	}
	_ = tx.Rollback()
}

func TestBidirectionalAndSpace(t *testing.T) {
	_, a, b := fixture(t)
	install(t, a)
	tx := begin(t, a)
	if e := Create(ctx, tx, "a", "lease1", 4, ip(10), "OFFER"); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(); e != nil {
		t.Fatal(e)
	}
	tx = begin(t, b)
	if e := static(tx, "s", "static1", 10); e == nil {
		t.Fatal("static overlaps DHCP")
	}
	tx.Rollback()
	tx = begin(t, b)
	if e := static(tx, "s", "static2", 11); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(); e != nil {
		t.Fatal(e)
	}
	tx = begin(t, a)
	if e := Create(ctx, tx, "b", "lease2", 4, ip(11), "ACTIVE"); e == nil {
		t.Fatal("DHCP overlaps static")
	}
	tx.Rollback()
	tx = begin(t, b)
	if e := Create(ctx, tx, "b", "lease3", 4, ip(10), "OFFER"); e == nil {
		t.Fatal("scope bypass")
	}
	tx.Rollback()
	tx = begin(t, b)
	if e := Create(ctx, tx, "c", "lease4", 4, ip(10), "QUARANTINE"); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(); e != nil {
		t.Fatal(e)
	}
}
func TestValidationAndNoImplicitRelease(t *testing.T) {
	_, a, _ := fixture(t)
	install(t, a)
	for _, tc := range []struct {
		scope, lease string
		family       int
		address      []byte
		state        string
	}{{"missing", "x", 4, ip(12), "OFFER"}, {"a", "x", 4, ip(0), "OFFER"}, {"a", "x", 4, ip(255), "OFFER"}, {"a", "x", 4, []byte{1}, "OFFER"}, {"a", "x", 7, ip(12), "OFFER"}, {"a", "x", 4, ip(12), "UNKNOWN"}, {"a", "", 4, ip(12), "OFFER"}} {
		tx := begin(t, a)
		if e := Create(ctx, tx, tc.scope, tc.lease, tc.family, tc.address, tc.state); e == nil {
			t.Fatalf("accepted %+v", tc)
		}
		tx.Rollback()
	}
	tx := begin(t, a)
	if _, e := tx.Exec(`INSERT INTO dhcp_scope_map VALUES ('new','t','s','n')`); e == nil {
		t.Fatal("mapping is not sealed")
	}
	if e := Create(ctx, tx, "a", "lease", 4, ip(12), "OFFER"); e != nil {
		t.Fatal(e)
	}
	if _, e := tx.Exec(`UPDATE dhcp_address_holds SET state='ACTIVE',revision=2 WHERE lease_id='lease'`); e != nil {
		t.Fatal(e)
	}
	if _, e := tx.Exec(`UPDATE dhcp_address_holds SET state='OFFER',revision=3 WHERE lease_id='lease'`); e == nil {
		t.Fatal("reverse transition")
	}
	if _, e := tx.Exec(`DELETE FROM dhcp_address_holds`); e == nil {
		t.Fatal("release permitted")
	}
	tx.Rollback()
}
func TestInstallRejectsExtraProtectedObjects(t *testing.T) {
	for _, tc := range []struct {
		name, ddl string
		temp      bool
	}{
		{"ignore trigger", `CREATE TRIGGER extra_suppress BEFORE INSERT ON ipam_occupancies BEGIN SELECT RAISE(IGNORE); END`, false},
		{"extra index", `CREATE INDEX custom_index ON ipam_occupancies(owner_ref)`, false},
		{"ipam named view", `CREATE VIEW IPAM_unexpected AS SELECT 1`, false},
		{"ipam named unrelated index", `CREATE INDEX ipam_marker_index ON enterprise_schema_migrations(version)`, false},
		{"temp trigger", `CREATE TEMP TRIGGER temp_suppress BEFORE INSERT ON main.ipam_occupancies BEGIN SELECT RAISE(IGNORE); END`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, a, _ := fixture(t)
			if _, err := a.ExecContext(ctx, `CREATE TABLE enterprise_schema_migrations(version INTEGER)`); err != nil {
				t.Fatal(err)
			}
			if _, err := a.ExecContext(ctx, tc.ddl); err != nil {
				t.Fatal(err)
			}
			tx := begin(t, a)
			if err := Install(ctx, tx, []Mapping{{"a", "t", "s", "n"}}); err == nil {
				t.Fatal("extra protected object accepted")
			}
			_ = tx.Rollback()
			_ = db
		})
	}
}

func TestInstallAllowsUnrelatedObjects(t *testing.T) {
	_, a, _ := fixture(t)
	for _, q := range []string{`CREATE TABLE enterprise_schema_migrations(version INTEGER)`, `CREATE INDEX marker_index ON enterprise_schema_migrations(version)`, `CREATE TRIGGER marker_trigger BEFORE INSERT ON enterprise_schema_migrations BEGIN SELECT 1; END`, `CREATE VIEW ordinary_view AS SELECT 1`} {
		if _, err := a.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	install(t, a)
}

func TestInstallAtomicAndRequirements(t *testing.T) {
	db, a, _ := fixture(t)
	tx := begin(t, a)
	if e := Install(ctx, tx, nil); e == nil {
		t.Fatal("missing map")
	}
	tx.Rollback()
	if _, e := db.Exec(`CREATE TABLE unrelated_marker (id INTEGER); CREATE TRIGGER dhcp_ipam_update BEFORE UPDATE ON unrelated_marker BEGIN SELECT 1; END`); e != nil {
		t.Fatal(e)
	}
	tx = begin(t, a)
	if e := Install(ctx, tx, []Mapping{{"a", "t", "s", "n"}}); e == nil {
		t.Fatal("expected trigger collision")
	}
	var n int
	if e := tx.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='dhcp_address_holds'`).Scan(&n); e != nil || n != 0 {
		t.Fatalf("partial schema %d %v", n, e)
	}
	tx.Rollback()
	if _, e := db.Exec(`DROP TRIGGER dhcp_ipam_update`); e != nil {
		t.Fatal(e)
	}
	tx = begin(t, a)
	if e := Install(ctx, tx, []Mapping{{"a", "t", "s", "absent"}}); e == nil {
		t.Fatal("missing subnet")
	}
	if e := tx.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='dhcp_scope_map'`).Scan(&n); e != nil || n != 0 {
		t.Fatalf("partial schema %d %v", n, e)
	}
	tx.Rollback()
	if _, e := db.Exec(`DROP TRIGGER ipam_occupancies_insert_guard`); e != nil {
		t.Fatal(e)
	}
	tx = begin(t, a)
	if e := Install(ctx, tx, []Mapping{{"a", "t", "s", "n"}}); e == nil {
		t.Fatal("drifted schema")
	}
	tx.Rollback()
}
