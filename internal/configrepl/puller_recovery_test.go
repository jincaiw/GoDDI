package configrepl

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// An author outage must be visible as an error, and a later successful pull
// must clear it in the same durable state the console reads.
func TestPullerClearsErrorAfterAuthorRecovers(t *testing.T) {
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

	failing := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing {
			http.Error(w, "author temporarily down", http.StatusInternalServerError)
			return
		}
		snap, err := ReadSnapshot(r.Context(), src, "node-a", specs())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		body, _ := json.Marshal(snap)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	p := NewPuller(dst, PullerConfig{SourceURL: srv.URL, Token: "t"}, specs(), nil)
	p.recordError(ctx, errors.New("author temporarily down"))
	var before string
	if err := dst.QueryRow(`SELECT last_error FROM config_repl_state`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before == "" {
		t.Fatal("outage was not recorded")
	}

	failing = false
	applied, err := p.PullOnce(ctx)
	if err != nil || !applied {
		t.Fatalf("recovery pull: applied=%v err=%v", applied, err)
	}

	var lastErr string
	var version, rows int64
	if err := dst.QueryRow(`SELECT last_error, applied_version, applied_rows FROM config_repl_state`).
		Scan(&lastErr, &version, &rows); err != nil {
		t.Fatal(err)
	}
	if lastErr != "" {
		t.Fatalf("error not cleared after recovery: %q", lastErr)
	}
	if version == 0 || rows == 0 {
		t.Fatalf("watermark not advanced: version=%d rows=%d", version, rows)
	}
	if got := countScopeRows(t, dst); got != 1 {
		t.Fatalf("scope rows = %d, want 1", got)
	}
}
