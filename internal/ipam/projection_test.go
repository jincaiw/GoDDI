package ipam

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dataplane"
)

func newProjectionStore(t *testing.T) *dataplane.Store {
	t.Helper()
	store, err := dataplane.Open(config.DataPlaneLease, filepath.Join(t.TempDir(), "lease.db"))
	if err != nil {
		t.Fatalf("open lease store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// TestProjectionGapsAndGuard proves the D-09 detector lists scopes whose subnet
// is missing from the local IPAM projection, and the opt-in guard fails closed
// for those scopes while passing projected ones.
func TestProjectionGapsAndGuard(t *testing.T) {
	store := newProjectionStore(t)
	db := store.DB
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO dhcp_scopes (id,name,subnet,start_ip,end_ip,enabled) VALUES ('s1','scoped','192.0.2.0/24','192.0.2.10','192.0.2.20',1)`)
	mustExec(`INSERT INTO dhcp_scopes (id,name,subnet,start_ip,end_ip,enabled) VALUES ('s2','unprojected','198.51.100.0/24','198.51.100.10','198.51.100.20',1)`)
	mustExec(`INSERT INTO dhcp_scopes (id,name,subnet,start_ip,end_ip,enabled) VALUES ('s3','disabled','203.0.113.0/24','203.0.113.10','203.0.113.20',0)`)
	mustExec(`INSERT INTO ipam_subnets (id,space_id,cidr) VALUES ('sub1','sp1','192.0.2.0/24')`)

	gaps, err := ProjectionGaps(context.Background(), db)
	if err != nil {
		t.Fatalf("ProjectionGaps: %v", err)
	}
	if len(gaps) != 1 || gaps[0].ScopeID != "s2" {
		t.Fatalf("gaps = %+v, want exactly s2", gaps)
	}

	guard := NewProjectionGuard(db)
	if err := guard("s1", "192.0.2.0/24"); err != nil {
		t.Fatalf("projected scope refused: %v", err)
	}
	if err := guard("s2", "198.51.100.0/24"); !errors.Is(err, ErrProjectionMissing) {
		t.Fatalf("unprojected scope = %v, want ErrProjectionMissing", err)
	}
}
