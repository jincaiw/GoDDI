package server

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestDefaultMutationPathsRetainExplicitMigrationBoundary(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	cases := []struct {
		name   string
		path   string
		must   []string
		forbid []string
	}{
		{
			name: "dhcp handler",
			path: filepath.Join(root, "internal", "dhcp", "server", "handler.go"),
			must: []string{
				"s.leaseMgr.ReserveAddress", "s.leaseMgr.CreateLease", "s.leaseMgr.ActivateLease",
				"s.leaseMgr.RenewLease", "s.leaseMgr.ReleaseLease", "s.leaseMgr.QuarantineIP",
				"resolveLeaseFactSpace", "Caller.BindLease", "Caller.RenewLease", "Caller.DeclineLease",
			},
			forbid: []string{"MutationCommand{", "dhcp_ipam_observation_events"},
		},
		{
			name:   "dhcp expiry",
			path:   filepath.Join(root, "internal", "dhcp", "server", "server.go"),
			must:   []string{"s.ExpireLeasesFromDataPlane()", "ExpireFactsBatch", "resolveLeaseFactSpace"},
			forbid: []string{"MutationCommand{", "dhcp_ipam_observation_events"},
		},
		{
			name: "management delete",
			path: filepath.Join(root, "internal", "api", "handler", "dhcp_lease.go"),
			must: []string{"DHCPServices.MutationOwner.ReleaseLeaseFromManagement(id)"},
			forbid: []string{
				"DHCPServices.LeaseMgr.ReleaseLease(id)",
				"MutationCommand{",
				"enqueueDNSEvent",
				"ObserveLease",
				"replicateLeaseState",
			},
		},
		{
			name: "management owner wiring",
			path: filepath.Join(root, "cmd", "goddi", "main.go"),
			must: []string{"MutationOwner:    dhcpSrv"},
			forbid: []string{
				"MutationOwner:    lease.NewManager(leaseMgrDB)",
			},
		},
		{
			name: "management owner seam",
			path: filepath.Join(root, "internal", "dhcp", "server", "server.go"),
			must: []string{
				"s.leaseMgr.ReleaseLease(id)",
				"return s.leaseMgr.ExpireLeases()",
				"Caller.ReleaseLease",
			},
			forbid: []string{
				"dhcp_ipam_observation_events",
			},
		},
		{
			name:   "management lease view",
			path:   filepath.Join(root, "internal", "api", "handler", "dhcp_scope.go"),
			must:   []string{"LeaseMgr      dhcpserver.LeaseReader"},
			forbid: []string{"LeaseMgr      *lease.Manager"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("read %s: %v", tc.path, err)
			}
			text := string(data)
			for _, pattern := range tc.must {
				if !strings.Contains(text, pattern) {
					t.Fatalf("%s no longer contains boundary evidence %q", tc.path, pattern)
				}
			}
			for _, pattern := range tc.forbid {
				if strings.Contains(text, pattern) {
					t.Fatalf("%s unexpectedly contains production migration hook %q", tc.path, pattern)
				}
			}
		})
	}

	t.Run("management handlers do not bypass the lease owner", func(t *testing.T) {
		handlerDir := filepath.Join(root, "internal", "api", "handler")
		entries, err := os.ReadDir(handlerDir)
		if err != nil {
			t.Fatalf("read %s: %v", handlerDir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(handlerDir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			text := string(data)
			for _, pattern := range []string{
				"lease.NewManager(",
				".CreateLease(",
				".ReserveAddress(",
				".ActivateLease(",
				".RenewLease(",
				".ReleaseLease(",
				".DeclineLease(",
				".ExpireLeases(",
				".QuarantineIP(",
				"INSERT INTO dhcp_leases",
				"UPDATE dhcp_leases",
				"DELETE FROM dhcp_leases",
				"REPLACE INTO dhcp_leases",
			} {
				if strings.Contains(text, pattern) {
					t.Fatalf("%s unexpectedly bypasses the packet-path lease owner with %q", path, pattern)
				}
			}
		}
	})
}
