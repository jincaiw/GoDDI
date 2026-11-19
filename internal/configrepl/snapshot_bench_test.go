package configrepl

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/jasonwa/goddi/internal/dataplane"
)

// These benchmarks measure the CURRENT full-snapshot cost on this machine.
// They are lab numbers, not a production threshold: target hardware, row
// widths and write concurrency still have to be supplied by the deployment.
func seedScopesBench(tb testing.TB, db *sql.DB, n int) {
	tb.Helper()
	tx, err := db.Begin()
	if err != nil {
		tb.Fatal(err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO dhcp_scopes
		(id,name,subnet,start_ip,end_ip,lease_time,enabled,ping_check_enabled,dns_updates)
		VALUES (?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tb.Fatal(err)
	}
	defer stmt.Close()
	for i := 0; i < n; i++ {
		if _, err := stmt.Exec(fmt.Sprintf("scope-%08d", i), "lab", "10.0.0.0/24",
			"10.0.0.10", "10.0.0.200", 3600, 1, 0, 0); err != nil {
			tb.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		tb.Fatal(err)
	}
}

func benchSpecs() []dataplane.ConfigTableSpec {
	return []dataplane.ConfigTableSpec{specByName("dhcp_scopes")}
}

func BenchmarkReadSnapshotFull(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			ctx := context.Background()
			db := newBenchDB(b)
			if _, err := db.Exec(schemaScopes); err != nil {
				b.Fatal(err)
			}
			seedScopesBench(b, db, n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := ReadSnapshot(ctx, db, "node-a", benchSpecs()); err != nil {
					b.Fatal(err)
				}
			}
			db.Close()
		})
	}
}

func BenchmarkApplySnapshotFull(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			ctx := context.Background()
			src := newBenchDB(b)
			defer src.Close()
			if _, err := src.Exec(schemaScopes); err != nil {
				b.Fatal(err)
			}
			seedScopesBench(b, src, n)
			snap, err := ReadSnapshot(ctx, src, "node-a", benchSpecs())
			if err != nil {
				b.Fatal(err)
			}
			blob, err := EncodeSnapshotJSON(snap)
			if err != nil {
				b.Fatal(err)
			}
			snap, err = DecodeSnapshotJSON(blob)
			if err != nil {
				b.Fatal(err)
			}
			dst := newBenchDB(b)
			defer dst.Close()
			if _, err := dst.Exec(schemaScopes); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				if _, err := dst.Exec(`DELETE FROM dhcp_scopes`); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if _, err := ApplySnapshot(ctx, dst, snap, benchSpecs()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func newBenchDB(b *testing.B) *sql.DB {
	b.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	return db
}
func BenchmarkSnapshotWireRoundTrip(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			ctx := context.Background()
			db := newBenchDB(b)
			defer db.Close()
			if _, err := db.Exec(schemaScopes); err != nil {
				b.Fatal(err)
			}
			seedScopesBench(b, db, n)
			snap, err := ReadSnapshot(ctx, db, "node-a", benchSpecs())
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				blob, err := EncodeSnapshotJSON(snap)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := DecodeSnapshotJSON(blob); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
