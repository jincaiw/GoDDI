package server

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

// ReadShadowMismatch describes a difference between the authoritative result
// and a candidate read store. The authoritative store always wins; this event
// is diagnostic evidence for the staged LeaseStore migration.
type ReadShadowMismatch struct {
	Operation        string
	Arguments        string
	Authoritative    string
	Candidate        string
	AuthoritativeErr string
	CandidateErr     string
}

// ReadShadowLeaseStore keeps the primary store authoritative and compares
// selected reads against a candidate asynchronously. Writes are sent only to
// the primary store. The bounded queue and non-blocking admission ensure that
// a slow or broken candidate cannot delay DHCP handling or change ACK results.
type ReadShadowLeaseStore struct {
	primary   LeaseStore
	candidate LeaseStore
	reports   chan func()
	report    func(ReadShadowMismatch)
	dropped   atomic.Uint64
	stop      chan struct{}
	done      chan struct{}
	stopOnce  sync.Once
}

// NewReadShadowLeaseStore starts a bounded diagnostic comparison worker. A
// non-positive queueCapacity disables comparisons while preserving the primary
// store behavior.
func NewReadShadowLeaseStore(primary, candidate LeaseStore, queueCapacity int, report func(ReadShadowMismatch)) *ReadShadowLeaseStore {
	if primary == nil {
		panic("dhcp server: nil primary lease store")
	}
	s := &ReadShadowLeaseStore{
		primary:   primary,
		candidate: candidate,
		report:    report,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	if candidate == nil || queueCapacity <= 0 || report == nil {
		close(s.done)
		return s
	}
	s.reports = make(chan func(), queueCapacity)
	go s.reportWorker()
	return s
}

// Close stops the comparison worker. It never affects the primary store.
func (s *ReadShadowLeaseStore) Close() {
	s.stopOnce.Do(func() {
		close(s.stop)
		<-s.done
	})
}

// DroppedReports reports comparison jobs rejected because the bounded queue
// was full or comparisons were disabled.
func (s *ReadShadowLeaseStore) DroppedReports() uint64 { return s.dropped.Load() }

func (s *ReadShadowLeaseStore) reportWorker() {
	defer close(s.done)
	for {
		select {
		case job := <-s.reports:
			job()
		case <-s.stop:
			return
		}
	}
}

func (s *ReadShadowLeaseStore) enqueue(job func()) {
	if s.reports == nil {
		s.dropped.Add(1)
		return
	}
	select {
	case s.reports <- job:
	default:
		s.dropped.Add(1)
	}
}

func (s *ReadShadowLeaseStore) emit(event ReadShadowMismatch) {
	if s.report != nil {
		s.report(event)
	}
}

func (s *ReadShadowLeaseStore) compareLease(op, args string, primary *lease.Lease, primaryErr error, candidate *lease.Lease, candidateErr error) {
	if !sameError(primaryErr, candidateErr) || !sameLease(primary, candidate) {
		s.emit(ReadShadowMismatch{
			Operation:        op,
			Arguments:        args,
			Authoritative:    leaseSummary(primary),
			Candidate:        leaseSummary(candidate),
			AuthoritativeErr: errorSummary(primaryErr),
			CandidateErr:     errorSummary(candidateErr),
		})
	}
}

func (s *ReadShadowLeaseStore) shadowLease(op, args string, primary *lease.Lease, primaryErr error, read func() (*lease.Lease, error)) {
	if s.candidate == nil || s.report == nil {
		s.dropped.Add(1)
		return
	}
	primaryCopy := cloneLease(primary)
	s.enqueue(func() {
		candidate, candidateErr := read()
		s.compareLease(op, args, primaryCopy, primaryErr, candidate, candidateErr)
	})
}

func (s *ReadShadowLeaseStore) GetLease(id string) (*lease.Lease, error) {
	value, err := s.primary.GetLease(id)
	s.shadowLease("GetLease", id, value, err, func() (*lease.Lease, error) { return s.candidate.GetLease(id) })
	return value, err
}

func (s *ReadShadowLeaseStore) GetLeaseByMAC(mac string) (*lease.Lease, error) {
	value, err := s.primary.GetLeaseByMAC(mac)
	s.shadowLease("GetLeaseByMAC", mac, value, err, func() (*lease.Lease, error) {
		return s.candidate.GetLeaseByMAC(mac)
	})
	return value, err
}

func (s *ReadShadowLeaseStore) GetHeldLeaseByIP(ip string) (*lease.Lease, error) {
	value, err := s.primary.GetHeldLeaseByIP(ip)
	s.shadowLease("GetHeldLeaseByIP", ip, value, err, func() (*lease.Lease, error) {
		return s.candidate.GetHeldLeaseByIP(ip)
	})
	return value, err
}

func (s *ReadShadowLeaseStore) FindAvailableIP(scopeID string) (string, error) {
	value, err := s.primary.FindAvailableIP(scopeID)
	if s.candidate == nil || s.report == nil {
		s.dropped.Add(1)
		return value, err
	}
	s.enqueue(func() {
		candidate, candidateErr := s.candidate.FindAvailableIP(scopeID)
		if value != candidate || !sameError(err, candidateErr) {
			s.emit(ReadShadowMismatch{
				Operation:        "FindAvailableIP",
				Arguments:        scopeID,
				Authoritative:    value,
				Candidate:        candidate,
				AuthoritativeErr: errorSummary(err),
				CandidateErr:     errorSummary(candidateErr),
			})
		}
	})
	return value, err
}

func (s *ReadShadowLeaseStore) CreateLease(scopeID, ip, mac, hostname string, duration time.Duration) (*lease.Lease, error) {
	return s.primary.CreateLease(scopeID, ip, mac, hostname, duration)
}
func (s *ReadShadowLeaseStore) ReserveAddress(scopeID, ip, mac, hostname string) (*lease.Lease, error) {
	return s.primary.ReserveAddress(scopeID, ip, mac, hostname)
}
func (s *ReadShadowLeaseStore) ActivateLease(id string, duration time.Duration) (*lease.Lease, error) {
	return s.primary.ActivateLease(id, duration)
}
func (s *ReadShadowLeaseStore) RenewLease(id string, duration time.Duration) (*lease.Lease, error) {
	return s.primary.RenewLease(id, duration)
}
func (s *ReadShadowLeaseStore) ReleaseLease(id string) error { return s.primary.ReleaseLease(id) }
func (s *ReadShadowLeaseStore) QuarantineIP(scopeID, ip, mac string) (*lease.Lease, error) {
	return s.primary.QuarantineIP(scopeID, ip, mac)
}
func (s *ReadShadowLeaseStore) ExpireLeases() ([]*lease.Lease, error) {
	return s.primary.ExpireLeases()
}

func cloneLease(value *lease.Lease) *lease.Lease {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func sameLease(a, b *lease.Lease) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameError(a, b error) bool {
	if a == nil || b == nil {
		return a == b
	}
	return errors.Is(a, b) || errors.Is(b, a) || a.Error() == b.Error()
}

func errorSummary(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func leaseSummary(value *lease.Lease) string {
	if value == nil {
		return "<nil>"
	}
	return value.ID + "/" + value.IPAddress + "/" + string(value.Status) + "/g" + formatInt(value.Generation)
}

func formatInt(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
