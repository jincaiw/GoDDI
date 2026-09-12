package scope

// The scope write path: validation, storage, and deletion.
//
// These were the uncovered half of this package. The selection half
// (FindScopeByIP, scopeContainsIP, ipInRange) already had cases because a wrong
// answer there is visible as a client that gets no address; the write half goes
// wrong more quietly, and one of the cases below is exactly that kind of quiet.
//
// The table in the middle of this file is the reason ValidateScopeOptions
// exists as a function: the same rules have to hold whether a scope is created
// through the API, updated, or published as part of a configuration revision. A
// second, slightly different copy of the rules is how a change passes validation
// at publish time and is then refused -- or accepted -- by storage.

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newScopeCrudDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	schema := `
	CREATE TABLE dhcp_scopes (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		interface TEXT,
		subnet TEXT NOT NULL,
		start_ip TEXT NOT NULL,
		end_ip TEXT NOT NULL,
		subnet_mask TEXT,
		router TEXT,
		dns_servers TEXT,
		ntp_servers TEXT,
		domain_name TEXT,
		lease_time INTEGER DEFAULT 86400,
		max_lease_time INTEGER,
		enabled BOOLEAN DEFAULT TRUE,
		ping_check_enabled BOOLEAN DEFAULT TRUE,
		dns_updates BOOLEAN DEFAULT FALSE,
		comment TEXT,
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE dhcp_leases (
		id TEXT PRIMARY KEY,
		scope_id TEXT NOT NULL,
		ip_address TEXT NOT NULL,
		mac_address TEXT,
		status TEXT NOT NULL,
		lease_end DATETIME NOT NULL
	);
	CREATE TABLE dhcp_options (id TEXT PRIMARY KEY, scope_id TEXT NOT NULL);
	CREATE TABLE dhcp_reservations (id TEXT PRIMARY KEY, scope_id TEXT NOT NULL);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestTheValidationGateRefusesWhatStorageWouldOtherwiseStore(t *testing.T) {
	neg := func(i int) *int { return &i }
	zero := 0

	for _, tc := range []struct {
		name    string
		opts    ScopeOptions
		refused bool
		why     string
	}{
		{
			name: "every field empty is an update with nothing to check",
			opts: ScopeOptions{},
			why:  "a partial update carries only the fields it changes",
		},
		{
			name: "an ordinary scope",
			opts: ScopeOptions{Name: "lan", Subnet: "192.0.2.0/24", StartIP: "192.0.2.100", EndIP: "192.0.2.110"},
		},
		{
			name: "a single-address range is allowed",
			opts: ScopeOptions{Name: "host", Subnet: "192.0.2.0/24", StartIP: "192.0.2.5", EndIP: "192.0.2.5"},
			why:  "a /32 has exactly one usable address and equality is how it is expressed",
		},
		{
			name:    "a range that runs backwards",
			opts:    ScopeOptions{Name: "bad", Subnet: "192.0.2.0/24", StartIP: "192.0.2.110", EndIP: "192.0.2.100"},
			refused: true,
		},
		{
			name:    "a start address that is not an address",
			opts:    ScopeOptions{Name: "bad", Subnet: "192.0.2.0/24", StartIP: "192.0.2.300", EndIP: "192.0.2.110"},
			refused: true,
		},
		{
			name:    "an end address that is not an address",
			opts:    ScopeOptions{Name: "bad", Subnet: "192.0.2.0/24", StartIP: "192.0.2.100", EndIP: "not-an-ip"},
			refused: true,
		},
		{
			name:    "a subnet that is not a CIDR",
			opts:    ScopeOptions{Name: "bad", Subnet: "192.0.2.0/33", StartIP: "192.0.2.100", EndIP: "192.0.2.110"},
			refused: true,
		},
		{
			name:    "a range that starts outside its subnet",
			opts:    ScopeOptions{Name: "bad", Subnet: "192.0.2.0/24", StartIP: "198.51.100.10", EndIP: "198.51.100.20"},
			refused: true,
		},
		{
			name:    "a range that ends outside its subnet",
			opts:    ScopeOptions{Name: "bad", Subnet: "192.0.2.0/24", StartIP: "192.0.2.100", EndIP: "192.0.3.10"},
			refused: true,
		},
		{
			name:    "a negative lease time",
			opts:    ScopeOptions{Name: "bad", Subnet: "192.0.2.0/24", StartIP: "192.0.2.100", EndIP: "192.0.2.110", LeaseTime: neg(-1)},
			refused: true,
		},
		{
			name:    "a negative maximum lease time",
			opts:    ScopeOptions{Name: "bad", Subnet: "192.0.2.0/24", StartIP: "192.0.2.100", EndIP: "192.0.2.110", MaxLeaseTime: neg(-1)},
			refused: true,
		},
		{
			name: "a lease time above the maximum",
			opts: ScopeOptions{Name: "bad", Subnet: "192.0.2.0/24", StartIP: "192.0.2.100", EndIP: "192.0.2.110",
				LeaseTime: neg(7200), MaxLeaseTime: neg(3600)},
			refused: true,
		},
		{
			name: "a lease time above a maximum of zero",
			opts: ScopeOptions{Name: "ok", Subnet: "192.0.2.0/24", StartIP: "192.0.2.100", EndIP: "192.0.2.110",
				LeaseTime: neg(7200), MaxLeaseTime: &zero},
			why: "zero is how \"no maximum\" is written, and every lease time is above no maximum",
		},
		{
			name: "no subnet, so nothing to check the range against",
			opts: ScopeOptions{Name: "ok", StartIP: "192.0.2.100", EndIP: "192.0.2.110"},
			why:  "the subnet check is skipped rather than guessed at",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateScopeOptions(tc.opts)
			if tc.refused && err == nil {
				t.Fatalf("accepted %+v, which storage must refuse", tc.opts)
			}
			if !tc.refused && err != nil {
				t.Fatalf("refused %+v: %v", tc.opts, err)
			}
		})
	}
}

func TestTheGateIsTheSameOneCreateGoesThrough(t *testing.T) {
	m := NewManager(newScopeCrudDB(t))

	// The rules above are only load-bearing if the write path uses them.
	if _, err := m.CreateScope(ScopeOptions{
		Name: "bad", Subnet: "192.0.2.0/24", StartIP: "192.0.2.110", EndIP: "192.0.2.100",
	}); err == nil {
		t.Fatal("CreateScope stored a range that runs backwards")
	}
}

func TestAScopeRoundTripsAndDefaultsAreExplicit(t *testing.T) {
	db := newScopeCrudDB(t)
	m := NewManager(db)
	str := func(s string) *string { return &s }

	created, err := m.CreateScope(ScopeOptions{
		Name: "lan", Subnet: "192.0.2.0/24", StartIP: "192.0.2.100", EndIP: "192.0.2.110",
		Comment: str("the office"),
	})
	if err != nil {
		t.Fatalf("CreateScope: %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreateScope returned a scope with no id")
	}

	read, err := m.GetScope(created.ID)
	if err != nil {
		t.Fatalf("GetScope: %v", err)
	}
	if read.Name != "lan" || read.Subnet != "192.0.2.0/24" ||
		read.StartIP != "192.0.2.100" || read.EndIP != "192.0.2.110" || read.Comment != "the office" {
		t.Errorf("a field was lost in the round trip: %+v", read)
	}
	// The defaults are decisions, so they are asserted rather than assumed: an
	// omitted lease time is a day, a new scope serves, and publishing names is
	// opt-in -- a default of true there would put every client's hostname into
	// DNS the moment a scope was created.
	if read.LeaseTime != 86400 {
		t.Errorf("lease time = %d, want 86400", read.LeaseTime)
	}
	if !read.Enabled {
		t.Error("a newly created scope is disabled")
	}
	if read.DNSUpdates {
		t.Error("a newly created scope publishes DNS names without being asked to")
	}

	if _, err := m.GetScope("no-such-scope"); err == nil {
		t.Error("GetScope invented a scope that does not exist")
	}

	// An absent comment is stored the same way an emptied one is. Asserted
	// against the column rather than the read-back, because the two spellings
	// ('' and NULL) both read as "" and only the column shows which one was
	// written -- the console sends "" for an empty description box, so a scope
	// that had been edited would otherwise differ from one that had not.
	bare, err := m.CreateScope(ScopeOptions{
		Name: "bare", Subnet: "192.0.2.0/24", StartIP: "192.0.2.120", EndIP: "192.0.2.130",
		Comment: str(""),
	})
	if err != nil {
		t.Fatalf("CreateScope: %v", err)
	}
	if raw := readComment(t, db, bare.ID); raw != nil {
		t.Errorf("an absent comment was stored as %q, want NULL", *raw)
	}
	if _, err := m.UpdateScope(created.ID, ScopeOptions{Comment: str("")}); err != nil {
		t.Fatalf("UpdateScope: %v", err)
	}
	if raw := readComment(t, db, created.ID); raw != nil {
		t.Errorf("a cleared comment was stored as %q, want NULL -- create and update must agree", *raw)
	}
}

// readComment returns the raw comment column, or nil when it is NULL. The
// distinction is invisible through Scope.Comment, which is a plain string.
func readComment(t *testing.T, db *sql.DB, id string) *string {
	t.Helper()
	var raw sql.NullString
	if err := db.QueryRow("SELECT comment FROM dhcp_scopes WHERE id = ?", id).Scan(&raw); err != nil {
		t.Fatalf("read comment: %v", err)
	}
	if !raw.Valid {
		return nil
	}
	return &raw.String
}

func TestListingCountsTheFilteredSetAndBoundsThePage(t *testing.T) {
	m := NewManager(newScopeCrudDB(t))
	for _, opts := range []ScopeOptions{
		{Name: "lan-a", Subnet: "192.0.2.0/24", StartIP: "192.0.2.100", EndIP: "192.0.2.110"},
		{Name: "lan-b", Subnet: "192.0.2.0/24", StartIP: "192.0.2.120", EndIP: "192.0.2.130"},
		{Name: "wifi", Subnet: "198.51.100.0/24", StartIP: "198.51.100.10", EndIP: "198.51.100.20"},
	} {
		if _, err := m.CreateScope(opts); err != nil {
			t.Fatalf("CreateScope(%s): %v", opts.Name, err)
		}
	}
	off := false

	for _, tc := range []struct {
		name   string
		filter ScopeFilter
		want   int
	}{
		{"everything", ScopeFilter{}, 3},
		{"by name", ScopeFilter{Name: "lan"}, 2},
		{"by subnet", ScopeFilter{Subnet: "198.51.100.0/24"}, 1},
		{"by state", ScopeFilter{Enabled: &off}, 0},
		{"by name and subnet together", ScopeFilter{Name: "wifi", Subnet: "198.51.100.0/24"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scopes, total, err := m.ListScopes(tc.filter)
			if err != nil {
				t.Fatalf("ListScopes: %v", err)
			}
			if int(total) != tc.want {
				t.Errorf("total = %d, want %d", total, tc.want)
			}
			if len(scopes) != tc.want {
				t.Errorf("returned %d rows, want %d", len(scopes), tc.want)
			}
		})
	}

	// A page size beyond the cap is clamped, so one request cannot ask for the
	// whole table.
	scopes, _, err := m.ListScopes(ScopeFilter{PageSize: 10000})
	if err != nil {
		t.Fatalf("ListScopes: %v", err)
	}
	if len(scopes) != 3 {
		t.Errorf("returned %d rows, want 3", len(scopes))
	}
	// Page 0 is page 1 rather than a negative offset.
	zero, _, err := m.ListScopes(ScopeFilter{Page: 0})
	if err != nil {
		t.Fatalf("ListScopes: %v", err)
	}
	if len(zero) != 3 {
		t.Errorf("page 0 returned %d rows, want 3", len(zero))
	}
}

func TestUpdatingKeepsWhatWasNotMentionedAndRechecksWhatItBecomes(t *testing.T) {
	m := NewManager(newScopeCrudDB(t))
	str := func(s string) *string { return &s }
	created, err := m.CreateScope(ScopeOptions{
		Name: "lan", Subnet: "192.0.2.0/24", StartIP: "192.0.2.100", EndIP: "192.0.2.110",
		Router: str("192.0.2.1"), Comment: str("the office"),
	})
	if err != nil {
		t.Fatalf("CreateScope: %v", err)
	}

	// A partial update: only the name is given. Everything else has to survive,
	// because the caller has no way to say "and keep the rest" differently.
	empty := ""
	updated, err := m.UpdateScope(created.ID, ScopeOptions{Name: "lan-renamed", Comment: &empty})
	if err != nil {
		t.Fatalf("UpdateScope: %v", err)
	}
	if updated.Name != "lan-renamed" {
		t.Errorf("name = %q, want %q", updated.Name, "lan-renamed")
	}
	if updated.Router != "192.0.2.1" || updated.StartIP != "192.0.2.100" || updated.Subnet != "192.0.2.0/24" {
		t.Errorf("a field the update did not mention was lost: %+v", updated)
	}
	// And an explicit empty string clears, which is the whole reason the
	// optional fields are pointers: with a plain string, "not provided" and
	// "cleared" would be the same message.
	if updated.Comment != "" {
		t.Errorf("comment = %q, want it cleared", updated.Comment)
	}

	// The result is validated, not just the request: widening the range out of
	// the subnet has to be refused.
	if _, err := m.UpdateScope(created.ID, ScopeOptions{EndIP: "198.51.100.20"}); err == nil {
		t.Error("UpdateScope accepted a range that leaves its subnet")
	}
	// And the refusal left the stored scope alone.
	after, err := m.GetScope(created.ID)
	if err != nil {
		t.Fatalf("GetScope: %v", err)
	}
	if after.EndIP != "192.0.2.110" {
		t.Errorf("end_ip = %q after a refused update, want 192.0.2.110", after.EndIP)
	}

	if _, err := m.UpdateScope("no-such-scope", ScopeOptions{Name: "x"}); err == nil {
		t.Error("UpdateScope updated a scope that does not exist")
	}
}

func TestDeletingRefusesWhileABindingIsStillLive(t *testing.T) {
	db := newScopeCrudDB(t)
	m := NewManager(db)
	created, err := m.CreateScope(ScopeOptions{
		Name: "lan", Subnet: "192.0.2.0/24", StartIP: "192.0.2.100", EndIP: "192.0.2.110",
	})
	if err != nil {
		t.Fatalf("CreateScope: %v", err)
	}
	exec(t, db, `INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, status, lease_end)
		VALUES ('l1', ?, '192.0.2.100', 'aa:bb:cc:dd:ee:01', 'active', ?)`,
		created.ID, futureLeaseEnd())

	err = m.DeleteScope(created.ID)
	if err == nil {
		t.Fatal("a scope with a live binding was deleted; the client would keep an address the server no longer knows about")
	}
	if !strings.Contains(err.Error(), "active lease") {
		t.Errorf("the refusal does not say why: %v", err)
	}
	if _, err := m.GetScope(created.ID); err != nil {
		t.Errorf("the scope is gone after a refused delete: %v", err)
	}

	// Only a binding blocks. An offer is a two-minute reservation that holds an
	// address out of the pool, but no client owns it, so it must not make the
	// scope undeletable -- otherwise an operator retries and succeeds, which is
	// a refusal that depends on the clock.
	exec(t, db, `UPDATE dhcp_leases SET status = 'offered' WHERE id = 'l1'`)
	if err := m.DeleteScope(created.ID); err != nil {
		t.Fatalf("an unconfirmed offer blocked deletion: %v", err)
	}
}

// TestDeletingIsNotBlockedByABindingThatAlreadyRanOut covers the seam between
// the two statements inside DeleteScope: it refuses while any lease is 'active',
// and then deletes the leases that have expired by time or been released.
//
// Those are two different predicates for the same question. A lease that is past
// its end has stopped being a binding -- that is what the sweep does, and what
// the cleanup below already assumes -- so it must not hold the scope hostage.
// It matters where no sweep runs: a control-only node has no DHCP data plane, so
// nothing ever moves those rows off 'active', and the scope becomes impossible
// to delete.
func TestDeletingIsNotBlockedByABindingThatAlreadyRanOut(t *testing.T) {
	db := newScopeCrudDB(t)
	m := NewManager(db)
	created, err := m.CreateScope(ScopeOptions{
		Name: "lan", Subnet: "192.0.2.0/24", StartIP: "192.0.2.100", EndIP: "192.0.2.110",
	})
	if err != nil {
		t.Fatalf("CreateScope: %v", err)
	}
	// Still marked active, but its end passed an hour ago: exactly the row the
	// cleanup below intends to remove.
	exec(t, db, `INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, status, lease_end)
		VALUES ('l1', ?, '192.0.2.100', 'aa:bb:cc:dd:ee:01', 'active', ?)`,
		created.ID, "2000-01-01T00:00:00Z")

	if err := m.DeleteScope(created.ID); err != nil {
		t.Fatalf("a scope whose only binding ran out in 2000 could not be deleted: %v", err)
	}
	if _, err := m.GetScope(created.ID); err == nil {
		t.Error("the scope is still there after DeleteScope returned success")
	}
}

func TestDeletingTakesTheAssociationsWithIt(t *testing.T) {
	db := newScopeCrudDB(t)
	m := NewManager(db)
	created, err := m.CreateScope(ScopeOptions{
		Name: "lan", Subnet: "192.0.2.0/24", StartIP: "192.0.2.100", EndIP: "192.0.2.110",
	})
	if err != nil {
		t.Fatalf("CreateScope: %v", err)
	}
	exec(t, db, `INSERT INTO dhcp_options (id, scope_id) VALUES ('o1', ?)`, created.ID)
	exec(t, db, `INSERT INTO dhcp_reservations (id, scope_id) VALUES ('r1', ?)`, created.ID)
	exec(t, db, `INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, status, lease_end)
		VALUES ('l-released', ?, '192.0.2.101', 'aa:bb:cc:dd:ee:02', 'released', ?)`,
		created.ID, futureLeaseEnd())
	// An offer that has not lapsed yet, and a quarantine whose hour ran out
	// while the node was down. Neither is a binding, and neither was in the old
	// cleanup's WHERE clause -- the quarantine because it is only matched once
	// its time has passed, the offer because its time has not. So these are the
	// rows that used to be left orphaned, pointing at a scope that no longer
	// existed.
	exec(t, db, `INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, status, lease_end)
		VALUES ('l-offer', ?, '192.0.2.102', 'aa:bb:cc:dd:ee:03', 'offered', ?)`,
		created.ID, futureLeaseEnd())
	exec(t, db, `INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, status, lease_end)
		VALUES ('l-quarantine', ?, '192.0.2.103', 'aa:bb:cc:dd:ee:04', 'conflict', '2000-01-01T00:00:00Z')`,
		created.ID)

	if err := m.DeleteScope(created.ID); err != nil {
		t.Fatalf("DeleteScope: %v", err)
	}
	// A lease that is not a binding goes with the scope. Leaving it would leave
	// rows pointing at a scope that no longer exists.
	for _, tc := range []struct{ table, id string }{
		{"dhcp_options", "o1"},
		{"dhcp_reservations", "r1"},
		{"dhcp_leases", "l-released"},
		{"dhcp_leases", "l-offer"},
		{"dhcp_leases", "l-quarantine"},
	} {
		if n := countWhere(t, db, tc.table, "id = ?", tc.id); n != 0 {
			t.Errorf("%s still holds %s", tc.table, tc.id)
		}
	}
	if err := m.DeleteScope(created.ID); err == nil {
		t.Error("deleting a scope that is already gone reported success")
	}
}

// ---- helpers ---------------------------------------------------------------

// futureLeaseEnd is an RFC3339 instant an hour from now, which is the format
// the sweep and the deletion cleanup both compare with julianday().
func futureLeaseEnd() string {
	return time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
}

func exec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec: %v", err)
	}
}

func countWhere(t *testing.T, db *sql.DB, table, condition string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+condition, args...).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}
