package address

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jasonwa/goddi"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Production caps the SQLite pool at one connection. Keeping that here
	// means an unclosed cursor inside a transaction fails the test instead of
	// hanging only in production.
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

	// Production enables foreign key enforcement (internal/database issues the
	// PRAGMA after connecting). Without it here, ON DELETE CASCADE silently
	// does nothing and a test asserting that links disappear with their
	// address would be asserting a behaviour nothing provides.
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}

// seedSubnet creates a space and a subnet directly, without materialising the
// addresses. Tests that need rows create them through the API under test.
func seedSubnet(t *testing.T, db *sql.DB, spaceID, subnetID, cidr string) {
	t.Helper()
	// ipam_spaces.name is unique, so a test that seeds two subnets in one space
	// must not reuse the space name.
	if _, err := db.Exec(
		`INSERT OR IGNORE INTO ipam_spaces (id, name, description) VALUES (?, ?, '')`,
		spaceID, "space-"+spaceID); err != nil {
		t.Fatalf("seed space: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO ipam_subnets (id, space_id, name, cidr) VALUES (?, ?, ?, ?)`,
		subnetID, spaceID, "subnet-"+subnetID, cidr); err != nil {
		t.Fatalf("seed subnet: %v", err)
	}
}

func insertAddress(t *testing.T, db *sql.DB, spaceID, subnetID, ip string, status Status) string {
	t.Helper()
	id := uuid.New().String()
	if _, err := db.Exec(`
		INSERT INTO ipam_addresses (id, subnet_id, space_id, ip_address, status, observed_state)
		VALUES (?, ?, ?, ?, ?, 'unknown')`, id, subnetID, spaceID, ip, string(status)); err != nil {
		t.Fatalf("insert address %s: %v", ip, err)
	}
	return id
}

// --- allocation ------------------------------------------------------------

func TestAllocateIP_Explicit(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	m := NewManager(db)

	a, err := m.AllocateIP(AllocateRequest{
		SubnetID: "sn1", IPAddress: "192.0.2.10", Owner: "alice",
		Hostname: "host10", Status: StatusStatic, Actor: "tester",
	})
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if a.IPAddress != "192.0.2.10" || a.Status != StatusStatic || a.SpaceID != "sp1" {
		t.Fatalf("address = %+v", a)
	}
	if a.AllocatedBy != "tester" || a.AllocatedAt == "" {
		t.Fatalf("allocation metadata missing: allocated_by=%q allocated_at=%q", a.AllocatedBy, a.AllocatedAt)
	}

	// A second allocation of the same address must be refused, and the refusal
	// must be the sentinel the API maps onto 409.
	if _, err := m.AllocateIP(AllocateRequest{SubnetID: "sn1", IPAddress: "192.0.2.10"}); !errors.Is(err, ErrAddressTaken) {
		t.Fatalf("second allocate err = %v, want ErrAddressTaken", err)
	}

	// The history must show the allocation exactly once.
	h, err := m.HistoryFor("sp1", "192.0.2.10", 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(h) != 1 || h[0].Action != "allocate" || h[0].NewStatus != string(StatusStatic) {
		t.Fatalf("history = %+v", h)
	}
}

func TestAllocateIP_NormalizesTheAddress(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedSubnet(t, db, "sp2", "sn6", "2001:db8::/64")
	m := NewManager(db)

	// The IPv4-mapped spelling must land on the IPv4 address, so allocating
	// the plain form afterwards collides instead of creating a second owner.
	if _, err := m.AllocateIP(AllocateRequest{SubnetID: "sn1", IPAddress: "::ffff:192.0.2.9"}); err != nil {
		t.Fatalf("allocate v4-mapped: %v", err)
	}
	got, err := m.GetAddressByIP("sn1", "192.0.2.9")
	if err != nil || got == nil {
		t.Fatalf("lookup by canonical form: addr=%v err=%v", got, err)
	}
	if _, err := m.AllocateIP(AllocateRequest{SubnetID: "sn1", IPAddress: "192.0.2.9"}); !errors.Is(err, ErrAddressTaken) {
		t.Fatalf("duplicate via another spelling err = %v, want ErrAddressTaken", err)
	}

	// A long-form IPv6 address must be stored canonically.
	full := "2001:0db8:0000:0000:0000:0000:0000:0001"
	a, err := m.AllocateIP(AllocateRequest{SubnetID: "sn6", IPAddress: full})
	if err != nil {
		t.Fatalf("allocate IPv6: %v", err)
	}
	if a.IPAddress != "2001:db8::1" {
		t.Fatalf("stored IPv6 = %q, want the canonical form", a.IPAddress)
	}
	if _, err := m.AllocateIP(AllocateRequest{SubnetID: "sn6", IPAddress: "2001:db8::1"}); !errors.Is(err, ErrAddressTaken) {
		t.Fatalf("duplicate IPv6 spelling err = %v, want ErrAddressTaken", err)
	}
}

func TestAllocateIP_RefusesInvalidAndOutOfSubnet(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	m := NewManager(db)

	if _, err := m.AllocateIP(AllocateRequest{SubnetID: "sn1", IPAddress: "192.0.3.1"}); !errors.Is(err, ErrOutOfSubnet) {
		t.Fatalf("out-of-subnet err = %v, want ErrOutOfSubnet", err)
	}
	if _, err := m.AllocateIP(AllocateRequest{SubnetID: "sn1", IPAddress: "192.168.001.001"}); !errors.Is(err, ErrInvalidIP) {
		t.Fatalf("leading-zero err = %v, want ErrInvalidIP", err)
	}
	if _, err := m.AllocateIP(AllocateRequest{SubnetID: "sn1", Status: Status("bogus")}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("bad status err = %v, want ErrInvalidStatus", err)
	}
	if _, err := m.AllocateIP(AllocateRequest{SubnetID: "missing", IPAddress: "192.0.2.1"}); !errors.Is(err, ErrSubnetNotFound) {
		t.Fatalf("missing subnet err = %v, want ErrSubnetNotFound", err)
	}
}

func TestAllocateIP_RefusesUnallocatableEvenWhenNamed(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	m := NewManager(db)
	insertAddress(t, db, "sp1", "sn1", "192.0.2.1", StatusGateway)
	insertAddress(t, db, "sp1", "sn1", "192.0.2.2", StatusReserved)
	insertAddress(t, db, "sp1", "sn1", "192.0.2.3", StatusConflict)

	for _, ip := range []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"} {
		if _, err := m.AllocateIP(AllocateRequest{SubnetID: "sn1", IPAddress: ip}); !errors.Is(err, ErrAddressTaken) {
			t.Errorf("allocating %s err = %v, want ErrAddressTaken", ip, err)
		}
	}
}

func TestAutoAllocate_SkipsReservedAddresses(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/29")
	m := NewManager(db)
	// 192.0.2.1 must be skipped; .2 is held by a gateway; .3 by a conflict.
	insertAddress(t, db, "sp1", "sn1", "192.0.2.1", StatusReserved)
	insertAddress(t, db, "sp1", "sn1", "192.0.2.2", StatusGateway)
	insertAddress(t, db, "sp1", "sn1", "192.0.2.3", StatusConflict)

	got := map[string]bool{}
	for i := 0; i < 3; i++ {
		a, err := m.AutoAllocateIP("sn1", AllocateRequest{Actor: "tester"})
		if err != nil {
			t.Fatalf("auto allocate %d: %v", i, err)
		}
		got[a.IPAddress] = true
	}
	// /29 has 8 addresses: .0 network, .7 broadcast, leaving .1-.6 usable.
	// .1-.3 are held, so the three allocations must be .4, .5, .6.
	for _, want := range []string{"192.0.2.4", "192.0.2.5", "192.0.2.6"} {
		if !got[want] {
			t.Errorf("auto allocation did not hand out %s; got %v", want, got)
		}
	}
	for _, forbidden := range []string{"192.0.2.0", "192.0.2.7"} {
		if got[forbidden] {
			t.Errorf("auto allocation handed out %s (network/broadcast)", forbidden)
		}
	}
}

func TestAutoAllocate_Slash30Slash31Slash32(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn30", "192.0.2.0/30")
	seedSubnet(t, db, "sp1", "sn31", "198.51.100.0/31")
	seedSubnet(t, db, "sp1", "sn32", "203.0.113.7/32")
	m := NewManager(db)

	// /30: two usable addresses, network and broadcast excluded.
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		a, err := m.AutoAllocateIP("sn30", AllocateRequest{})
		if err != nil {
			t.Fatalf("/30 allocate %d: %v", i, err)
		}
		seen[a.IPAddress] = true
	}
	if !seen["192.0.2.1"] || !seen["192.0.2.2"] {
		t.Fatalf("/30 handed out %v, want .1 and .2", seen)
	}
	if _, err := m.AutoAllocateIP("sn30", AllocateRequest{}); !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("/30 third allocate err = %v, want ErrPoolExhausted", err)
	}

	// /31 is a point-to-point link (RFC 3021): both addresses are usable and
	// neither is reserved.
	seen = map[string]bool{}
	for i := 0; i < 2; i++ {
		a, err := m.AutoAllocateIP("sn31", AllocateRequest{})
		if err != nil {
			t.Fatalf("/31 allocate %d: %v", i, err)
		}
		seen[a.IPAddress] = true
	}
	if !seen["198.51.100.0"] || !seen["198.51.100.1"] {
		t.Fatalf("/31 handed out %v, want both addresses", seen)
	}

	// /32 is a host route: the single address is usable.
	a, err := m.AutoAllocateIP("sn32", AllocateRequest{})
	if err != nil {
		t.Fatalf("/32 allocate: %v", err)
	}
	if a.IPAddress != "203.0.113.7" {
		t.Fatalf("/32 handed out %s", a.IPAddress)
	}
	if _, err := m.AutoAllocateIP("sn32", AllocateRequest{}); !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("/32 second allocate err = %v, want ErrPoolExhausted", err)
	}
}

func TestAutoAllocate_SparseIPv6DoesNotMaterialise(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn6", "2001:db8::/64")
	m := NewManager(db)

	a, err := m.AutoAllocateIP("sn6", AllocateRequest{})
	if err != nil {
		t.Fatalf("allocate in /64: %v", err)
	}
	if a.IPAddress != "2001:db8::1" {
		t.Fatalf("first address = %s, want 2001:db8::1", a.IPAddress)
	}
	if a.Status != StatusUsed {
		t.Fatalf("status = %s", a.Status)
	}

	// The whole point of the sparse policy: allocating in a /64 must create one
	// row, not 2^64 (which is why a bitmap or a pre-created row is never an
	// option for IPv6).
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ipam_addresses WHERE subnet_id = 'sn6'`).Scan(&rows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("address rows = %d, want 1", rows)
	}

	// The next allocation must move forward, not hand out the same address.
	next, err := m.AutoAllocateIP("sn6", AllocateRequest{})
	if err != nil {
		t.Fatalf("second allocate: %v", err)
	}
	if next.IPAddress != "2001:db8::2" {
		t.Fatalf("second address = %s, want 2001:db8::2", next.IPAddress)
	}
}

func TestAutoAllocate_ConcurrentAllocationsAreUnique(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "10.20.0.0/22")
	m := NewManager(db)

	const workers = 1000
	var wg sync.WaitGroup
	results := make([]string, workers)
	errs := make([]error, workers)
	start := make(chan struct{})

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			a, err := m.AutoAllocateIP("sn1", AllocateRequest{Actor: fmt.Sprintf("worker-%d", i)})
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = a.IPAddress
		}(i)
	}
	close(start)
	wg.Wait()

	seen := make(map[string]int, workers)
	failures := 0
	for i, ip := range results {
		if errs[i] != nil {
			failures++
			continue
		}
		seen[ip]++
	}
	if failures > 0 {
		t.Fatalf("%d of %d allocations failed; first error: %v", failures, workers, firstErr(errs))
	}

	// No address may be handed out twice. This is the property the whole
	// atomic-allocation design exists for.
	for ip, n := range seen {
		if n > 1 {
			t.Fatalf("address %s allocated %d times", ip, n)
		}
	}
	if len(seen) != workers {
		t.Fatalf("distinct addresses = %d, want %d", len(seen), workers)
	}

	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ipam_addresses WHERE subnet_id = 'sn1'`).Scan(&rows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != workers {
		t.Fatalf("address rows = %d, want %d", rows, workers)
	}

	var distinct int
	if err := db.QueryRow(
		`SELECT COUNT(DISTINCT ip_address) FROM ipam_addresses WHERE subnet_id = 'sn1'`).Scan(&distinct); err != nil {
		t.Fatalf("count distinct: %v", err)
	}
	if distinct != workers {
		t.Fatalf("distinct stored addresses = %d, want %d", distinct, workers)
	}
}

func firstErr(errs []error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// TestClaimCandidateTx_IsDecidedByTheWrite proves that a stale read cannot
// hand out an address somebody else already took.
//
// This is the defect the old AutoAssignIP-then-AllocateIP pair had: the first
// call chose an address from a snapshot, and the second was teleported back in
// time to allocate it. The snapshot is replayed here deliberately, and the
// claim must still fail: the decision reads the row's own state, never the
// snapshot the caller was working from.
func TestClaimCandidateTx_IsDecidedByTheWrite(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	m := NewManager(db)

	// Take the snapshot the old code would have used.
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	taken, err := takenBySubnetTx(tx, "sn1")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if len(taken) != 0 {
		t.Fatalf("snapshot = %v, want empty", taken)
	}
	candidates := candidatesInSubnet(mustParseNet(t, "192.0.2.0/24"), taken, 4)
	if len(candidates) == 0 || candidates[0] != "192.0.2.1" {
		t.Fatalf("candidates = %v", candidates)
	}

	// Another writer claims the address the snapshot still believes is free.
	if _, err := m.AllocateIP(AllocateRequest{SubnetID: "sn1", IPAddress: "192.0.2.1"}); err != nil {
		t.Fatalf("competing allocate: %v", err)
	}

	// Replaying the stale snapshot must not succeed.
	tx2, err := db.Begin()
	if err != nil {
		t.Fatalf("begin 2: %v", err)
	}
	_, err = m.claimCandidateTx(tx2, &subnetRow{ID: "sn1", SpaceID: "sp1", CIDR: "192.0.2.0/24"},
		"192.0.2.1", StatusUsed, AllocateRequest{})
	_ = tx2.Rollback()
	if !errors.Is(err, ErrAddressTaken) {
		t.Fatalf("stale claim err = %v, want ErrAddressTaken", err)
	}
}

// --- transitions -----------------------------------------------------------

func TestTransitionAddress_RefusesIllegalAndRecordsLegal(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	m := NewManager(db)
	id := insertAddress(t, db, "sp1", "sn1", "192.0.2.1", StatusGateway)

	if _, err := m.TransitionAddress(id, StatusDHCP, "alice", "hand it out", SourceAdmin); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("gateway -> dhcp err = %v, want ErrIllegalTransition", err)
	}
	a, err := m.GetAddress(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a.Status != StatusGateway {
		t.Fatalf("status = %s, want it unchanged", a.Status)
	}
	h, err := m.HistoryFor("sp1", "192.0.2.1", 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(h) != 0 {
		t.Fatalf("a refused transition wrote history: %+v", h)
	}

	legal, err := m.TransitionAddress(id, StatusAvailable, "alice", "no longer the gateway", SourceAdmin)
	if err != nil {
		t.Fatalf("gateway -> available: %v", err)
	}
	if legal.Status != StatusAvailable {
		t.Fatalf("status = %s", legal.Status)
	}

	// Re-asserting the same status is idempotent and writes nothing.
	if _, err := m.TransitionAddress(id, StatusAvailable, "alice", "again", SourceAdmin); err != nil {
		t.Fatalf("idempotent transition: %v", err)
	}
	h, err = m.HistoryFor("sp1", "192.0.2.1", 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(h) != 1 {
		t.Fatalf("history has %d entries, want 1: %+v", len(h), h)
	}
	if h[0].Actor != "alice" || h[0].Reason != "no longer the gateway" {
		t.Fatalf("history entry lost its provenance: %+v", h[0])
	}
	if h[0].Source != SourceAdmin {
		t.Fatalf("history source = %q, want %q", h[0].Source, SourceAdmin)
	}
}

// TestTransitionAddress_HistoryAndStatusShareATransaction is the regression
// test for the discarded-error bug.
//
// A BEFORE INSERT trigger on ipam_history aborts the history write. If the
// status were committed separately from the history -- or if the history error
// were discarded the way `_, _ = m.db.Exec(...)` discarded it -- the status
// would change and the audit trail would silently lose the event. Both must
// fail together.
func TestTransitionAddress_HistoryAndStatusShareATransaction(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	m := NewManager(db)
	id := insertAddress(t, db, "sp1", "sn1", "192.0.2.1", StatusAvailable)

	if _, err := db.Exec(`
		CREATE TRIGGER block_history BEFORE INSERT ON ipam_history
		WHEN NEW.ip_address = '192.0.2.1'
		BEGIN SELECT RAISE(ABORT, 'history blocked'); END;`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if _, err := m.TransitionAddress(id, StatusStatic, "alice", "pinned", SourceAdmin); err == nil {
		t.Fatal("transition succeeded although the history write was blocked")
	}

	a, err := m.GetAddress(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a.Status != StatusAvailable {
		t.Fatalf("status = %s; the status was committed although its history entry was not", a.Status)
	}
}

func TestAllocateIP_RollsBackWhenHistoryFails(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	m := NewManager(db)

	if _, err := db.Exec(`
		CREATE TRIGGER block_history BEFORE INSERT ON ipam_history
		WHEN NEW.ip_address = '192.0.2.7'
		BEGIN SELECT RAISE(ABORT, 'history blocked'); END;`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if _, err := m.AllocateIP(AllocateRequest{SubnetID: "sn1", IPAddress: "192.0.2.7"}); err == nil {
		t.Fatal("allocation succeeded although its history entry could not be written")
	}
	got, err := m.GetAddressByIP("sn1", "192.0.2.7")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got != nil {
		t.Fatalf("address %s survived a failed allocation: %+v", got.IPAddress, got)
	}
}

func TestReleaseAddress_RefusesStructural(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	m := NewManager(db)

	for _, status := range []Status{StatusGateway, StatusExcluded} {
		id := insertAddress(t, db, "sp1", "sn1", "192.0.2."+map[Status]string{StatusGateway: "1", StatusExcluded: "2"}[status], status)
		if _, err := m.ReleaseAddress(id, "alice", "free it", SourceAdmin); !errors.Is(err, ErrIllegalTransition) {
			t.Errorf("releasing %s err = %v, want ErrIllegalTransition", status, err)
		}
		// The explicit transition path is still available, with a reason.
		if _, err := m.TransitionAddress(id, StatusAvailable, "alice", "decommissioned", SourceAdmin); err != nil {
			t.Errorf("explicit transition from %s: %v", status, err)
		}
	}

	id := insertAddress(t, db, "sp1", "sn1", "192.0.2.9", StatusDHCP)
	if _, err := m.ReleaseAddress(id, "alice", "client left", SourceAdmin); err != nil {
		t.Fatalf("releasing a dhcp address: %v", err)
	}
	a, err := m.GetAddress(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a.Status != StatusAvailable {
		t.Fatalf("status = %s, want available", a.Status)
	}
}

// --- observations ----------------------------------------------------------

func TestObserveBySpaceIP(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	m := NewManager(db)
	free := insertAddress(t, db, "sp1", "sn1", "192.0.2.10", StatusAvailable)
	pinned := insertAddress(t, db, "sp1", "sn1", "192.0.2.11", StatusReserved)

	changed, err := m.ObserveBySpaceIP("sp1", "192.0.2.10", Observation{
		State: ObservedInUse, MAC: "aa:bb:cc:dd:ee:01", Hostname: "laptop",
		Source: SourceDHCP, LeaseID: "lease-1", Actor: "dhcp",
	})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if !changed {
		t.Fatal("an unallocated address bound by DHCP must change status")
	}
	a, err := m.GetAddress(free)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a.Status != StatusDHCP || a.ObservedState != ObservedInUse || a.ObservedMAC != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("address = %+v", a)
	}
	if a.DHCPLeaseID != "lease-1" {
		t.Fatalf("lease id not recorded: %q", a.DHCPLeaseID)
	}

	// A renewal must not overwrite the allocation intent, and must not add a
	// second history entry for an unchanged status.
	changed, err = m.ObserveBySpaceIP("sp1", "192.0.2.10", Observation{
		State: ObservedInUse, Source: SourceDHCP, LeaseID: "lease-1", Actor: "dhcp",
	})
	if err != nil {
		t.Fatalf("renew observation: %v", err)
	}
	if changed {
		t.Fatal("a renewal changed the status of an already-bound address")
	}

	// A reserved address handed out by DHCP is a conflict.
	if _, err := m.ObserveBySpaceIP("sp1", "192.0.2.11", Observation{
		State: ObservedInUse, Source: SourceDHCP, Actor: "dhcp",
	}); err != nil {
		t.Fatalf("observe reserved: %v", err)
	}
	a, err = m.GetAddress(pinned)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a.Status != StatusConflict {
		t.Fatalf("status = %s, want conflict", a.Status)
	}
	h, err := m.HistoryFor("sp1", "192.0.2.11", 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(h) != 1 || h[0].OldStatus != string(StatusReserved) || h[0].NewStatus != string(StatusConflict) {
		t.Fatalf("history = %+v", h)
	}
	if h[0].Source != SourceDHCP {
		t.Fatalf("history source = %q, want %q", h[0].Source, SourceDHCP)
	}

	// A release sets the observation without releasing an allocation: the
	// administrator decides when the address is free again.
	if _, err := m.ObserveBySpaceIP("sp1", "192.0.2.10", Observation{
		State: ObservedFree, Source: SourceDHCP, Actor: "dhcp",
	}); err != nil {
		t.Fatalf("release observation: %v", err)
	}
	a, err = m.GetAddress(free)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a.Status != StatusDHCP {
		t.Fatalf("a free observation released the allocation: status = %s", a.Status)
	}
	if a.ObservedState != ObservedFree {
		t.Fatalf("observed state = %s, want free", a.ObservedState)
	}
}

func TestObserveBySpaceIP_UnknownAddressIsNotAnError(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	m := NewManager(db)

	changed, err := m.ObserveBySpaceIP("sp1", "192.0.2.99", Observation{State: ObservedInUse, Source: SourceDHCP})
	if err != nil {
		t.Fatalf("observe an untracked address: %v", err)
	}
	if changed {
		t.Fatal("observed an address with no row and reported a change")
	}
}

func TestEnsureAddress(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	m := NewManager(db)

	a, err := m.EnsureAddress("sn1", "192.0.2.50", "dhcp")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if a == nil || a.Status != StatusAvailable || a.SpaceID != "sp1" {
		t.Fatalf("address = %+v", a)
	}
	// Idempotent.
	b, err := m.EnsureAddress("sn1", "192.0.2.50", "dhcp")
	if err != nil {
		t.Fatalf("ensure twice: %v", err)
	}
	if b.ID != a.ID {
		t.Fatalf("EnsureAddress created a second row: %s vs %s", b.ID, a.ID)
	}
	if _, err := m.EnsureAddress("sn1", "192.0.3.1", "dhcp"); !errors.Is(err, ErrOutOfSubnet) {
		t.Fatalf("out-of-subnet ensure err = %v", err)
	}
}

// --- DNS links -------------------------------------------------------------

func TestDNSLinks_OneAddressManyRecords(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	m := NewManager(db)
	a, err := m.AllocateIP(AllocateRequest{SubnetID: "sn1", IPAddress: "192.0.2.20"})
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}

	links := []DNSLink{
		{AddressID: a.ID, RecordID: "rec-a", ZoneID: "z1", RecordName: "host20.example.test.",
			RecordType: "A", Value: "192.0.2.20"},
		{AddressID: a.ID, RecordID: "rec-ptr", ZoneID: "z2", RecordName: "20.2.0.192.in-addr.arpa.",
			RecordType: "PTR", Value: "host20.example.test."},
		{AddressID: a.ID, RecordID: "rec-alias", ZoneID: "z1", RecordName: "www.example.test.",
			RecordType: "A", Value: "192.0.2.20"},
	}
	for _, l := range links {
		if err := m.LinkDNSRecord(l); err != nil {
			t.Fatalf("link %s: %v", l.RecordID, err)
		}
	}

	got, err := m.DNSLinksFor(a.ID)
	if err != nil {
		t.Fatalf("links: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("links = %d, want 3 (one address can have several names): %+v", len(got), got)
	}

	// Re-linking refreshes rather than duplicating.
	if err := m.LinkDNSRecord(DNSLink{
		AddressID: a.ID, RecordID: "rec-a", ZoneID: "z1",
		RecordName: "renamed.example.test.", RecordType: "A", Value: "192.0.2.20",
	}); err != nil {
		t.Fatalf("re-link: %v", err)
	}
	got, err = m.DNSLinksFor(a.ID)
	if err != nil {
		t.Fatalf("links: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("re-linking created a duplicate row: %d links", len(got))
	}

	// Removing one record leaves the others alone.
	if err := m.UnlinkDNSRecordByRecordID("rec-alias"); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	got, err = m.DNSLinksFor(a.ID)
	if err != nil {
		t.Fatalf("links: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("links after unlink = %d, want 2", len(got))
	}

	// Deleting the address cascades to its links.
	if _, err := db.Exec(`DELETE FROM ipam_addresses WHERE id = ?`, a.ID); err != nil {
		t.Fatalf("delete address: %v", err)
	}
	got, err = m.DNSLinksFor(a.ID)
	if err != nil {
		t.Fatalf("links: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("links survived the address: %+v", got)
	}
}

func TestNonCanonicalAddresses(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	m := NewManager(db)

	// Write a legacy row directly, the way the pre-normalisation code did.
	if _, err := db.Exec(`
		INSERT INTO ipam_addresses (id, subnet_id, space_id, ip_address, status, observed_state)
		VALUES ('legacy', 'sn1', 'sp1', '2001:0db8::0001', 'used', 'unknown')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO ipam_addresses (id, subnet_id, space_id, ip_address, status, observed_state)
		VALUES ('good', 'sn1', 'sp1', '192.0.2.1', 'used', 'unknown')`); err != nil {
		t.Fatalf("insert canonical row: %v", err)
	}

	bad, err := m.NonCanonicalAddresses(50)
	if err != nil {
		t.Fatalf("non canonical: %v", err)
	}
	if len(bad) != 1 || bad[0].IPAddress != "2001:0db8::0001" {
		t.Fatalf("non-canonical rows = %+v, want only the legacy row", bad)
	}
}

func mustParseNet(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("parse %s: %v", cidr, err)
	}
	return n
}

// TestTheSearchBoxMatchesTheColumnsItStandsFor.
//
// The console puts one search input on the address list, and it could mean an
// address, a hostname, a MAC or an owner. Nothing in the API read the parameter
// the console sends, so the box answered every query with the whole subnet --
// and a list of everything reads as a list of matches. This pins the three
// things that make it a filter: it matches all four columns, and it does not
// match an address it was not given.
func TestTheSearchBoxMatchesTheColumnsItStandsFor(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")

	for _, row := range []struct{ ip, hostname, mac, owner string }{
		{"192.0.2.20", "host-a", "aa:bb:cc:dd:ee:20", "ops"},
		{"192.0.2.200", "host-b", "aa:bb:cc:dd:ee:21", "dev"},
	} {
		if _, err := db.Exec(`
			INSERT INTO ipam_addresses
				(id, subnet_id, space_id, ip_address, status, observed_state, hostname, mac_address, owner)
			VALUES (?, 'sn1', 'sp1', ?, 'used', 'unknown', ?, ?, ?)`,
			uuid.NewString(), row.ip, row.hostname, row.mac, row.owner); err != nil {
			t.Fatalf("seed %s: %v", row.ip, err)
		}
	}

	manager := NewManager(db)
	search := func(term string) string {
		t.Helper()
		list, total, err := manager.ListAddresses(AddressFilter{Search: term})
		if err != nil {
			t.Fatalf("searching %q: %v", term, err)
		}
		// total is counted by a second query with the same WHERE clause, so a
		// mismatch means the two disagree about what matched.
		if int(total) != len(list) {
			t.Fatalf("searching %q: total = %d but %d row(s) came back", term, total, len(list))
		}
		found := make([]string, 0, len(list))
		for _, a := range list {
			found = append(found, a.IPAddress)
		}
		return strings.Join(found, ",")
	}

	cases := map[string]string{
		// A full address is exact. As a substring it would also return
		// 192.0.2.200, so "find this address" would come back with its
		// neighbours and the operator would pick their row out by eye.
		"192.0.2.20": "192.0.2.20",
		"host-b":     "192.0.2.200",
		"ee:21":      "192.0.2.200",
		"ops":        "192.0.2.20",
		// A term nothing carries returns nothing, which is the property the
		// old behaviour could not have: it returned everything.
		"nobody-owns-this": "",
	}
	for term, want := range cases {
		if got := search(term); got != want {
			t.Errorf("searching %q matched %q, want %q", term, got, want)
		}
	}
}
