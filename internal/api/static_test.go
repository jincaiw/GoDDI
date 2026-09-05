package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticHandler_UnrelatedDistDoesNotHideEmbeddedUI(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("dist", 0755); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	newStaticHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "id=\"app\"") {
		t.Fatalf("embedded UI unavailable: %d %s", rec.Code, rec.Body.String())
	}
}

// writeMinimalDist creates a minimal web/dist directory containing the
// files needed to exercise the static handler and returns the absolute
// path to it. The directory lives inside t.TempDir() so it is cleaned
// up automatically when the test finishes.
func writeMinimalDist(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	indexHTML := `<!doctype html>
<html lang="en">
  <head><title>GoDDI Test</title></head>
  <body><div id="app"></div></body>
</html>`
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexHTML), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "favicon.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatalf("write favicon.svg: %v", err)
	}

	assetsDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatalf("write app.js: %v", err)
	}

	return dir
}

// TestStaticHandler_ServesIndexForRoot verifies that GET / returns the
// index.html file (the SPA shell).
func TestStaticHandler_ServesIndexForRoot(t *testing.T) {
	dir := writeMinimalDist(t)
	h := newStaticHandlerInDir(dir)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "GoDDI Test") {
		t.Fatalf("expected index.html body, got %q", string(body))
	}
}

// TestStaticHandler_FallsBackForSPARoutes verifies that unknown paths
// (e.g. /dashboard, /login) are served as index.html so the Vue Router
// can take over.
func TestStaticHandler_FallsBackForSPARoutes(t *testing.T) {
	dir := writeMinimalDist(t)
	h := newStaticHandlerInDir(dir)

	for _, p := range []string{"/login", "/dashboard", "/dns/zones/abc", "/some/deep/route"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", p, rec.Code)
		}
		body, _ := io.ReadAll(rec.Body)
		if !strings.Contains(string(body), "<title>GoDDI Test</title>") {
			t.Fatalf("%s: expected index.html fallback, got %q", p, string(body))
		}
	}
}

// TestStaticHandler_ServesRealFiles verifies that files present on disk
// (e.g. /favicon.svg, /assets/app.js) are served as-is, with the
// correct cache headers.
func TestStaticHandler_ServesRealFiles(t *testing.T) {
	dir := writeMinimalDist(t)
	h := newStaticHandlerInDir(dir)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/svg+xml") {
		t.Fatalf("expected image/svg+xml content type, got %q", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("expected immutable cache-control on /assets/, got %q", got)
	}
}

func TestStaticHandler_MissingAssetReturns404(t *testing.T) {
	dir := writeMinimalDist(t)
	h := newStaticHandlerInDir(dir)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "<title>GoDDI Test</title>") {
		t.Fatal("missing asset must not fall back to index.html")
	}
}

// TestStaticHandler_RejectsPathTraversal verifies that traversal
// attempts (../secret.txt) fall back to index.html instead of leaking
// files from outside the static directory.
func TestStaticHandler_RejectsPathTraversal(t *testing.T) {
	dir := writeMinimalDist(t)

	// Plant a sentinel file outside the static dir to prove it cannot
	// be reached.
	sentinel := filepath.Join(filepath.Dir(dir), "secret.txt")
	if err := os.WriteFile(sentinel, []byte("top-secret"), 0o600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	h := newStaticHandlerInDir(dir)

	rec := httptest.NewRecorder()
	// The Go http stack cleans ".." segments out of the request path
	// before it reaches handlers, so the only way to attempt
	// traversal through this handler is with a path that resolves
	// outside `dir` once joined. The handler defends against that.
	req := httptest.NewRequest(http.MethodGet, "/../secret.txt", nil)
	h.ServeHTTP(rec, req)

	body, _ := io.ReadAll(rec.Body)
	if strings.Contains(string(body), "top-secret") {
		t.Fatalf("path traversal succeeded: %q", string(body))
	}
}

// TestStaticHandler_NoDirReturns404 verifies that if no static dir is
// configured the handler returns a clear 404 instead of crashing.
func TestStaticHandler_NoDirReturns404(t *testing.T) {
	h := newStaticHandlerInDir("")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "frontend assets not found") {
		t.Fatalf("expected hint message, got %q", string(body))
	}
}
