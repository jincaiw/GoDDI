package server

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

type shadowStore struct {
	mu           sync.Mutex
	lease        *lease.Lease
	available    string
	err          error
	calls        int
	availableErr error
}

func (s *shadowStore) CreateLease(string, string, string, string, time.Duration) (*lease.Lease, error) {
	return s.lease, s.err
}
func (s *shadowStore) ReserveAddress(string, string, string, string) (*lease.Lease, error) {
	return s.lease, s.err
}
func (s *shadowStore) ActivateLease(string, time.Duration) (*lease.Lease, error) {
	return s.lease, s.err
}
func (s *shadowStore) RenewLease(string, time.Duration) (*lease.Lease, error) { return s.lease, s.err }
func (s *shadowStore) GetLease(string) (*lease.Lease, error)                  { return s.read() }
func (s *shadowStore) GetLeaseByMAC(string) (*lease.Lease, error)             { return s.read() }
func (s *shadowStore) GetHeldLeaseByIP(string) (*lease.Lease, error)          { return s.read() }
func (s *shadowStore) FindAvailableIP(string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.available, s.availableErr
}
func (s *shadowStore) ReleaseLease(string) error { return s.err }
func (s *shadowStore) QuarantineIP(string, string, string) (*lease.Lease, error) {
	return s.lease, s.err
}
func (s *shadowStore) ExpireLeases() ([]*lease.Lease, error) { return nil, s.err }
func (s *shadowStore) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *shadowStore) read() (*lease.Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return cloneLease(s.lease), s.err
}

func TestReadShadowLeaseStoreKeepsPrimaryReadAndReportsMatch(t *testing.T) {
	want := &lease.Lease{ID: "l1", IPAddress: "192.0.2.10", Status: lease.LeaseStatusActive, Generation: 2}
	primary := &shadowStore{lease: want, available: "192.0.2.11"}
	candidate := &shadowStore{lease: cloneLease(want), available: "192.0.2.11"}
	var mu sync.Mutex
	var reports []ReadShadowMismatch
	shadow := NewReadShadowLeaseStore(primary, candidate, 4, func(report ReadShadowMismatch) {
		mu.Lock()
		reports = append(reports, report)
		mu.Unlock()
	})
	defer shadow.Close()

	got, err := shadow.GetHeldLeaseByIP(want.IPAddress)
	if err != nil || !sameLease(got, want) {
		t.Fatalf("primary result = %#v, %v; want %#v, nil", got, err, want)
	}
	if _, err := shadow.FindAvailableIP("scope-1"); err != nil {
		t.Fatalf("find available: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for candidate.callCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	shadow.Close()
	mu.Lock()
	defer mu.Unlock()
	if len(reports) != 0 {
		t.Fatalf("reports = %+v, want no mismatch", reports)
	}
	if candidate.calls != 2 {
		t.Fatalf("candidate calls = %d, want 2", candidate.calls)
	}
}

func TestReadShadowLeaseStoreReportsMismatchWithoutChangingPrimary(t *testing.T) {
	primaryLease := &lease.Lease{ID: "primary", IPAddress: "192.0.2.10", Status: lease.LeaseStatusActive, Generation: 1}
	candidateLease := &lease.Lease{ID: "candidate", IPAddress: "192.0.2.10", Status: lease.LeaseStatusActive, Generation: 1}
	primary := &shadowStore{lease: primaryLease, available: "192.0.2.12"}
	candidate := &shadowStore{lease: candidateLease, available: "192.0.2.13"}
	reports := make(chan ReadShadowMismatch, 2)
	shadow := NewReadShadowLeaseStore(primary, candidate, 4, func(report ReadShadowMismatch) { reports <- report })
	defer shadow.Close()

	got, err := shadow.GetLease("l1")
	if err != nil || !sameLease(got, primaryLease) {
		t.Fatalf("primary result = %#v, %v", got, err)
	}
	select {
	case report := <-reports:
		if report.Operation != "GetLease" || report.Authoritative != "primary/192.0.2.10/active/g1" || report.Candidate != "candidate/192.0.2.10/active/g1" {
			t.Fatalf("report = %+v", report)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for mismatch report")
	}
}

func TestReadShadowLeaseStoreNeverReturnsCandidateError(t *testing.T) {
	primary := &shadowStore{available: "192.0.2.10"}
	candidate := &shadowStore{availableErr: errors.New("candidate unavailable")}
	shadow := NewReadShadowLeaseStore(primary, candidate, 1, func(ReadShadowMismatch) {})
	defer shadow.Close()

	got, err := shadow.FindAvailableIP("scope-1")
	if got != "192.0.2.10" || err != nil {
		t.Fatalf("primary result = %q, %v; candidate error leaked", got, err)
	}
}

func TestReadShadowLeaseStoreDropsWhenQueueFull(t *testing.T) {
	primary := &shadowStore{available: "192.0.2.10"}
	candidate := &shadowStore{available: "192.0.2.10"}
	block := make(chan struct{})
	shadow := NewReadShadowLeaseStore(primary, candidate, 1, func(ReadShadowMismatch) { <-block })
	defer func() {
		close(block)
		shadow.Close()
	}()

	for i := 0; i < 10; i++ {
		_, _ = shadow.FindAvailableIP("scope-1")
	}
	if shadow.DroppedReports() == 0 {
		t.Fatal("full comparison queue did not report dropped jobs")
	}
}

func TestReadShadowLeaseStoreWithoutCandidatePreservesPrimary(t *testing.T) {
	primary := &shadowStore{available: "192.0.2.10"}
	shadow := NewReadShadowLeaseStore(primary, nil, 4, nil)
	defer shadow.Close()

	got, err := shadow.FindAvailableIP("scope-1")
	if got != "192.0.2.10" || err != nil {
		t.Fatalf("primary result = %q, %v", got, err)
	}
	if shadow.DroppedReports() != 1 {
		t.Fatalf("dropped reports = %d, want 1", shadow.DroppedReports())
	}
}
