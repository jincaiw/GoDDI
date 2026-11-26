package zone

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jincaiw/GoDDI/internal/config"
	"github.com/jincaiw/GoDDI/internal/dataplane"
	"github.com/miekg/dns"
)

func lookupA(t *testing.T, s *Store, name string) string {
	t.Helper()
	_, answers, found := s.Lookup(name, dns.TypeA)
	if !found || len(answers) == 0 {
		t.Fatalf("no A answer for %s", name)
	}
	a, ok := answers[0].(*dns.A)
	if !ok {
		t.Fatalf("answer type %T, want *dns.A", answers[0])
	}
	return a.A.String()
}

// Two concurrent reloads can finish out of order. The one that started first
// read older data; if it publishes after a newer reload, the DNS data plane
// silently rolls back. A generation guard must discard the stale result.
func TestReloadDoesNotPublishStaleSnapshot(t *testing.T) {
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
	rm := NewRecordManager(db.DB, s, zm)
	rec, err := rm.CreateRecord(z.ID, RecordOptions{Name: "www", Type: "A", Value: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	s.Load()
	if got := lookupA(t, s, "www.example.test."); got != "192.0.2.1" {
		t.Fatalf("baseline = %s", got)
	}

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var first atomic.Bool
	prev := zoneLoadAfterDBForTest
	defer func() { zoneLoadAfterDBForTest = prev }()
	zoneLoadAfterDBForTest = func() {
		if first.CompareAndSwap(false, true) {
			started <- struct{}{}
			<-release
		}
	}

	done := make(chan struct{})
	go func() { s.Load(); close(done) }()
	<-started

	if _, err := db.Exec(`UPDATE dns_records SET value = ? WHERE id = ?`, "192.0.2.2", rec.ID); err != nil {
		t.Fatal(err)
	}
	s.Load()
	close(release)
	<-done

	if got := lookupA(t, s, "www.example.test."); got != "192.0.2.2" {
		t.Fatalf("served %s after out-of-order reload; want the newer 192.0.2.2", got)
	}
}
