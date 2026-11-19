package configrepl

import (
	"context"
	"database/sql"
	"testing"
)

// The onApplied callback wakes the DNS/DHCP runners. It must not fire on a
// failed or uncommitted apply: a runner woken before the watermark commit can
// observe a half-updated state, and the console can report a version that was
// never durable.
func TestPullerOnAppliedOnlyAfterWatermarkCommit(t *testing.T) {
	ctx := context.Background()
	src := newDB(t)
	dst := newDB(t)
	for _, db := range []*sql.DB{src, dst} {
		if _, err := db.Exec(schemaScopes); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := dst.Exec(`CREATE TABLE config_repl_state (
		source_node TEXT PRIMARY KEY, applied_version INTEGER, applied_rows INTEGER,
		last_error TEXT, synced_at DATETIME, gate TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	seedScope(t, src, "scope-1", "lan", "10.0.0.0/24")
	srv := authorServer(t, src, "s3cret")
	defer srv.Close()
	wakes := 0
	p := NewPuller(dst, PullerConfig{SourceURL: srv.URL, Token: "s3cret"}, specs(), func() { wakes++ })
	if _, err := p.PullOnce(ctx); err == nil {
		t.Fatal("state write should fail")
	}
	if wakes != 0 {
		t.Fatalf("onApplied fired %d times before commit", wakes)
	}

	if _, err := dst.Exec(`DROP TABLE config_repl_state`); err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Exec(schemaReplState); err != nil {
		t.Fatal(err)
	}
	if applied, err := p.PullOnce(ctx); err != nil || !applied {
		t.Fatalf("retry: applied=%v err=%v", applied, err)
	}
	if wakes != 1 {
		t.Fatalf("onApplied fired %d times after successful commit, want 1", wakes)
	}
	var ver int64
	if err := dst.QueryRow(`SELECT applied_version FROM config_repl_state`).Scan(&ver); err != nil {
		t.Fatal(err)
	}
	if ver == 0 {
		t.Fatal("watermark not committed")
	}
}
