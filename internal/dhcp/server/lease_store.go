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
// LeaseReader is the read-only lease view used by the management API. Keeping
// mutations out of this interface makes the control plane unable to write a
// lease through its normal service container.
type LeaseReader interface {
	ListLeases(filter lease.LeaseFilter) ([]lease.Lease, int64, error)
	GetLease(id string) (*lease.Lease, error)
}

// LeaseMutationOwner is the explicit owner seam for management-plane and
// maintenance lease mutations. A control process may read a lease replica, but
// it must not write one unless the packet-path owner is installed here.
type LeaseMutationOwner interface {
	ReleaseLeaseFromManagement(id string) error
	ExpireLeasesFromDataPlane() ([]*lease.Lease, error)
}

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
