package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// D-03: POST /api/v1/dns/client is a read-only DNS diagnostic, not a
// configuration write. The single-author rule must not block it on a standby.
// Both the versioned path (real router) and the bare path (unit-level mount)
// must reach the handler with the body intact.
func TestConfigAuthorAllowsReadOnlyDNSDiagnostic(t *testing.T) {
	var seen []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen = append(seen, string(b))
		w.WriteHeader(http.StatusOK)
	})
	mw := ConfigAuthor(false, "node-b")(h)
	body := `{"name":"www.demo.test","type":"A"}`
	for _, path := range []string{"/api/v1/dns/client", "/dns/client"} {
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("POST %s on standby = %d, want 200", path, rec.Code)
		}
	}
	if len(seen) != 2 || seen[0] != body || seen[1] != body {
		t.Fatalf("handler bodies = %v, want two copies of %q", seen, body)
	}

	// The author node is unaffected (it never installs the refusal wrapper).
	rec := httptest.NewRecorder()
	ConfigAuthor(true, "node-a")(h).ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/api/v1/dns/client", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("author diagnostic = %d, want 200", rec.Code)
	}
}

// The exemption must be exact: any sibling path, other method, or other
// configuration write on the standby still fails closed with 409 and the
// explicit single-author reason.
func TestConfigAuthorDNSDiagnosticExemptionIsNarrow(t *testing.T) {
	reached := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true; w.WriteHeader(http.StatusOK) })
	mw := ConfigAuthor(false, "node-b")(h)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/dns/client/export"},
		{http.MethodPost, "/api/v1/dns/clients"},
		{http.MethodPut, "/api/v1/dns/client"},
		{http.MethodPatch, "/api/v1/dns/client"},
		{http.MethodDelete, "/api/v1/dns/client"},
		{http.MethodPost, "/api/v1/dhcp/client"},
		{http.MethodPost, "/api/v1/dns/zones"},
		{http.MethodPost, "/api/v1/dhcp/scopes"},
		{http.MethodPost, "/api/v1/ipam/spaces"},
		{http.MethodDelete, "/api/v1/dns/zones/z1"},
	} {
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != http.StatusConflict {
			t.Fatalf("%s %s = %d, want 409", tc.method, tc.path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "配置作者") {
			t.Fatalf("%s %s body = %q, want the explicit refusal reason", tc.method, tc.path, rec.Body.String())
		}
	}
	if reached {
		t.Fatal("a refused write reached the handler")
	}
}

// A real configuration write is still refused with 409; the 409/read-only
// pair is the contract this change must preserve.
func TestConfigAuthorStillRefusesRealConfigWrites(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) })
	mw := ConfigAuthor(false, "node-b")(h)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/dns/zones",
		strings.NewReader(`{"name":"z.test.","type":"primary"}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("config write = %d, want 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "cluster.config_author=false") {
		t.Fatalf("body = %q, want the deployment reason", rec.Body.String())
	}
}
