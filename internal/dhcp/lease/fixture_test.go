package lease

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dataplane"
)

// newLeaseStore opens the lease store the product ships, for a test.
//
// Not a hand-written copy of the schema, and the reason is specific to what
// these tests are for. The lease manager now writes an audit row for every
// state change it makes, and a fixture that lacks the audit table does not fail
// -- the write is logged and dropped, exactly as it is in production when the
// database is unhappy. A fixture that is a table behind therefore turns a
// working feature into silence, and silence is what tests exist to replace.
//
// The store is the real one: real migrations, real indexes, real durability
// pragmas. Tests seed the scope rows they need on top.
func newLeaseStore(t *testing.T) *dataplane.Store {
	t.Helper()

	store, err := dataplane.Open(config.DataPlaneLease, filepath.Join(t.TempDir(), "leases.db"))
	if err != nil {
		t.Fatalf("open lease store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// seedScope inserts one allocation pool. The store's dhcp_scopes has more
// columns than any test needs; a scope with no name or subnet would not be a
// scope, so those are always supplied.
func seedScope(t *testing.T, db *sql.DB, id, name, subnet, from, to string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip, enabled)
		VALUES (?, ?, ?, ?, ?, 1)`, id, name, subnet, from, to); err != nil {
		t.Fatalf("seed scope %s: %v", id, err)
	}
}
