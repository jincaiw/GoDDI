package snapshotipamcheck

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/ipamstore"
	"github.com/jincaiw/GoDDI/internal/sqlitepolicy"
)

var baseCounts = Counts{Tenants: 2, Spaces: 2, Prefixes: 3, Occupancies: 2, Ledger: 9}

func fixture(t *testing.T) *sql.DB {
	t.Helper()
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "staging.db"))
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if err := ipamstore.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	queries := []string{
		`INSERT INTO ipam_tenants(id,slug,display_name) VALUES('t1','one','One'),('t2','two','Two')`,
		`INSERT INTO ipam_spaces(tenant_id,id,slug,display_name) VALUES('t1','s','s1','S1'),('t2','s','s2','S2')`,
		`INSERT INTO ipam_prefixes(tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES('t1','s','parent',4,16,x'000000000000000000000000c0000000',x'000000000000000000000000c000ffff','CONTAINER','AGGREGATE')`,
		`INSERT INTO ipam_prefixes(tenant_id,space_id,id,parent_id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES('t1','s','net','parent',4,24,x'000000000000000000000000c0000200',x'000000000000000000000000c00002ff','SUBNET','STANDARD')`,
		`INSERT INTO ipam_prefixes(tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES('t2','s','net',6,64,x'20010db8000000000000000000000000',x'20010db800000000ffffffffffffffff','SUBNET','STANDARD')`,
		`INSERT INTO ipam_occupancies(tenant_id,space_id,id,subnet_id,family,address,kind,owner_ref,created_at_ms) VALUES('t1','s','a','net',4,x'000000000000000000000000c000020a','STATIC','a',1)`,
		`INSERT INTO ipam_occupancies(tenant_id,space_id,id,subnet_id,family,address,kind,owner_ref,created_at_ms) VALUES('t2','s','a','net',6,x'20010db8000000000000000000000001','STATIC','a',1)`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return db
}
func conn(t *testing.T, db *sql.DB, queryOnly bool) *sql.Conn {
	t.Helper()
	c, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	if queryOnly {
		if _, err = c.ExecContext(context.Background(), "PRAGMA query_only=ON"); err != nil {
			t.Fatal(err)
		}
	}
	return c
}
func limits() Limits { return Limits{Rows: 50, Bytes: 1 << 20, Expected: baseCounts} }
func TestValidMultipleTenantsAndIPv6(t *testing.T) {
	db := fixture(t)
	result, err := Check(context.Background(), conn(t, db, true), limits())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Integrity || !result.ForeignKeys || !result.RowShapes || !result.Scopes || !result.PrefixRelations || !result.OccupancyRelations || result.Counts != baseCounts {
		t.Fatalf("evidence %+v", result)
	}
}
func TestRequiresConnectionGuardsAndExpectedCounts(t *testing.T) {
	t.Run("query only", func(t *testing.T) {
		db := fixture(t)
		if got, err := Check(context.Background(), conn(t, db, false), limits()); err == nil || got != (Evidence{}) {
			t.Fatalf("got %+v %v", got, err)
		}
	})
	t.Run("foreign keys", func(t *testing.T) {
		db := fixture(t)
		c := conn(t, db, false)
		if _, err := c.ExecContext(context.Background(), "PRAGMA foreign_keys=OFF"); err != nil {
			t.Fatal(err)
		}
		if _, err := c.ExecContext(context.Background(), "PRAGMA query_only=ON"); err != nil {
			t.Fatal(err)
		}
		if _, err := Check(context.Background(), c, limits()); err == nil {
			t.Fatal("foreign keys disabled")
		}
	})
	t.Run("row cap", func(t *testing.T) {
		db := fixture(t)
		l := limits()
		l.Rows = 8
		if _, err := Check(context.Background(), conn(t, db, true), l); err == nil {
			t.Fatal("row limit ignored")
		}
	})
	t.Run("byte cap", func(t *testing.T) {
		db := fixture(t)
		l := limits()
		l.Bytes = 20
		if _, err := Check(context.Background(), conn(t, db, true), l); err == nil {
			t.Fatal("byte limit ignored")
		}
	})
	t.Run("truncation", func(t *testing.T) {
		db := fixture(t)
		l := limits()
		l.Expected.Occupancies++
		if _, err := Check(context.Background(), conn(t, db, true), l); err == nil {
			t.Fatal("count mismatch accepted")
		}
	})
	t.Run("cancel", func(t *testing.T) {
		db := fixture(t)
		c := conn(t, db, true)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if got, err := Check(ctx, c, limits()); err == nil || got != (Evidence{}) {
			t.Fatalf("canceled %+v %v", got, err)
		}
	})
}
func TestInvalidRelationshipsCannotHide(t *testing.T) {
	cases := map[string]struct {
		queries []string
		counts  Counts
	}{
		"overlap":                            {[]string{`DROP TRIGGER ipam_prefixes_insert_guard`, `INSERT INTO ipam_prefixes(tenant_id,space_id,id,parent_id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES('t1','s','overlap','parent',4,24,x'000000000000000000000000c0000200',x'000000000000000000000000c00002ff','SUBNET','STANDARD')`}, Counts{Tenants: 2, Spaces: 2, Prefixes: 4, Occupancies: 2, Ledger: 10}},
		"noncontainer parent":                {[]string{`DROP TRIGGER ipam_prefixes_topology_immutable`, `UPDATE ipam_prefixes SET kind='SUBNET',usage='STANDARD' WHERE tenant_id='t1' AND id='parent'`}, baseCounts},
		"parent does not contain child":      {[]string{`INSERT INTO ipam_prefixes(tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES('t1','s','other',4,16,x'000000000000000000000000c6330000',x'000000000000000000000000c633ffff','CONTAINER','AGGREGATE')`, `DROP TRIGGER ipam_prefixes_topology_immutable`, `UPDATE ipam_prefixes SET parent_id='other' WHERE tenant_id='t1' AND id='net'`}, Counts{Tenants: 2, Spaces: 2, Prefixes: 4, Occupancies: 2, Ledger: 10}},
		"family mismatch":                    {[]string{`DROP TRIGGER ipam_occupancies_identity_immutable`, `DROP TRIGGER ipam_occupancies_transition_guard`, `UPDATE ipam_occupancies SET family=6,address=x'20010db8000000000000000000000001' WHERE tenant_id='t1'`}, baseCounts},
		"occupancy outside":                  {[]string{`DROP TRIGGER ipam_occupancies_identity_immutable`, `DROP TRIGGER ipam_occupancies_transition_guard`, `UPDATE ipam_occupancies SET address=x'000000000000000000000000c000030a' WHERE tenant_id='t1'`}, baseCounts},
		"illegal text":                       {[]string{`DROP TRIGGER ipam_tenants_update_guard`, `UPDATE ipam_tenants SET display_name=CAST(x'80' AS TEXT) WHERE id='t1'`}, baseCounts},
		"wrong subnet family across tenants": {[]string{`DROP TRIGGER ipam_occupancies_identity_immutable`, `DROP TRIGGER ipam_occupancies_transition_guard`, `UPDATE ipam_occupancies SET subnet_id='parent' WHERE tenant_id='t1'`}, baseCounts},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			db := fixture(t)
			for _, q := range tc.queries {
				if _, err := db.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			l := limits()
			l.Expected = tc.counts
			got, err := Check(context.Background(), conn(t, db, true), l)
			if err == nil || got != (Evidence{}) {
				t.Fatalf("accepted invalid data %+v %v", got, err)
			}
		})
	}
}
func TestDanglingForeignKey(t *testing.T) {
	db := fixture(t)
	c := conn(t, db, false)
	ctx := context.Background()
	for _, q := range []string{`PRAGMA foreign_keys=OFF`, `DROP TRIGGER ipam_spaces_delete_guard`, `DELETE FROM ipam_spaces WHERE tenant_id='t1'`, `PRAGMA foreign_keys=ON`, `PRAGMA query_only=ON`} {
		if _, err := c.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	l := limits()
	l.Expected.Spaces--
	if got, err := Check(ctx, c, l); err == nil || got != (Evidence{}) {
		t.Fatalf("dangling accepted %+v %v", got, err)
	}
}
func TestTruncatedDatabaseFailsClosed(t *testing.T) {
	db := fixture(t)
	var seq int
	var name, path string
	if err := db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, 4096); err != nil {
		t.Fatal(err)
	}
	reopened := sqlitepolicy.OpenWriter(path)
	defer reopened.Close()
	reopened.SetMaxOpenConns(1)
	c, err := reopened.Conn(context.Background())
	if err != nil {
		return
	}
	defer c.Close()
	if _, err = c.ExecContext(context.Background(), `PRAGMA query_only=ON`); err != nil {
		return
	}
	if got, err := Check(context.Background(), c, limits()); err == nil || got != (Evidence{}) {
		t.Fatalf("truncated database accepted: %+v %v", got, err)
	}
}
func TestDifferentExpectedEmptyNotImplicitlyAccepted(t *testing.T) {
	db := fixture(t)
	l := limits()
	l.Expected = Counts{}
	if _, err := Check(context.Background(), conn(t, db, true), l); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unexpected error: %v", err)
	}
}
