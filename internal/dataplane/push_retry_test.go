package dataplane

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/config"
)

// ---------------------------------------------------------------------------
// The defect these tests exist for
// ---------------------------------------------------------------------------
// Every upward queue used to return on the first error. The markers are cleared
// only after the transaction commits, so one row the control database would not
// accept left every marker in the batch in place -- the next pass read the same
// batch, hit the same row and stopped in the same place, once a second, while
// the runner reported it as a database error.
//
// So the tests below are written against two properties, and both of them fail
// against the old shape:
//
//   - the rows the control database accepted still travel, even though a row
//     beside them was refused;
//   - the refused row is recorded and paced rather than retried at the poll
//     interval forever.

// refuseWritesTo makes every write to one control table fail, which is how a
// test stands in for the constraint, type or permission error a real control
// database would raise. A trigger rather than a stub: it is the same mechanism
// a real refusal travels through, and it does not care which statement the
// pushing code happens to use.
func refuseWritesTo(t *testing.T, control *sql.DB, table, verb string) {
	t.Helper()
	if _, err := control.Exec(fmt.Sprintf(`
		CREATE TRIGGER w_refuse_on_%s BEFORE %s ON %s
		BEGIN SELECT RAISE(ABORT, 'refused by the test'); END`, table, verb, table)); err != nil {
		t.Fatalf("install the refusal trigger on %s: %v", table, err)
	}
}

func allowWritesTo(t *testing.T, control *sql.DB, table string) {
	t.Helper()
	if _, err := control.Exec("DROP TRIGGER IF EXISTS w_refuse_on_" + table); err != nil {
		t.Fatalf("drop the refusal trigger on %s: %v", table, err)
	}
}

// refusalState reads back what was written about a refused row.
func refusalState(t *testing.T, s *Store, table, keyCol, key string) (attempts int, retryAt, lastError string) {
	t.Helper()
	var at sql.NullString
	err := s.QueryRow(fmt.Sprintf(
		`SELECT attempts, next_attempt_at, last_error FROM %s WHERE %s = ?`, table, keyCol),
		key).Scan(&attempts, &at, &lastError)
	if err != nil {
		t.Fatalf("read the refusal state of %s in %s: %v", key, table, err)
	}
	return attempts, at.String, lastError
}

func countControlLease(t *testing.T, control *sql.DB, id string) int {
	t.Helper()
	var n int
	if err := control.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count the control lease %s: %v", id, err)
	}
	return n
}

// ---------------------------------------------------------------------------
// One refused row does not take the batch with it
// ---------------------------------------------------------------------------

func TestOneRefusedRowDoesNotTakeTheBatchWithIt(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()
	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")

	insertLease(t, store, "l1", "s1", "10.0.0.11", "active")
	insertLease(t, store, "l2", "s1", "10.0.0.12", "active")
	insertLease(t, store, "l3", "s1", "10.0.0.13", "active")

	// One row of the three is unacceptable. The other two must not pay for it:
	// they are separate obligations that happened to be read in the same batch.
	if _, err := control.Exec(`
		CREATE TRIGGER w_refuse_on_dhcp_leases BEFORE INSERT ON dhcp_leases
		WHEN NEW.id = 'l2'
		BEGIN SELECT RAISE(ABORT, 'refused by the test'); END`); err != nil {
		t.Fatalf("install the refusal trigger: %v", err)
	}
	defer allowWritesTo(t, control, "dhcp_leases")

	res, err := rep.Push(ctx, 100)
	if err != nil {
		t.Fatalf("Push = %v; a refused row is a fact to record, not an error to return", err)
	}
	if res.Upserted != 2 {
		t.Errorf("upserted %d leases, want the 2 the control database accepted", res.Upserted)
	}
	if res.Refused != 1 {
		t.Errorf("Refused = %d, want 1", res.Refused)
	}
	for _, id := range []string{"l1", "l3"} {
		if n := countControlLease(t, control, id); n != 1 {
			t.Errorf("lease %s is in the control database %d times, want 1", id, n)
		}
	}
	if n := countControlLease(t, control, "l2"); n != 0 {
		t.Errorf("the refused lease is in the control database %d times, want 0", n)
	}

	// The markers are the other half of the claim. The two rows that landed are
	// no longer owed; the one that did not is still owed, because writing it off
	// would make the backlog a smaller number that means less.
	pending, err := rep.PendingLeases()
	if err != nil {
		t.Fatalf("PendingLeases: %v", err)
	}
	if pending != 1 {
		t.Errorf("PendingLeases = %d, want only the refused row still owed", pending)
	}
	if found, _ := dirtyMarker(t, store, "l1"); found {
		t.Error("a lease that reached the control database still has a marker")
	}
	if found, _ := dirtyMarker(t, store, "l2"); !found {
		t.Error("the refused lease lost its marker; the change is still owed")
	}

	attempts, retryAt, lastError := refusalState(t, store, "dhcp_lease_dirty", "lease_id", "l2")
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
	if retryAt == "" {
		t.Error("the refused row has no retry time; it would be retried on every tick")
	}
	if lastError == "" {
		t.Error("the refused row kept no reason; an operator would have no way to find the cause")
	}
}

// TestRefusedRowsAreHeldBackAndThenTravel is the self-healing half. A row that
// is refused is not written off: it waits its delay, and when the cause is
// repaired it arrives.
func TestRefusedRowsAreHeldBackAndThenTravel(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()
	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	insertLease(t, store, "l1", "s1", "10.0.0.11", "active")

	refuseWritesTo(t, control, "dhcp_leases", "INSERT")
	if _, err := rep.Push(ctx, 100); err != nil {
		t.Fatalf("first Push = %v", err)
	}

	// While the delay is running the row is not read, so the poll does not
	// re-send the statement the control database just refused.
	pending, err := rep.pendingLeases(ctx, 100)
	if err != nil {
		t.Fatalf("pendingLeases: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("the refused row was read again immediately (%d rows); the delay is not being honoured", len(pending))
	}

	// Let the delay run out and repair the cause.
	if _, err := store.Exec(
		`UPDATE dhcp_lease_dirty SET next_attempt_at = datetime('now', '-1 second')
		  WHERE lease_id = 'l1'`); err != nil {
		t.Fatalf("expire the retry delay: %v", err)
	}
	allowWritesTo(t, control, "dhcp_leases")

	res, err := rep.Push(ctx, 100)
	if err != nil {
		t.Fatalf("third Push = %v", err)
	}
	if res.Upserted != 1 || res.Refused != 0 {
		t.Errorf("after the cause was repaired the pass upserted %d / refused %d, want 1 / 0",
			res.Upserted, res.Refused)
	}
	if n := countControlLease(t, control, "l1"); n != 1 {
		t.Fatalf("the lease is in the control database %d times after the repair, want 1", n)
	}
	if found, _ := dirtyMarker(t, store, "l1"); found {
		t.Error("the marker survived a successful push")
	}
}

// TestASecondRefusalWidensTheDelay anchors the ladder to the row's own count,
// not to the queue's: it is the row that keeps being refused, and it is the row
// whose delay has to grow.
func TestASecondRefusalWidensTheDelay(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()
	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	insertLease(t, store, "l1", "s1", "10.0.0.11", "active")
	refuseWritesTo(t, control, "dhcp_leases", "INSERT")

	expire := func() {
		t.Helper()
		if _, err := store.Exec(
			`UPDATE dhcp_lease_dirty SET next_attempt_at = datetime('now', '-1 second')
			  WHERE lease_id = 'l1'`); err != nil {
			t.Fatalf("expire the retry delay: %v", err)
		}
	}

	for pass := 1; pass <= 3; pass++ {
		expire()
		if _, err := rep.Push(ctx, 100); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
		attempts, retryAt, _ := refusalState(t, store, "dhcp_lease_dirty", "lease_id", "l1")
		if attempts != pass {
			t.Fatalf("after %d refusals attempts = %d", pass, attempts)
		}
		// The row must be pushed at least as far out as its ladder step, or a
		// row nobody will ever accept turns the poll into a retry loop.
		inside := countQueuedNow(t, store, "dhcp_lease_dirty")
		if inside != 0 {
			t.Fatalf("after %d refusals the row is due already (next_attempt_at = %q)", pass, retryAt)
		}
	}
}

// countQueuedNow counts the rows in a marker table that the retry filter lets
// through right now.
func countQueuedNow(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.QueryRow(fmt.Sprintf(
		`SELECT COUNT(*) FROM %s
		  WHERE next_attempt_at IS NULL OR julianday(next_attempt_at) <= julianday('now')`,
		table)).Scan(&n); err != nil {
		t.Fatalf("count the due rows in %s: %v", table, err)
	}
	return n
}

// TestTheRetryDelayWidensAndThenStops pins the schedule itself. The ceiling is
// the whole reason there is no terminal state, so it is asserted rather than
// left to be discovered by someone waiting for a row to travel.
func TestTheRetryDelayWidensAndThenStops(t *testing.T) {
	want := []time.Duration{
		time.Second, 5 * time.Second, 15 * time.Second,
		time.Minute, 5 * time.Minute, 5 * time.Minute, 5 * time.Minute,
	}
	for i, w := range want {
		if got := pushBackoff(i + 1); got != w {
			t.Errorf("pushBackoff(%d) = %s, want %s", i+1, got, w)
		}
	}
	if got := pushBackoff(0); got != time.Second {
		t.Errorf("pushBackoff(0) = %s, want the first step rather than a zero delay", got)
	}
}

// ---------------------------------------------------------------------------
// The same mechanism, on every queue
// ---------------------------------------------------------------------------

// TestEveryUpwardQueueRecordsARefusal covers the six queues rather than the one
// the defect was found on. They share pendingRows and recordPushFailures on
// purpose, and a queue that forgot to call the second one would differ from the
// others in a way nothing else in the suite would notice: it would return on
// the first error, which is the bug.
func TestEveryUpwardQueueRecordsARefusal(t *testing.T) {
	cases := []struct {
		name string
		// seed creates one owed change and returns the marker's key.
		seed func(t *testing.T, s *Store) string
		// seedControl creates whatever the push needs to exist on the far side.
		// Nil when the push's statement creates it.
		seedControl func(t *testing.T, control *sql.DB)
		// controlTable is what the push writes to; verb is the statement that
		// statement performs on it.
		controlTable string
		verb         string
		markerTable  string
		markerKey    string
		push         func(context.Context, *Replicator) error
	}{
		{
			name: "lease changes",
			seed: func(t *testing.T, s *Store) string {
				insertLease(t, s, "l1", "s1", "10.0.0.11", "active")
				return "l1"
			},
			controlTable: "dhcp_leases", verb: "INSERT",
			markerTable: "dhcp_lease_dirty", markerKey: "lease_id",
			push: func(ctx context.Context, r *Replicator) error {
				_, err := r.Push(ctx, 100)
				return err
			},
		},
		{
			name: "queued DNS work",
			seed: func(t *testing.T, s *Store) string {
				if _, err := s.Exec(`INSERT INTO dhcp_dns_events (lease_id, generation, action)
					VALUES ('l1', 1, 'create')`); err != nil {
					t.Fatalf("seed a DNS event: %v", err)
				}
				return "1"
			},
			controlTable: "dhcp_dns_events", verb: "INSERT",
			markerTable: "dhcp_dns_event_dirty", markerKey: "event_id",
			push: func(ctx context.Context, r *Replicator) error {
				_, err := r.PushEvents(ctx, 100)
				return err
			},
		},
		{
			name: "the DHCP event log",
			seed: func(t *testing.T, s *Store) string {
				if _, err := s.Exec(`INSERT INTO dhcp_logs (id, event_type, message)
					VALUES ('g1', 'lease', 'seeded')`); err != nil {
					t.Fatalf("seed a log entry: %v", err)
				}
				return "g1"
			},
			controlTable: "dhcp_logs", verb: "INSERT",
			markerTable: "dhcp_log_dirty", markerKey: "log_id",
			push: func(ctx context.Context, r *Replicator) error {
				_, err := r.PushLogs(ctx, 100)
				return err
			},
		},
		{
			name: "DNS records",
			seed: func(t *testing.T, s *Store) string {
				// authored_locally = 1 is the trigger's condition: this is the
				// other plane's record and is not ours to send up.
				if _, err := s.Exec(`INSERT INTO dns_records
					(id, zone_id, name, type, value, authored_locally)
					VALUES ('r1', 'z1', 'a.example.test', 'A', '10.0.0.11', 1)`); err != nil {
					t.Fatalf("seed a record: %v", err)
				}
				return "r1"
			},
			controlTable: "dns_records", verb: "INSERT",
			markerTable: "dns_record_dirty", markerKey: "record_id",
			push: func(ctx context.Context, r *Replicator) error {
				_, err := r.PushRecords(ctx, 100)
				return err
			},
		},
		{
			name: "zone serials",
			seed: func(t *testing.T, s *Store) string {
				if _, err := s.Exec(`INSERT INTO dns_zones
					(id, name, type, soa_mname, soa_rname, serial)
					VALUES ('z1', 'example.test', 'master', 'ns.example.test', 'hostmaster.example.test', 1)`); err != nil {
					t.Fatalf("seed a zone: %v", err)
				}
				// The marker is written by an AFTER UPDATE OF serial trigger, and
				// only for an increase.
				if _, err := s.Exec(`UPDATE dns_zones SET serial = 2 WHERE id = 'z1'`); err != nil {
					t.Fatalf("bump the serial: %v", err)
				}
				return "z1"
			},
			// The serial push is an UPDATE, so a BEFORE UPDATE trigger only
			// fires if the statement matches a row. Without this the update
			// would match nothing, move nothing and report no error -- and the
			// test would be asserting about a queue that was never tested.
			seedControl: func(t *testing.T, control *sql.DB) {
				t.Helper()
				if _, err := control.Exec(`INSERT INTO dns_zones
					(id, name, type, soa_mname, soa_rname, serial)
					VALUES ('z1', 'example.test', 'master', 'ns.example.test', 'hostmaster.example.test', 1)`); err != nil {
					t.Fatalf("seed the control zone: %v", err)
				}
			},
			controlTable: "dns_zones", verb: "UPDATE",
			markerTable: "dns_zone_serial_dirty", markerKey: "zone_id",
			push: func(ctx context.Context, r *Replicator) error {
				_, err := r.PushZoneSerials(ctx, 100)
				return err
			},
		},
		{
			name: "audit entries",
			seed: func(t *testing.T, s *Store) string {
				if _, err := s.Exec(`INSERT INTO audit_logs (id, action, resource_type)
					VALUES ('a1', 'lease.create', 'dhcp_lease')`); err != nil {
					t.Fatalf("seed an audit entry: %v", err)
				}
				return "a1"
			},
			controlTable: "audit_logs", verb: "INSERT",
			markerTable: "audit_log_dirty", markerKey: "log_id",
			push: func(ctx context.Context, r *Replicator) error {
				_, err := r.PushAuditLogs(ctx, 100)
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			control, store, rep := newPair(t)
			ctx := context.Background()
			key := tc.seed(t, store)
			if tc.seedControl != nil {
				tc.seedControl(t, control)
			}

			refuseWritesTo(t, control, tc.controlTable, tc.verb)
			defer allowWritesTo(t, control, tc.controlTable)

			if err := tc.push(ctx, rep); err != nil {
				t.Fatalf("the push returned %v; a refusal belongs in the marker, not in the return", err)
			}

			attempts, retryAt, lastError := refusalState(t, store, tc.markerTable, tc.markerKey, key)
			if attempts != 1 {
				t.Errorf("attempts = %d, want 1", attempts)
			}
			if retryAt == "" {
				t.Error("no retry time was written; the row would be retried on every tick")
			}
			if lastError == "" {
				t.Error("no reason was written; the refusal would be invisible")
			}
			if n := countQueuedNow(t, store, tc.markerTable); n != 0 {
				t.Errorf("%d rows are due immediately after being refused, want 0", n)
			}

			// And the mechanism is shared, not copied: the refusal is counted by
			// the same list the filter above was built from.
			if n, err := rep.Refused(); err != nil || n != 1 {
				t.Errorf("Refused() = %d, %v; want 1, nil", n, err)
			}
		})
	}
}

// TestEveryMarkerTableIsInTheUpwardQueueList reads the schema and compares it
// with the list the refusal count is summed over.
//
// Without it a seventh queue would be added, its markers would be cleared and
// pushed correctly, and the one number that says "the control database is
// refusing our rows" would not count it -- which is the same class of mistake
// as the queue that filled up while nothing reported it.
func TestEveryMarkerTableIsInTheUpwardQueueList(t *testing.T) {
	s := newStore(t, config.DataPlaneLease)
	rows, err := s.Query(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE '%dirty' ORDER BY name`)
	if err != nil {
		t.Fatalf("read the schema: %v", err)
	}
	var found []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan a table name: %v", err)
		}
		found = append(found, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the schema: %v", err)
	}
	rows.Close()

	listed := map[string]string{}
	for _, q := range upwardQueues {
		listed[q.table] = q.key
	}
	if len(found) != len(upwardQueues) {
		t.Errorf("the schema has %d marker tables (%v) and upwardQueues lists %d",
			len(found), found, len(upwardQueues))
	}
	for _, name := range found {
		key, ok := listed[name]
		if !ok {
			t.Errorf("%s is a marker table that upwardQueues does not list; its refusals would go uncounted", name)
			continue
		}
		// A key column that does not exist would make recordPushFailures'
		// statements fail at the worst possible moment -- right after a row was
		// refused -- so it is checked against the schema rather than trusted.
		var n int
		if err := s.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, name, key).Scan(&n); err != nil {
			t.Fatalf("inspect %s: %v", name, err)
		}
		if n != 1 {
			t.Errorf("upwardQueues says %s is keyed by %s, which the table does not have", name, key)
		}
	}
}
