package server

import (
	"testing"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

func TestDefaultDHCPPathKeepsSQLiteAndOptionalMigrationHooksDisabled(t *testing.T) {
	s, db := newDHCPTestServer(t)
	defer db.Close()

	if _, ok := s.leaseMgr.(*lease.Manager); !ok {
		t.Fatalf("default lease store = %T, want SQLite-backed *lease.Manager", s.leaseMgr)
	}
	if s.durableGate != nil {
		t.Fatal("default DHCP constructor unexpectedly enabled WAL durable ACK gate")
	}
	if s.leaseReplicator != nil {
		t.Fatal("default DHCP constructor unexpectedly enabled lease replication")
	}
}

// This test intentionally documents the migration boundary rather than wiring
// the future path: ClaimAvailable belongs to MemoryIndex only and is not part
// of LeaseStore, so HandleDiscover continues to use the SQLite-backed methods
// ReserveAddress/FindAvailableIP until the complete WAL projection contract is
// production-ready.
func TestClaimAvailableRemainsOutsideLeaseStoreContract(t *testing.T) {
	var _ LeaseStore = (*lease.Manager)(nil)
	index := lease.NewMemoryIndex()
	if _, ok := any(index).(LeaseStore); ok {
		t.Fatal("MemoryIndex unexpectedly satisfies the DHCP LeaseStore contract")
	}
}
