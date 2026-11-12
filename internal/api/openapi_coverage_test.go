package api

// Contract parity between the chi router and the OpenAPI catalogue.
//
// routeDocs is deliberately kept in code rather than a static JSON file so
// that route registrations and their documentation are reviewed in the same
// change. This test is the enforcement: a new /api/v1 route registered without
// a matching doc entry, or a doc entry left behind after a route is removed,
// fails the build.
//
// Paths are normalised before comparison because chi reports the registered
// spelling (which often carries a trailing slash) and different param names
// than the human-written docs (for example {zoneId} vs {id}). A trailing
// slash or a renamed path parameter is not a different operation; a genuinely
// new path or method is.

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
	"github.com/jasonwa/goddi/internal/rbac"
)

// normalizeRoutePath strips a trailing slash and replaces every {param} with
// {}. Parameter names are documentation detail, not operation identity.
func normalizeRoutePath(p string) string {
	p = strings.TrimSuffix(p, "/")
	var out strings.Builder
	for i := 0; i < len(p); {
		if p[i] == '{' {
			j := strings.IndexByte(p[i:], '}')
			if j < 0 {
				return p
			}
			out.WriteString("{}")
			i += j + 1
			continue
		}
		out.WriteByte(p[i])
		i++
	}
	return out.String()
}

func documentKey(method, path string) string {
	return method + " " + normalizeRoutePath(path)
}

// apiRouterFixture builds the real router with a disposable SQLite database.
// It mirrors newSessionTestServer but returns the chi.Router so chi.Walk can
// enumerate registrations.
func apiRouterFixture(t *testing.T) chi.Router {
	t.Helper()
	dir := t.TempDir()
	dbh, err := database.New(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(dir, "openapi-parity.db"),
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
	cfg.Security.JWTSecret = "openapi-parity-test-secret-0123456789"
	cfg.Security.LoginRateLimit = 100
	cfg.Security.LoginRateWindow = 60
	cfg.Server.ExposeOpenAPI = true

	h := NewRouter(cfg, dbh)
	router, ok := h.(chi.Router)
	if !ok {
		t.Fatalf("NewRouter returned %T, want chi.Router", h)
	}
	return router
}

func TestEveryRegisteredAPIRouteIsDocumentedInOpenAPI(t *testing.T) {
	router := apiRouterFixture(t)

	documented := make(map[string]bool, len(routeDocs))
	for _, rd := range routeDocs {
		documented[documentKey(rd.Method, "/api/v1"+rd.Path)] = true
	}

	var undocumented []string
	registered := make(map[string]bool)
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		// Probes and the documentation endpoint itself are not part of the
		// versioned API catalogue.
		if !strings.HasPrefix(route, "/api/v1/") || route == "/api/v1/openapi.json" {
			return nil
		}
		key := documentKey(method, route)
		registered[key] = true
		if !documented[key] {
			undocumented = append(undocumented, method+" "+route)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(undocumented) > 0 {
		t.Fatalf("%d registered /api/v1 operations are missing from routeDocs: %v", len(undocumented), undocumented)
	}

	// Reverse direction: a documented operation whose route disappeared is
	// stale documentation and would send generated clients to a 404.
	var stale []string
	for key := range documented {
		if !registered[key] {
			stale = append(stale, key)
		}
	}
	if len(stale) > 0 {
		t.Fatalf("%d routeDocs entries have no registered route: %v", len(stale), stale)
	}
}
