package transfer

import (
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
	_ "modernc.org/sqlite"
)

// newSingleConnDB mirrors production SQLite pool sizing: exactly one
// connection (see internal/database/database.go — SetMaxOpenConns(1)).
//
// That sizing is the whole point of these tests. A nested query issued while a
// cursor is still open cannot be served by opening a second connection, so the
// call blocks forever rather than returning an error — which means correctness
// here is only observable by asserting the call returns at all.
func newSingleConnDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	schema := `
	CREATE TABLE dns_zones (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL,
		enabled BOOLEAN DEFAULT TRUE,
		soa_mname TEXT NOT NULL,
		soa_rname TEXT NOT NULL,
		serial INTEGER NOT NULL,
		refresh INTEGER DEFAULT 3600,
		retry INTEGER DEFAULT 600,
		expire INTEGER DEFAULT 86400,
		minimum INTEGER DEFAULT 300,
		default_ttl INTEGER DEFAULT 3600,
		transfer_policy TEXT,
		updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
		last_sync_at DATETIME,
		last_sync_attempt_at DATETIME,
		sync_failure_count INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE dns_zone_changes (
		id TEXT PRIMARY KEY,
		zone_id TEXT NOT NULL,
		serial INTEGER NOT NULL,
		change_type TEXT NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		value TEXT NOT NULL,
		ttl INTEGER DEFAULT 300,
		priority INTEGER,
		weight INTEGER,
		port INTEGER,
		created_at DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE dns_zone_transfer (
		id TEXT PRIMARY KEY,
		zone_id TEXT NOT NULL,
		allowed_cidr TEXT NOT NULL,
		tsig_key_name TEXT,
		created_at DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// mustComplete runs fn on a goroutine and fails the test if it has not
// returned within the deadline. A call blocked on the single database
// connection never returns on its own, so a timeout is the only symptom the
// bug produces.
func mustComplete(t *testing.T, deadline time.Duration, what string, fn func()) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	select {
	case <-done:
	case <-time.After(deadline):
		t.Fatalf("%s did not return within %s — blocked waiting for the single DB connection", what, deadline)
	}
}

// Regression: loadChanges read the zone name with a nested QueryRow while the
// dns_zone_changes cursor was still open. With one connection the nested query
// waits for the connection the cursor itself holds, so the IXFR request hangs
// forever instead of failing.
func TestHandleIXFR_NoDeadlockWithSingleConnection(t *testing.T) {
	db := newSingleConnDB(t)

	mustExec(t, db, `INSERT INTO dns_zones
		(id, name, type, enabled, soa_mname, soa_rname, serial, expire, default_ttl)
		VALUES ('z1', 'example.com.', 'primary', 1,
		        'ns1.example.com.', 'hostmaster.example.com.', 5, 86400, 3600)`)
	mustExec(t, db, `INSERT INTO dns_zone_transfer (id, zone_id, allowed_cidr, tsig_key_name)
		VALUES ('t1', 'z1', '0.0.0.0/0', 'xfer-key')`)
	mustExec(t, db, `INSERT INTO dns_zone_changes
		(id, zone_id, serial, change_type, name, type, value, ttl)
		VALUES ('c1', 'z1', 4, 'add', 'www', 'A', '192.0.2.10', 300)`)

	h := NewAXFRHandler(db)

	var (
		rrs []dns.RR
		err error
	)
	mustComplete(t, 10*time.Second, "HandleIXFR", func() {
		rrs, err = h.HandleIXFR("example.com.", 3, "xfer-key")
	})

	if err != nil {
		t.Fatalf("HandleIXFR returned error: %v", err)
	}
	if len(rrs) == 0 {
		t.Fatal("expected an IXFR response, got none")
	}
}

// fakeExpirySink records withhold/resume decisions without a real zone store.
type fakeExpirySink struct {
	mu     sync.Mutex
	marked map[string]bool
}

func (f *fakeExpirySink) MarkZoneExpired(zoneName string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.marked == nil {
		f.marked = make(map[string]bool)
	}
	f.marked[zoneName] = true
}

func (f *fakeExpirySink) ClearZoneExpired(zoneName string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.marked, zoneName)
}

func (f *fakeExpirySink) isExpired(zoneName string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.marked[zoneName]
}

// Regression: the sweep performed per-zone lookups (the SOA refresh fallback
// and the sync clock read) inside the zone cursor loop, so it hung as soon as
// a single secondary zone existed. It is also the only path that enforces
// SOA EXPIRE, so the hang meant expiry was never enforced at all.
func TestSyncAllSecondaryZones_NoDeadlockAndWithholdsExpiredZone(t *testing.T) {
	db := newSingleConnDB(t)

	// refresh = 1h, expire = 60s, last successful sync = 2h ago: the zone is
	// well past EXPIRE and must be withheld even though a refresh is also due.
	mustExec(t, db, `INSERT INTO dns_zones
		(id, name, type, enabled, soa_mname, soa_rname, serial, refresh, expire, last_sync_at)
		VALUES ('z2', 'sec.example.com.', 'secondary', 1,
		        'ns1.example.com.', 'hostmaster.example.com.', 9, 3600, 60,
		        datetime('now', '-2 hours'))`)

	sink := &fakeExpirySink{}
	s := NewSecondarySync(db)
	s.SetZoneStore(sink)

	mustComplete(t, 10*time.Second, "syncAllSecondaryZones", func() {
		s.syncAllSecondaryZones()
	})

	if !sink.isExpired("sec.example.com.") {
		t.Fatal("zone past its SOA EXPIRE was not withheld from authoritative answers")
	}
}

// A secondary zone that has never completed a transfer has no data whose
// freshness can be vouched for, so it must be withheld from the start rather
// than answering from an unverified local copy.
func TestSyncAllSecondaryZones_WithholdsNeverSyncedZone(t *testing.T) {
	db := newSingleConnDB(t)

	mustExec(t, db, `INSERT INTO dns_zones
		(id, name, type, enabled, soa_mname, soa_rname, serial, refresh, expire, last_sync_at)
		VALUES ('z3', 'fresh.example.com.', 'secondary', 1,
		        'ns1.example.com.', 'hostmaster.example.com.', 1, 3600, 86400, NULL)`)

	sink := &fakeExpirySink{}
	s := NewSecondarySync(db)
	s.SetZoneStore(sink)

	mustComplete(t, 10*time.Second, "syncAllSecondaryZones", func() {
		s.syncAllSecondaryZones()
	})

	if !sink.isExpired("fresh.example.com.") {
		t.Fatal("never-synced secondary zone was allowed to answer authoritatively")
	}

	// The failing refresh must be recorded so operators can see why.
	var failures int
	if err := db.QueryRow(
		`SELECT sync_failure_count FROM dns_zones WHERE id = 'z3'`,
	).Scan(&failures); err != nil {
		t.Fatalf("read failure count: %v", err)
	}
	if failures == 0 {
		t.Error("expected the failed refresh attempt to be recorded")
	}
}

// A zone with no EXPIRE configured must not be withheld: expire = 0 disables
// the check rather than expiring immediately.
func TestSyncAllSecondaryZones_ZeroExpireDoesNotWithhold(t *testing.T) {
	db := newSingleConnDB(t)

	mustExec(t, db, `INSERT INTO dns_zones
		(id, name, type, enabled, soa_mname, soa_rname, serial, refresh, expire, last_sync_at)
		VALUES ('z4', 'noexpire.example.com.', 'secondary', 1,
		        'ns1.example.com.', 'hostmaster.example.com.', 2, 86400, 0,
		        datetime('now', '-30 days'))`)

	sink := &fakeExpirySink{}
	s := NewSecondarySync(db)
	s.SetZoneStore(sink)

	mustComplete(t, 10*time.Second, "syncAllSecondaryZones", func() {
		s.syncAllSecondaryZones()
	})

	if sink.isExpired("noexpire.example.com.") {
		t.Error("zone with expire = 0 was withheld; expire = 0 means no expiry")
	}
}

func TestParseSQLiteTime(t *testing.T) {
	cases := []struct {
		name string
		in   sql.NullString
		want string // empty means "not parseable"
	}{
		{"null", sql.NullString{}, ""},
		{"empty", sql.NullString{String: "", Valid: true}, ""},
		{"blank spaces", sql.NullString{String: "   ", Valid: true}, ""},
		{"sqlite datetime", sql.NullString{String: "2026-09-10 03:20:00", Valid: true},
			"2026-09-10T03:20:00Z"},
		{"rfc3339", sql.NullString{String: "2026-09-10T03:20:00Z", Valid: true},
			"2026-09-10T03:20:00Z"},
		{"rfc3339 offset", sql.NullString{String: "2026-09-10T11:20:00+08:00", Valid: true},
			"2026-09-10T03:20:00Z"},
		{"fractional", sql.NullString{String: "2026-09-10 03:20:00.500", Valid: true},
			"2026-09-10T03:20:00.5Z"},
		{"garbage", sql.NullString{String: "not a timestamp", Valid: true}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseSQLiteTime(tc.in)
			if tc.want == "" {
				if ok {
					t.Fatalf("expected parse failure, got %v", got)
				}
				return
			}
			if !ok {
				t.Fatalf("expected %s, got parse failure", tc.want)
			}
			if got.Format(time.RFC3339Nano) != tc.want {
				t.Errorf("got %s, want %s", got.Format(time.RFC3339Nano), tc.want)
			}
			// Timestamps must be compared as instants, so they are normalized
			// to UTC regardless of the host's local zone.
			if got.Location() != time.UTC {
				t.Errorf("location = %v, want UTC", got.Location())
			}
		})
	}
}
