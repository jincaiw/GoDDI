package dnssec

import (
	"path/filepath"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
	"github.com/jasonwa/goddi/internal/dns/zone"
)

// setupDSSECTest builds a migrated sqlite database with zone + dnssec
// managers wired together (same pattern as internal/backup tests).
func setupDSSECTest(t *testing.T) (*DNSSECManager, *zone.ZoneManager) {
	t.Helper()
	dir := t.TempDir()
	dbh, err := database.New(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(dir, "test.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbh.Close() })
	if err := dbh.RunMigrations(filepath.Join("..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	zm := zone.NewZoneManager(dbh.DB, nil)
	return NewDNSSECManager(dbh.DB, zm, nil, "test-secret"), zm
}

// TestDSRecordsAndKeyLifecycle covers DS export for a generated KSK plus
// key delete/toggle and the NSEC3 parameter round trip.
func TestDSRecordsAndKeyLifecycle(t *testing.T) {
	mgr, zm := setupDSSECTest(t)

	z, err := zm.CreateZone(zone.ZoneOptions{Name: "signed.example.com", Type: string(zone.ZoneTypePrimary)})
	if err != nil {
		t.Fatal(err)
	}

	ksk, err := mgr.GenerateKSK(z.ID, string(AlgECDSAP256SHA256))
	if err != nil {
		t.Fatalf("generate KSK: %v", err)
	}
	if _, err := mgr.GenerateZSK(z.ID, string(AlgECDSAP256SHA256)); err != nil {
		t.Fatalf("generate ZSK: %v", err)
	}

	// DS export: exactly one entry for the single active KSK.
	dsRecords, err := mgr.GetDSRecords(z.ID)
	if err != nil {
		t.Fatalf("get DS records: %v", err)
	}
	if len(dsRecords) != 1 {
		t.Fatalf("DS records = %d, want 1", len(dsRecords))
	}
	if dsRecords[0].KeyTag != ksk.KeyTag || dsRecords[0].DigestType != 2 || dsRecords[0].Digest == "" {
		t.Fatalf("unexpected DS record: %+v", dsRecords[0])
	}

	// NSEC3 round trip.
	if err := mgr.SetNSEC3Params(z.ID, NSEC3Params{Iterations: 10, Salt: "aabb", OptOut: false}); err != nil {
		t.Fatalf("set NSEC3: %v", err)
	}
	params, err := mgr.GetNSEC3Params(z.ID)
	if err != nil {
		t.Fatal(err)
	}
	if params.Iterations != 10 || params.Salt != "aabb" {
		t.Fatalf("NSEC3 params = %+v", params)
	}
	// Validation guards.
	if err := mgr.SetNSEC3Params(z.ID, NSEC3Params{Iterations: 5000}); err == nil {
		t.Fatal("expected error for excessive iterations")
	}
	if err := mgr.SetNSEC3Params(z.ID, NSEC3Params{Salt: "zzzz"}); err == nil {
		t.Fatal("expected error for non-hex salt")
	}

	// Toggle + delete.
	if err := mgr.SetKeyEnabled(z.ID, ksk.ID, false); err != nil {
		t.Fatalf("disable key: %v", err)
	}
	dsRecords, err = mgr.GetDSRecords(z.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dsRecords) != 0 {
		t.Fatalf("DS records after disable = %d, want 0", len(dsRecords))
	}
	if err := mgr.DeleteKey(z.ID, ksk.ID); err != nil {
		t.Fatalf("delete key: %v", err)
	}
	if err := mgr.DeleteKey(z.ID, "no-such-key"); err == nil {
		t.Fatal("expected error deleting unknown key")
	}
}
