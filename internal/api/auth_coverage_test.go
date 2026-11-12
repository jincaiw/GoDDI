package api

// Authentication coverage of the documented API surface.
//
// routeDocs marks each operation Secure. A Secure operation that answers an
// unauthenticated request with anything other than 401/403 is an auth bypass
// (or a routing mistake that hides the handler). This test walks the whole
// catalogue against the real router, so adding a new route without wiring it
// behind the authentication middleware fails the build.
//
// Public (Secure: false) operations are only checked for existence: a 404
// would mean the catalogue and router have drifted. 401 is legitimate for
// /auth/refresh without a refresh token, and 403 would mean an allowlist
// unexpectedly blocked a public endpoint.
//
// This test pins the observable auth boundary, not one specific middleware.
// The group Authentication middleware is only one of several layers: per-route
// rbac.RequirePermission and handlers that read the identity from context also
// refuse anonymous callers. Removing any single layer therefore does not open
// a bypass, and this test keeps passing -- which is the desired behaviour.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
	"github.com/jasonwa/goddi/internal/rbac"
)

func authCoverageFixture(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	dbh, err := database.New(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(dir, "auth-coverage.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbh.Close() })
	if err := dbh.RunMigrations(filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	if err := rbac.NewRBACManager(dbh.DB).InitializePredefinedData(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Security.JWTSecret = "auth-coverage-test-secret-0123456789"
	cfg.Security.LoginRateLimit = 1000
	cfg.Security.LoginRateWindow = 60
	cfg.Server.ExposeOpenAPI = true
	srv := httptest.NewServer(NewRouter(cfg, dbh))
	t.Cleanup(srv.Close)
	return srv
}

// dummyPath replaces every {param} with a syntactically valid segment so the
// request reaches the route's middleware chain. The handler may reject the
// dummy value later, but authentication runs first.
func dummyPath(p string) string {
	return regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString("/api/v1"+p, "auth-probe-id")
}

func unauthenticatedStatus(t *testing.T, srv *httptest.Server, method, path string) int {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestEverySecureOperationRefusesUnauthenticatedRequests(t *testing.T) {
	srv := authCoverageFixture(t)
	var leaks []string
	secureCount := 0
	for _, rd := range routeDocs {
		if !rd.Secure {
			continue
		}
		secureCount++
		status := unauthenticatedStatus(t, srv, rd.Method, dummyPath(rd.Path))
		if status != http.StatusUnauthorized && status != http.StatusForbidden {
			leaks = append(leaks, fmt.Sprintf("%s %s -> %d", rd.Method, rd.Path, status))
		}
	}
	if secureCount == 0 {
		t.Fatal("routeDocs contains no Secure operations")
	}
	if len(leaks) > 0 {
		t.Fatalf("%d secure operations did not refuse an unauthenticated request: %v", len(leaks), leaks)
	}
}

func TestPublicOperationsAreRegistered(t *testing.T) {
	srv := authCoverageFixture(t)
	for _, rd := range routeDocs {
		if rd.Secure {
			continue
		}
		status := unauthenticatedStatus(t, srv, rd.Method, dummyPath(rd.Path))
		if status == http.StatusNotFound {
			t.Fatalf("public operation %s %s is documented but not registered", rd.Method, rd.Path)
		}
		if status == http.StatusForbidden {
			t.Fatalf("public operation %s %s was blocked by the admin allowlist", rd.Method, rd.Path)
		}
	}
}
