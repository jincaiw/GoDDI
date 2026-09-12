package api

// Guards on the management surface, exercised through the real router rather
// than on the middleware alone. The middleware tests pin the decision; these
// pin where the decision is made -- which routes it covers, which it must not
// cover, and that it did not quietly become a replacement for authentication.

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
	"github.com/jasonwa/goddi/internal/rbac"
)

// newGuardTestServer boots the router against a migrated temp database. It is
// deliberately leaner than the release fixture: everything these tests touch --
// the probes, the allowlist, the metrics endpoint and the authentication
// middleware -- is wired in NewRouter and needs no process-wide container.
//
// Containers are avoided on purpose. They are guarded by sync.Once, so a second
// server in the same process would silently reuse the first one's database, and
// a test that leaks state between cases is worse than no test.
func newGuardTestServer(t *testing.T, tune func(*config.Config)) *httptest.Server {
	t.Helper()

	dir := t.TempDir()
	dbh, err := database.New(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(dir, "guard.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbh.Close() })
	if err := dbh.RunMigrations(filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	if err := rbac.NewRBACManager(dbh.DB).InitializePredefinedData(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Security.JWTSecret = "guard-test-secret-key-0123456789"
	cfg.Security.LoginRateLimit = 100
	cfg.Security.LoginRateWindow = 60
	// Set rather than inherited: this fixture builds the struct directly, so
	// the shipped file's default would not be applied and an empty path would
	// register the metrics endpoint at the root.
	cfg.Metrics.Enabled = true
	cfg.Metrics.Path = "/metrics"
	if tune != nil {
		tune(cfg)
	}

	srv := httptest.NewServer(NewRouter(cfg, dbh))
	t.Cleanup(srv.Close)
	return srv
}

// TestTheManagementSurfaceRefusesASourceOutsideTheAllowList checks the reach of
// the allowlist, not just its decision. Each of these paths answers differently
// when it is reached -- a 405 for the GET-only-on-POST route, a 401 for the
// unauthenticated API read, a Prometheus payload for metrics -- so a 403 on all
// three says the request was stopped before the router looked at the path.
func TestTheManagementSurfaceRefusesASourceOutsideTheAllowList(t *testing.T) {
	srv := newGuardTestServer(t, func(cfg *config.Config) {
		cfg.Security.AdminAllowCIDRs = []string{"192.0.2.0/24"}
	})

	// The test client is 127.0.0.1, which is not in 192.0.2.0/24.
	for _, path := range []string{"/api/v1/auth/login", "/api/v1/dns/zones", "/metrics"} {
		code, _, _ := doJSON(t, "GET", srv.URL+path, "", "", nil)
		if code != http.StatusForbidden {
			t.Errorf("GET %s from an unlisted client = %d, want 403", path, code)
		}
	}
}

// TestTheProbesStayReachableFromASourceOutsideTheAllowList is the reason the
// probes are registered outside the allowlist's group. An orchestrator whose
// probe is refused does not report a configuration error -- it restarts the
// process, which is the outage the allowlist was meant to prevent.
func TestTheProbesStayReachableFromASourceOutsideTheAllowList(t *testing.T) {
	srv := newGuardTestServer(t, func(cfg *config.Config) {
		cfg.Security.AdminAllowCIDRs = []string{"192.0.2.0/24"}
	})

	code, _, _ := doJSON(t, "GET", srv.URL+"/health", "", "", nil)
	if code != http.StatusOK {
		t.Errorf("GET /health from an unlisted client = %d, want 200 (liveness must not depend on the allowlist)", code)
	}

	// Readiness reports a level; a bare router has no data plane registered, so
	// "failing" and a 503 are the expected answer. What matters here is only
	// that the allowlist did not answer instead.
	code, _, _ = doJSON(t, "GET", srv.URL+"/ready", "", "", nil)
	if code == http.StatusForbidden {
		t.Error("GET /ready was refused by the allowlist; the orchestrator would restart a process it can no longer inspect")
	}
	if code != http.StatusOK && code != http.StatusServiceUnavailable {
		t.Errorf("GET /ready = %d, want 200 or 503", code)
	}
}

// TestAnAllowListThatCannotBeReadBlindsNeitherProbesNorClientsForcedOut keeps
// the broken-list backstop honest end to end. Refusing the console is the
// intended failure; taking the probes with it would be a different, and much
// larger, one.
func TestAnAllowListThatCannotBeReadBlindsNeitherProbesNorClientsForcedOut(t *testing.T) {
	srv := newGuardTestServer(t, func(cfg *config.Config) {
		// The bare address is the likely typo; config validation refuses it at
		// startup, so this shape is only reachable if that check is bypassed.
		cfg.Security.AdminAllowCIDRs = []string{"10.0.0.5"}
	})

	if code, _, _ := doJSON(t, "GET", srv.URL+"/api/v1/dns/zones", "", "", nil); code != http.StatusForbidden {
		t.Errorf("management API behind an unreadable allowlist = %d, want 403 (a list that failed to load must not act like no list)", code)
	}
	if code, _, _ := doJSON(t, "GET", srv.URL+"/health", "", "", nil); code != http.StatusOK {
		t.Errorf("GET /health behind an unreadable allowlist = %d, want 200", code)
	}
}

// TestTheAllowListDidNotReplaceAuthentication pins the order of the two checks.
// An allowlisted client is let through to the authenticator and stopped there:
// the allowlist narrows who may reach the console, it does not decide who they
// are.
func TestTheAllowListDidNotReplaceAuthentication(t *testing.T) {
	srv := newGuardTestServer(t, func(cfg *config.Config) {
		cfg.Security.AdminAllowCIDRs = []string{"127.0.0.0/8"}
	})

	if code, _, _ := doJSON(t, "GET", srv.URL+"/api/v1/dns/zones", "", "", nil); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated read from an allowlisted client = %d, want 401", code)
	}
	// Metrics are part of the management surface, so the allowlist covers them
	// and authentication still has to refuse an anonymous caller.
	if code, _, _ := doJSON(t, "GET", srv.URL+"/metrics", "", "", nil); code != http.StatusUnauthorized {
		t.Errorf("anonymous GET /metrics from an allowlisted client = %d, want 401", code)
	}
}
