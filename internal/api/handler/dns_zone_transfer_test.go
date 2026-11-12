package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"
)

func ztTestSetup(t *testing.T) (*sql.DB, *chi.Mux) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE dns_zones (id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL);
		CREATE TABLE dns_tsig_keys (id TEXT PRIMARY KEY, name TEXT NOT NULL, algorithm TEXT, secret TEXT, secret_ciphertext TEXT, created_at DATETIME);
		CREATE TABLE dns_zone_transfer (id TEXT PRIMARY KEY, zone_id TEXT NOT NULL, allowed_cidr TEXT NOT NULL, tsig_key_name TEXT, created_at DATETIME);
		INSERT INTO dns_zones (id,name,type) VALUES ('z1','t.test.','primary');
		INSERT INTO dns_tsig_keys (id,name,algorithm,secret,secret_ciphertext) VALUES ('k1','xfr-key.','hmac-sha256','','sealed');
	`); err != nil {
		t.Fatal(err)
	}
	old := DNSServices
	DNSServices = &DNSServiceContainer{DB: db}
	t.Cleanup(func() { DNSServices = old })
	r := chi.NewRouter()
	r.Route("/dns/zones/{id}/transfers", func(r chi.Router) {
		r.Get("/", ListZoneTransfers)
		r.Post("/", CreateZoneTransfer)
		r.Delete("/{transferId}", DeleteZoneTransfer)
	})
	return db, r
}

func TestZoneTransferEndpoints(t *testing.T) {
	db, r := ztTestSetup(t)

	// Missing CIDR → 400.
	req := httptest.NewRequest("POST", "/dns/zones/z1/transfers", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty body code = %d, want 400", w.Code)
	}

	// Unknown TSIG key → 400.
	req = httptest.NewRequest("POST", "/dns/zones/z1/transfers", strings.NewReader(`{"allowed_cidr":"127.0.0.1/32","tsig_key_name":"nope."}`))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown tsig code = %d, want 400", w.Code)
	}

	// Valid create → 201 and a row with the host CIDR normalized.
	req = httptest.NewRequest("POST", "/dns/zones/z1/transfers", strings.NewReader(`{"allowed_cidr":"127.0.0.1","tsig_key_name":"xfr-key."}`))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create code = %d body=%s", w.Code, w.Body.String())
	}
	var created zoneTransferEntry
	if err := json.Unmarshal(extractData(t, w.Body.Bytes()), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.AllowedCIDR != "127.0.0.1/32" || created.TSIGKeyName != "xfr-key." {
		t.Fatalf("created = %+v, want 127.0.0.1/32 + xfr-key.", created)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM dns_zone_transfer WHERE zone_id='z1'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows = %d err=%v, want 1", n, err)
	}

	// List → contains the entry.
	req = httptest.NewRequest("GET", "/dns/zones/z1/transfers", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list code = %d", w.Code)
	}

	// Delete → 200, then 404 on repeat.
	req = httptest.NewRequest("DELETE", "/dns/zones/z1/transfers/"+created.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete code = %d", w.Code)
	}
	req = httptest.NewRequest("DELETE", "/dns/zones/z1/transfers/"+created.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("repeat delete code = %d, want 404", w.Code)
	}
}

// extractData pulls the "data" field out of the standard response envelope.
func extractData(t *testing.T, body []byte) json.RawMessage {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope %s: %v", body, err)
	}
	return env.Data
}
