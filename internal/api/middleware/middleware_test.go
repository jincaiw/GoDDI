package middleware

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/auth"
	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/rbac"
	_ "modernc.org/sqlite"
)

// deriveTestCSRFKey reproduces the HKDF-SHA256 derivation used by the
// middleware so tests can mint tokens the server will accept.
func deriveTestCSRFKey(jwtSecret string) []byte {
	key, err := hkdf.Key(sha256.New, []byte(jwtSecret), []byte("goddi-csrf-v1"), "csrf-auth", 32)
	if err != nil {
		sum := sha256.Sum256([]byte(jwtSecret + "goddi-csrf-v1"))
		return sum[:]
	}
	return key
}

func setupAPITokenAuthTest(t *testing.T, readonly bool, scope ...string) (http.Handler, string, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
		CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT NOT NULL, enabled BOOLEAN NOT NULL);
		CREATE TABLE permissions (id TEXT PRIMARY KEY, resource TEXT NOT NULL, action TEXT NOT NULL, description TEXT, UNIQUE(resource, action));
		CREATE TABLE roles (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT, is_builtin BOOLEAN NOT NULL DEFAULT 0, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL);
		CREATE TABLE role_permissions (role_id TEXT NOT NULL, permission_id TEXT NOT NULL, created_at DATETIME NOT NULL, PRIMARY KEY (role_id, permission_id));
		CREATE TABLE user_roles (user_id TEXT NOT NULL, role_id TEXT NOT NULL, created_at DATETIME NOT NULL, PRIMARY KEY (user_id, role_id));
		CREATE TABLE api_tokens (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, name TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE, token_prefix TEXT NOT NULL,
			description TEXT, scope TEXT, is_single_use BOOLEAN DEFAULT FALSE,
			is_readonly BOOLEAN DEFAULT FALSE, expires_at DATETIME,
			last_used_at DATETIME, last_used_ip TEXT, enabled BOOLEAN DEFAULT TRUE,
			created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
		);
		CREATE TABLE token_ip_restrictions (
			id TEXT PRIMARY KEY, token_id TEXT NOT NULL, cidr TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);
		INSERT INTO users (id, username, enabled) VALUES ('user-1', 'api-user', 1);
		INSERT INTO permissions (id, resource, action, description) VALUES ('perm-dns-write', 'dns', 'write', 'dns write');
		INSERT INTO roles (id, name, description, is_builtin, created_at, updated_at) VALUES ('role-admin', 'admin', 'admin', 1, datetime('now'), datetime('now'));
		INSERT INTO role_permissions (role_id, permission_id, created_at) VALUES ('role-admin', 'perm-dns-write', datetime('now'));
		INSERT INTO user_roles (user_id, role_id, created_at) VALUES ('user-1', 'role-admin', datetime('now'));
	`)
	if err != nil {
		t.Fatal(err)
	}
	tokenMgr := auth.NewTokenManager(db)
	tokenScope := ""
	if len(scope) > 0 {
		tokenScope = scope[0]
	}
	_, fullToken, err := tokenMgr.CreateToken("user-1", "test", tokenScope, auth.TokenOptions{IsReadonly: readonly})
	if err != nil {
		t.Fatal(err)
	}
	jwtMgr, err := auth.NewJWTManager("test-secret-key-for-api-token-auth")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Security: config.SecurityConfig{JWTSecret: "test-secret-key-for-api-token-auth"}}
	rbacMgr := rbac.NewRBACManager(db)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := Authentication(jwtMgr, nil, tokenMgr, db)(CSRFProtection(cfg)(rbac.RequirePermission(rbacMgr, "dns", "write")(next)))
	return handler, fullToken, db
}

func TestAuthentication_APITokenAllowsPermissionAndSkipsCSRFForWrite(t *testing.T) {
	handler, token, db := setupAPITokenAuthTest(t, false)
	defer db.Close()

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req := httptest.NewRequest(method, "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s with API token returned %d, want 200", method, rec.Code)
		}
	}
}

func TestAuthentication_ReadonlyAPITokenRejectsMutation(t *testing.T) {
	handler, token, db := setupAPITokenAuthTest(t, true)
	defer db.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("readonly API token returned %d, want 403", rec.Code)
	}
}

func TestAuthentication_APITokenScopeRestrictsPermissions(t *testing.T) {
	handler, token, db := setupAPITokenAuthTest(t, false, "dns:read")
	defer db.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("scoped API token returned %d, want 403", rec.Code)
	}
}

func TestAuthentication_APITokenScopeAllowsMatchingPermission(t *testing.T) {
	handler, token, db := setupAPITokenAuthTest(t, false, "dns:write")
	defer db.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("matching-scope API token returned %d, want 200", rec.Code)
	}
}

// --- CORS Tests ---

func TestCORS_NonWhitelistedOrigin(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			PublicURL: "http://localhost:8080",
		},
	}

	handler := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Non-whitelisted origin should NOT get CORS headers
	if v := rec.Header().Get("Access-Control-Allow-Origin"); v != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for non-whitelisted origin", v)
	}
}

func TestCORS_WhitelistedOrigin(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			PublicURL: "http://localhost:8080",
		},
	}

	handler := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Whitelisted origin should get CORS headers
	if v := rec.Header().Get("Access-Control-Allow-Origin"); v != "http://localhost:8080" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", v, "http://localhost:8080")
	}
	if v := rec.Header().Get("Access-Control-Allow-Methods"); v == "" {
		t.Error("Access-Control-Allow-Methods should be set")
	}
	if v := rec.Header().Get("Access-Control-Allow-Credentials"); v != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want %q", v, "true")
	}
}

func TestCORS_LocalhostDevMode(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			PublicURL: "http://localhost:8080",
		},
	}

	handler := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Any localhost port should be allowed in dev mode
	tests := []string{
		"http://localhost:3000",
		"http://localhost:5173",
		"http://127.0.0.1:3000",
	}

	for _, origin := range tests {
		t.Run(origin, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/test", nil)
			req.Header.Set("Origin", origin)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if v := rec.Header().Get("Access-Control-Allow-Origin"); v == "" {
				t.Errorf("Access-Control-Allow-Origin should be set for dev origin %q", origin)
			}
		})
	}
}

func TestCORS_PreflightOptions(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			PublicURL: "http://localhost:8080",
		},
	}

	handler := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called for OPTIONS")
	}))

	req := httptest.NewRequest("OPTIONS", "/api/v1/test", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("OPTIONS response status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// --- CSRF Tests ---

func TestCSRF_ValidToken(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			JWTSecret: "test-secret-key-for-csrf-testing",
		},
	}

	csrfKey := deriveTestCSRFKey(cfg.Security.JWTSecret)

	// Generate a valid CSRF token
	token := GenerateCSRFToken(csrfKey)

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api/v1/test", nil)
	req.Header.Set("X-CSRF-Token", token)
	req.Header.Set("Cookie", "session=abc123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Valid CSRF token: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestCSRF_InvalidToken(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			JWTSecret: "test-secret-key-for-csrf-testing",
		},
	}

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called with invalid CSRF token")
	}))

	req := httptest.NewRequest("POST", "/api/v1/test", nil)
	req.Header.Set("X-CSRF-Token", "invalid-token-value")
	req.Header.Set("Cookie", "session=abc123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Invalid CSRF token: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCSRF_ExpiredToken(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			JWTSecret: "test-secret-key-for-csrf-testing",
		},
	}

	csrfKey := deriveTestCSRFKey(cfg.Security.JWTSecret)

	// Generate an expired token (timestamp 25 hours ago)
	expiredTimestamp := strconv.FormatInt(time.Now().Unix()-90000, 10)
	mac := hmac.New(sha256.New, csrfKey)
	mac.Write([]byte(expiredTimestamp))
	signature := hex.EncodeToString(mac.Sum(nil))
	expiredToken := expiredTimestamp + ":" + signature

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called with expired CSRF token")
	}))

	req := httptest.NewRequest("POST", "/api/v1/test", nil)
	req.Header.Set("X-CSRF-Token", expiredToken)
	req.Header.Set("Cookie", "session=abc123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expired CSRF token: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCSRF_BearerTokenDoesNotSkipCSRF(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			JWTSecret: "test-secret-key-for-csrf-testing",
		},
	}

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called without a valid CSRF token, even with a Bearer header")
	}))

	req := httptest.NewRequest("POST", "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiJ9.test")
	// No CSRF token; the Bearer header must not bypass CSRF.
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Bearer token should NOT skip CSRF: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCSRF_SafeMethodsSkipCSRF(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			JWTSecret: "test-secret-key-for-csrf-testing",
		},
	}

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, method := range []string{"GET", "HEAD", "OPTIONS"} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/v1/test", nil)
			// No CSRF token, but safe method should skip CSRF
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("Safe method %s should skip CSRF: status = %d, want %d", method, rec.Code, http.StatusOK)
			}
		})
	}
}
