package server

import (
	"time"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

// LeaseStore is the narrow DHCP data-plane contract. It deliberately contains
// only operations used by packet handling and expiry processing; management
// listing, utilization, and SQL-specific maintenance stay outside this seam.
//
// The current production implementation is still lease.Manager (SQLite-backed).
// This interface is an injection boundary for the staged LeaseStore migration;
// it does not, by itself, imply memory durability or WAL-backed ACK semantics.
type LeaseStore interface {
	CreateLease(scopeID, ip, mac, hostname string, duration time.Duration) (*lease.Lease, error)
	ReserveAddress(scopeID, ip, mac, hostname string) (*lease.Lease, error)
	ActivateLease(id string, duration time.Duration) (*lease.Lease, error)
	RenewLease(id string, duration time.Duration) (*lease.Lease, error)

	GetLease(id string) (*lease.Lease, error)
	GetLeaseByMAC(mac string) (*lease.Lease, error)
	GetHeldLeaseByIP(ip string) (*lease.Lease, error)
	FindAvailableIP(scopeID string) (string, error)

	ReleaseLease(id string) error
	QuarantineIP(scopeID, ip, mac string) (*lease.Lease, error)
	ExpireLeases() ([]*lease.Lease, error)
}

var _ LeaseStore = (*lease.Manager)(nil)
