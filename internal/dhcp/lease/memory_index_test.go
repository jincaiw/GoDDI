package lease

import (
	"errors"
	"sync"
	"testing"
)

func TestMemoryIndexReplaceAndReadIndexes(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace([]Lease{
		{ID: "offered", ScopeID: "scope", IPAddress: "192.0.2.10", MACAddress: "aa", LeaseEnd: "2026-01-01T00:00:00Z", Status: LeaseStatusOffered},
		{ID: "active", ScopeID: "scope", IPAddress: "192.0.2.11", MACAddress: "bb", LeaseEnd: "2026-01-02T00:00:00Z", Status: LeaseStatusActive, Generation: 2},
		{ID: "released", ScopeID: "scope", IPAddress: "192.0.2.12", MACAddress: "cc", Status: LeaseStatusReleased},
	}, []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.13"}})

	if value, ok := index.HeldByIP("192.0.2.10"); !ok || value.Status != LeaseStatusOffered {
		t.Fatalf("offered lookup = %#v, %v", value, ok)
	}
	if value, ok := index.ByMAC("bb"); !ok || value.ID != "active" {
		t.Fatalf("active MAC lookup = %#v, %v", value, ok)
	}
	if _, ok := index.ByMAC("cc"); ok {
		t.Fatal("released lease appeared in active MAC index")
	}
	ip, err := index.FindAvailableIP("scope")
	if err != nil || ip != "192.0.2.12" {
		t.Fatalf("first available = %q, %v; want 192.0.2.12", ip, err)
	}
}

func TestMemoryIndexReservationsAndConflictAreHeld(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace([]Lease{
		{ID: "conflict", ScopeID: "scope", IPAddress: "192.0.2.10", Status: LeaseStatusConflict},
	}, []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.12", Reservations: map[string]bool{"192.0.2.11": true}}})

	ip, err := index.FindAvailableIP("scope")
	if err != nil || ip != "192.0.2.12" {
		t.Fatalf("first available = %q, %v; want 192.0.2.12", ip, err)
	}
}

func TestMemoryIndexUpsertAndRemoveRebuildIndexes(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace(nil, []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.11"}})
	index.Upsert(Lease{ID: "l1", ScopeID: "scope", IPAddress: "192.0.2.10", MACAddress: "aa", LeaseEnd: "2026-01-01T00:00:00Z", Status: LeaseStatusActive})
	if ip, err := index.FindAvailableIP("scope"); err != nil || ip != "192.0.2.11" {
		t.Fatalf("after upsert = %q, %v", ip, err)
	}
	index.Remove("l1")
	if ip, err := index.FindAvailableIP("scope"); err != nil || ip != "192.0.2.10" {
		t.Fatalf("after remove = %q, %v", ip, err)
	}
}

func TestMemoryIndexReplaceDuplicateIDDoesNotLeaveStaleIndexes(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace([]Lease{
		{ID: "same", ScopeID: "scope", IPAddress: "192.0.2.10", MACAddress: "aa", LeaseEnd: "2026-01-01T00:00:00Z", Status: LeaseStatusActive},
		{ID: "same", ScopeID: "scope", IPAddress: "192.0.2.11", MACAddress: "bb", LeaseEnd: "2026-01-02T00:00:00Z", Status: LeaseStatusActive},
	}, []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.12"}})
	index.Remove("same")
	for _, ip := range []string{"192.0.2.10", "192.0.2.11"} {
		if _, ok := index.HeldByIP(ip); ok {
			t.Fatalf("stale IP index survived duplicate-ID removal: %s", ip)
		}
	}
	for _, mac := range []string{"aa", "bb"} {
		if _, ok := index.ByMAC(mac); ok {
			t.Fatalf("stale MAC index survived duplicate-ID removal: %s", mac)
		}
	}
}

func TestMemoryIndexSameIPInDifferentScopesRemainsHeldPerScope(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace([]Lease{
		{ID: "scope-a", ScopeID: "scope-a", IPAddress: "192.0.2.10", LeaseEnd: "2026-01-01T00:00:00Z", Status: LeaseStatusActive},
		{ID: "scope-b", ScopeID: "scope-b", IPAddress: "192.0.2.10", LeaseEnd: "2026-01-02T00:00:00Z", Status: LeaseStatusActive},
	}, []AddressPool{
		{ScopeID: "scope-a", StartIP: "192.0.2.10", EndIP: "192.0.2.10"},
		{ScopeID: "scope-b", StartIP: "192.0.2.10", EndIP: "192.0.2.10"},
	})
	for _, scope := range []string{"scope-a", "scope-b"} {
		if _, err := index.FindAvailableIP(scope); !errors.Is(err, ErrNoAvailableAddress) {
			t.Fatalf("%s treated held address as available: %v", scope, err)
		}
	}
}

func TestMemoryIndexSameMACMultipleActiveLeasesRestoresOlderWinner(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace([]Lease{
		{ID: "older", ScopeID: "scope-a", IPAddress: "192.0.2.10", MACAddress: "aa", LeaseEnd: "2026-01-01T00:00:00Z", Status: LeaseStatusActive},
		{ID: "newer", ScopeID: "scope-b", IPAddress: "192.0.2.11", MACAddress: "aa", LeaseEnd: "2026-01-02T00:00:00Z", Status: LeaseStatusActive},
	}, nil)
	if value, ok := index.ByMAC("aa"); !ok || value.ID != "newer" {
		t.Fatalf("MAC winner = %#v, %v", value, ok)
	}
	index.Remove("newer")
	if value, ok := index.ByMAC("aa"); !ok || value.ID != "older" {
		t.Fatalf("older active lease was not restored: %#v, %v", value, ok)
	}
}

func TestMemoryIndexSameMACEqualLeaseEndUsesStableWinner(t *testing.T) {
	index := NewMemoryIndex()
	leases := []Lease{
		{ID: "a", ScopeID: "scope-a", IPAddress: "192.0.2.10", MACAddress: "aa", LeaseEnd: "2026-01-01T00:00:00Z", Generation: 1, Status: LeaseStatusActive},
		{ID: "b", ScopeID: "scope-b", IPAddress: "192.0.2.11", MACAddress: "aa", LeaseEnd: "2026-01-01T00:00:00Z", Generation: 1, Status: LeaseStatusActive},
	}
	for n := 0; n < 10; n++ {
		index.Replace(leases, nil)
		value, ok := index.ByMAC("aa")
		if !ok || value.ID != "b" {
			t.Fatalf("MAC winner on replace %d = %#v, %v; want b", n, value, ok)
		}
	}
}

func TestMemoryIndexSameIPEqualLeaseEndUsesStableWinner(t *testing.T) {
	index := NewMemoryIndex()
	leases := []Lease{
		{ID: "a", ScopeID: "scope", IPAddress: "192.0.2.10", LeaseEnd: "2026-01-01T00:00:00Z", Generation: 1, Status: LeaseStatusActive},
		{ID: "b", ScopeID: "scope", IPAddress: "192.0.2.10", LeaseEnd: "2026-01-01T00:00:00Z", Generation: 1, Status: LeaseStatusConflict},
	}
	for n := 0; n < 10; n++ {
		index.Replace(leases, nil)
		value, ok := index.HeldByIP("192.0.2.10")
		if !ok || value.ID != "b" {
			t.Fatalf("winner on replace %d = %#v, %v; want b", n, value, ok)
		}
	}
}

func TestMemoryIndexDoesNotInferExpiryFromLeaseEnd(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace([]Lease{{ID: "expired-in-memory", ScopeID: "scope", IPAddress: "192.0.2.10", LeaseEnd: "2000-01-01T00:00:00Z", Status: LeaseStatusActive}}, []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.10"}})
	if _, err := index.FindAvailableIP("scope"); !errors.Is(err, ErrNoAvailableAddress) {
		t.Fatalf("expired status-active lease was treated as available: %v", err)
	}
	index.Upsert(Lease{ID: "expired-in-memory", ScopeID: "scope", IPAddress: "192.0.2.10", LeaseEnd: "2000-01-01T00:00:00Z", Status: LeaseStatusExpired})
	if ip, err := index.FindAvailableIP("scope"); err != nil || ip != "192.0.2.10" {
		t.Fatalf("after explicit expiry projection = %q, %v", ip, err)
	}
}

func TestMemoryIndexClaimAvailableIsAtomic(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace(nil, []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.10"}})

	const workers = 32
	results := make(chan Lease, workers)
	errorsCh := make(chan error, workers)
	var wg sync.WaitGroup
	for n := 0; n < workers; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			value, err := index.ClaimAvailable("scope", Lease{ID: string(rune('a' + n)), MACAddress: "mac"})
			if err != nil {
				errorsCh <- err
				return
			}
			results <- value
		}(n)
	}
	wg.Wait()
	close(results)
	close(errorsCh)

	var claimed []Lease
	for value := range results {
		claimed = append(claimed, value)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d leases, want exactly one", len(claimed))
	}
	if claimed[0].IPAddress != "192.0.2.10" || claimed[0].Status != LeaseStatusOffered {
		t.Fatalf("claim = %#v", claimed[0])
	}
	for err := range errorsCh {
		if !errors.Is(err, ErrNoAvailableAddress) {
			t.Fatalf("claim error = %v, want ErrNoAvailableAddress", err)
		}
	}
	if value, ok := index.HeldByIP("192.0.2.10"); !ok || value.ID != claimed[0].ID {
		t.Fatalf("held lookup = %#v, %v", value, ok)
	}
}

func TestMemoryIndexClaimAvailableRejectsDuplicateID(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace([]Lease{{ID: "existing", ScopeID: "scope", IPAddress: "192.0.2.11", Status: LeaseStatusActive}}, []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.12"}})
	if _, err := index.ClaimAvailable("scope", Lease{ID: "existing"}); err == nil {
		t.Fatal("duplicate lease ID was accepted")
	}
	if value, ok := index.Get("existing"); !ok || value.IPAddress != "192.0.2.11" {
		t.Fatalf("existing lease changed after duplicate claim = %#v, %v", value, ok)
	}
}

func TestMemoryIndexClaimAvailableSkipsHeldAndReservations(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace([]Lease{
		{ID: "active", ScopeID: "scope", IPAddress: "192.0.2.10", Status: LeaseStatusActive},
		{ID: "offered", ScopeID: "scope", IPAddress: "192.0.2.11", Status: LeaseStatusOffered},
		{ID: "conflict", ScopeID: "scope", IPAddress: "192.0.2.12", Status: LeaseStatusConflict},
	}, []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.14", Reservations: map[string]bool{"192.0.2.13": true}}})

	value, err := index.ClaimAvailable("scope", Lease{ID: "new", MACAddress: "dd"})
	if err != nil {
		t.Fatal(err)
	}
	if value.IPAddress != "192.0.2.14" || value.Status != LeaseStatusOffered {
		t.Fatalf("claim = %#v", value)
	}
	if got, ok := index.Get("new"); !ok || got.IPAddress != value.IPAddress {
		t.Fatalf("claimed lease lookup = %#v, %v", got, ok)
	}
}

func TestMemoryIndexRemoveRestoresOlderHeldLease(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace([]Lease{
		{ID: "conflict", ScopeID: "scope", IPAddress: "192.0.2.10", LeaseEnd: "2026-01-03T00:00:00Z", Status: LeaseStatusConflict},
		{ID: "active", ScopeID: "scope", IPAddress: "192.0.2.10", LeaseEnd: "2026-01-04T00:00:00Z", Status: LeaseStatusActive},
	}, []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.10"}})
	index.Remove("active")
	if value, ok := index.HeldByIP("192.0.2.10"); !ok || value.ID != "conflict" {
		t.Fatalf("held lookup after removal = %#v, %v", value, ok)
	}
	if _, err := index.FindAvailableIP("scope"); !errors.Is(err, ErrNoAvailableAddress) {
		t.Fatalf("available after conflict restore = %v, want ErrNoAvailableAddress", err)
	}
}

func TestMemoryIndexClaimAvailableRequiresID(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace(nil, []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.10"}})
	if _, err := index.ClaimAvailable("scope", Lease{}); err == nil {
		t.Fatal("claim without lease ID was accepted")
	}
}

func TestMemoryIndexPoolEndingAtMaxIPv4Terminates(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace([]Lease{{ID: "held", ScopeID: "scope", IPAddress: "255.255.255.254", Status: LeaseStatusActive}}, []AddressPool{{ScopeID: "scope", StartIP: "255.255.255.254", EndIP: "255.255.255.255"}})
	ip, err := index.FindAvailableIP("scope")
	if err != nil || ip != "255.255.255.255" {
		t.Fatalf("available = %q, %v", ip, err)
	}
}

func TestMemoryIndexRejectsBadPools(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace(nil, []AddressPool{{ScopeID: "scope", StartIP: "bad", EndIP: "192.0.2.10"}})
	if _, err := index.FindAvailableIP("scope"); err == nil {
		t.Fatal("bad IPv4 bounds were accepted")
	}
	if _, err := index.FindAvailableIP("missing"); err == nil {
		t.Fatal("missing scope was accepted")
	}
}

func TestMemoryIndexReplaceCheckedRejectsDuplicateIDsWithoutChangingIndex(t *testing.T) {
	index := NewMemoryIndex()
	index.Replace([]Lease{{ID: "keep", ScopeID: "scope", IPAddress: "192.0.2.10", Status: LeaseStatusActive}}, []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.11"}})
	err := index.ReplaceChecked([]Lease{{ID: "dup", ScopeID: "scope", IPAddress: "192.0.2.10", Status: LeaseStatusActive}, {ID: "dup", ScopeID: "scope", IPAddress: "192.0.2.11", Status: LeaseStatusActive}}, []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.11"}})
	if err == nil {
		t.Fatal("duplicate snapshot ID was accepted")
	}
	if got, ok := index.Get("keep"); !ok || got.IPAddress != "192.0.2.10" {
		t.Fatalf("index changed after rejected snapshot: %#v, %v", got, ok)
	}
}

func TestMemoryIndexReplaceCheckedRejectsInvalidPoolAndLease(t *testing.T) {
	index := NewMemoryIndex()
	cases := []struct {
		name   string
		leases []Lease
		pools  []AddressPool
	}{
		{name: "duplicate pool", pools: []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.11"}, {ScopeID: "scope", StartIP: "192.0.2.12", EndIP: "192.0.2.13"}}},
		{name: "invalid reservation", pools: []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.11", Reservations: map[string]bool{"bad": true}}}},
		{name: "lease outside pool", leases: []Lease{{ID: "l1", ScopeID: "scope", IPAddress: "192.0.2.12", Status: LeaseStatusActive}}, pools: []AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.11"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := index.ReplaceChecked(tc.leases, tc.pools); err == nil {
				t.Fatal("invalid snapshot was accepted")
			}
		})
	}
}
