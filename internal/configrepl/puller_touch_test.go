package configrepl

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

// A healthy idle follower must keep its replication health clock fresh: the
// second pull is a no-op for the data but still has to advance synced_at and
// clear a transient error, or the console raises a false "stale replica" alarm.
func TestPullerNoOpTouchRefreshesHealthClock(t *testing.T) {
	ctx := context.Background()
	src := newDB(t)
	dst := newDB(t)
	for _, db := range []*sql.DB{src, dst} {
		if _, err := db.Exec(schemaScopes); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := dst.Exec(schemaReplState); err != nil {
		t.Fatal(err)
	}
	seedScope(t, src, "scope-1", "lan", "10.0.0.0/24")
	srv := authorServer(t, src, "s3cret")
	defer srv.Close()

	p := NewPuller(dst, PullerConfig{SourceURL: srv.URL, Token: "s3cret"}, specs(), nil)
	if applied, err := p.PullOnce(ctx); err != nil || !applied {
		t.Fatalf("first pull: applied=%v err=%v", applied, err)
	}
	// Simulate an old watermark plus a transient error.
	old := time.Now().UTC().Add(-10 * time.Minute).Format("2006-01-02 15:04:05")
	if _, err := dst.Exec(`UPDATE config_repl_state SET synced_at = ?, last_error = 'author unreachable'`, old); err != nil {
		t.Fatal(err)
	}
	if applied, err := p.PullOnce(ctx); err != nil || applied {
		t.Fatalf("second pull must be a no-op: applied=%v err=%v", applied, err)
	}
	var syncedAt, lastError string
	var expectedVersion int64
	if err := dst.QueryRow(`SELECT synced_at, last_error, expected_version FROM config_repl_state`).Scan(&syncedAt, &lastError, &expectedVersion); err != nil {
		t.Fatal(err)
	}
	if lastError != "" {
		t.Fatalf("transient error not cleared: %q", lastError)
	}
	if expectedVersion == 0 {
		t.Fatal("no-op pull did not record the author version")
	}
	if syncedAt == old {
		t.Fatal("no-op pull did not refresh synced_at")
	}
	// modernc/sqlite returns DATETIME columns as time.Time, so a string scan
	// can yield either SQLite's 'YYYY-MM-DD HH:MM:SS' or RFC3339.
	var parsed time.Time
	var err error
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339Nano, time.RFC3339} {
		parsed, err = time.ParseInLocation(layout, syncedAt, time.UTC)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("synced_at %q not parseable: %v", syncedAt, err)
	}
	if time.Since(parsed) > time.Minute {
		t.Fatalf("synced_at not fresh: %s", syncedAt)
	}
}

// A transient error must not zero the applied-rows watermark.
func TestRecordErrorPreservesAppliedRows(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	if _, err := db.Exec(schemaReplState); err != nil {
		t.Fatal(err)
	}
	src := "http://node-a:6080"
	if _, err := db.Exec(`INSERT INTO config_repl_state
		(source_node, applied_version, applied_rows, last_error, synced_at)
		VALUES (?, 9, 20, '', datetime('now'))`, src); err != nil {
		t.Fatal(err)
	}
	p := NewPuller(db, PullerConfig{SourceURL: src}, specs(), nil)
	p.recordError(ctx, errors.New("author unreachable"))
	var rows int64
	var lastError string
	if err := db.QueryRow(`SELECT applied_rows, last_error FROM config_repl_state`).Scan(&rows, &lastError); err != nil {
		t.Fatal(err)
	}
	if rows != 20 {
		t.Fatalf("applied_rows = %d after error, want 20", rows)
	}
	if lastError != "author unreachable" {
		t.Fatalf("last_error = %q", lastError)
	}
}
