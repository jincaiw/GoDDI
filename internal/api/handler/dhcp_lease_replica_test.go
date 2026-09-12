package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dataplane"
	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

// These tests pin the console's write guard on the lease replica.
//
// When the data plane runs in its own process it owns the lease rows; the
// console reads a copy that is overwritten on every replication pass. A release
// issued against that copy reports success and is then silently undone, which
// is worse than a refusal: the operator believes the address is free. The guard
// is only worth having if something fails when it is removed, so both halves
// are asserted -- refused when the rows are a copy, released when they are not.
//
// The table comes from the real data-plane schema rather than a hand-written
// one, so this test fails for the reason under test and not because a column
// was mistyped here.

// newLeaseReplicaTestDB opens a real lease-plane store and inserts one active
// lease, so a release has something to act on.
func newLeaseReplicaTestDB(t *testing.T) *sql.DB {
	t.Helper()
	store, err := dataplane.Open(config.DataPlaneLease, filepath.Join(t.TempDir(), "leases.db"))
	if err != nil {
		t.Fatalf("opening the lease store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	if _, err := store.DB.Exec(`INSERT INTO dhcp_leases
		(id, scope_id, ip_address, mac_address, hostname, client_id,
		 lease_start, lease_end, status, last_seen, generation)
		VALUES ('l1', 's1', '10.0.0.10', 'aa:bb:cc:dd:ee:01', 'host1', NULL,
		 '2026-01-01T00:00:00Z', '2030-01-01T00:00:00Z', 'active', '2026-01-01T00:00:00Z', 1)`); err != nil {
		t.Fatalf("inserting the lease: %v", err)
	}
	return store.DB
}

func leaseStatusIn(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var status string
	if err := db.QueryRow(`SELECT status FROM dhcp_leases WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatalf("reading lease %s: %v", id, err)
	}
	return status
}

// withDHCPServices installs a container for the duration of one test. It
// assigns the package variable directly rather than going through
// InitDHCPServices, which is guarded by a sync.Once and would therefore ignore
// every call after the first.
func withDHCPServices(t *testing.T, svc *DHCPServiceContainer) {
	t.Helper()
	previous := DHCPServices
	DHCPServices = svc
	t.Cleanup(func() { DHCPServices = previous })
}

func deleteLeaseRequest(id string) *http.Request {
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/dhcp/leases/"+id, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestTheConsoleRefusesToReleaseALeaseItOnlyHasACopyOf(t *testing.T) {
	db := newLeaseReplicaTestDB(t)
	withDHCPServices(t, &DHCPServiceContainer{
		LeaseMgr:         lease.NewManager(db),
		LeasesAreReplica: true,
	})

	rec := httptest.NewRecorder()
	DeleteDHCPLease(rec, deleteLeaseRequest("l1"))

	if rec.Code != http.StatusConflict {
		t.Fatalf("releasing a replicated lease = %d, want %d", rec.Code, http.StatusConflict)
	}
	if got := leaseStatusIn(t, db, "l1"); got != string(lease.LeaseStatusActive) {
		t.Fatalf("lease status after a refused release = %q, want %q: the write went through anyway",
			got, lease.LeaseStatusActive)
	}
	if body := rec.Body.String(); !strings.Contains(body, "数据面") {
		t.Fatalf("refusal does not say where the lease lives: %s", body)
	}
}

func TestTheConsoleReleasesALeaseItOwns(t *testing.T) {
	db := newLeaseReplicaTestDB(t)
	withDHCPServices(t, &DHCPServiceContainer{LeaseMgr: lease.NewManager(db)})

	rec := httptest.NewRecorder()
	DeleteDHCPLease(rec, deleteLeaseRequest("l1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("releasing an owned lease = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := leaseStatusIn(t, db, "l1"); got != string(lease.LeaseStatusReleased) {
		t.Fatalf("lease status after a release = %q, want %q", got, lease.LeaseStatusReleased)
	}
}

// TestListDHCPLeasesStillReadsAReplica guards the other half of the decision:
// reads are not refused. A console that cannot even show the leases it is not
// allowed to change would be unusable.
func TestListDHCPLeasesStillReadsAReplica(t *testing.T) {
	db := newLeaseReplicaTestDB(t)
	withDHCPServices(t, &DHCPServiceContainer{
		LeaseMgr:         lease.NewManager(db),
		LeasesAreReplica: true,
	})

	rec := httptest.NewRecorder()
	ListDHCPLeases(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dhcp/leases", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("listing replicated leases = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "l1") {
		t.Fatalf("the list does not contain the lease that exists: %s", rec.Body.String())
	}
}
