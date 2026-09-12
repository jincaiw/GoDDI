package server

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

// IndexedLeaseStore is the N3.3 transition adapter. SQLite-backed primary
// writes remain authoritative; after a successful write the returned lease is
// reflected into MemoryIndex. Reads use the index only after Rebuild has
// marked it ready. Before that, every read falls back to primary.
//
// This is a migration step, not the final WAL-backed store: a process restart
// must rebuild from durable data before enabling indexed reads, and ACKs still
// depend on the primary write result.
type IndexedLeaseStore struct {
	primary LeaseStore
	index   *lease.MemoryIndex
	ready   atomic.Bool
}

func NewIndexedLeaseStore(primary LeaseStore, index *lease.MemoryIndex) *IndexedLeaseStore {
	if primary == nil {
		panic("dhcp server: nil primary lease store")
	}
	if index == nil {
		panic("dhcp server: nil memory lease index")
	}
	return &IndexedLeaseStore{primary: primary, index: index}
}

// MarkReady enables memory reads after the caller has rebuilt the index from a
// durable snapshot. It is intentionally explicit so an empty/uninitialised
// index cannot silently make every pool look free.
func (s *IndexedLeaseStore) MarkReady()   { s.ready.Store(true) }
func (s *IndexedLeaseStore) MarkUnready() { s.ready.Store(false) }
func (s *IndexedLeaseStore) Ready() bool  { return s.ready.Load() }

func (s *IndexedLeaseStore) GetLease(id string) (*lease.Lease, error) {
	if !s.Ready() {
		return s.primary.GetLease(id)
	}
	value, ok := s.index.Get(id)
	if !ok {
		return nil, fmt.Errorf("lease not found")
	}
	return value, nil
}

func (s *IndexedLeaseStore) GetLeaseByMAC(mac string) (*lease.Lease, error) {
	if !s.Ready() {
		return s.primary.GetLeaseByMAC(mac)
	}
	value, _ := s.index.ByMAC(mac)
	return value, nil
}

func (s *IndexedLeaseStore) GetHeldLeaseByIP(ip string) (*lease.Lease, error) {
	if !s.Ready() {
		return s.primary.GetHeldLeaseByIP(ip)
	}
	value, _ := s.index.HeldByIP(ip)
	return value, nil
}

func (s *IndexedLeaseStore) FindAvailableIP(scopeID string) (string, error) {
	if !s.Ready() {
		return s.primary.FindAvailableIP(scopeID)
	}
	return s.index.FindAvailableIP(scopeID)
}

func (s *IndexedLeaseStore) CreateLease(scopeID, ip, mac, hostname string, duration time.Duration) (*lease.Lease, error) {
	value, err := s.primary.CreateLease(scopeID, ip, mac, hostname, duration)
	if err == nil && value != nil {
		s.index.Upsert(*value)
	}
	return value, err
}

func (s *IndexedLeaseStore) ReserveAddress(scopeID, ip, mac, hostname string) (*lease.Lease, error) {
	value, err := s.primary.ReserveAddress(scopeID, ip, mac, hostname)
	if err == nil && value != nil {
		s.index.Upsert(*value)
	}
	return value, err
}

func (s *IndexedLeaseStore) ActivateLease(id string, duration time.Duration) (*lease.Lease, error) {
	value, err := s.primary.ActivateLease(id, duration)
	if err == nil && value != nil {
		s.index.Upsert(*value)
	}
	return value, err
}

func (s *IndexedLeaseStore) RenewLease(id string, duration time.Duration) (*lease.Lease, error) {
	value, err := s.primary.RenewLease(id, duration)
	if err == nil && value != nil {
		s.index.Upsert(*value)
	}
	return value, err
}

func (s *IndexedLeaseStore) ReleaseLease(id string) error {
	if err := s.primary.ReleaseLease(id); err != nil {
		return err
	}
	if value, err := s.primary.GetLease(id); err == nil && value != nil {
		s.index.Upsert(*value)
	} else if err != nil {
		return err
	}
	return nil
}

func (s *IndexedLeaseStore) QuarantineIP(scopeID, ip, mac string) (*lease.Lease, error) {
	value, err := s.primary.QuarantineIP(scopeID, ip, mac)
	if err == nil && value != nil {
		s.index.Upsert(*value)
	}
	return value, err
}

func (s *IndexedLeaseStore) ExpireLeases() ([]*lease.Lease, error) {
	values, err := s.primary.ExpireLeases()
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		if value == nil {
			continue
		}
		copy := *value
		copy.Status = lease.LeaseStatusExpired
		s.index.Upsert(copy)
	}
	return values, nil
}

var _ LeaseStore = (*IndexedLeaseStore)(nil)
