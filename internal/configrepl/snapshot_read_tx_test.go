package configrepl

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// A snapshot that spans several tables must be read from one point in time.
// The hook mutates the source after the first table is read: with a single
// read transaction the second table still sees the old value; without one,
// the second query observes the new commit and the snapshot is torn.
func TestReadSnapshotIsPointInTimeAcrossTables(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "source.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(2) // one for the read tx, one for the concurrent write
	for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON"} {
		if _, err := db.Exec(pragma); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(controlIPAMSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schemaScopes); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_spaces(id,name) VALUES ('sp-1','lab')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO dhcp_scopes(id,name,subnet) VALUES ('scope-1','old','10.0.0.0/24')`); err != nil {
		t.Fatal(err)
	}

	prev := readSnapshotAfterTableForTest
	defer func() { readSnapshotAfterTableForTest = prev }()
	fired := false
	readSnapshotAfterTableForTest = func(name string) {
		if name != "ipam_spaces" || fired {
			return
		}
		fired = true
		if _, err := db.ExecContext(ctx, `UPDATE dhcp_scopes SET name='new' WHERE id='scope-1'`); err != nil {
			t.Errorf("concurrent write: %v", err)
		}
	}

	snap, err := ReadSnapshot(ctx, db, "node-a", specs())
	if err != nil {
		t.Fatal(err)
	}
	if !fired {
		t.Fatal("hook never fired; spec order changed?")
	}
	var scopeName string
	for _, tbl := range snap.Tables {
		if tbl.Name != "dhcp_scopes" {
			continue
		}
		for _, row := range tbl.Rows {
			if row[0] == "scope-1" {
				scopeName, _ = row[1].(string)
			}
		}
	}
	if scopeName != "old" {
		t.Fatalf("torn snapshot: dhcp_scopes name = %q, want the pre-commit value old", scopeName)
	}
}
