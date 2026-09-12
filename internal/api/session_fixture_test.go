package api

// A server fixture for tests about sessions and permissions.
//
// It exists because newReleaseTestServer goes through the process-wide
// containers, and two of those are guarded by a sync.Once:
// handler.InitSystemServices binds on the first call and keeps the first
// caller's database and directories for the rest of the process. A second test
// that used it would therefore decide, purely by file order, whether
// release_flow_test.go still works -- and it fails there with a 500 about a
// backup directory that has already been cleaned up.
//
// So this fixture wires nothing but the router: authentication, the RBAC
// middleware and the handlers that read the database directly. The tests that
// use it stay on /auth/* and /users*, which need no container at all.

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
	"github.com/jasonwa/goddi/internal/rbac"
)

func newSessionTestServer(t *testing.T) (*httptest.Server, *database.DB) {
	t.Helper()

	dir := t.TempDir()
	dbh, err := database.New(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(dir, "session.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbh.Close() })
	if err := dbh.RunMigrations(filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	// Mirror the startup bootstrap in cmd/goddi: predefined roles and
	// permissions are seeded at serve time, not by migrations.
	if err := rbac.NewRBACManager(dbh.DB).InitializePredefinedData(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Security.JWTSecret = "session-test-secret-key-0123456789"
	cfg.Security.LoginRateLimit = 100
	cfg.Security.LoginRateWindow = 60

	srv := httptest.NewServer(NewRouter(cfg, dbh))
	t.Cleanup(srv.Close)
	return srv, dbh
}
