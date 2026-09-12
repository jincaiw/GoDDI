package lease

import (
	"database/sql"
	"math"
	"testing"
)

// seedLease inserts a lease row directly. Only the columns the utilisation
// query looks at are supplied; the store's own constraints decide the rest.
func seedLease(t *testing.T, db *sql.DB, id, scopeID, ip, status string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, status, lease_start, lease_end, last_seen)
		VALUES (?, ?, ?, 'aa:bb:cc:dd:ee:ff', ?, datetime('now'), datetime('now', '+1 hour'), datetime('now'))`,
		id, scopeID, ip, status); err != nil {
		t.Fatalf("seed lease %s: %v", id, err)
	}
}

// TestTheUsageCountIsTheSameSetTheAllocatorRefuses is the invariant behind the
// pool gauge. The utilisation figure is computed from heldStatuses -- the same
// list FindAvailableIP refuses addresses from -- so a state that blocks
// allocation but is not counted here would be a pool reporting capacity it does
// not have. The alert written against it would stay silent while clients were
// being turned away.
func TestTheUsageCountIsTheSameSetTheAllocatorRefuses(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.19")

	// The three held states, one of each.
	seedLease(t, store.DB, "l-active", "scope-1", "192.0.2.10", string(LeaseStatusActive))
	seedLease(t, store.DB, "l-offered", "scope-1", "192.0.2.11", string(LeaseStatusOffered))
	seedLease(t, store.DB, "l-conflict", "scope-1", "192.0.2.12", string(LeaseStatusConflict))
	// And two that are not held: they must not inflate the figure.
	seedLease(t, store.DB, "l-released", "scope-1", "192.0.2.13", string(LeaseStatusReleased))
	seedLease(t, store.DB, "l-expired", "scope-1", "192.0.2.14", string(LeaseStatusExpired))

	usage, err := NewManager(store.DB).ScopeUtilization()
	if err != nil {
		t.Fatalf("ScopeUtilization: %v", err)
	}
	if len(usage) != 1 {
		t.Fatalf("ScopeUtilization returned %d scopes, want 1: %+v", len(usage), usage)
	}

	got := usage[0]
	if got.Scope != "scope-1" {
		t.Errorf("scope = %q, want scope-1 (the metric label is the id, not the name)", got.Scope)
	}
	if got.Held != len(heldStatuses) {
		t.Errorf("held = %d, want %d: one lease in each held state, and released/expired must not count",
			got.Held, len(heldStatuses))
	}
	if got.Pool != 10 {
		t.Errorf("pool = %d, want 10 (start and end inclusive)", got.Pool)
	}
	if want := float64(len(heldStatuses)) / 10; math.Abs(got.Ratio-want) > 1e-9 {
		t.Errorf("ratio = %v, want %v", got.Ratio, want)
	}
}

// TestADisabledScopeIsNotCapacity guards the alert rather than the number. A
// scope that has been switched off cannot hand out addresses, so a ratio for it
// is not a capacity signal -- and an operator who disabled a full scope to stop
// the noise would find the alert still firing.
func TestADisabledScopeIsNotCapacity(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-on", "on", "192.0.2.0/24", "192.0.2.10", "192.0.2.19")
	if _, err := store.Exec(`
		INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip, enabled)
		VALUES ('scope-off', 'off', '198.51.100.0/24', '198.51.100.10', '198.51.100.19', 0)`); err != nil {
		t.Fatalf("seed disabled scope: %v", err)
	}
	seedLease(t, store.DB, "l1", "scope-off", "198.51.100.10", string(LeaseStatusActive))

	usage, err := NewManager(store.DB).ScopeUtilization()
	if err != nil {
		t.Fatalf("ScopeUtilization: %v", err)
	}
	for _, u := range usage {
		if u.Scope == "scope-off" {
			t.Errorf("a disabled scope was reported as capacity: %+v", u)
		}
	}
}

// TestAnUnusablePoolRangeDoesNotInventADenominator covers the shapes a scope
// row can be in that do not describe a range. A zero ratio understates a full
// pool; an invented denominator would overstate an empty one, and the second
// mistake is the one that silences the alert.
func TestAnUnusablePoolRangeDoesNotInventADenominator(t *testing.T) {
	for _, tc := range []struct {
		name     string
		from, to string
		wantPool int
	}{
		{name: "single address", from: "192.0.2.7", to: "192.0.2.7", wantPool: 1},
		{name: "two addresses", from: "192.0.2.7", to: "192.0.2.8", wantPool: 2},
		{name: "reversed range", from: "192.0.2.20", to: "192.0.2.10", wantPool: 0},
		{name: "not an address", from: "not-an-ip", to: "192.0.2.10", wantPool: 0},
		{name: "ipv6 pool", from: "2001:db8::1", to: "2001:db8::9", wantPool: 0},
		{name: "whole address space does not wrap", from: "0.0.0.0", to: "255.255.255.255", wantPool: 1 << 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newLeaseStore(t)
			if _, err := store.Exec(`
				INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip, enabled)
				VALUES ('s', 's', '192.0.2.0/24', ?, ?, 1)`, tc.from, tc.to); err != nil {
				t.Fatalf("seed scope: %v", err)
			}

			usage, err := NewManager(store.DB).ScopeUtilization()
			if err != nil {
				t.Fatalf("ScopeUtilization: %v", err)
			}
			if len(usage) != 1 {
				t.Fatalf("got %d scopes, want 1", len(usage))
			}
			if usage[0].Pool != tc.wantPool {
				t.Errorf("pool = %d, want %d", usage[0].Pool, tc.wantPool)
			}
			if usage[0].Pool == 0 && usage[0].Ratio != 0 {
				t.Errorf("ratio = %v with no usable pool, want 0", usage[0].Ratio)
			}
		})
	}
}

// TestNoScopesIsNotAnError pins the empty case. A deployment that serves DHCP
// with nothing configured yet must not lose its metrics to a startup error.
func TestNoScopesIsNotAnError(t *testing.T) {
	store := newLeaseStore(t)

	usage, err := NewManager(store.DB).ScopeUtilization()
	if err != nil {
		t.Fatalf("ScopeUtilization with no scopes: %v", err)
	}
	if len(usage) != 0 {
		t.Fatalf("got %d scopes, want none: %+v", len(usage), usage)
	}
}
