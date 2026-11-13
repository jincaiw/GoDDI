package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestConfigAuthorRefusesWritesOnNonAuthor proves a non-author node refuses a
// configuration write with a clear 409 and never reaches the handler, while an
// author node is untouched.
func TestConfigAuthorRefusesWritesOnNonAuthor(t *testing.T) {
	reached := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true; w.WriteHeader(http.StatusCreated) })

	rec := httptest.NewRecorder()
	ConfigAuthor(false, "node-b")(h).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/dns/zones", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("non-author status = %d, want 409", rec.Code)
	}
	if reached {
		t.Fatal("non-author request reached the handler")
	}

	reached = false
	rec = httptest.NewRecorder()
	ConfigAuthor(true, "node-a")(h).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/dns/zones", nil))
	if rec.Code != http.StatusCreated || !reached {
		t.Fatalf("author status = %d reached=%v, want 201/true", rec.Code, reached)
	}
}

// TestConfigAuthorLetsReadsThrough proves a non-author node still serves reads,
// health and the data planes; only configuration writes are refused.
func TestConfigAuthorLetsReadsThrough(t *testing.T) {
	reached := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached++; w.WriteHeader(http.StatusOK) })
	mw := ConfigAuthor(false, "node-b")(h)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/dns/zones"},
		{http.MethodGet, "/api/v1/dhcp/leases"},
		{http.MethodGet, "/health"},
		{http.MethodPost, "/api/v1/auth/login"},
	} {
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s = %d, want 200", tc.method, tc.path, rec.Code)
		}
	}
	if reached != 4 {
		t.Fatalf("reads reaching the handler = %d, want 4", reached)
	}
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/dns/zones"},
		{http.MethodPut, "/api/v1/dhcp/scopes/x"},
		{http.MethodDelete, "/api/v1/ipam/addresses/x"},
	} {
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != http.StatusConflict {
			t.Fatalf("%s %s = %d, want 409", tc.method, tc.path, rec.Code)
		}
	}
}

// TestConfigAuthorAllowsDNSSecondaryCreate proves the single-author rule does
// not block DNS redundancy: a non-author node may create a *secondary* zone
// (a follower, which cannot diverge), while every other configuration write
// stays refused. It also proves the request body survives the middleware, since
// consuming it would silently break the write the middleware just allowed.
func TestConfigAuthorAllowsDNSSecondaryCreate(t *testing.T) {
	var seen string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen = string(b)
		w.WriteHeader(http.StatusCreated)
	})
	mw := ConfigAuthor(false, "node-b")(h)

	body := `{"name":"z.test.","type":"secondary","transfer_policy":"10.0.0.1|k.|s.|hmac-sha256"}`
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/dns/zones", strings.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("secondary zone create on non-author = %d, want 201", rec.Code)
	}
	if seen != body {
		t.Fatalf("handler saw %q, want the original body", seen)
	}

	// A primary zone is authored configuration and stays refused.
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/dns/zones",
		strings.NewReader(`{"name":"z.test.","type":"primary"}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("primary zone create on non-author = %d, want 409", rec.Code)
	}

	// A secondary zone on a path that is not the zone-create endpoint stays
	// refused: the exemption is deliberately narrow.
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/dhcp/scopes",
		strings.NewReader(`{"type":"secondary"}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("non-zone write with type=secondary = %d, want 409", rec.Code)
	}
}
