package dataplane

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
)

// The two-author rule, and the queues that carry a data plane's work back up.
//
// These are the tests for the half of the split that is easy to get wrong in a
// way nothing notices: a downward sync that quietly withdraws the records this
// process is serving, an upward queue that resets its own retry state on every
// pass, and a counter that makes a process fetch back what it just pushed.

// newZonePair builds a control database and a zone store wired together.
func newZonePair(t *testing.T) (*sql.DB, *Store, *Replicator) {
	t.Helper()
	control := newControlDB(t)
	store := newStore(t, config.DataPlaneZone)
	return control, store, NewReplicator(control, store)
}

func insertControlZone(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO dns_zones (id, name, type, soa_mname, soa_rname, serial)
		VALUES (?, ?, 'master', 'ns1.example.test', 'hostmaster.example.test', 1)`, id, name); err != nil {
		t.Fatalf("insert control zone %s: %v", id, err)
	}
}

// insertRecord writes a row directly, which is what both authors ultimately do.
// authored is 0 for the control plane's records and 1 for this plane's.
func insertRecord(t *testing.T, db *sql.DB, id, zoneID, name, value string, authored int) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, enabled, authored_locally)
		VALUES (?, ?, ?, 'A', ?, 1, ?)`, id, zoneID, name, value, authored); err != nil {
		t.Fatalf("insert record %s: %v", id, err)
	}
}

func recordValue(t *testing.T, db *sql.DB, id string) (string, bool) {
	t.Helper()
	var value string
	err := db.QueryRow(`SELECT value FROM dns_records WHERE id = ?`, id).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		t.Fatalf("read record %s: %v", id, err)
	}
	return value, true
}

// ---------------------------------------------------------------------------
// The downward boundary.
// ---------------------------------------------------------------------------

// TestADownwardSyncLeavesTheRecordsThisPlaneAuthored is the property the
// authored_locally column exists for. Without it, one operator renaming one
// zone would withdraw every name this process is currently answering for.
func TestADownwardSyncLeavesTheRecordsThisPlaneAuthored(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()

	insertControlZone(t, control, "z1", "example.test")
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// What the applier does for a confirmed binding.
	insertRecord(t, store.DB, "ddns1", "z1", "host1.example.test.", "10.0.0.5", 1)

	// An operator adds a record and removes another. The sync must pick both up
	// and must not touch the row above.
	insertRecord(t, control, "manual1", "z1", "www.example.test.", "10.0.0.9", 0)
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	if got, ok := recordValue(t, store.DB, "manual1"); !ok || got != "10.0.0.9" {
		t.Errorf("the control plane's record did not arrive: value=%q present=%v", got, ok)
	}
	if got, ok := recordValue(t, store.DB, "ddns1"); !ok || got != "10.0.0.5" {
		t.Errorf("a configuration change withdrew the record this plane authored: value=%q present=%v", got, ok)
	}

	// And the control plane deleting its own record still removes it here.
	if _, err := control.Exec(`DELETE FROM dns_records WHERE id = 'manual1'`); err != nil {
		t.Fatalf("delete the control record: %v", err)
	}
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("third sync: %v", err)
	}
	if _, ok := recordValue(t, store.DB, "manual1"); ok {
		t.Error("a record the control plane deleted is still here")
	}
	if _, ok := recordValue(t, store.DB, "ddns1"); !ok {
		t.Error("the locally authored record was withdrawn along with it")
	}
}

// TestTheControlPlanesCopyOfALocalRecordDoesNotComeBack is the other half of the
// same boundary.
//
// The control database holds a copy of every record this plane pushed up, for
// the console to show. That copy lags: a record this process has already
// withdrawn on a teardown can still be sitting there. Importing it would
// publish a name no binding justifies.
func TestTheControlPlanesCopyOfALocalRecordDoesNotComeBack(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()

	insertControlZone(t, control, "z1", "example.test")
	// The stale copy, already in the control database before the first sync.
	insertRecord(t, control, "ddns1", "z1", "host1.example.test.", "10.0.0.5", 1)

	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, ok := recordValue(t, store.DB, "ddns1"); ok {
		t.Error("a record this plane authored was imported back from the control plane's copy of it")
	}
}

// ---------------------------------------------------------------------------
// The event stream.
// ---------------------------------------------------------------------------

func insertControlEvent(t *testing.T, db *sql.DB, leaseID, action string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO dhcp_dns_events (lease_id, generation, action, scope_id, ip_address, hostname)
		VALUES (?, 1, ?, 's1', '10.0.0.5', 'host1')`, leaseID, action)
	if err != nil {
		t.Fatalf("insert control event: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return id
}

// TestTheOutboxIsAppendedRatherThanReplaced pins the one table that is a log.
//
// Replacing it wholesale would be wrong twice: the consumer's retry counter
// would reset on every pass, and every pass would re-deliver the whole history.
func TestTheOutboxIsAppendedRatherThanReplaced(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()

	first := insertControlEvent(t, control, "l1", "create")
	if _, err := rep.Sync(ctx, DomainDDNS); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_dns_events`); got != 1 {
		t.Fatalf("events copied = %d, want 1", got)
	}

	// The consumer has worked on it and failed twice.
	if _, err := store.Exec(`
		UPDATE dhcp_dns_events SET status = 'pending', attempts = 3, last_error = 'boom'
		WHERE id = ?`, first); err != nil {
		t.Fatalf("record consumer state: %v", err)
	}

	insertControlEvent(t, control, "l2", "delete")
	if _, err := rep.Sync(ctx, DomainDDNS); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_dns_events`); got != 2 {
		t.Errorf("events after the second pass = %d, want 2", got)
	}
	var attempts int
	var lastError string
	if err := store.QueryRow(`SELECT attempts, last_error FROM dhcp_dns_events WHERE id = ?`, first).
		Scan(&attempts, &lastError); err != nil {
		t.Fatalf("read back consumer state: %v", err)
	}
	if attempts != 3 || lastError != "boom" {
		t.Errorf("the consumer's own retry state was overwritten: attempts=%d last_error=%q", attempts, lastError)
	}
}

// TestTheOutboxDoesNotReplayWhatItAlreadyHas covers the id filter directly: a
// second pass over an unchanged revision must add nothing, and a row the store
// already holds must not be re-inserted even if it arrives again.
func TestTheOutboxDoesNotReplayWhatItAlreadyHas(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()

	id := insertControlEvent(t, control, "l1", "create")
	if _, err := rep.Sync(ctx, DomainDDNS); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	// Force a second pass by touching the revision the way a later insert
	// would: a new row, then check the old one was not duplicated.
	insertControlEvent(t, control, "l2", "create")
	if _, err := rep.Sync(ctx, DomainDDNS); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_dns_events WHERE id = ?`, id); got != 1 {
		t.Errorf("event %d appears %d times, want 1", id, got)
	}
}

// ---------------------------------------------------------------------------
// The counters.
// ---------------------------------------------------------------------------

// TestOnlyTheControlPlaneWritesMoveTheDnsRevision is the loop guard.
//
// A DNS process both reads its records from the control database and writes to
// it. If the counter moved on the rows it pushed up, it would fetch back what
// it had just sent, every pass, forever.
func TestOnlyTheControlPlaneWritesMoveTheDnsRevision(t *testing.T) {
	control := newControlDB(t)

	insertControlZone(t, control, "z1", "example.test")
	before := revision(t, control, "dns")

	// A row this plane pushed up, including its update and its deletion.
	insertRecord(t, control, "ddns1", "z1", "host1.example.test.", "10.0.0.5", 1)
	if got := revision(t, control, "dns"); got != before {
		t.Errorf("a record a data plane pushed up moved the dns revision from %d to %d", before, got)
	}
	if _, err := control.Exec(`UPDATE dns_records SET value = '10.0.0.6' WHERE id = 'ddns1'`); err != nil {
		t.Fatalf("update the pushed-up record: %v", err)
	}
	if got := revision(t, control, "dns"); got != before {
		t.Errorf("updating a pushed-up record moved the dns revision from %d to %d", before, got)
	}
	if _, err := control.Exec(`DELETE FROM dns_records WHERE id = 'ddns1'`); err != nil {
		t.Fatalf("delete the pushed-up record: %v", err)
	}
	if got := revision(t, control, "dns"); got != before {
		t.Errorf("deleting a pushed-up record moved the dns revision from %d to %d", before, got)
	}

	// An operator's write still does.
	insertRecord(t, control, "manual1", "z1", "www.example.test.", "10.0.0.9", 0)
	if got := revision(t, control, "dns"); got <= before {
		t.Errorf("a console write did not move the dns revision (still %d)", got)
	}
}

// TestTheLeaseCounterIsItsOwn pins why the lease replica does not ride on the
// dhcp counter: lease churn is constant and scope configuration is not, and
// sharing one counter would make every renewal look like a configuration edit.
func TestTheLeaseCounterIsItsOwn(t *testing.T) {
	control := newControlDB(t)
	dhcpBefore := revision(t, control, "dhcp")
	leasesBefore := revision(t, control, "leases")

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	if revision(t, control, "dhcp") == dhcpBefore {
		t.Error("a scope write did not move the dhcp revision")
	}
	if got := revision(t, control, "leases"); got != leasesBefore {
		t.Errorf("a scope write moved the leases revision from %d to %d", leasesBefore, got)
	}

	if _, err := control.Exec(`INSERT INTO dhcp_leases
		(id, scope_id, ip_address, mac_address, lease_start, lease_end, status, last_seen, generation)
		VALUES ('l1', 's1', '10.0.0.10', 'aa:bb:cc:dd:ee:01', '2026-01-01T00:00:00Z',
		        '2030-01-01T00:00:00Z', 'active', '2026-01-01T00:00:00Z', 1)`); err != nil {
		t.Fatalf("insert control lease: %v", err)
	}
	if revision(t, control, "leases") == leasesBefore {
		t.Error("a lease write did not move the leases revision")
	}
}

// ---------------------------------------------------------------------------
// The upward queues.
// ---------------------------------------------------------------------------

func TestTheOutboxReachesTheControlDatabase(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()

	if _, err := store.Exec(`INSERT INTO dhcp_dns_events
		(lease_id, generation, action, scope_id, ip_address, hostname)
		VALUES ('l1', 2, 'create', 's1', '10.0.0.5', 'host1')`); err != nil {
		t.Fatalf("queue an event: %v", err)
	}

	moved, err := rep.PushEvents(ctx, 0)
	if err != nil {
		t.Fatalf("push events: %v", err)
	}
	if moved != 1 {
		t.Errorf("events moved = %d, want 1", moved)
	}
	var leaseID, action string
	var generation int64
	if err := control.QueryRow(
		`SELECT lease_id, action, generation FROM dhcp_dns_events`).Scan(&leaseID, &action, &generation); err != nil {
		t.Fatalf("read the pushed event: %v", err)
	}
	if leaseID != "l1" || action != "create" || generation != 2 {
		t.Errorf("the pushed event is wrong: lease=%q action=%q generation=%d", leaseID, action, generation)
	}
	if n, err := rep.PendingEvents(); err != nil || n != 0 {
		t.Errorf("PendingEvents = %d, %v; want 0, nil", n, err)
	}

	// Nothing owed, nothing touched.
	moved, err = rep.PushEvents(ctx, 0)
	if err != nil || moved != 0 {
		t.Errorf("a second push moved %d rows (%v), want 0", moved, err)
	}
}

func TestTheEventLogReachesTheControlDatabase(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()

	if _, err := store.Exec(`INSERT INTO dhcp_logs
		(id, scope_id, mac_address, ip_address, event_type, message)
		VALUES ('log1', 's1', 'aa:bb:cc:dd:ee:01', '10.0.0.5', 'lease_ack', 'bound')`); err != nil {
		t.Fatalf("write a log entry: %v", err)
	}

	moved, err := rep.PushLogs(ctx, 0)
	if err != nil {
		t.Fatalf("push logs: %v", err)
	}
	if moved != 1 {
		t.Errorf("log entries moved = %d, want 1", moved)
	}
	if got := countControl(t, control, `SELECT COUNT(*) FROM dhcp_logs WHERE id = 'log1'`); got != 1 {
		t.Errorf("log entries in the control database = %d, want 1", got)
	}
	// A log entry is never deleted, so its marker is only ever for an insert.
	moved, err = rep.PushLogs(ctx, 0)
	if err != nil || moved != 0 {
		t.Errorf("a second push moved %d entries (%v), want 0", moved, err)
	}
}

func countControl(t *testing.T, db *sql.DB, query string, args ...interface{}) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count (%s): %v", query, err)
	}
	return n
}

func TestRecordsThisPlaneAuthoredReachTheControlDatabase(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()

	insertControlZone(t, control, "z1", "example.test")

	insertRecord(t, store.DB, "ddns1", "z1", "host1.example.test.", "10.0.0.5", 1)
	res, err := rep.PushRecords(ctx, 0)
	if err != nil {
		t.Fatalf("push records: %v", err)
	}
	if res.Upserted != 1 {
		t.Errorf("records pushed = %d, want 1", res.Upserted)
	}
	if got, ok := recordValue(t, control, "ddns1"); !ok || got != "10.0.0.5" {
		t.Errorf("the record did not reach the console's view: value=%q present=%v", got, ok)
	}

	// The withdrawal travels too.
	if _, err := store.Exec(`DELETE FROM dns_records WHERE id = 'ddns1'`); err != nil {
		t.Fatalf("withdraw the record: %v", err)
	}
	res, err = rep.PushRecords(ctx, 0)
	if err != nil {
		t.Fatalf("second push: %v", err)
	}
	if res.Deleted != 1 {
		t.Errorf("records withdrawn = %d, want 1", res.Deleted)
	}
	if _, ok := recordValue(t, control, "ddns1"); ok {
		t.Error("the withdrawn record is still in the console's view")
	}
}

// TestARecordPushCannotWithdrawAnOperatorsRecord is the far-side guard.
//
// The delete is restricted to authored_locally = 1 so that a withdrawal decided
// by the data plane can never remove a record an operator typed in, even if the
// two ever end up describing the same id.
func TestARecordPushCannotWithdrawAnOperatorsRecord(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()

	insertControlZone(t, control, "z1", "example.test")
	insertRecord(t, control, "shared", "z1", "typed.example.test.", "10.0.0.9", 0)

	// The same id, authored here, then withdrawn.
	insertRecord(t, store.DB, "shared", "z1", "served.example.test.", "10.0.0.5", 1)
	if _, err := store.Exec(`DELETE FROM dns_records WHERE id = 'shared'`); err != nil {
		t.Fatalf("withdraw the local record: %v", err)
	}
	if _, err := rep.PushRecords(ctx, 0); err != nil {
		t.Fatalf("push records: %v", err)
	}

	got, ok := recordValue(t, control, "shared")
	if !ok || got != "10.0.0.9" {
		t.Errorf("the operator's record was withdrawn by a data-plane push: value=%q present=%v", got, ok)
	}
}

// TestARecordTheControlPlaneAuthoredIsNotPushedBack is the mirror guard on the
// near side: a row that landed here by a downward sync is not this plane's to
// report, and pushing it back would be a round trip that says nothing.
func TestARecordTheControlPlaneAuthoredIsNotPushedBack(t *testing.T) {
	control, _, rep := newZonePair(t)
	ctx := context.Background()

	insertControlZone(t, control, "z1", "example.test")
	insertRecord(t, control, "manual1", "z1", "www.example.test.", "10.0.0.9", 0)
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if n, err := rep.PendingRecords(); err != nil || n != 0 {
		t.Errorf("a control-authored row was queued for the push: pending=%d err=%v", n, err)
	}
	res, err := rep.PushRecords(ctx, 0)
	if err != nil || res.Upserted != 0 || res.Deleted != 0 {
		t.Errorf("a control-authored row was pushed back: %+v err=%v", res, err)
	}
}

// ---------------------------------------------------------------------------
// Cold start, and the takeover gate.
// ---------------------------------------------------------------------------

// TestHasAnyReplicaAnswersForTheWholeStore pins why the startup question is
// asked once rather than per domain: a DNS process holding its zones must start
// while the control database is down even though it holds no DHCP scopes.
func TestHasAnyReplicaAnswersForTheWholeStore(t *testing.T) {
	control, store, rep := newZonePair(t)

	has, err := store.HasAnyReplica([]Domain{DomainDDNS, DomainDNS})
	if err != nil {
		t.Fatalf("HasAnyReplica on an empty store: %v", err)
	}
	if has {
		t.Error("an empty store claims to have something to serve")
	}

	insertControlZone(t, control, "z1", "example.test")
	if _, err := rep.Sync(context.Background(), DomainDNS); err != nil {
		t.Fatalf("sync: %v", err)
	}

	has, err = store.HasAnyReplica([]Domain{DomainDDNS, DomainDNS})
	if err != nil {
		t.Fatalf("HasAnyReplica after a sync: %v", err)
	}
	if !has {
		t.Error("a store holding a zone claims to have nothing to serve")
	}
}

// TestADnsStoreDoesNotTakeOverLeases pins the gate on the one-off upgrade path.
//
// The DNS process syncs DomainDHCP too -- it needs the scopes to qualify a
// client's short hostname -- but it does not own lease state. Taking the control
// database's lease rows would duplicate what DomainLeases brings across and mark
// outbox entries it should still consume as already seen.
func TestADnsStoreDoesNotTakeOverLeases(t *testing.T) {
	control, store, _ := newZonePair(t)
	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	controlLease(t, control, "l1", "s1", "10.0.0.10")

	runner := NewRunner(NewReplicator(control, store), RunnerConfig{
		Domains: []Domain{DomainDHCP, DomainDNS, DomainDDNS},
		// PushLeases is what says "this store owns leases".
	})
	if err := runner.Prime(context.Background()); err != nil {
		t.Fatalf("prime: %v", err)
	}

	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_leases`); got != 0 {
		t.Errorf("a DNS store took over %d leases it does not own", got)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_scopes`); got != 1 {
		t.Errorf("the DNS store did not copy the scopes it needs to qualify names (got %d)", got)
	}
}

// ---------------------------------------------------------------------------
// Zone serials.
// ---------------------------------------------------------------------------

// TestALocallyBumpedSerialSurvivesADownwardSync is the property the serial
// marker exists for.
//
// The zone row is a replica, so a configuration change anywhere replaces it --
// including the serial. Without the marker, a dynamic update's bump would be
// overwritten by a value the control plane computed without it, and the zone
// would look unchanged to every secondary.
func TestALocallyBumpedSerialSurvivesADownwardSync(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()

	insertControlZone(t, control, "z1", "example.test")
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// A dynamic update bumps the serial where the zone is served.
	if _, err := store.Exec(`UPDATE dns_zones SET serial = 200 WHERE id = 'z1'`); err != nil {
		t.Fatalf("bump the local serial: %v", err)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dns_zone_serial_dirty`); got != 1 {
		t.Fatalf("serial markers queued = %d, want 1", got)
	}

	// The console edits something, computing its next serial from the value it
	// holds -- which predates the bump.
	if _, err := control.Exec(`UPDATE dns_zones SET serial = 101 WHERE id = 'z1'`); err != nil {
		t.Fatalf("console edit: %v", err)
	}
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	var serial int64
	if err := store.QueryRow(`SELECT serial FROM dns_zones WHERE id = 'z1'`).Scan(&serial); err != nil {
		t.Fatalf("read the local serial: %v", err)
	}
	if serial != 200 {
		t.Errorf("the downward sync moved the served serial to %d, want the locally bumped 200", serial)
	}
}

// TestTheSerialReachesTheControlPlaneAndOnlyEverRises covers both ends of the
// push: the value travels, and a value the control plane has already passed is
// refused rather than written back over it.
func TestTheSerialReachesTheControlPlaneAndOnlyEverRises(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()

	insertControlZone(t, control, "z1", "example.test")
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := store.Exec(`UPDATE dns_zones SET serial = 200 WHERE id = 'z1'`); err != nil {
		t.Fatalf("bump the local serial: %v", err)
	}

	moved, err := rep.PushZoneSerials(ctx, 0)
	if err != nil {
		t.Fatalf("push serials: %v", err)
	}
	if moved != 1 {
		t.Errorf("serials moved = %d, want 1", moved)
	}
	if got := controlSerial(t, control, "z1"); got != 200 {
		t.Errorf("control serial = %d, want 200", got)
	}
	if n, err := rep.PendingZoneSerials(); err != nil || n != 0 {
		t.Errorf("PendingZoneSerials = %d, %v; want 0, nil", n, err)
	}

	// A marker holding a value the control plane has already passed must not
	// move it backwards: a secondary told the serial went down has to treat the
	// zone as either unchanged or wrapped.
	if _, err := store.Exec(
		`INSERT INTO dns_zone_serial_dirty (zone_id, serial) VALUES ('z1', 150)`); err != nil {
		t.Fatalf("queue a stale serial: %v", err)
	}
	moved, err = rep.PushZoneSerials(ctx, 0)
	if err != nil {
		t.Fatalf("second push: %v", err)
	}
	if moved != 0 {
		t.Errorf("a stale serial was pushed (%d rows), want 0", moved)
	}
	if got := controlSerial(t, control, "z1"); got != 200 {
		t.Errorf("control serial = %d after a stale push, want 200", got)
	}
}

func controlSerial(t *testing.T, db *sql.DB, zoneID string) int64 {
	t.Helper()
	var serial int64
	if err := db.QueryRow(`SELECT serial FROM dns_zones WHERE id = ?`, zoneID).Scan(&serial); err != nil {
		t.Fatalf("read the control serial of %s: %v", zoneID, err)
	}
	return serial
}

// ---------------------------------------------------------------------------
// The outbox push.
// ---------------------------------------------------------------------------

// TestTheOutboxPushCannotResetAnotherConsumersState is the producer's rule.
//
// Two planes hold a copy of this queue: the one that writes the payload, and
// each one that works through it. The producer cannot observe a consumer, so a
// push that overwrote the state would be a guess about work it does not own --
// and the guess would always be "not done yet".
func TestTheOutboxPushCannotResetAnotherConsumersState(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()

	// An entry the consumer has already finished.
	if _, err := control.Exec(`INSERT INTO dhcp_dns_events
		(id, lease_id, generation, action, scope_id, ip_address, hostname,
		 attempts, status, last_error)
		VALUES (5, 'l1', 1, 'create', 's1', '10.0.0.5', 'host1', 3, 'done', '')`); err != nil {
		t.Fatalf("seed a finished entry: %v", err)
	}

	// The producer's own copy of the same entry, still pending.
	if _, err := store.Exec(`INSERT INTO dhcp_dns_events
		(id, lease_id, generation, action, scope_id, ip_address, hostname)
		VALUES (5, 'l1', 1, 'create', 's1', '10.0.0.5', 'host1')`); err != nil {
		t.Fatalf("queue the entry: %v", err)
	}

	moved, err := rep.PushEvents(ctx, 0)
	if err != nil {
		t.Fatalf("push events: %v", err)
	}
	if moved != 0 {
		t.Errorf("an entry that was already there was reported as moved (%d), want 0", moved)
	}

	var status, lastError string
	var attempts int
	if err := control.QueryRow(
		`SELECT status, attempts, last_error FROM dhcp_dns_events WHERE id = 5`).
		Scan(&status, &attempts, &lastError); err != nil {
		t.Fatalf("read the entry back: %v", err)
	}
	if status != "done" || attempts != 3 {
		t.Errorf("the push reset the consumer's state: status=%q attempts=%d, want done/3", status, attempts)
	}
}
