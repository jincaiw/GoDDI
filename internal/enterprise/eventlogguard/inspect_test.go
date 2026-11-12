package eventlogguard

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestInspectClassifiesBothReferencesWithoutWrites(t *testing.T) {
	for _, version := range []Version{LegacyReference, HardenedReference} {
		t.Run(map[Version]string{LegacyReference: "legacy", HardenedReference: "hardened"}[version], func(t *testing.T) {
			db := legacy(t)
			if version == HardenedReference {
				if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
					t.Fatal(err)
				}
			}
			c := connQueryOnly(t, db)
			var before, after int
			if err := c.QueryRowContext(context.Background(), `SELECT total_changes()`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			v, err := Inspect(context.Background(), c)
			if err != nil || v != version {
				t.Fatalf("version=%v err=%v", v, err)
			}
			if err = c.QueryRowContext(context.Background(), `SELECT total_changes()`).Scan(&after); err != nil || after != before {
				t.Fatalf("changed db: %d %d %v", before, after, err)
			}
			err = Verify(context.Background(), c)
			if (version == LegacyReference && !errors.Is(err, ErrInvalid)) || (version == HardenedReference && err != nil) {
				t.Fatalf("Verify mismatch %v", err)
			}
		})
	}
}
func TestInspectRejectsPartialMixedAndUnknown(t *testing.T) {
	for _, tc := range []struct {
		name     string
		hardened bool
		sql      string
	}{{"legacy forged guard marker", false, markerDDL}, {"legacy lone guard", false, `CREATE TRIGGER writer_authority_rowid_insert_guard BEFORE INSERT ON writer_authority BEGIN SELECT 1; END`}, {"hardened missing marker", true, `DROP TABLE eventlog_reference_guard_migrations`}, {"hardened wrong marker", true, `UPDATE eventlog_reference_guard_migrations SET base_digest='wrong'`}, {"hardened extra", true, `CREATE VIEW extra AS SELECT 1`}, {"hardened missing guard", true, `DROP TRIGGER replica_head_rowid_insert_guard`}} {
		t.Run(tc.name, func(t *testing.T) {
			db := legacy(t)
			if tc.hardened {
				if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(tc.sql); err != nil {
				t.Fatal(err)
			}
			c := connQueryOnly(t, db)
			v, err := Inspect(context.Background(), c)
			if v != Unknown || !errors.Is(err, ErrInvalid) {
				t.Fatalf("version=%v err=%v", v, err)
			}
		})
	}
}
func TestInspectLegacyStrictPreflight(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		id        int64
	}{{"nonpositive rowid", tableCases[1].first, 0}, {"null head key", strings.Replace(tableCases[1].first, "x'01000000000000000000000000000000'", "NULL", 1), 10}, {"receipt tenant text", strings.Replace(tableCases[4].first, "x'01000000000000000000000000000000'", "'abcdefghijklmnop'", 1), 10}} {
		t.Run(tc.name, func(t *testing.T) {
			db := legacy(t)
			if _, err := db.Exec(tc.sql, tc.id); err != nil {
				t.Fatal(err)
			}
			c := connQueryOnly(t, db)
			v, err := Inspect(context.Background(), c)
			if v != Unknown || !errors.Is(err, ErrInvalid) {
				t.Fatalf("bad legacy accepted version=%v err=%v", v, err)
			}
		})
	}
}
func TestInspectCanceledNilAndClosed(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		db := legacy(t)
		c := connQueryOnly(t, db)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		v, err := Inspect(ctx, c)
		if v != Unknown || !errors.Is(err, context.Canceled) || !errors.Is(err, ErrInvalid) {
			t.Fatalf("cancel version=%v err=%v", v, err)
		}
		if e := c.Close(); e != nil && !errors.Is(e, sql.ErrConnDone) {
			t.Fatalf("close canceled connection: %v", e)
		}
		fresh := connQueryOnly(t, db)
		v, err = Inspect(context.Background(), fresh)
		if err != nil || v != LegacyReference {
			t.Fatalf("fresh connection version=%v err=%v", v, err)
		}
	})
	t.Run("nil", func(t *testing.T) {
		db := legacy(t)
		c := connQueryOnly(t, db)
		v, err := Inspect(nil, c)
		if v != Unknown || !errors.Is(err, ErrInvalid) {
			t.Fatal("nil ctx")
		}
		v, err = Inspect(context.Background(), nil)
		if v != Unknown || !errors.Is(err, ErrInvalid) {
			t.Fatal("nil conn")
		}
	})
	t.Run("closed", func(t *testing.T) {
		db := legacy(t)
		c := connQueryOnly(t, db)
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		v, err := Inspect(context.Background(), c)
		if v != Unknown || !errors.Is(err, ErrInvalid) || !errors.Is(err, sql.ErrConnDone) {
			t.Fatalf("closed version=%v err=%v", v, err)
		}
	})
}
func TestInspectConcurrentSeparateConnections(t *testing.T) {
	db := legacy(t)
	db.SetMaxOpenConns(3)
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for i := 0; i < 2; i++ {
		c, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.ExecContext(context.Background(), `PRAGMA query_only=ON`); err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer c.Close()
			v, err := Inspect(context.Background(), c)
			if err == nil && v != LegacyReference {
				err = errors.New("incorrect version")
			}
			out <- err
		}()
	}
	wg.Wait()
	close(out)
	for err := range out {
		if err != nil {
			t.Fatal(err)
		}
	}
}
