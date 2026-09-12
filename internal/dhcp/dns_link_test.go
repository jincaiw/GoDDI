package dhcp

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jasonwa/goddi"
)

// newLinkageDB builds a database with the real migration schema.
//
// Using the migrations rather than a hand-written subset is deliberate: the
// ownership columns and the event table were added by migration 018, and a test
// that invented its own schema would keep passing after the migration changed.
func newLinkageDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Production sets the same limit, and it is what makes an unclosed cursor
	// fatal rather than merely wasteful: a second query waits on the one
	// connection the cursor is holding, with no error and no timeout.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	goose.SetBaseFS(goddiassets.Migrations())
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}

// recordingReloader counts how often the zone store was asked to republish.
type recordingReloader struct {
	mu    sync.Mutex
	calls int
}

func (r *recordingReloader) ReloadNow() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
}

func (r *recordingReloader) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func seedZone(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO dns_zones (id, name, type, enabled, soa_mname, soa_rname, serial,
			refresh, retry, expire, minimum)
		VALUES (?, ?, 'primary', 1, 'ns1.example.', 'hostmaster.example.', 1, 3600, 600, 86400, 300)`,
		id, name); err != nil {
		t.Fatalf("seed zone %s: %v", name, err)
	}
}

func seedScope(t *testing.T, db *sql.DB, id string, dnsUpdates bool) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO dhcp_scopes (id, name, interface, subnet, start_ip, end_ip, subnet_mask,
			router, dns_servers, domain_name, lease_time, enabled, ping_check_enabled, dns_updates)
		VALUES (?, 'lan', 'eth0', '192.0.2.0/24', '192.0.2.100', '192.0.2.200',
			'255.255.255.0', '192.0.2.1', '192.0.2.1', 'example.test', 3600, 1, 0, ?)`,
		id, dnsUpdates); err != nil {
		t.Fatalf("seed scope %s: %v", id, err)
	}
}

func seedLease(t *testing.T, db *sql.DB, id, scopeID, ip, mac, hostname string,
	status lease.LeaseStatus, generation int64, leaseEnd time.Time) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen, generation)
		VALUES (?, ?, ?, ?, ?, '', datetime('now'), ?, ?, datetime('now'), ?)`,
		id, scopeID, ip, mac, hostname,
		leaseEnd.UTC().Format("2006-01-02T15:04:05Z"), string(status), generation); err != nil {
		t.Fatalf("seed lease %s: %v", id, err)
	}
}

type recordRow struct {
	ID         string
	Name       string
	Type       string
	Value      string
	Owner      sql.NullString
	OwnerRef   sql.NullString
	Generation sql.NullInt64
	ExpiresAt  sql.NullString
}

func recordsFor(t *testing.T, db *sql.DB, zoneID, name, recordType string) []recordRow {
	t.Helper()
	rows, err := db.Query(`
		-- CAST keeps the column from being handed back as a Go time.Time,
		-- which database/sql would re-render as RFC3339 and hide the stored text.
		SELECT id, name, type, value, owner, owner_ref, owner_generation, CAST(expires_at AS TEXT)
		FROM dns_records WHERE zone_id = ? AND name = ? AND type = ?`,
		zoneID, name, recordType)
	if err != nil {
		t.Fatalf("query records: %v", err)
	}
	defer rows.Close()

	var out []recordRow
	for rows.Next() {
		var r recordRow
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Value, &r.Owner,
			&r.OwnerRef, &r.Generation, &r.ExpiresAt); err != nil {
			t.Fatalf("scan record: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate records: %v", err)
	}
	return out
}

func countAllRecords(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM dns_records").Scan(&n); err != nil {
		t.Fatalf("count records: %v", err)
	}
	return n
}

// seedLinkage creates the forward zone, the matching reverse zone and a
// DNS-updating scope. It returns the two zone ids.
func seedLinkage(t *testing.T, db *sql.DB) (forwardZone, reverseZone string) {
	t.Helper()
	seedZone(t, db, "zone-forward", "example.test.")
	seedZone(t, db, "zone-reverse", "2.0.192.in-addr.arpa.")
	seedScope(t, db, "scope-1", true)
	return "zone-forward", "zone-reverse"
}

const testLeaseEnd = "2035-01-01T00:00:00Z"

func leaseEndTime(t *testing.T) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, testLeaseEnd)
	if err != nil {
		t.Fatalf("parse lease end: %v", err)
	}
	return ts
}

// ---------------------------------------------------------------------------
// create
// ---------------------------------------------------------------------------

func TestApplyCreate_PublishesForwardAndReverse(t *testing.T) {
	db := newLinkageDB(t)
	seedLinkage(t, db)
	seedLease(t, db, "lease-1", "scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:01",
		"host1", lease.LeaseStatusActive, 1, leaseEndTime(t))

	reloader := &recordingReloader{}
	link := NewDNSLink(Same(db), reloader)

	if err := link.ApplyEvent(DNSEvent{
		LeaseID: "lease-1", Generation: 1, Action: DNSEventCreate,
		ScopeID: "scope-1", IPAddress: "192.0.2.10",
		MACAddress: "aa:bb:cc:dd:ee:01", Hostname: "host1",
	}); err != nil {
		t.Fatalf("ApplyEvent: %v", err)
	}

	fwd := recordsFor(t, db, "zone-forward", "host1.example.test.", "A")
	if len(fwd) != 1 {
		t.Fatalf("forward records = %d, want 1", len(fwd))
	}
	if fwd[0].Value != "192.0.2.10" {
		t.Errorf("A value = %q, want 192.0.2.10", fwd[0].Value)
	}
	if fwd[0].OwnerRef.String != "lease-1" || fwd[0].Generation.Int64 != 1 {
		t.Errorf("ownership = (%q, %d), want (lease-1, 1)",
			fwd[0].OwnerRef.String, fwd[0].Generation.Int64)
	}
	if fwd[0].Owner.String != "dhcp" {
		t.Errorf("owner = %q, want dhcp", fwd[0].Owner.String)
	}
	// expires_at is the age-out backstop for the record, so it has to be
	// comparable with SQLite's own datetimes. A stored RFC3339 value keeps its
	// "T", which sorts greater than every datetime('now') output, and the
	// record would then never age out.
	if got := fwd[0].ExpiresAt.String; got != "2035-01-01 00:00:00" {
		t.Errorf("expires_at = %q, want 2035-01-01 00:00:00", got)
	}
	var stillValid int
	if err := db.QueryRow(
		`SELECT CASE WHEN expires_at > datetime('now') THEN 1 ELSE 0 END
		 FROM dns_records WHERE id = ?`, fwd[0].ID).Scan(&stillValid); err != nil {
		t.Fatalf("compare expires_at: %v", err)
	}
	if stillValid != 1 {
		t.Error("expires_at does not compare as a future SQLite datetime")
	}

	ptr := recordsFor(t, db, "zone-reverse", "10.2.0.192.in-addr.arpa.", "PTR")
	if len(ptr) != 1 {
		t.Fatalf("reverse records = %d, want 1", len(ptr))
	}
	if ptr[0].Value != "host1.example.test." {
		t.Errorf("PTR value = %q, want host1.example.test.", ptr[0].Value)
	}

	// The record is written to the database, but queries are answered from the
	// in-memory store: without this reload the name stays unresolvable and the
	// whole linkage is decorative.
	if reloader.count() != 1 {
		t.Errorf("reload calls = %d, want 1", reloader.count())
	}
}

func TestApplyCreate_IsIdempotent(t *testing.T) {
	db := newLinkageDB(t)
	seedLinkage(t, db)
	seedLease(t, db, "lease-1", "scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:01",
		"host1", lease.LeaseStatusActive, 1, leaseEndTime(t))

	link := NewDNSLink(Same(db), nil)
	event := DNSEvent{
		LeaseID: "lease-1", Generation: 1, Action: DNSEventCreate,
		ScopeID: "scope-1", IPAddress: "192.0.2.10", Hostname: "host1",
	}
	for i := 0; i < 3; i++ {
		if err := link.ApplyEvent(event); err != nil {
			t.Fatalf("ApplyEvent #%d: %v", i+1, err)
		}
	}

	// Replay is inherent in a durable queue: the same event is delivered again
	// whenever the consumer could not confirm it, so it must converge rather
	// than accumulate a record per attempt.
	if n := countAllRecords(t, db); n != 2 {
		t.Errorf("total records = %d, want 2 (one A, one PTR)", n)
	}
}

func TestApplyCreate_DropsSupersededGeneration(t *testing.T) {
	db := newLinkageDB(t)
	seedLinkage(t, db)
	// The lease has already been renewed to generation 2.
	seedLease(t, db, "lease-1", "scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:01",
		"host1", lease.LeaseStatusActive, 2, leaseEndTime(t))

	link := NewDNSLink(Same(db), nil)
	if err := link.ApplyEvent(DNSEvent{
		LeaseID: "lease-1", Generation: 1, Action: DNSEventCreate,
		IPAddress: "192.0.2.10", Hostname: "host1",
	}); err != nil {
		t.Fatalf("ApplyEvent: %v", err)
	}

	if n := countAllRecords(t, db); n != 0 {
		t.Fatalf("records = %d, want 0: a create from a superseded generation must not publish", n)
	}
}

func TestApplyCreate_SkipsUnconfirmedBinding(t *testing.T) {
	db := newLinkageDB(t)
	seedLinkage(t, db)
	seedLease(t, db, "lease-1", "scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:01",
		"host1", lease.LeaseStatusReleased, 1, leaseEndTime(t))

	link := NewDNSLink(Same(db), nil)
	if err := link.ApplyEvent(DNSEvent{
		LeaseID: "lease-1", Generation: 1, Action: DNSEventCreate,
		IPAddress: "192.0.2.10", Hostname: "host1",
	}); err != nil {
		t.Fatalf("ApplyEvent: %v", err)
	}

	if n := countAllRecords(t, db); n != 0 {
		t.Fatalf("records = %d, want 0: a released binding must not be published", n)
	}
}

func TestApplyCreate_TakesOverTheNameFromAPreviousLease(t *testing.T) {
	db := newLinkageDB(t)
	seedLinkage(t, db)
	seedLease(t, db, "lease-old", "scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:01",
		"host1", lease.LeaseStatusExpired, 1, leaseEndTime(t))
	seedLease(t, db, "lease-new", "scope-1", "192.0.2.20", "aa:bb:cc:dd:ee:02",
		"host1", lease.LeaseStatusActive, 1, leaseEndTime(t))

	link := NewDNSLink(Same(db), nil)
	if err := link.ApplyEvent(DNSEvent{
		LeaseID: "lease-old", Generation: 1, Action: DNSEventCreate,
		IPAddress: "192.0.2.10", Hostname: "host1",
	}); err != nil {
		t.Fatalf("publish old: %v", err)
	}
	if err := link.ApplyEvent(DNSEvent{
		LeaseID: "lease-new", Generation: 1, Action: DNSEventCreate,
		IPAddress: "192.0.2.20", Hostname: "host1",
	}); err != nil {
		t.Fatalf("publish new: %v", err)
	}

	// One name, one answer: a host that gets a new address must not keep
	// answering with the address it no longer holds.
	fwd := recordsFor(t, db, "zone-forward", "host1.example.test.", "A")
	if len(fwd) != 1 {
		t.Fatalf("forward records = %d, want 1", len(fwd))
	}
	if fwd[0].Value != "192.0.2.20" || fwd[0].OwnerRef.String != "lease-new" {
		t.Errorf("record = (%s, %s), want (192.0.2.20, lease-new)",
			fwd[0].Value, fwd[0].OwnerRef.String)
	}
}

// ---------------------------------------------------------------------------
// delete
// ---------------------------------------------------------------------------

func TestApplyDelete_OnlyRemovesItsOwnRecord(t *testing.T) {
	db := newLinkageDB(t)
	seedLinkage(t, db)
	seedLease(t, db, "lease-old", "scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:01",
		"host1", lease.LeaseStatusExpired, 1, leaseEndTime(t))
	seedLease(t, db, "lease-new", "scope-1", "192.0.2.20", "aa:bb:cc:dd:ee:02",
		"host1", lease.LeaseStatusActive, 1, leaseEndTime(t))

	link := NewDNSLink(Same(db), nil)
	for _, e := range []DNSEvent{
		{LeaseID: "lease-old", Generation: 1, Action: DNSEventCreate,
			IPAddress: "192.0.2.10", Hostname: "host1"},
		{LeaseID: "lease-new", Generation: 1, Action: DNSEventCreate,
			IPAddress: "192.0.2.20", Hostname: "host1"},
	} {
		if err := link.ApplyEvent(e); err != nil {
			t.Fatalf("publish %s: %v", e.LeaseID, err)
		}
	}

	// The old binding is torn down after the new one was published. Matching
	// on owner='dhcp' alone used to delete the live client's record here,
	// leaving a running host unresolvable.
	if err := link.ApplyEvent(DNSEvent{
		LeaseID: "lease-old", Generation: 1, Action: DNSEventDelete,
		IPAddress: "192.0.2.10", Hostname: "host1",
	}); err != nil {
		t.Fatalf("teardown old: %v", err)
	}

	fwd := recordsFor(t, db, "zone-forward", "host1.example.test.", "A")
	if len(fwd) != 1 {
		t.Fatalf("forward records = %d, want 1 (the live binding's)", len(fwd))
	}
	if fwd[0].OwnerRef.String != "lease-new" {
		t.Errorf("surviving record owner = %q, want lease-new", fwd[0].OwnerRef.String)
	}

	// The old lease's reverse record is its own and must go.
	if n := len(recordsFor(t, db, "zone-reverse", "10.2.0.192.in-addr.arpa.", "PTR")); n != 0 {
		t.Errorf("stale reverse records = %d, want 0", n)
	}
	if n := len(recordsFor(t, db, "zone-reverse", "20.2.0.192.in-addr.arpa.", "PTR")); n != 1 {
		t.Errorf("live reverse records = %d, want 1", n)
	}
}

func TestApplyDelete_StaleGenerationLeavesTheRenewedRecord(t *testing.T) {
	db := newLinkageDB(t)
	seedLinkage(t, db)

	scopes := lease.NewManager(db)
	created, err := scopes.CreateLease("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:01",
		"host1", time.Hour)
	if err != nil {
		t.Fatalf("CreateLease: %v", err)
	}

	link := NewDNSLink(Same(db), nil)
	publish := func(l *lease.Lease) {
		t.Helper()
		if err := link.ApplyEvent(DNSEvent{
			LeaseID: l.ID, Generation: l.Generation, Action: DNSEventCreate,
			IPAddress: l.IPAddress, Hostname: l.Hostname,
		}); err != nil {
			t.Fatalf("publish at generation %d: %v", l.Generation, err)
		}
	}
	publish(created)

	// A renewal lands while a teardown for the previous generation is queued.
	renewed, err := scopes.RenewLease(created.ID, time.Hour)
	if err != nil {
		t.Fatalf("RenewLease: %v", err)
	}
	if renewed.Generation <= created.Generation {
		t.Fatalf("renewal did not advance the generation: %d -> %d",
			created.Generation, renewed.Generation)
	}
	publish(renewed)

	staleDelete := DNSEvent{
		LeaseID: created.ID, Generation: created.Generation,
		Action: DNSEventDelete, IPAddress: "192.0.2.10", Hostname: "host1",
	}
	if err := link.ApplyEvent(staleDelete); err != nil {
		t.Fatalf("stale teardown: %v", err)
	}
	if n := len(recordsFor(t, db, "zone-forward", "host1.example.test.", "A")); n != 1 {
		t.Fatalf("records after stale teardown = %d, want 1: a renewal must survive", n)
	}

	// The teardown at the current generation does remove it.
	if err := link.ApplyEvent(DNSEvent{
		LeaseID: renewed.ID, Generation: renewed.Generation,
		Action: DNSEventDelete, IPAddress: "192.0.2.10", Hostname: "host1",
	}); err != nil {
		t.Fatalf("current teardown: %v", err)
	}
	if n := len(recordsFor(t, db, "zone-forward", "host1.example.test.", "A")); n != 0 {
		t.Errorf("records after current teardown = %d, want 0", n)
	}
}

func TestApplyDelete_LeavesManuallyManagedRecords(t *testing.T) {
	db := newLinkageDB(t)
	seedLinkage(t, db)
	seedLease(t, db, "lease-1", "scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:01",
		"host1", lease.LeaseStatusExpired, 1, leaseEndTime(t))

	// A record an operator created by hand (owner NULL) and one the API wrote
	// with a non-DHCP owner.
	if _, err := db.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, enabled, owner)
		VALUES ('manual-1', 'zone-forward', 'host1.example.test.', 'A', '192.0.2.99', 300, 1, NULL),
		       ('manual-2', 'zone-forward', 'host1.example.test.', 'A', '192.0.2.98', 300, 1, 'api')`); err != nil {
		t.Fatalf("seed manual records: %v", err)
	}

	link := NewDNSLink(Same(db), nil)
	if err := link.ApplyEvent(DNSEvent{
		LeaseID: "lease-1", Generation: 1, Action: DNSEventDelete,
		IPAddress: "192.0.2.10", Hostname: "host1",
	}); err != nil {
		t.Fatalf("ApplyEvent: %v", err)
	}

	if n := len(recordsFor(t, db, "zone-forward", "host1.example.test.", "A")); n != 2 {
		t.Fatalf("records = %d, want 2: manual records must not be touched", n)
	}
}

func TestApplyDelete_RemovesRecordsWrittenBeforeOwnershipExisted(t *testing.T) {
	db := newLinkageDB(t)
	seedLinkage(t, db)

	// A row from before migration 018: owner='dhcp' but no owner_ref, so it can
	// only be recognised by the address it points at.
	if _, err := db.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, enabled, owner)
		VALUES ('legacy-1', 'zone-forward', 'host1.example.test.', 'A', '192.0.2.10', 300, 1, 'dhcp'),
		       ('legacy-other', 'zone-forward', 'host1.example.test.', 'A', '192.0.2.77', 300, 1, 'dhcp')`); err != nil {
		t.Fatalf("seed legacy records: %v", err)
	}

	link := NewDNSLink(Same(db), nil)
	if err := link.ApplyEvent(DNSEvent{
		LeaseID: "lease-1", Generation: 1, Action: DNSEventDelete,
		IPAddress: "192.0.2.10", Hostname: "host1",
	}); err != nil {
		t.Fatalf("ApplyEvent: %v", err)
	}

	rows := recordsFor(t, db, "zone-forward", "host1.example.test.", "A")
	if len(rows) != 1 {
		t.Fatalf("records = %d, want 1", len(rows))
	}
	// Only the row whose value matches the torn-down binding may go.
	if rows[0].Value != "192.0.2.77" {
		t.Errorf("surviving value = %q, want 192.0.2.77", rows[0].Value)
	}
}

// ---------------------------------------------------------------------------
// reconcile
// ---------------------------------------------------------------------------

func TestReconcile_WithdrawsOrphansAndRepublishesMissing(t *testing.T) {
	db := newLinkageDB(t)
	seedLinkage(t, db)

	// A confirmed binding whose record was never published: the crash window
	// between the lease commit and the event insert leaves exactly this.
	seedLease(t, db, "lease-missing", "scope-1", "192.0.2.30", "aa:bb:cc:dd:ee:03",
		"host-missing", lease.LeaseStatusActive, 1, leaseEndTime(t))

	// A record whose owner no longer holds the address (the lease was released
	// long ago). The lease row is still there and says so: the sweep acts on
	// that, not on the row being absent. See the note on orphanedRecords.
	seedLease(t, db, "lease-gone", "scope-1", "192.0.2.40", "aa:bb:cc:dd:ee:04",
		"host-orphan", lease.LeaseStatusReleased, 1, leaseEndTime(t))
	if _, err := db.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, enabled, owner, owner_ref, owner_generation)
		VALUES ('orphan-1', 'zone-forward', 'host-orphan.example.test.', 'A', '192.0.2.40', 300, 1, 'dhcp', 'lease-gone', 1)`); err != nil {
		t.Fatalf("seed orphan record: %v", err)
	}

	reloader := &recordingReloader{}
	link := NewDNSLink(Same(db), reloader)

	repaired, err := link.Reconcile(50)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if repaired != 2 {
		t.Errorf("repaired = %d, want 2 (one withdrawn, one published)", repaired)
	}

	if n := len(recordsFor(t, db, "zone-forward", "host-orphan.example.test.", "A")); n != 0 {
		t.Errorf("orphaned records = %d, want 0", n)
	}
	published := recordsFor(t, db, "zone-forward", "host-missing.example.test.", "A")
	if len(published) != 1 || published[0].Value != "192.0.2.30" {
		t.Fatalf("republished record = %+v, want host-missing -> 192.0.2.30", published)
	}
	if reloader.count() == 0 {
		t.Error("reconcile did not republish the zone store")
	}
}

// TestReconcile_LeavesARecordWhoseLeaseIsNotInTheReplica pins the rule that
// makes the sweep safe under replication.
//
// The lease table the sweep reads is a replica. A record whose lease row has not
// arrived yet is indistinguishable, from here, from one whose lease row was
// deleted -- and the two call for opposite actions. Treating the absence as
// death would take every DHCP-published name out of the zone each time
// replication lagged, which is worse than leaving a record that expires_at
// already bounds.
func TestReconcile_LeavesARecordWhoseLeaseIsNotInTheReplica(t *testing.T) {
	db := newLinkageDB(t)
	seedLinkage(t, db)

	// A record with a live binding behind it, in a store that has not been told
	// about the binding yet.
	if _, err := db.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, enabled, owner,
		                         owner_ref, owner_generation, expires_at, authored_locally)
		VALUES ('unreplicated-1', 'zone-forward', 'host-late.example.test.', 'A',
		        '192.0.2.41', 300, 1, 'dhcp', 'lease-not-here-yet', 1,
		        datetime('now', '+1 hour'), 1)`); err != nil {
		t.Fatalf("seed unreplicated record: %v", err)
	}

	link := NewDNSLink(Same(db), nil)
	for pass := 1; pass <= 3; pass++ {
		if _, err := link.Reconcile(50); err != nil {
			t.Fatalf("Reconcile pass %d: %v", pass, err)
		}
		if n := len(recordsFor(t, db, "zone-forward", "host-late.example.test.", "A")); n != 1 {
			t.Fatalf("pass %d: record count = %d, want 1: a lease replica that has not caught up withdrew a live name",
				pass, n)
		}
	}
}

func TestReconcile_IgnoresScopesWithoutDnsUpdates(t *testing.T) {
	db := newLinkageDB(t)
	seedZone(t, db, "zone-forward", "example.test.")
	seedScope(t, db, "scope-1", false) // dns_updates disabled
	seedLease(t, db, "lease-1", "scope-1", "192.0.2.30", "aa:bb:cc:dd:ee:03",
		"host1", lease.LeaseStatusActive, 1, leaseEndTime(t))

	link := NewDNSLink(Same(db), nil)
	repaired, err := link.Reconcile(50)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if repaired != 0 {
		t.Errorf("repaired = %d, want 0: the scope opted out of DNS updates", repaired)
	}
	if n := countAllRecords(t, db); n != 0 {
		t.Errorf("records = %d, want 0", n)
	}
}

func TestReconcile_DoesNotChurnOnNamesOutsideEveryZone(t *testing.T) {
	db := newLinkageDB(t)
	seedLinkage(t, db)
	// A hostname that matches no zone: re-publishing it can never succeed, so
	// the sweep must not report it as work every pass.
	seedLease(t, db, "lease-1", "scope-1", "192.0.2.30", "aa:bb:cc:dd:ee:03",
		"printer.other-domain.invalid", lease.LeaseStatusActive, 1, leaseEndTime(t))

	link := NewDNSLink(Same(db), nil)
	for pass := 1; pass <= 3; pass++ {
		repaired, err := link.Reconcile(50)
		if err != nil {
			t.Fatalf("Reconcile pass %d: %v", pass, err)
		}
		if repaired != 0 {
			t.Fatalf("pass %d repaired = %d, want 0", pass, repaired)
		}
	}
}

func TestApplyEvent_UnknownActionIsRejected(t *testing.T) {
	db := newLinkageDB(t)
	link := NewDNSLink(Same(db), nil)
	if err := link.ApplyEvent(DNSEvent{LeaseID: "l", Action: "sideways"}); err == nil {
		t.Fatal("expected an error for an unknown action")
	}
}

// TestReconcileTerminatesWithSingleConnection guards the trap that already bit
// the DNS transfer paths: with one connection, a query issued while a cursor is
// still open waits forever instead of failing. The sweep issues several
// queries per pass, so it is exactly the shape that would hang.
func TestReconcileTerminatesWithSingleConnection(t *testing.T) {
	db := newLinkageDB(t)
	seedLinkage(t, db)
	for i := 0; i < 8; i++ {
		seedLease(t, db, fmt.Sprintf("lease-%d", i), "scope-1",
			fmt.Sprintf("192.0.2.%d", 100+i), fmt.Sprintf("aa:bb:cc:dd:ee:%02x", i),
			fmt.Sprintf("host%d", i), lease.LeaseStatusActive, 1, leaseEndTime(t))
	}
	seedLease(t, db, "lease-expired", "scope-1", "192.0.2.50", "aa:bb:cc:dd:ee:99",
		"host-expired", lease.LeaseStatusExpired, 1, leaseEndTime(t))

	link := NewDNSLink(Same(db), nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 3; i++ {
			if _, err := link.Reconcile(50); err != nil {
				t.Errorf("Reconcile: %v", err)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Reconcile did not return: a query is waiting on the single connection")
	}
}
