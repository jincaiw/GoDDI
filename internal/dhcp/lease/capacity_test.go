package lease

import (
	"database/sql"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// ceilings is the sizing target ADR 0001 fixes, written once here so the two
// tests below cannot disagree about what "the documented ceiling" means.
const (
	ceilingLeases = 20000
	// A /16 pool, so the ceiling fits with room to spare and the address that
	// allocation should return is a real gap rather than the end of the range.
	ceilingSubnet = "10.0.0.0/16"
	ceilingFrom   = "10.0.0.1"
	ceilingTo     = "10.255.255.254"
)

// fillCeiling writes n held leases starting at first, in one transaction.
//
// Deliberately not through the manager: this is the fixture, not the thing
// being measured, and 20,000 commits at synchronous=FULL would spend the whole
// test's budget producing setup.
func fillCeiling(t *testing.T, db *sql.DB, scopeID, first string, n int) {
	t.Helper()
	fillCeilingHole(t, db, scopeID, first, n, -1)
}

// fillCeilingHole is fillCeiling with one offset left free, so two fixtures can
// hold the same number of rows with the first free address in different places.
func fillCeilingHole(t *testing.T, db *sql.DB, scopeID, first string, n, hole int) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("beginning the fixture: %v", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen, generation)
		VALUES (?, ?, ?, ?, 'ceiling', '', ?, ?, 'active', ?, 1)`)
	if err != nil {
		t.Fatalf("preparing the fixture insert: %v", err)
	}
	defer stmt.Close()

	base := net4ToUint32(t, first)
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	end := time.Now().UTC().Add(time.Hour).Format("2006-01-02T15:04:05Z")
	for i := 0; i < n; i++ {
		if i == hole {
			continue
		}
		ip := uint32ToNet4(base + uint32(i))
		if _, err := stmt.Exec(fmt.Sprintf("ceiling-%d", i), scopeID, ip, ceilingMAC(i),
			now, end, now); err != nil {
			t.Fatalf("inserting fixture lease %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("committing the fixture: %v", err)
	}
}

func ceilingMAC(i int) string {
	return fmt.Sprintf("02:00:00:%02x:%02x:%02x", (i>>16)&0xff, (i>>8)&0xff, i&0xff)
}

func net4ToUint32(t *testing.T, ip string) uint32 {
	t.Helper()
	parsed := parseIPv4(ip)
	if parsed == 0 {
		t.Fatalf("%q is not an IPv4 address", ip)
	}
	return parsed
}

func uint32ToNet4(v uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// parseIPv4 and the two helpers above exist so the fixture and the assertion
// can talk about addresses as numbers; the product's own conversion is not
// reused because a test that shares the code under test cannot catch it being
// wrong.
func parseIPv4(ip string) uint32 {
	var a, b, c, d uint32
	if _, err := fmt.Sscanf(ip, "%d.%d.%d.%d", &a, &b, &c, &d); err != nil {
		return 0
	}
	return a<<24 | b<<16 | c<<8 | d
}

// TestTheCostOfAllocatingAtTheDocumentedLeaseCeiling measures the one path whose
// cost could plausibly be proportional to the size of the pool.
//
// ADR 0001 fixes the sizing target at twenty thousand leases per node, and this
// is the steady state an operator reaches: the pool is mostly handed out and the
// next client asks for whatever is left. The measurement is logged rather than
// only asserted against, because the number is the point -- a bound alone would
// hide a tenfold change that still fits under it.
func TestTheCostOfAllocatingAtTheDocumentedLeaseCeiling(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "ceiling", ceilingSubnet, ceilingFrom, ceilingTo)
	fillCeiling(t, store.DB, "scope-1", ceilingFrom, ceilingLeases)

	var held int
	if err := store.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE status = 'active'`).Scan(&held); err != nil {
		t.Fatalf("counting the fixture: %v", err)
	}
	if held != ceilingLeases {
		t.Fatalf("the fixture holds %d leases, want %d", held, ceilingLeases)
	}

	m := NewManager(store.DB)

	// The first free address after the block the fixture filled: the answer has
	// to be right as well as fast, or the timing is about a different query.
	want := uint32ToNet4(net4ToUint32(t, ceilingFrom) + uint32(ceilingLeases))
	started := time.Now()
	got, err := m.FindAvailableIP("scope-1")
	first := time.Since(started)
	if err != nil {
		t.Fatalf("FindAvailableIP at the ceiling: %v", err)
	}
	if got != want {
		t.Errorf("FindAvailableIP returned %s, want %s (the first address the fixture left free)", got, want)
	}
	t.Logf("%d held leases: the first free address took %s", ceilingLeases, first.Round(time.Millisecond))

	// A confirmed binding at that address, which is what a REQUEST costs the
	// store beyond the search.
	started = time.Now()
	if _, err := m.CreateLease("scope-1", want, "02:00:00:ff:ff:01", "ceiling", time.Hour); err != nil {
		t.Fatalf("CreateLease at the ceiling: %v", err)
	}
	t.Logf("%d held leases: a confirmed write at that address took %s",
		ceilingLeases, time.Since(started).Round(time.Millisecond))

	// The sweep runs on every tick, so its cost at the ceiling is part of the
	// steady state rather than a maintenance oddity.
	started = time.Now()
	if _, err := m.ExpireLeases(); err != nil {
		t.Fatalf("ExpireLeases at the ceiling: %v", err)
	}
	t.Logf("%d held leases: the expiry sweep took %s",
		ceilingLeases, time.Since(started).Round(time.Millisecond))

	// And the pool-utilisation figure the console and the alerts read.
	started = time.Now()
	util, err := m.ScopeUtilization()
	if err != nil {
		t.Fatalf("ScopeUtilization at the ceiling: %v", err)
	}
	t.Logf("%d held leases: the utilisation query took %s (%d scopes)",
		ceilingLeases, time.Since(started).Round(time.Millisecond), len(util))
}

// TestTheFirstFreeAddressIsTheOneBruteForceWouldFind pins the answer of the gap
// walk against a reference implementation.
//
// FindAvailableIP was rewritten in W14-e from "probe every candidate address
// with its own query" to "read the held set once and walk it" -- the probing
// version cost 310 ms per call at the documented ceiling. The rewrite is only
// acceptable if it hands out the *same* address, so the expected answer here is
// computed in Go from the description of what was seeded, never from a query
// that the product also runs.
//
// The states that must not block are included on purpose: an expired binding
// and a released one are not held, and a disabled reservation is not a
// reservation. A walk that treated "there is a row" as "the address is taken"
// would pass a simpler test and fail this one.
func TestTheFirstFreeAddressIsTheOneBruteForceWouldFind(t *testing.T) {
	const (
		poolFrom = "192.0.2.1"
		poolTo   = "192.0.2.14"
	)

	statuses := []struct {
		status string
		holds  bool
	}{
		{"active", true},
		{"offered", true},
		{"conflict", true},
		{"expired", false},
		{"released", false},
	}

	rng := rand.New(rand.NewSource(20260911))
	base := net4ToUint32(t, poolFrom)
	span := int(net4ToUint32(t, poolTo) - base + 1)

	for round := 0; round < 60; round++ {
		store := newLeaseStore(t)

		// A random sub-range, so the walk has to respect the scope's own bounds
		// rather than the pool it was handed.
		lo := rng.Intn(span / 2)
		hi := lo + rng.Intn(span-lo)
		scopeFrom := uint32ToNet4(base + uint32(lo))
		scopeTo := uint32ToNet4(base + uint32(hi))
		seedScope(t, store.DB, "scope-1", "brute", ceilingSubnet, scopeFrom, scopeTo)

		held := map[string]bool{}
		for i := 0; i < span; i++ {
			ip := uint32ToNet4(base + uint32(i))
			switch rng.Intn(4) {
			case 0:
				// Leave it free.
				continue
			case 1:
				choice := statuses[rng.Intn(len(statuses))]
				if _, err := store.Exec(`
					INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname,
						client_id, lease_start, lease_end, status, last_seen, generation)
					VALUES (?, 'scope-1', ?, ?, '', '', '2026-01-01T00:00:00Z',
						'2030-01-01T00:00:00Z', ?, '2026-01-01T00:00:00Z', 1)`,
					fmt.Sprintf("brute-%d", i), ip, ceilingMAC(i), choice.status); err != nil {
					t.Fatalf("seeding a %s lease for %s: %v", choice.status, ip, err)
				}
				if choice.holds {
					held[ip] = true
				}
			case 2:
				enabled := rng.Intn(2) == 0
				if _, err := store.Exec(`
					INSERT INTO dhcp_reservations (id, scope_id, ip_address, mac_address,
						hostname, description, enabled)
					VALUES (?, 'scope-1', ?, ?, '', '', ?)`,
					fmt.Sprintf("brute-res-%d", i), ip, ceilingMAC(1000+i), enabled); err != nil {
					t.Fatalf("seeding a reservation for %s: %v", ip, err)
				}
				if enabled {
					held[ip] = true
				}
			}
		}

		// The reference: the first address in the scope's range that nothing
		// holds, where "holds" comes from the seed above and not from a query.
		want := ""
		for i := lo; i <= hi; i++ {
			candidate := uint32ToNet4(base + uint32(i))
			if !held[candidate] {
				want = candidate
				break
			}
		}

		got, err := NewManager(store.DB).FindAvailableIP("scope-1")
		if want == "" {
			if err == nil {
				t.Errorf("round %d: the scope %s-%s is full but allocation returned %s",
					round, scopeFrom, scopeTo, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("round %d: allocation failed over %s-%s although %s is free: %v",
				round, scopeFrom, scopeTo, want, err)
			continue
		}
		if got != want {
			t.Errorf("round %d: allocation returned %s over %s-%s, but the first free address is %s",
				round, got, scopeFrom, scopeTo, want)
		}
	}
}

// TestTheCostOfAllocatingDoesNotDependOnWhereTheGapIs is the guard, and the
// shape of the comparison is the whole point.
//
// The obvious guard -- "twenty thousand leases must not cost ten times what two
// thousand do" -- was measured and does not work: the old per-candidate version
// came in at a ratio of 6.7 (43 ms against 287 ms), which is under any threshold
// that a shared machine can be trusted with. A ratio cannot separate the two
// shapes, because both do grow with the pool.
//
// What does separate them is *where the answer is* rather than how much is
// held. Two stores are built with exactly the same 20,000 held leases; in one
// the first free address is the very first candidate, in the other it is the
// 20,001st. A cost that is one query plus a sort over what is held is the same
// in both. A cost that is a probe per candidate differs by four orders of
// magnitude. Nothing about the machine survives into the comparison, and the
// measured numbers are logged so the claim can be checked by eye.
func TestTheCostOfAllocatingDoesNotDependOnWhereTheGapIs(t *testing.T) {
	measure := func(hole int, describe string) time.Duration {
		store := newLeaseStore(t)
		seedScope(t, store.DB, "scope-1", "shape", ceilingSubnet, ceilingFrom, ceilingTo)
		fillCeilingHole(t, store.DB, "scope-1", ceilingFrom, ceilingLeases, hole)
		m := NewManager(store.DB)

		started := time.Now()
		got, err := m.FindAvailableIP("scope-1")
		took := time.Since(started)
		if err != nil {
			t.Fatalf("FindAvailableIP with the gap %s: %v", describe, err)
		}
		want := uint32ToNet4(net4ToUint32(t, ceilingFrom) + uint32(hole))
		if got != want {
			t.Fatalf("FindAvailableIP with the gap %s returned %s, want %s", describe, got, want)
		}
		return took
	}

	// Same held set both times: 20,000 rows, 20,001 candidates scanned in one
	// case and one candidate in the other.
	early := measure(0, "at the first address")
	late := measure(ceilingLeases, "at the end of what has been handed out")
	t.Logf("%d held leases, gap at the first address: %s; gap after all of them: %s",
		ceilingLeases, early.Round(time.Millisecond), late.Round(time.Millisecond))

	// The additive term is timer slack for a call that is already sub-millisecond
	// in the early case; it is not a licence for a call that scales.
	if budget := 5*early + 5*time.Millisecond; late > budget {
		t.Errorf("finding a gap after %d held leases took %s while finding one at the "+
			"first address took %s; the %s allowed means the cost has grown back towards "+
			"one probe per candidate address, which is what made a busy pool slow",
			ceilingLeases, late.Round(time.Millisecond), early.Round(time.Millisecond),
			budget.Round(time.Millisecond))
	}
}

// TestTheStoreAtTheDocumentedLeaseCeilingFitsTheSizing answers a question an
// operator asks before choosing a disk: what does the documented ceiling
// actually occupy.
func TestTheStoreAtTheDocumentedLeaseCeilingFitsTheSizing(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "ceiling", ceilingSubnet, ceilingFrom, ceilingTo)
	fillCeiling(t, store.DB, "scope-1", ceilingFrom, ceilingLeases)

	if _, err := store.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("checkpointing before measuring: %v", err)
	}
	var pageCount, pageSize int64
	if err := store.QueryRow(`PRAGMA page_count`).Scan(&pageCount); err != nil {
		t.Fatalf("reading page_count: %v", err)
	}
	if err := store.QueryRow(`PRAGMA page_size`).Scan(&pageSize); err != nil {
		t.Fatalf("reading page_size: %v", err)
	}
	bytes := pageCount * pageSize
	t.Logf("%d leases occupy %d bytes (%.1f KiB, %.0f bytes per lease)",
		ceilingLeases, bytes, float64(bytes)/1024, float64(bytes)/float64(ceilingLeases))
}
