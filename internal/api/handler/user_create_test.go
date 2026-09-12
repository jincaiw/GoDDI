package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
)

func newCreateUserFixture(t *testing.T) (*Handlers, *sql.DB) {
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
			username TEXT NOT NULL UNIQUE,
			email TEXT,
			password_hash TEXT NOT NULL,
			display_name TEXT,
			enabled BOOLEAN NOT NULL DEFAULT 1,
			must_change_password BOOLEAN NOT NULL DEFAULT 0,
			last_login_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		);
		CREATE TABLE audit_logs (
			id TEXT PRIMARY KEY, user_id TEXT, username TEXT, action TEXT NOT NULL,
			resource_type TEXT NOT NULL, resource_id TEXT, detail TEXT, source_ip TEXT,
			user_agent TEXT, success BOOLEAN DEFAULT TRUE,
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		);
	`); err != nil {
		t.Fatal(err)
	}

	return NewHandlers(db, "test-secret-key-for-handler-tests", "test-encryption-key-000000", 0, 0), db
}

func postCreateUser(t *testing.T, h *Handlers, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.CreateUser(rec, req)
	return rec
}

func mustChangePasswordOf(t *testing.T, db *sql.DB, username string) bool {
	t.Helper()
	var flag bool
	if err := db.QueryRow(`SELECT must_change_password FROM users WHERE username = ?`, username).Scan(&flag); err != nil {
		t.Fatalf("reading the flag for %s: %v", username, err)
	}
	return flag
}

// A password an administrator typed is a password the administrator knows. The
// account has to belong to its owner before it can be used, so the flag
// defaults on.
//
// Before this default, the only way to set it was a hand-written API call, so
// a column that reads as a security control was in practice never set — and
// the middleware that enforces it never had anything to enforce.
func TestANewlyCreatedUserOwesAPasswordChangeByDefault(t *testing.T) {
	h, db := newCreateUserFixture(t)

	rec := postCreateUser(t, h, `{"username":"newop","password":"Str0ng-Passw0rd!","display_name":"New Operator"}`)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create returned %d: %s", rec.Code, rec.Body.String())
	}
	if !mustChangePasswordOf(t, db, "newop") {
		t.Fatal("a user created without saying otherwise does not owe a password change")
	}
}

func TestACallerCanOptOutOfTheForcedPasswordChange(t *testing.T) {
	h, db := newCreateUserFixture(t)

	rec := postCreateUser(t, h, `{"username":"svcacct","password":"Str0ng-Passw0rd!","must_change_password":false}`)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create returned %d: %s", rec.Code, rec.Body.String())
	}
	if mustChangePasswordOf(t, db, "svcacct") {
		t.Fatal("must_change_password:false was ignored")
	}
}

// The response has to carry the flag, or the console cannot tell the operator
// that the account it just created is not ready to use.
func TestTheCreateResponseReportsThePasswordChangeObligation(t *testing.T) {
	h, _ := newCreateUserFixture(t)

	rec := postCreateUser(t, h, `{"username":"newop","password":"Str0ng-Passw0rd!"}`)

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v (%s)", err, rec.Body.String())
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("response has no data object: %s", rec.Body.String())
	}
	if data["must_change_password"] != true {
		t.Fatalf("response reports must_change_password=%v", data["must_change_password"])
	}
}
