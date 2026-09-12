package lease

import "testing"

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
