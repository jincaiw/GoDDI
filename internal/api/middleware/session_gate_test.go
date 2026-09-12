package middleware

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/auth"
	_ "modernc.org/sqlite"
)

const sessionGateSecret = "test-secret-key-for-session-gate"

// accountFlags says what the seeded account looks like. The two flags are
// separate because they gate different things and must not be conflated.
type accountFlags struct {
	mustChangePassword bool
	enabled            bool
	seedUser           bool
}

func newSessionGateHandler(t *testing.T, flags accountFlags, sessionUserID string) (http.Handler, *sql.DB, string) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT 1,
			must_change_password BOOLEAN NOT NULL DEFAULT 0
		);
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			ip_address TEXT,
			user_agent TEXT,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL
		);
	`); err != nil {
		t.Fatal(err)
	}

	if flags.seedUser {
		if _, err := db.Exec(
			`INSERT INTO users (id, username, enabled, must_change_password) VALUES ('u1', 'operator', ?, ?)`,
			flags.enabled, flags.mustChangePassword,
		); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := db.Exec(`
		INSERT INTO sessions (id, user_id, token_hash, ip_address, user_agent, expires_at, created_at)
		VALUES ('s1', ?, '', '127.0.0.1', 'test', ?, ?)`,
		sessionUserID,
		time.Now().Add(time.Hour).Format(time.RFC3339),
		time.Now().Format(time.RFC3339),
	); err != nil {
		t.Fatal(err)
	}

	jwtMgr, err := auth.NewJWTManager(sessionGateSecret)
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwtMgr.GenerateToken("u1", "operator", nil, "s1")
	if err != nil {
		t.Fatal(err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return JWTAuth(jwtMgr, auth.NewSessionManager(db))(next), db, token
}

func serveWithSession(t *testing.T, handler http.Handler, token, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
	}
	return rec.Code, body
}

func TestAnOrdinarySessionReachesEverything(t *testing.T) {
	handler, _, token := newSessionGateHandler(t, accountFlags{enabled: true, seedUser: true}, "u1")

	for _, path := range []string{"/api/v1/dns/zones", "/api/v1/auth/me", "/api/v1/metrics-ish"} {
		code, _ := serveWithSession(t, handler, token, path)
		if code != http.StatusOK {
			t.Fatalf("%s returned %d, want 200", path, code)
		}
	}
}

// The whole point of the flag: an account still holding its initial password
// must not be able to reach anything except the screens that let it change
// that password.
func TestASessionOwingAPasswordChangeIsConfinedToAccountSecurityRoutes(t *testing.T) {
	handler, _, token := newSessionGateHandler(t, accountFlags{enabled: true, mustChangePassword: true, seedUser: true}, "u1")

	allowed := []string{
		"/api/v1/auth/me",
		"/api/v1/auth/change-password",
		"/api/v1/auth/logout",
		"/api/v1/auth/refresh",
	}
	for _, path := range allowed {
		code, _ := serveWithSession(t, handler, token, path)
		if code != http.StatusOK {
			t.Fatalf("%s returned %d, want 200 while a password change is owed", path, code)
		}
	}

	blocked := []string{
		"/api/v1/dns/zones",
		"/api/v1/dhcp/scopes",
		"/api/v1/ipam/subnets",
		"/api/v1/users",
		"/api/v1/auth/login", // not an account-security action mid-session
		"/api/v1/backup",
	}
	for _, path := range blocked {
		code, body := serveWithSession(t, handler, token, path)
		if code != http.StatusForbidden {
			t.Fatalf("%s returned %d, want 403 while a password change is owed", path, code)
		}
		if body["code"] != passwordChangeRequiredCode {
			t.Fatalf("%s returned code %v, want %q", path, body["code"], passwordChangeRequiredCode)
		}
	}
}

// A client has to be able to tell "change your password" apart from "you are
// not allowed here" without parsing a translated message.
func TestThePasswordChangeGateCarriesItsOwnCode(t *testing.T) {
	handler, _, token := newSessionGateHandler(t, accountFlags{enabled: true, mustChangePassword: true, seedUser: true}, "u1")

	blocked, blockedBody := serveWithSession(t, handler, token, "/api/v1/dns/zones")
	allowed, _ := serveWithSession(t, handler, token, "/api/v1/auth/me")

	if blocked != http.StatusForbidden || allowed != http.StatusOK {
		t.Fatalf("gate returned (%d, %d); the two paths must differ", blocked, allowed)
	}
	if blockedBody["code"] != passwordChangeRequiredCode {
		t.Fatalf("gate code is %v, want %q", blockedBody["code"], passwordChangeRequiredCode)
	}
	if blockedBody["code"] == float64(http.StatusForbidden) {
		t.Fatal("the gate reported the bare HTTP status as its code, so a client cannot distinguish it")
	}
}

// Sessions live for days. Reading the flag from the account row on every
// request is what makes "the password has been changed" take effect at once,
// including for sessions opened before the change.
func TestTheGateLiftsAsSoonAsTheAccountFlagIsCleared(t *testing.T) {
	handler, db, token := newSessionGateHandler(t, accountFlags{enabled: true, mustChangePassword: true, seedUser: true}, "u1")

	if code, _ := serveWithSession(t, handler, token, "/api/v1/dns/zones"); code != http.StatusForbidden {
		t.Fatalf("before the change the request returned %d, want 403", code)
	}

	if _, err := db.Exec(`UPDATE users SET must_change_password = 0 WHERE id = 'u1'`); err != nil {
		t.Fatal(err)
	}

	if code, _ := serveWithSession(t, handler, token, "/api/v1/dns/zones"); code != http.StatusOK {
		t.Fatalf("after the change the same session returned %d, want 200", code)
	}
}

// Disabling an account has to stop it working now, not when its session
// happens to expire.
func TestASessionForADisabledAccountIsRefused(t *testing.T) {
	handler, _, token := newSessionGateHandler(t, accountFlags{enabled: false, seedUser: true}, "u1")

	code, _ := serveWithSession(t, handler, token, "/api/v1/auth/me")
	if code != http.StatusUnauthorized {
		t.Fatalf("a disabled account reached %d, want 401", code)
	}
}

// A session whose user row is gone belongs to nobody. Treating it as valid
// would grant a deleted operator full access until the session expired.
func TestASessionWhoseUserNoLongerExistsIsRefused(t *testing.T) {
	handler, _, token := newSessionGateHandler(t, accountFlags{enabled: true, seedUser: false}, "u1")

	code, _ := serveWithSession(t, handler, token, "/api/v1/auth/me")
	if code != http.StatusUnauthorized {
		t.Fatalf("an orphaned session reached %d, want 401", code)
	}
}

// A session read back for a different user must not inherit the flags of the
// session that was seeded, which is what a mis-joined query would do.
func TestTheFlagsComeFromTheSessionsOwnAccount(t *testing.T) {
	handler, db, token := newSessionGateHandler(t, accountFlags{enabled: true, mustChangePassword: false, seedUser: true}, "u1")

	if _, err := db.Exec(
		`INSERT INTO users (id, username, enabled, must_change_password) VALUES ('u2', 'other', 0, 1)`,
	); err != nil {
		t.Fatal(err)
	}

	if code, _ := serveWithSession(t, handler, token, "/api/v1/dns/zones"); code != http.StatusOK {
		t.Fatalf("the session returned %d; it picked up a different account's flags", code)
	}
}
