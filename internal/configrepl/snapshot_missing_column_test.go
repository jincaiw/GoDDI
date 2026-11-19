package configrepl

import (
	"context"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/dataplane"
)

// A missing table is an intentional "feature not deployed here" signal and
// must remain the ONLY condition that omits a table from the snapshot. A
// missing column is a schema mismatch and must fail closed: swallowing it
// would publish an incomplete snapshot whose fingerprint can match a
// legitimately empty table.
func TestReadSnapshotMissingColumnIsNotTreatedAsAbsentTable(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	if _, err := db.Exec(`CREATE TABLE dhcp_scopes (id TEXT PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO dhcp_scopes(id,name) VALUES ('s-1','lan')`); err != nil {
		t.Fatal(err)
	}
	badSpec := []dataplane.ConfigTableSpec{specByName("dhcp_scopes")}
	if _, err := ReadSnapshot(ctx, db, "node-a", badSpec); err == nil {
		t.Fatal("missing column must fail the snapshot read")
	} else if !strings.Contains(err.Error(), "dhcp_scopes") {
		t.Fatalf("error = %v, want table context", err)
	}
}

func TestReadSnapshotAbsentTableIsStillOmitted(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	snap, err := ReadSnapshot(ctx, db, "node-a", []dataplane.ConfigTableSpec{specByName("dhcp_scopes")})
	if err != nil {
		t.Fatalf("absent table should be skipped: %v", err)
	}
	if len(snap.Tables) != 0 {
		t.Fatalf("tables = %d, want 0", len(snap.Tables))
	}
}
