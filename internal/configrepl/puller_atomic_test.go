package configrepl

import (
	"context"
	"database/sql"
	"testing"
)

// A failed watermark write must roll back the configuration apply and must
// NOT advance the in-memory version. Otherwise the next pull looks like a
// no-op forever and the follower silently keeps an old configuration.
func TestPullerApplyAndWatermarkAreAtomic(t *testing.T) {
	ctx := context.Background()
	src := newDB(t)
	dst := newDB(t)
	for _, db := range []*sql.DB{src, dst} {
		if _, err := db.Exec(schemaScopes); err != nil {
			t.Fatal(err)
		}
	}
	// State table with an unsatisfiable NOT NULL column: the watermark write
	// fails after the config rows have been applied inside the same tx.
	if _, err := dst.Exec(`CREATE TABLE config_repl_state (
		source_node TEXT PRIMARY KEY, applied_version INTEGER,
		applied_rows INTEGER, last_error TEXT, synced_at DATETIME,
		gate TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	seedScope(t, src, "scope-1", "lan", "10.0.0.0/24")
	srv := authorServer(t, src, "s3cret")
	defer srv.Close()
	p := NewPuller(dst, PullerConfig{SourceURL: srv.URL, Token: "s3cret"}, specs(), nil)

	if applied, err := p.PullOnce(ctx); err == nil || applied {
		t.Fatalf("first pull: applied=%v err=%v; want a state-write error", applied, err)
	}
	if got := countScopeRows(t, dst); got != 0 {
		t.Fatalf("config apply survived a failed watermark: %d scope rows", got)
	}

	// Repair the state table; the same puller must retry from the old version.
	if _, err := dst.Exec(`DROP TABLE config_repl_state`); err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Exec(schemaReplState); err != nil {
		t.Fatal(err)
	}
	if applied, err := p.PullOnce(ctx); err != nil || !applied {
		t.Fatalf("retry pull: applied=%v err=%v; want a fresh apply", applied, err)
	}
	if got := countScopeRows(t, dst); got != 1 {
		t.Fatalf("scope rows after retry = %d, want 1", got)
	}
	var ver int64
	if err := dst.QueryRow(`SELECT applied_version FROM config_repl_state`).Scan(&ver); err != nil {
		t.Fatal(err)
	}
	if ver == 0 {
		t.Fatal("watermark not recorded after retry")
	}

	// A further pull with an unchanged author is a genuine no-op.
	if applied, err := p.PullOnce(ctx); err != nil || applied {
		t.Fatalf("third pull: applied=%v err=%v; want a no-op", applied, err)
	}
}

func countScopeRows(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_scopes`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
