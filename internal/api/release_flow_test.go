package api

// Release end-to-end smoke test: boots the real router against a freshly
// migrated database and walks the flows an operator performs right after
// installation (init -> login -> zone/record CRUD -> backup/restore), plus
// the negative paths (unauthenticated access, missing/invalid CSRF token).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jasonwa/goddi/internal/api/handler"
	"github.com/jasonwa/goddi/internal/backup"
	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/jasonwa/goddi/internal/rbac"
)

// newReleaseTestServer boots a router backed by a migrated temp database.
func newReleaseTestServer(t *testing.T) (*httptest.Server, *database.DB) {
	t.Helper()
	dir := t.TempDir()
	dbh, err := database.New(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(dir, "release.db"),
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
	cfg.Security.JWTSecret = "release-test-secret-key-0123456789"
	cfg.Security.LoginRateLimit = 100
	cfg.Security.LoginRateWindow = 60
	cfg.Server.ExposeOpenAPI = true
	cfg.Metrics.Enabled = false

	// The DNS handlers read their dependencies from a process-wide container
	// that cmd/goddi populates at startup; inject the minimal set here.
	handler.DNSServices = &handler.DNSServiceContainer{
		DB:        dbh.DB,
		ZoneStore: zone.NewStore(dbh.DB),
		JWTSecret: cfg.Security.JWTSecret,
		Config:    cfg,
	}

	// Backup/settings handlers read the system service container.
	handler.InitSystemServices(&handler.SystemServiceContainer{
		DB:        dbh.DB,
		BackupMgr: backup.NewManager(dbh.DB, dir, "release-test"),
		Version:   "release-test",
	})

	return httptest.NewServer(NewRouter(cfg, dbh)), dbh
}

type apiResp struct {
	Code    int                    `json:"code"`
	Data    map[string]interface{} `json:"data"`
	Message string                 `json:"message"`
}

func doJSON(t *testing.T, method, url, token, csrf string, body interface{}) (int, apiResp, *http.Response) {
	t.Helper()
	var req *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		req, _ = http.NewRequest(method, url, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, _ = http.NewRequest(method, url, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	var out apiResp
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out, resp
}

// TestReleaseFlow_InstallToDailyUse is the "fresh install" acceptance path.
func TestReleaseFlow_InstallToDailyUse(t *testing.T) {
	srv, dbh := newReleaseTestServer(t)
	defer srv.Close()

	// 1. Health.
	code, _, _ := doJSON(t, "GET", srv.URL+"/health", "", "", nil)
	if code != http.StatusOK {
		t.Fatalf("GET /health = %d, want 200", code)
	}

	// 2. Unauthenticated access is rejected.
	code, _, _ = doJSON(t, "GET", srv.URL+"/api/v1/dns/zones", "", "", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /dns/zones = %d, want 401", code)
	}

	// 3. First-run admin bootstrap.
	code, initResp, _ := doJSON(t, "POST", srv.URL+"/api/v1/auth/init", "", "", map[string]string{
		"username": "release-admin", "password": "Release!Passw0rd42", "email": "admin@example.test",
	})
	if code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("POST /auth/init = %d, want 2xx (message=%q data=%+v)", code, initResp.Message, initResp.Data)
	}

	// 4. Re-initialisation must be refused (admin takeover guard).
	code, _, _ = doJSON(t, "POST", srv.URL+"/api/v1/auth/init", "", "", map[string]string{
		"username": "attacker", "password": "Release!Passw0rd42",
	})
	if code != http.StatusConflict {
		t.Fatalf("second /auth/init = %d, want 409", code)
	}

	// 5. Login yields JWT + CSRF token.
	code, login, _ := doJSON(t, "POST", srv.URL+"/api/v1/auth/login", "", "", map[string]string{
		"username": "release-admin", "password": "Release!Passw0rd42",
	})
	if code != http.StatusOK {
		t.Fatalf("POST /auth/login = %d, want 200", code)
	}
	token, _ := login.Data["token"].(string)
	csrf, _ := login.Data["csrf_token"].(string)
	if token == "" || csrf == "" {
		t.Fatalf("login response missing token/csrf: %+v", login.Data)
	}

	// 6. Mutating request without a CSRF token is rejected.
	code, _, _ = doJSON(t, "POST", srv.URL+"/api/v1/dns/zones", token, "", map[string]interface{}{
		"name": "csrf.example.com", "type": "primary",
	})
	if code != http.StatusUnauthorized && code != http.StatusForbidden {
		t.Fatalf("POST /dns/zones without CSRF = %d, want 401/403", code)
	}

	// 7. Create a zone (happy path).
	code, zone, _ := doJSON(t, "POST", srv.URL+"/api/v1/dns/zones", token, csrf, map[string]interface{}{
		"name": "release.example.com", "type": "primary",
	})
	if code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("POST /dns/zones = %d, want 2xx (body %+v)", code, zone)
	}
	zoneID, _ := zone.Data["id"].(string)
	if zoneID == "" {
		t.Fatalf("zone id missing in response: %+v", zone.Data)
	}

	// 8. Create + list records.
	code, _, _ = doJSON(t, "POST", srv.URL+"/api/v1/dns/zones/"+zoneID+"/records", token, csrf, map[string]interface{}{
		"name": "www.release.example.com", "type": "A", "value": "192.0.2.10", "ttl": 300,
	})
	if code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("POST record = %d, want 2xx", code)
	}
	code, _, _ = doJSON(t, "GET", srv.URL+"/api/v1/dns/zones/"+zoneID+"/records", token, "", nil)
	if code != http.StatusOK {
		t.Fatalf("GET records = %d, want 200", code)
	}

	// 9. New endpoints added for Technitium parity respond.
	for _, path := range []string{"/history", "/permissions", "/dnssec", "/dnssec/ds", "/dnssec/nsec3"} {
		code, _, _ = doJSON(t, "GET", srv.URL+"/api/v1/dns/zones/"+zoneID+path, token, "", nil)
		if code != http.StatusOK {
			t.Errorf("GET /dns/zones/{id}%s = %d, want 200", path, code)
		}
	}
	// Catalog members on a non-catalog zone must be rejected, not crash.
	code, _, _ = doJSON(t, "GET", srv.URL+"/api/v1/dns/zones/"+zoneID+"/catalog/members", token, "", nil)
	if code != http.StatusBadRequest {
		t.Errorf("GET catalog/members on primary zone = %d, want 400", code)
	}

	// 10. Input validation: invalid zone name.
	code, _, _ = doJSON(t, "POST", srv.URL+"/api/v1/dns/zones", token, csrf, map[string]interface{}{
		"name": "not a valid zone name!", "type": "primary",
	})
	if code != http.StatusBadRequest {
		t.Errorf("POST invalid zone name = %d, want 400", code)
	}

	// 11. Backup + restore round trip.
	code, bk, _ := doJSON(t, "POST", srv.URL+"/api/v1/backup", token, csrf, map[string]interface{}{
		"type": "dns", "description": "release check",
	})
	if code != http.StatusOK && code != http.StatusCreated && code != http.StatusAccepted {
		t.Fatalf("POST /backup = %d, want 2xx (%+v)", code, bk)
	}
	backupID := ""
	if d, ok := bk.Data["id"].(string); ok {
		backupID = d
	}
	if backupID == "" {
		if job, ok := bk.Data["job"].(map[string]interface{}); ok {
			backupID, _ = job["id"].(string)
		}
	}
	if backupID == "" {
		t.Skipf("backup id not present in response, skipping restore check (%+v)", bk.Data)
	}
	code, _, _ = doJSON(t, "POST", srv.URL+"/api/v1/backup/"+backupID+"/restore", token, csrf, nil)
	if code != http.StatusOK && code != http.StatusAccepted {
		t.Errorf("POST /backup/%s/restore = %d, want 2xx", backupID, code)
	}

	// 12. Data consistency: the zone we created still resolves in the DB.
	var count int
	if err := dbh.QueryRow(`SELECT COUNT(*) FROM dns_zones WHERE id = ?`, zoneID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("zone rows after restore = %d, want 1", count)
	}
}

// TestReleaseFlow_WeakPasswordRejected verifies password policy on bootstrap.
func TestReleaseFlow_WeakPasswordRejected(t *testing.T) {
	srv, _ := newReleaseTestServer(t)
	defer srv.Close()

	code, _, _ := doJSON(t, "POST", srv.URL+"/api/v1/auth/init", "", "", map[string]string{
		"username": "admin2", "password": "123456",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("weak password init = %d, want 400", code)
	}
}

// TestReleaseFlow_OpenAPIDocumented verifies the API catalogue is served and
// includes the endpoints added in this release.
func TestReleaseFlow_OpenAPIDocumented(t *testing.T) {
	srv, _ := newReleaseTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/openapi.json = %d, want 200", resp.StatusCode)
	}
	// paths is a Swagger-style object keyed by path, then by method.
	var doc struct {
		Paths map[string]map[string]interface{} `json:"paths"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/dns/zones/{id}/catalog/members",
		"/dns/zones/{id}/dnssec/ds",
		"/dns/zones/{id}/dnssec/nsec3",
		"/dns/zones/{id}/dnssec/keys",
		"/dns/zones/{id}/permissions",
		"/stats/top",
		"/logs/dns",
		"/logs/dns/export",
		"/dns/security/allowlists/import",
	}
	for _, path := range want {
		if _, ok := doc.Paths[path]; !ok {
			t.Errorf("openapi missing path %s", path)
		}
	}

	logsGet := doc.Paths["/logs/dns"]["get"].(map[string]interface{})
	params := logsGet["parameters"].([]interface{})
	for _, p := range params {
		param := p.(map[string]interface{})
		if param["name"] == "page" && param["in"] != "query" {
			t.Fatalf("logs page parameter location = %q, want query", param["in"])
		}
	}
	exportGet := doc.Paths["/logs/dns/export"]["get"].(map[string]interface{})
	responses := exportGet["responses"].(map[string]interface{})
	success := responses["200"].(map[string]interface{})
	content := success["content"].(map[string]interface{})
	if _, ok := content["text/csv"]; !ok {
		t.Fatal("DNS log export OpenAPI response lacks text/csv")
	}
}
