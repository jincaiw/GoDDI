package configrepl

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
)

const schemaReplState = `CREATE TABLE config_repl_state (
	source_node TEXT PRIMARY KEY, applied_version INTEGER NOT NULL DEFAULT 0,
	applied_rows INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '',
	synced_at DATETIME, expected_version INTEGER NOT NULL DEFAULT 0)`

// authorServer stands in for the author node's snapshot endpoint.
func authorServer(t *testing.T, src *sql.DB, token string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/cluster/config/snapshot" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Goddi-Replication-Token") != token {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		snap, err := ReadSnapshot(r.Context(), src, "node-a", specs())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		body, _ := EncodeSnapshotJSON(snap)
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
}

// The follower pulls the author's scope and applies it; a second pull with no
// change is a no-op. This is the end-to-end contract P5 relies on.
func TestPullerAppliesScopeThenNoOps(t *testing.T) {
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
	if _, err := src.Exec(`CREATE TABLE config_release_outbox (id INTEGER PRIMARY KEY AUTOINCREMENT, revision_id TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(`INSERT INTO config_release_outbox(revision_id) VALUES ('r1')`); err != nil {
		t.Fatal(err)
	}
	seedScope(t, src, "scope-1", "lan", "10.0.0.0/24")

	srv := authorServer(t, src, "s3cret")
	defer srv.Close()

	p := NewPuller(dst, PullerConfig{SourceURL: srv.URL, Token: "s3cret"}, specs(), nil)
	applied, err := p.PullOnce(context.Background())
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if !applied {
		t.Fatalf("first pull should apply")
	}
	var name string
	if err := dst.QueryRow(`SELECT name FROM dhcp_scopes WHERE id='scope-1'`).Scan(&name); err != nil {
		t.Fatalf("scope did not land on follower: %v", err)
	}
	// Watermark recorded.
	var ver int64
	if err := dst.QueryRow(`SELECT applied_version FROM config_repl_state`).Scan(&ver); err != nil {
		t.Fatalf("state: %v", err)
	}
	if ver == 0 {
		t.Fatalf("applied_version = 0, want the applied content fingerprint")
	}
	// Second pull, no change: no-op.
	applied, err = p.PullOnce(context.Background())
	if err != nil {
		t.Fatalf("second pull: %v", err)
	}
	if applied {
		t.Fatalf("second pull with an unchanged version must be a no-op")
	}
}

// A bad token is refused and the follower applies nothing.
func TestPullerRejectsBadToken(t *testing.T) {
	src := newDB(t)
	dst := newDB(t)
	for _, db := range []*sql.DB{src, dst} {
		if _, err := db.Exec(schemaScopes); err != nil {
			t.Fatal(err)
		}
	}
	srv := authorServer(t, src, "s3cret")
	defer srv.Close()
	p := NewPuller(dst, PullerConfig{SourceURL: srv.URL, Token: "wrong"}, specs(), nil)
	if _, err := p.PullOnce(context.Background()); err == nil {
		t.Fatalf("pull with a bad token must error")
	}
}
