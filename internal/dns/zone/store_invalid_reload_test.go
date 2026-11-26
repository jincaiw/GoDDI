package zone

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jincaiw/GoDDI/internal/config"
	"github.com/jincaiw/GoDDI/internal/dataplane"
)

// A record that cannot be turned into an RR must never reach the query path.
// The last good snapshot stays authoritative until a valid one is loaded.
func TestReloadWithInvalidRecordKeepsLastGoodSnapshot(t *testing.T) {
	db, err := dataplane.Open(config.DataPlaneZone, filepath.Join(t.TempDir(), "zones.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &Store{zones: make(map[string]*zoneData), expired: make(map[string]time.Time), db: db.DB}
	defer s.Close()
	zm := NewZoneManager(db.DB, s)
	z, err := zm.CreateZone(ZoneOptions{Name: "example.test", Type: string(ZoneTypePrimary)})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := NewRecordManager(db.DB, s, zm).CreateRecord(z.ID,
		RecordOptions{Name: "www", Type: "A", Value: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	s.Load()
	if got := lookupA(t, s, "www.example.test."); got != "192.0.2.1" {
		t.Fatalf("baseline = %s", got)
	}

	// Corrupt the record directly so validation at the API layer is bypassed.
	if _, err := db.Exec(`UPDATE dns_records SET value = ? WHERE id = ?`, "not-an-ip", rec.ID); err != nil {
		t.Fatal(err)
	}
	s.Load()
	if got := lookupA(t, s, "www.example.test."); got != "192.0.2.1" {
		t.Fatalf("invalid reload served %s; want the last good 192.0.2.1", got)
	}

	// A valid replacement is picked up normally.
	if _, err := db.Exec(`UPDATE dns_records SET value = ? WHERE id = ?`, "192.0.2.3", rec.ID); err != nil {
		t.Fatal(err)
	}
	s.Load()
	if got := lookupA(t, s, "www.example.test."); got != "192.0.2.3" {
		t.Fatalf("valid reload served %s; want 192.0.2.3", got)
	}
}
