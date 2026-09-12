package transfer

import (
	"database/sql"
	"testing"
	"time"
)

// insertZone seeds one dns_zones row. Only the columns the health query reads
// are meaningful here; the others are what the schema requires.
func insertZone(t *testing.T, db *sql.DB, id, name, zoneType string, enabled bool, failures int, lastSync any) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO dns_zones (id, name, type, enabled, soa_mname, soa_rname, serial,
			last_sync_at, sync_failure_count)
		VALUES (?, ?, ?, ?, 'ns1.example.test.', 'hostmaster.example.test.', 1, ?, ?)`,
		id, name, zoneType, enabled, lastSync, failures); err != nil {
		t.Fatalf("seed zone %s: %v", name, err)
	}
}

// TestSecondaryZoneHealthReportsTheStatesAnAlertMustTellApart covers the three
// readings that matter: a zone refreshing normally, one that is failing, and
// one that has never transferred at all. The last is why the sample carries a
// presence flag -- a never-synced zone reported as a zero timestamp would be
// "last synced in 1970", which is the wrong alarm.
func TestSecondaryZoneHealthReportsTheStatesAnAlertMustTellApart(t *testing.T) {
	db := newSingleConnDB(t)

	insertZone(t, db, "z-ok", "ok.example.test", "secondary", true, 0, "2026-09-10 12:00:00")
	insertZone(t, db, "z-failing", "failing.example.test", "secondary", true, 7, "2026-09-01 03:04:05")
	insertZone(t, db, "z-never", "never.example.test", "secondary", true, 0, nil)
	// Neither of these is a secondary the sweeper touches.
	insertZone(t, db, "z-primary", "primary.example.test", "primary", true, 0, nil)
	insertZone(t, db, "z-off", "off.example.test", "secondary", false, 3, "2026-09-09 09:09:09")

	health, err := SecondaryZoneHealth(db)
	if err != nil {
		t.Fatalf("SecondaryZoneHealth: %v", err)
	}

	byName := make(map[string]ZoneRefreshHealth, len(health))
	for _, h := range health {
		byName[h.Zone] = h
	}
	if len(health) != 3 {
		t.Fatalf("reported %d zones (%v), want the three enabled secondaries", len(health), byName)
	}
	if _, ok := byName["primary.example.test"]; ok {
		t.Error("a primary zone was reported as a secondary")
	}
	if _, ok := byName["off.example.test"]; ok {
		t.Error("a disabled zone was reported; the sweeper does not refresh it")
	}

	ok := byName["ok.example.test"]
	if !ok.HasSynced {
		t.Error("a zone with a recorded transfer reports no sync")
	}
	if ok.Failures != 0 {
		t.Errorf("healthy zone failures = %d, want 0", ok.Failures)
	}
	if want := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC); !ok.LastSync.Equal(want) {
		t.Errorf("healthy zone last sync = %v, want %v", ok.LastSync, want)
	}

	failing := byName["failing.example.test"]
	if failing.Failures != 7 {
		t.Errorf("failing zone failures = %d, want 7", failing.Failures)
	}
	if !failing.HasSynced {
		t.Error("a zone that transferred once before failing reports no sync at all")
	}
	if want := time.Date(2026, 9, 1, 3, 4, 5, 0, time.UTC); !failing.LastSync.Equal(want) {
		t.Errorf("failing zone last sync = %v, want %v", failing.LastSync, want)
	}

	never := byName["never.example.test"]
	if never.HasSynced {
		t.Error("a zone that has never transferred reports a sync time")
	}
}

// TestAnUnreadableSyncTimestampDoesNotBecomeTheEpoch covers a row edited by
// hand. Falling back to the zero time would produce an alarm about a zone
// decades overdue when the actual problem is that one column holds nonsense.
func TestAnUnreadableSyncTimestampDoesNotBecomeTheEpoch(t *testing.T) {
	db := newSingleConnDB(t)
	insertZone(t, db, "z-bad", "bad.example.test", "secondary", true, 2, "last tuesday")

	health, err := SecondaryZoneHealth(db)
	if err != nil {
		t.Fatalf("SecondaryZoneHealth: %v", err)
	}
	if len(health) != 1 {
		t.Fatalf("got %d zones, want 1", len(health))
	}
	if health[0].HasSynced {
		t.Errorf("an unparsable timestamp was accepted as %v", health[0].LastSync)
	}
	if health[0].Failures != 2 {
		t.Errorf("failures = %d, want 2: an unreadable timestamp must not hide the count",
			health[0].Failures)
	}
}

// TestSecondaryZoneHealthClosesItsCursor is the hang guard. The production pool
// has a single connection, so a query issued while this cursor is open would
// block forever. Nothing here issues one -- the guarantee is that the function
// returns, which is only observable by calling it with the same pool sizing.
func TestSecondaryZoneHealthClosesItsCursor(t *testing.T) {
	db := newSingleConnDB(t)
	insertZone(t, db, "z1", "one.example.test", "secondary", true, 1, "2026-09-10 12:00:00")
	insertZone(t, db, "z2", "two.example.test", "secondary", true, 0, "2026-09-10 12:00:00")

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := SecondaryZoneHealth(db); err != nil {
			t.Errorf("SecondaryZoneHealth: %v", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("SecondaryZoneHealth did not return; it is holding the only connection")
	}
}
