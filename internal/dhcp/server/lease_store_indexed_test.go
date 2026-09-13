package server

import (
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

type indexedProbeStore struct {
	value     *lease.Lease
	available string
	calls     int
}

func (s *indexedProbeStore) CreateLease(string, string, string, string, time.Duration) (*lease.Lease, error) {
	return s.value, nil
}
func (s *indexedProbeStore) ReserveAddress(string, string, string, string) (*lease.Lease, error) {
	return s.value, nil
}
func (s *indexedProbeStore) ActivateLease(string, time.Duration) (*lease.Lease, error) {
	return s.value, nil
}
func (s *indexedProbeStore) RenewLease(string, time.Duration) (*lease.Lease, error) {
	return s.value, nil
}
func (s *indexedProbeStore) GetLease(string) (*lease.Lease, error) { s.calls++; return s.value, nil }
func (s *indexedProbeStore) GetLeaseByMAC(string) (*lease.Lease, error) {
	s.calls++
	return s.value, nil
}
func (s *indexedProbeStore) GetHeldLeaseByIP(string) (*lease.Lease, error) {
	s.calls++
	return s.value, nil
}
func (s *indexedProbeStore) FindAvailableIP(string) (string, error) {
	s.calls++
	return s.available, nil
}
func (s *indexedProbeStore) ReleaseLease(string) error { return nil }
func (s *indexedProbeStore) QuarantineIP(string, string, string) (*lease.Lease, error) {
	return s.value, nil
}
func (s *indexedProbeStore) ExpireLeases() ([]*lease.Lease, error) { return nil, nil }

func TestIndexedLeaseStoreFallsBackUntilReady(t *testing.T) {
	value := &lease.Lease{ID: "l1", ScopeID: "scope", IPAddress: "192.0.2.10", MACAddress: "aa", Status: lease.LeaseStatusActive}
	primary := &indexedProbeStore{value: value, available: "192.0.2.10"}
	index := lease.NewMemoryIndex()
	store := NewIndexedLeaseStore(primary, index)
	if _, err := store.GetLease("l1"); err != nil {
		t.Fatalf("fallback get: %v", err)
	}
	if primary.calls != 1 {
		t.Fatalf("primary calls = %d, want 1", primary.calls)
	}
}

func TestIndexedLeaseStoreUsesRebuiltIndexWhenReady(t *testing.T) {
	value := &lease.Lease{ID: "l1", ScopeID: "scope", IPAddress: "192.0.2.10", MACAddress: "aa", Status: lease.LeaseStatusActive}
	primary := &indexedProbeStore{value: value, available: "192.0.2.11"}
	index := lease.NewMemoryIndex()
	index.Replace([]lease.Lease{*value}, []lease.AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.12"}})
	store := NewIndexedLeaseStore(primary, index)
	store.MarkReady()
	got, err := store.GetHeldLeaseByIP(value.IPAddress)
	if err != nil || got == nil || got.ID != value.ID {
		t.Fatalf("indexed get = %#v, %v", got, err)
	}
	ip, err := store.FindAvailableIP("scope")
	if err != nil || ip != "192.0.2.11" {
		t.Fatalf("indexed available = %q, %v", ip, err)
	}
	if primary.calls != 0 {
		t.Fatalf("primary calls = %d, want 0 after ready", primary.calls)
	}
}

func TestIndexedLeaseStoreQuarantineTombstoneUpdatesReadyIndex(t *testing.T) {
	primary := &indexedProbeStore{available: "192.0.2.10"}
	index := lease.NewMemoryIndex()
	index.Replace(nil, []lease.AddressPool{{ScopeID: "scope", StartIP: "192.0.2.10", EndIP: "192.0.2.11"}})
	store := NewIndexedLeaseStore(primary, index)
	store.MarkReady()

	primary.value = &lease.Lease{ID: "tombstone", ScopeID: "scope", IPAddress: "192.0.2.10", MACAddress: "aa", Status: lease.LeaseStatusConflict}
	got, err := store.QuarantineIP("scope", "192.0.2.10", "aa")
	if err != nil || got == nil {
		t.Fatalf("quarantine = %#v, %v", got, err)
	}
	if ip, err := store.FindAvailableIP("scope"); err != nil || ip != "192.0.2.11" {
		t.Fatalf("available after tombstone = %q, %v", ip, err)
	}
}

func TestIndexedLeaseStoreWriteReflectsPostCommitState(t *testing.T) {
	value := &lease.Lease{ID: "l1", ScopeID: "scope", IPAddress: "192.0.2.10", MACAddress: "aa", Status: lease.LeaseStatusActive}
	primary := &indexedProbeStore{value: value}
	index := lease.NewMemoryIndex()
	store := NewIndexedLeaseStore(primary, index)
	if _, err := store.CreateLease("scope", value.IPAddress, value.MACAddress, "host", time.Hour); err != nil {
		t.Fatalf("create: %v", err)
	}
	store.MarkReady()
	got, err := store.GetLease(value.ID)
	if err != nil || got == nil || got.ID != value.ID {
		t.Fatalf("indexed post-commit state = %#v, %v", got, err)
	}
}

func TestIndexedLeaseStoreRejectsNilDependencies(t *testing.T) {
	index := lease.NewMemoryIndex()
	defer func() {
		if recover() == nil {
			t.Fatal("nil primary did not panic")
		}
	}()
	_ = NewIndexedLeaseStore(nil, index)
}
