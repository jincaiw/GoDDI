package replicaschema

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlogguard"
	"github.com/jincaiw/GoDDI/internal/sqlitepolicy"
)

// hardenedFixture builds an isolated on-disk database that matches the frozen
// hardened reference exactly (DDL + guard triggers + both marker rows), then
// returns a query_only connection over it. It is a test fixture, not a
// migration: eventlogguard.Migrate remains closed.
func hardenedFixture(t *testing.T, mutate ...func(context.Context, *sql.DB)) *sql.Conn {
	t.Helper()
	ctx := context.Background()
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "hardened.db"))
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	ddls, err := eventlogguard.ReferenceDDLs(eventlogguard.HardenedReference)
	if err != nil {
		t.Fatalf("hardened DDLs: %v", err)
	}
	for _, ddl := range ddls {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatalf("apply hardened DDL: %v", err)
		}
	}
	markers, err := eventlogguard.ReferenceMarkerStatements(eventlogguard.HardenedReference)
	if err != nil {
		t.Fatalf("hardened markers: %v", err)
	}
	for _, stmt := range markers {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("apply hardened marker: %v", err)
		}
	}
	for _, fn := range mutate {
		fn(ctx, db)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := conn.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestHardenedReferenceIsAccepted(t *testing.T) {
	conn := hardenedFixture(t)
	if err := Verify(context.Background(), conn); err != nil {
		t.Fatalf("hardened reference rejected: %v", err)
	}
}

// The two frozen structures must be distinguishable: a hardened database with
// one guard removed matches neither reference and must be refused.
func TestHardenedReferenceWithMissingGuardIsRejected(t *testing.T) {
	conn := hardenedFixture(t, func(ctx context.Context, db *sql.DB) {
		if _, err := db.ExecContext(ctx, `DROP TRIGGER writer_authority_rowid_insert_guard`); err != nil {
			t.Fatalf("drop guard: %v", err)
		}
	})
	if err := Verify(context.Background(), conn); err == nil {
		t.Fatal("a database matching neither frozen reference was accepted")
	}
}

// A hardened database with a corrupted guard marker must be refused: the
// extra marker table is part of the frozen structure, not optional telemetry.
func TestHardenedReferenceWithBadGuardMarkerIsRejected(t *testing.T) {
	conn := hardenedFixture(t, func(ctx context.Context, db *sql.DB) {
		if _, err := db.ExecContext(ctx, `UPDATE eventlog_reference_guard_migrations SET guard_digest = 'bad'`); err != nil {
			t.Fatalf("corrupt marker: %v", err)
		}
	})
	if err := Verify(context.Background(), conn); err == nil {
		t.Fatal("a hardened database with a corrupted guard marker was accepted")
	}
}

func TestReferenceDDLsAreVersioned(t *testing.T) {
	legacy, err := eventlogguard.ReferenceDDLs(eventlogguard.LegacyReference)
	if err != nil {
		t.Fatal(err)
	}
	hardened, err := eventlogguard.ReferenceDDLs(eventlogguard.HardenedReference)
	if err != nil {
		t.Fatal(err)
	}
	if len(hardened) <= len(legacy) {
		t.Fatalf("hardened DDL set (%d) is not a strict superset of legacy (%d)", len(hardened), len(legacy))
	}
	if _, err := eventlogguard.ReferenceDDLs(eventlogguard.Unknown); err == nil {
		t.Fatal("an unknown version must not produce DDLs")
	}
	if _, err := eventlogguard.ReferenceMarkerStatements(eventlogguard.Unknown); err == nil {
		t.Fatal("an unknown version must not produce marker statements")
	}
	legacyMarkers, err := eventlogguard.ReferenceMarkerStatements(eventlogguard.LegacyReference)
	if err != nil {
		t.Fatal(err)
	}
	hardenedMarkers, err := eventlogguard.ReferenceMarkerStatements(eventlogguard.HardenedReference)
	if err != nil {
		t.Fatal(err)
	}
	if len(hardenedMarkers) != len(legacyMarkers)+1 {
		t.Fatalf("hardened markers = %d, want %d", len(hardenedMarkers), len(legacyMarkers)+1)
	}
}
