package zone

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
)

// setupCatalogTest builds a migrated sqlite database plus a ZoneManager, the
// same way internal/backup tests bootstrap their fixtures.
func setupCatalogTest(t *testing.T) *ZoneManager {
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
	return NewZoneManager(dbh.DB, nil)
}

// TestCatalogMembershipLifecycle covers the RFC 9432 flow implemented in
// GoDDI: create a catalog zone, join member zones, verify the membership PTR
// records, list members, and leave the catalog on deletion.
func TestCatalogMembershipLifecycle(t *testing.T) {
	m := setupCatalogTest(t)

	catalog, err := m.CreateZone(ZoneOptions{Name: "catalog.example.com", Type: string(ZoneTypeCatalog)})
	if err != nil {
		t.Fatalf("create catalog zone: %v", err)
	}

	member, err := m.CreateZone(ZoneOptions{Name: "member.example.com", Type: string(ZoneTypePrimary), Catalog: "catalog.example.com"})
	if err != nil {
		t.Fatalf("create member zone: %v", err)
	}
	if member.Catalog != "catalog.example.com" {
		t.Fatalf("member zone catalog = %q, want catalog.example.com", member.Catalog)
	}

	members, err := m.ListCatalogMembers(catalog.ID)
	if err != nil {
		t.Fatalf("list catalog members: %v", err)
	}
	if len(members) != 1 || !strings.EqualFold(members[0], "member.example.com") {
		t.Fatalf("members = %v, want [member.example.com]", members)
	}

	// The membership record must be a PTR inside the catalog zone.
	owner := catalogMembershipOwner("catalog.example.com", "member.example.com")
	var cnt int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM dns_records WHERE zone_id = ? AND type = 'PTR' AND name = ?`, catalog.ID, owner).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("membership PTR count = %d, want 1 (owner %q)", cnt, owner)
	}

	// Leave the catalog via UpdateZone ("-" semantics).
	if _, err := m.UpdateZone(member.ID, ZoneOptions{Catalog: "-"}); err != nil {
		t.Fatalf("leave catalog: %v", err)
	}
	updated, err := m.GetZone(member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Catalog != "" {
		t.Fatalf("catalog after leave = %q, want empty", updated.Catalog)
	}
	members, err = m.ListCatalogMembers(catalog.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Fatalf("members after leave = %v, want empty", members)
	}

	// Deleting a member zone also removes any lingering membership record.
	if _, err := m.CreateZone(ZoneOptions{Name: "doomed.example.com", Type: string(ZoneTypePrimary), Catalog: "catalog.example.com"}); err != nil {
		t.Fatalf("create doomed member: %v", err)
	}
	doomed, err := m.GetZoneByName("doomed.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteZone(doomed.ID); err != nil {
		t.Fatalf("delete member zone: %v", err)
	}
	members, err = m.ListCatalogMembers(catalog.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Fatalf("members after delete = %v, want empty", members)
	}
}

// TestCatalogValidation checks the guard rails: non-catalog targets, special
// zones, and catalog type conversion must all be rejected.
func TestCatalogValidation(t *testing.T) {
	m := setupCatalogTest(t)

	if _, err := m.CreateZone(ZoneOptions{Name: "plain.example.com", Type: string(ZoneTypePrimary)}); err != nil {
		t.Fatal(err)
	}
	// Joining a non-existent or non-catalog zone must fail.
	if _, err := m.CreateZone(ZoneOptions{Name: "bad.example.com", Type: string(ZoneTypePrimary), Catalog: "plain.example.com"}); err == nil {
		t.Fatal("expected error joining a non-catalog zone")
	}

	if _, err := m.CreateZone(ZoneOptions{Name: "catalog2.example.com", Type: string(ZoneTypeCatalog)}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateZone(ZoneOptions{Name: "allowed.example.com", Type: string(ZoneTypeAllowed), Catalog: "catalog2.example.com"}); err == nil {
		t.Fatal("expected error: special zones cannot join a catalog")
	}

	// Catalog zones cannot be type-converted.
	cat, err := m.GetZoneByName("catalog2.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ConvertZoneType(cat.ID, string(ZoneTypePrimary)); err == nil {
		t.Fatal("expected error converting a catalog zone")
	}
}
