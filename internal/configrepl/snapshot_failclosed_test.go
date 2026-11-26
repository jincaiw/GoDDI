package configrepl

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jincaiw/GoDDI/internal/dataplane"
)

// A malformed snapshot row must be refused before any existing follower row
// is pruned. ApplySnapshot collects keys before the DELETE and rolls back
// the whole transaction on error.
func TestApplySnapshotMalformedRowIsFailClosed(t *testing.T) {
	ctx := context.Background()
	src := newDB(t)
	dst := newDB(t)
	for _, db := range []*sql.DB{src, dst} {
		if _, err := db.Exec(schemaScopes); err != nil {
			t.Fatal(err)
		}
	}
	seedScope(t, dst, "scope-keep", "keep", "10.0.0.0/24")
	seedScope(t, src, "scope-1", "lan", "10.0.0.0/24")
	snap, err := ReadSnapshot(ctx, src, "node-a", specs())
	if err != nil {
		t.Fatal(err)
	}
	for i := range snap.Tables {
		if snap.Tables[i].Name == "dhcp_scopes" && len(snap.Tables[i].Rows) > 0 {
			snap.Tables[i].Rows[0] = snap.Tables[i].Rows[0][:1]
		}
	}
	if _, err := ApplySnapshot(ctx, dst, snap, specs()); err == nil {
		t.Fatal("apply must reject a row whose width does not match the spec")
	}
	if got := countScopeRows(t, dst); got != 1 {
		t.Fatalf("follower rows = %d after failed apply, want the pre-existing 1", got)
	}
	var name string
	if err := dst.QueryRow(`SELECT name FROM dhcp_scopes WHERE id='scope-keep'`).Scan(&name); err != nil {
		t.Fatalf("pre-existing row was changed: %v", err)
	}
	if name != "keep" {
		t.Fatalf("pre-existing row name = %q, want keep", name)
	}
}

// An unknown replace mode is a contract violation, not a reason to fall back
// to a destructive default.
func TestApplySnapshotRejectsUnknownReplaceMode(t *testing.T) {
	ctx := context.Background()
	dst := newDB(t)
	if _, err := dst.Exec(schemaScopes); err != nil {
		t.Fatal(err)
	}
	seedScope(t, dst, "scope-keep", "keep", "10.0.0.0/24")
	snap := Snapshot{Version: 1, NodeID: "node-a", Tables: []TableData{
		{Name: "dhcp_scopes", Rows: [][]any{{"x", "y"}}},
	}}
	bad := append([]dataplane.ConfigTableSpec{}, specByName("dhcp_scopes"))
	bad[0].Mode = "delete_everything"
	if _, err := ApplySnapshot(ctx, dst, snap, bad); err == nil {
		t.Fatal("apply must reject an unknown replace mode")
	}
	if got := countScopeRows(t, dst); got != 1 {
		t.Fatalf("follower rows = %d after rejected mode, want 1", got)
	}
}

// A table the author does not carry is not evidence that the follower should
// delete its local copy. ApplySnapshot skips absent tables entirely.
func TestApplySnapshotMissingTableKeepsLocalCopy(t *testing.T) {
	ctx := context.Background()
	dst := newDB(t)
	if _, err := dst.Exec(`CREATE TABLE local_only (id TEXT PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Exec(`INSERT INTO local_only(id,name) VALUES ('keep','local')`); err != nil {
		t.Fatal(err)
	}
	snap := Snapshot{Version: 1, NodeID: "node-a"}
	localSpec := []dataplane.ConfigTableSpec{{Name: "local_only", Columns: []string{"id", "name"}, Key: "id", Mode: dataplane.ConfigReplaceWhole}}
	if _, err := ApplySnapshot(ctx, dst, snap, localSpec); err != nil {
		t.Fatalf("apply: %v", err)
	}
	var name string
	if err := dst.QueryRow(`SELECT name FROM local_only WHERE id='keep'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "local" {
		t.Fatalf("local-only row = %q, want local", name)
	}
}
