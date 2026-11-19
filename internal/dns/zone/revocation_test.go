package zone

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dataplane"
	"github.com/miekg/dns"
)

// Disabling a zone revokes every authoritative answer it served. The API
// must not return success while the old snapshot is still answerable.
func TestDisableZoneStopsAnsweringImmediately(t *testing.T) {
	db, err := dataplane.Open(config.DataPlaneZone, filepath.Join(t.TempDir(), "zones.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	zs := &Store{zones: make(map[string]*zoneData), expired: make(map[string]time.Time), db: db.DB}
	defer zs.Close()
	zm := NewZoneManager(db.DB, zs)
	z, err := zm.CreateZone(ZoneOptions{Name: "revoke.test", Type: string(ZoneTypePrimary)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRecordManager(db.DB, zs, zm).CreateRecord(z.ID,
		RecordOptions{Name: "www", Type: "A", Value: "192.0.2.1"}); err != nil {
		t.Fatal(err)
	}
	zs.Load()
	if got := lookupA(t, zs, "www.revoke.test."); got != "192.0.2.1" {
		t.Fatalf("baseline = %s", got)
	}

	disabled := false
	if _, err := zm.UpdateZone(z.ID, ZoneOptions{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if _, _, found := zs.Lookup("www.revoke.test.", dns.TypeA); found {
		t.Fatal("disabled zone still answered immediately after update returned")
	}
}

func TestDeleteZoneStopsAnsweringImmediately(t *testing.T) {
	db, err := dataplane.Open(config.DataPlaneZone, filepath.Join(t.TempDir(), "zones.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	zs := &Store{zones: make(map[string]*zoneData), expired: make(map[string]time.Time), db: db.DB}
	defer zs.Close()
	zm := NewZoneManager(db.DB, zs)
	z, err := zm.CreateZone(ZoneOptions{Name: "revoke.test", Type: string(ZoneTypePrimary)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRecordManager(db.DB, zs, zm).CreateRecord(z.ID,
		RecordOptions{Name: "www", Type: "A", Value: "192.0.2.1"}); err != nil {
		t.Fatal(err)
	}
	zs.Load()
	if got := lookupA(t, zs, "www.revoke.test."); got != "192.0.2.1" {
		t.Fatalf("baseline = %s", got)
	}

	if err := zm.DeleteZone(z.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, found := zs.Lookup("www.revoke.test.", dns.TypeA); found {
		t.Fatal("deleted zone still answered immediately after delete returned")
	}
}
