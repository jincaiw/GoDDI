package middleware

import (
	"net/http"
	"net/http/httptest"
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
