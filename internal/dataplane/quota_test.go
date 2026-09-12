package dataplane

import (
	"fmt"
	"os"
	"testing"
)

// Quotas, and why crossing one is not an outage.
//
// A bound here raises the readiness level and writes a log line. It never
// refuses a client, a write, or a push. That is the property these tests pin:
// a DHPC server that stops handing out addresses at exactly the moment it
// starts alerting is an outage dressed up as a safeguard.

// TestAStoreOverItsLeaseCeilingReportsTheBound checks the counting path: the
// number that is compared is the number of leases the store is holding, and
// the breach names the bound and both figures.
func TestAStoreOverItsLeaseCeilingReportsTheBound(t *testing.T) {
	_, store, _ := newPair(t)

	insertLease(t, store, "l1", "s1", "10.0.0.10", "active")
	insertLease(t, store, "l2", "s1", "10.0.0.11", "active")

	held, err := store.HeldLeases()
	if err != nil {
		t.Fatalf("count held leases: %v", err)
	}
	if held != 2 {
		t.Fatalf("held leases = %d, want 2; the quota compares against this count", held)
	}

	q := Quota{MaxLeases: 1}
	breaches := q.Check(store, Status{HeldLeases: held})
	if len(breaches) != 1 {
		t.Fatalf("breaches = %v, want exactly one", breaches)
	}
	if got, want := breaches[0].String(), "leases=2/1"; got != want {
		t.Errorf("breach = %q, want %q", got, want)
	}
}

// TestTheStoreFootprintCountsTheWriteAheadLog pins the measurement the file
// bound is written against. In WAL mode the main file can lag the log by a
// checkpoint, so a footprint that stops at the database under-reports exactly
// the growth this bound exists to catch.
func TestTheStoreFootprintCountsTheWriteAheadLog(t *testing.T) {
	_, store, _ := newPair(t)
	insertLeases(t, store, 20)

	main, err := os.Stat(store.DSN())
	if err != nil {
		t.Fatalf("stat the store file: %v", err)
	}
	wal, err := os.Stat(store.DSN() + "-wal")
	if err != nil {
		t.Fatalf("stat the write-ahead log: %v; this test's premise is that WAL "+
			"mode leaves one behind", err)
	}
	if wal.Size() == 0 {
		t.Fatalf("the write-ahead log is empty, so this test cannot tell whether " +
			"it is counted; give it more to hold before removing this check")
	}

	got, err := store.FileSize()
	if err != nil {
		t.Fatalf("measure the store: %v", err)
	}
	if want := main.Size() + wal.Size(); got != want {
		t.Errorf("FileSize() = %d, want %d (the database plus its log); without "+
			"the log the growth would be under-reported", got, want)
	}
}

// TestADisabledBoundIsNotChecked: zero means off, so a store with no
// configured quota never reports a breach and never raises a readiness level.
func TestADisabledBoundIsNotChecked(t *testing.T) {
	_, store, _ := newPair(t)
	insertLeases(t, store, 5)
	held, err := store.HeldLeases()
	if err != nil {
		t.Fatalf("count held leases: %v", err)
	}

	q := Quota{}
	if q.Enabled() {
		t.Fatalf("an all-zero quota reports Enabled; zero is documented as off")
	}
	if breaches := q.Check(store, Status{
		HeldLeases:       held,
		PendingDNSEvents: 1 << 20,
	}); breaches != nil {
		t.Errorf("breaches = %v, want none for a disabled quota", breaches)
	}
}

// TestQuotaBreachesAreReportedInAFixedOrder: the breach list is served in a
// response body and written to a log line. A map would reorder it on every
// call, which turns a stable operator signal into noise.
func TestQuotaBreachesAreReportedInAFixedOrder(t *testing.T) {
	_, store, _ := newPair(t)
	insertLeases(t, store, 3)

	q := Quota{MaxLeases: 1, MaxBacklog: 1}
	st := Status{
		HeldLeases:          3,
		PendingLeaseChanges: 6,
		PendingDNSEvents:    5,
		PendingDHCPLogs:     4,
		PendingRecords:      3,
		PendingZoneSerials:  2,
	}

	first := q.Check(store, st)
	if len(first) != 6 {
		t.Fatalf("breaches = %v, want six (leases plus five queues)", first)
	}
	want := []string{
		"leases=3/1",
		"lease_changes=6/1", "dns_events=5/1", "dhcp_logs=4/1", "records=3/1", "zone_serials=2/1",
	}
	for i, b := range first {
		if b.String() != want[i] {
			t.Fatalf("breach %d = %q, want %q", i, b.String(), want[i])
		}
	}

	for i := 0; i < 20; i++ {
		again := q.Check(store, st)
		for j := range again {
			if again[j] != first[j] {
				t.Fatalf("call %d reordered the breaches: %v vs %v", i, again, first)
			}
		}
	}
}

// TestAMemoryStoreHasNoFootprint: the file bound must not be able to fail for
// a store that has no file. A store opened in memory reports zero rather than
// an error, so the bound simply does not fire.
func TestAMemoryStoreHasNoFootprint(t *testing.T) {
	for _, dsn := range []string{":memory:", "file::memory:?cache=shared"} {
		n, err := (&Store{dsn: dsn}).FileSize()
		if err != nil {
			t.Fatalf("FileSize() on %q: %v", dsn, err)
		}
		if n != 0 {
			t.Errorf("FileSize() on %q = %d, want 0 for a store with no file", dsn, n)
		}
	}
}

// insertLeases puts n held leases in a store. Each needs its own address:
// the replica carries the unique index over (scope_id, ip_address) for held
// statuses, so sharing one address is a constraint violation rather than a
// bigger lease table.
func insertLeases(t *testing.T, store *Store, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		insertLease(t, store, fmt.Sprintf("l%d", i), "s1",
			fmt.Sprintf("10.0.0.%d", 10+i), "active")
	}
}
