package snapshotipamcheck

import (
	"context"
	"errors"
	"testing"
)

func TestRejectTemporaryObjectsAndAttachedDatabases(t *testing.T) {
	cases := []struct {
		name  string
		setup []string
	}{
		{"shadowed tables with valid fake rows", []string{
			`CREATE TEMP TABLE ipam_tenants AS SELECT * FROM main.ipam_tenants`,
			`CREATE TEMP TABLE ipam_spaces AS SELECT * FROM main.ipam_spaces`,
			`CREATE TEMP TABLE ipam_prefixes AS SELECT * FROM main.ipam_prefixes`,
			`CREATE TEMP TABLE ipam_occupancies AS SELECT * FROM main.ipam_occupancies`,
			`DROP TRIGGER main.ipam_occupancies_identity_immutable`,
			`DROP TRIGGER main.ipam_occupancies_transition_guard`,
			`UPDATE main.ipam_occupancies SET address=x'000000000000000000000000c6336401' WHERE tenant_id='t1'`,
		}},
		{"temp table", []string{`CREATE TEMP TABLE unrelated(id INTEGER)`}},
		{"temp view", []string{`CREATE TEMP VIEW unrelated AS SELECT id FROM main.ipam_tenants`}},
		{"temp trigger", []string{`CREATE TEMP TRIGGER unrelated BEFORE INSERT ON main.ipam_tenants BEGIN SELECT 1; END`}},
		{"temp index", []string{`CREATE TEMP TABLE unrelated(id INTEGER)`, `CREATE INDEX temp.unrelated_idx ON unrelated(id)`}},
		{"attached memory", []string{`ATTACH DATABASE ':memory:' AS foreign_db`}},
		{"attached database name collides", []string{`ATTACH DATABASE ':memory:' AS ipam_tenants`, `CREATE TABLE ipam_tenants.ipam_tenants(id TEXT)`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := fixture(t)
			c := conn(t, db, false)
			ctx := context.Background()
			for _, q := range tc.setup {
				if _, err := c.ExecContext(ctx, q); err != nil {
					t.Fatalf("setup %s: %v", q, err)
				}
			}
			if tc.name == "shadowed tables with valid fake rows" {
				var fake, damaged int
				if err := c.QueryRowContext(ctx, `SELECT count(*) FROM temp.ipam_occupancies WHERE tenant_id='t1' AND address=x'000000000000000000000000c000020a'`).Scan(&fake); err != nil {
					t.Fatal(err)
				}
				if err := c.QueryRowContext(ctx, `SELECT count(*) FROM main.ipam_occupancies WHERE tenant_id='t1' AND address=x'000000000000000000000000c6336401'`).Scan(&damaged); err != nil {
					t.Fatal(err)
				}
				if fake != 1 || damaged != 1 {
					t.Fatalf("shadow fixture fake=%d damaged=%d", fake, damaged)
				}
			}
			if _, err := c.ExecContext(ctx, `PRAGMA query_only=ON`); err != nil {
				t.Fatal(err)
			}
			out, err := Check(ctx, c, limits())
			if !errors.Is(err, ErrInvalid) || out != (Evidence{}) {
				t.Fatalf("accepted isolation violation: %+v %v", out, err)
			}
		})
	}
}
func TestAllowedEmptyTempSchemaAndNoWrites(t *testing.T) {
	db := fixture(t)
	c := conn(t, db, false)
	ctx := context.Background()
	// An initialized but empty TEMP database is harmless.
	for _, q := range []string{`CREATE TEMP TABLE scratch(x)`, `DROP TABLE temp.scratch`, `PRAGMA query_only=ON`} {
		if _, err := c.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	var beforeChanges, afterChanges, beforeSchema, afterSchema int
	if err := c.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&beforeChanges); err != nil {
		t.Fatal(err)
	}
	if err := c.QueryRowContext(ctx, `PRAGMA main.schema_version`).Scan(&beforeSchema); err != nil {
		t.Fatal(err)
	}
	out, err := Check(ctx, c, limits())
	if err != nil || !out.Integrity || out.Counts != baseCounts {
		t.Fatalf("valid main: %+v %v", out, err)
	}
	if err := c.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&afterChanges); err != nil {
		t.Fatal(err)
	}
	if err := c.QueryRowContext(ctx, `PRAGMA main.schema_version`).Scan(&afterSchema); err != nil {
		t.Fatal(err)
	}
	if beforeChanges != afterChanges || beforeSchema != afterSchema {
		t.Fatal("check modified database")
	}
}
