package server

import (
	"database/sql"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
	_ "modernc.org/sqlite"
)

type leaseStoreProbe struct {
	manager       *lease.Manager
	getLeaseCalls int
}

func (p *leaseStoreProbe) CreateLease(scopeID, ip, mac, hostname string, duration time.Duration) (*lease.Lease, error) {
	return p.manager.CreateLease(scopeID, ip, mac, hostname, duration)
}

func (p *leaseStoreProbe) ReserveAddress(scopeID, ip, mac, hostname string) (*lease.Lease, error) {
	return p.manager.ReserveAddress(scopeID, ip, mac, hostname)
}

func (p *leaseStoreProbe) ActivateLease(id string, duration time.Duration) (*lease.Lease, error) {
	return p.manager.ActivateLease(id, duration)
}

func (p *leaseStoreProbe) RenewLease(id string, duration time.Duration) (*lease.Lease, error) {
	return p.manager.RenewLease(id, duration)
}

func (p *leaseStoreProbe) GetLease(id string) (*lease.Lease, error) {
	p.getLeaseCalls++
	return p.manager.GetLease(id)
}

func (p *leaseStoreProbe) GetLeaseByMAC(mac string) (*lease.Lease, error) {
	return p.manager.GetLeaseByMAC(mac)
}

func (p *leaseStoreProbe) GetHeldLeaseByIP(ip string) (*lease.Lease, error) {
	return p.manager.GetHeldLeaseByIP(ip)
}

func (p *leaseStoreProbe) FindAvailableIP(scopeID string) (string, error) {
	return p.manager.FindAvailableIP(scopeID)
}

func (p *leaseStoreProbe) ReleaseLease(id string) error { return p.manager.ReleaseLease(id) }

func (p *leaseStoreProbe) QuarantineIP(scopeID, ip, mac string) (*lease.Lease, error) {
	return p.manager.QuarantineIP(scopeID, ip, mac)
}

func (p *leaseStoreProbe) ExpireLeases() ([]*lease.Lease, error) { return p.manager.ExpireLeases() }

func TestNewWithLeaseStoreInjectsPacketPathStore(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	probe := &leaseStoreProbe{manager: lease.NewManager(db)}
	s := NewWithLeaseStore(probe, db, []string{"eth0"}, nil)
	if s.leaseMgr != probe {
		t.Fatal("server did not retain the injected lease store")
	}
	if _, err := s.leaseMgr.GetLease("missing"); err == nil {
		t.Fatal("missing lease unexpectedly returned nil error")
	}
	if probe.getLeaseCalls != 1 {
		t.Fatalf("GetLease calls = %d, want 1", probe.getLeaseCalls)
	}
}

func TestNewWithLeaseStoreRejectsNilStore(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("nil lease store did not panic")
		}
	}()
	_ = NewWithLeaseStore(nil, db, nil, nil)
}
