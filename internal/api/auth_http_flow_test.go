package api

// HTTP-level pins for three auth behaviors whose unit-level pieces are tested
// in internal/auth but whose end-to-end route behavior was only exercised by
// the old console e2e: login rate limiting answers 429, the TOTP
// setup→verify→disable flow enforces its state machine, and a revoked session
// dies while its sibling sessions survive.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/jincaiw/GoDDI/internal/config"
	"github.com/jincaiw/GoDDI/internal/database"
	"github.com/jincaiw/GoDDI/internal/rbac"
	"github.com/pquerna/otp/totp"
)

// authFlowFixture is authCoverageFixture with a configurable login rate limit,
// so the 429 path can be reached without waiting a minute.
func authFlowFixture(t *testing.T, loginRateLimit int) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	dbh, err := database.New(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(dir, "auth-flow.db"),
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
	cfg.Security.JWTSecret = "auth-flow-test-secret-0123456789"
	cfg.Security.LoginRateLimit = loginRateLimit
	cfg.Security.LoginRateWindow = 60
	cfg.Server.ExposeOpenAPI = true
	srv := httptest.NewServer(NewRouter(cfg, dbh))
	t.Cleanup(srv.Close)
	return srv
}

func postJSON(t *testing.T, url string, body interface{}, headers map[string]string) (int, []byte) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}

func getJSON(t *testing.T, url, token string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}

// The login rate limiter counts attempts per (username, source IP): after one
// username exhausts the window, a different username from the same source is
// still allowed — pinning the keying, not just the 429.
func TestLoginRateLimitAnswers429PerUsername(t *testing.T) {
	srv := authFlowFixture(t, 2)
	bootstrapSession(t, srv.URL)

	login := func(username string) int {
		st, _ := postJSON(t, srv.URL+"/api/v1/auth/login", map[string]string{
			"username": username, "password": "WrongPass-000000000000",
		}, nil)
		return st
	}

	// Two attempts pass the limiter (and fail authentication).
	if got := login("rate_victim"); got != http.StatusUnauthorized {
		t.Fatalf("attempt 1 = %d, want 401", got)
	}
	if got := login("rate_victim"); got != http.StatusUnauthorized {
		t.Fatalf("attempt 2 = %d, want 401", got)
	}
	// The third is refused by the limiter before authentication runs.
	if got := login("rate_victim"); got != http.StatusTooManyRequests {
		t.Fatalf("attempt 3 = %d, want 429 (limiter must fire)", got)
	}

	// A different username from the same source is untouched by the other key.
	if got := login("rate_other_user"); got != http.StatusUnauthorized {
		t.Fatalf("other username = %d, want 401 (the limiter keys per username)", got)
	}
}

// The TOTP state machine through the real routes: setup issues a secret, a
// wrong code is refused, the right code enables (with recovery codes), the
// enabled state conflicts with re-setup/re-verify, disable requires the
// account password, and disabling allows a fresh enroll.
func TestTOTPHttpFlowEnforcesTheStateMachine(t *testing.T) {
	srv := authCoverageFixture(t)
	admin := bootstrapSession(t, srv.URL)
	auth := map[string]string{
		"Authorization": "Bearer " + admin.token,
		"X-CSRF-Token":  admin.csrfToken,
	}

	// Wrong password on verify is not possible — verify takes a code; but the
	// setup of a fresh account succeeds.
	st, body := postJSON(t, srv.URL+"/api/v1/auth/totp/setup", map[string]string{}, auth)
	if st != http.StatusOK {
		t.Fatalf("setup = %d: %s", st, body)
	}
	var secretOut struct {
		Data struct {
			Secret string `json:"secret"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &secretOut); err != nil || secretOut.Data.Secret == "" {
		t.Fatalf("setup returned no secret: %s", body)
	}
	secret := secretOut.Data.Secret

	// A wrong code is refused and does not enable.
	st, _ = postJSON(t, srv.URL+"/api/v1/auth/totp/verify", map[string]string{
		"secret": secret, "code": "000000",
	}, auth)
	if st != http.StatusBadRequest {
		t.Fatalf("verify(wrong code) = %d, want 400", st)
	}

	// The right code enables and returns recovery codes.
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	st, body = postJSON(t, srv.URL+"/api/v1/auth/totp/verify", map[string]string{
		"secret": secret, "code": code,
	}, auth)
	if st != http.StatusOK {
		t.Fatalf("verify(correct code) = %d, want 200: %s", st, body)
	}
	if !bytes.Contains(body, []byte("recovery_codes")) {
		t.Fatalf("enable response carries no recovery codes: %s", body)
	}

	// The enabled state conflicts with re-setup and re-verify.
	if st, _ = postJSON(t, srv.URL+"/api/v1/auth/totp/setup", map[string]string{}, auth); st != http.StatusConflict {
		t.Fatalf("re-setup while enabled = %d, want 409", st)
	}
	st, _ = postJSON(t, srv.URL+"/api/v1/auth/totp/verify", map[string]string{
		"secret": secret, "code": code,
	}, auth)
	if st != http.StatusConflict {
		t.Fatalf("re-verify while enabled = %d, want 409", st)
	}

	// Disabling needs the account password; a wrong one is refused.
	st, _ = postJSON(t, srv.URL+"/api/v1/auth/totp/disable", map[string]string{
		"password": "WrongPass-000000000000",
	}, auth)
	if st != http.StatusUnauthorized {
		t.Fatalf("disable(wrong password) = %d, want 401", st)
	}
	st, _ = postJSON(t, srv.URL+"/api/v1/auth/totp/disable", map[string]string{
		"password": "AdminPass-0123456789",
	}, auth)
	if st != http.StatusOK {
		t.Fatalf("disable(correct password) = %d, want 200", st)
	}
	// Disabling an already-disabled TOTP is refused explicitly.
	st, _ = postJSON(t, srv.URL+"/api/v1/auth/totp/disable", map[string]string{
		"password": "AdminPass-0123456789",
	}, auth)
	if st != http.StatusBadRequest {
		t.Fatalf("disable while disabled = %d, want 400", st)
	}
	// And a fresh enroll is possible again.
	if st, _ = postJSON(t, srv.URL+"/api/v1/auth/totp/setup", map[string]string{}, auth); st != http.StatusOK {
		t.Fatalf("re-enroll after disable = %d, want 200", st)
	}
}

// Revoking one session kills exactly that session: its token stops working
// while the sibling session keeps serving — and the list shows what exists.
func TestSessionRevocationKillsOnlyTheRevokedSession(t *testing.T) {
	srv := authCoverageFixture(t)
	token1 := bootstrapSession(t, srv.URL)

	listIDs := func(tok string) map[string]bool {
		st, body := getJSON(t, srv.URL+"/api/v1/auth/sessions", tok)
		if st != http.StatusOK {
			t.Fatalf("GET /auth/sessions = %d: %s", st, body)
		}
		var out struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("decoding sessions: %v (%s)", err, body)
		}
		ids := make(map[string]bool, len(out.Data))
		for _, s := range out.Data {
			ids[s.ID] = true
		}
		return ids
	}

	before := listIDs(token1.token)

	// A second login creates a second session.
	st, body := postJSON(t, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "csrf-coverage-admin", "password": "AdminPass-0123456789",
	}, nil)
	if st != http.StatusOK {
		t.Fatalf("second login = %d: %s", st, body)
	}
	var out struct {
		Data struct {
			Token     string `json:"token"`
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	token2 := sessionTokens{token: out.Data.Token, csrfToken: out.Data.CSRFToken}

	after := listIDs(token1.token)
	var newID string
	for id := range after {
		if !before[id] {
			newID = id
		}
	}
	if newID == "" {
		t.Fatalf("the second login created no new session: before=%v after=%v", before, after)
	}

	// Revoke the new session from the first session.
	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/auth/sessions/"+newID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token1.token)
	req.Header.Set("X-CSRF-Token", token1.csrfToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE session = %d: %s", resp.StatusCode, raw)
	}

	// The revoked token is dead; the revoking session still works.
	if st, _ := getJSON(t, srv.URL+"/api/v1/auth/me", token2.token); st != http.StatusUnauthorized {
		t.Fatalf("revoked session GET /auth/me = %d, want 401", st)
	}
	if st, _ := getJSON(t, srv.URL+"/api/v1/auth/me", token1.token); st != http.StatusOK {
		t.Fatalf("surviving session GET /auth/me = %d, want 200", st)
	}
	if got := listIDs(token1.token); got[newID] {
		t.Fatalf("the revoked session still appears in the list: %v", got)
	}
}

// A session whose owner owes a password change is confined to the
// account-security endpoints. The confinement is distinguishable from an RBAC
// refusal by its response code: it carries the dedicated
// "password_change_required" string, not a numeric failure code. Changing the
// password lifts the restriction immediately (the flag is re-read from the
// account row on every request) and invalidates every OTHER session.
func TestMustChangePasswordConfinementLiftsAfterTheChange(t *testing.T) {
	srv := authCoverageFixture(t)
	admin := bootstrapSession(t, srv.URL)

	adminCall := func(method, path string, v interface{}) (int, []byte) {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(method, srv.URL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+admin.token)
		req.Header.Set("X-CSRF-Token", admin.csrfToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, raw
	}

	// A user that owes a password change, holding NO roles — so after the
	// change, /users is refused by RBAC (numeric code), never by confinement.
	st, body := adminCall(http.MethodPost, "/api/v1/users", map[string]interface{}{
		"username": "confined_user", "password": "AdminPass-0123456789",
		"must_change_password": true,
	})
	if st != http.StatusCreated {
		t.Fatalf("POST /users = %d: %s", st, body)
	}

	// Login succeeds — owing a change is not a login failure.
	st, body = postJSON(t, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "confined_user", "password": "AdminPass-0123456789",
	}, nil)
	if st != http.StatusOK {
		t.Fatalf("confined login = %d, want 200", st)
	}
	var sess struct {
		Data struct {
			Token     string `json:"token"`
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &sess); err != nil {
		t.Fatal(err)
	}
	auth := map[string]string{
		"Authorization": "Bearer " + sess.Data.Token,
		"X-CSRF-Token":  sess.Data.CSRFToken,
	}

	// Account-security endpoints are allowed.
	if st, _ = getJSON(t, srv.URL+"/api/v1/auth/me", sess.Data.Token); st != http.StatusOK {
		t.Fatalf("confined GET /auth/me = %d, want 200 (account security is allowed)", st)
	}

	// Everything else carries the dedicated confinement code, not a plain RBAC
	// refusal.
	st, body = getJSON(t, srv.URL+"/api/v1/users", sess.Data.Token)
	if st != http.StatusForbidden {
		t.Fatalf("confined GET /users = %d, want 403", st)
	}
	if !bytes.Contains(body, []byte("password_change_required")) {
		t.Fatalf("confined GET /users did not carry the confinement code: %s", body)
	}

	// Changing the password lifts the confinement immediately and invalidates
	// every other session (only one exists here, which must survive).
	st, body = postJSON(t, srv.URL+"/api/v1/auth/change-password", map[string]string{
		"old_password": "AdminPass-0123456789",
		"new_password": "NewPass-9876543210ab",
	}, auth)
	if st != http.StatusOK {
		t.Fatalf("change-password = %d: %s", st, body)
	}
	if st, _ = getJSON(t, srv.URL+"/api/v1/auth/me", sess.Data.Token); st != http.StatusOK {
		t.Fatalf("current session after change = %d, want 200 (the current session survives)", st)
	}

	// The same endpoint now answers as RBAC — no more confinement code.
	st, body = getJSON(t, srv.URL+"/api/v1/users", sess.Data.Token)
	if st != http.StatusForbidden {
		t.Fatalf("post-change GET /users = %d, want 403 (RBAC now, the user holds no roles)", st)
	}
	if bytes.Contains(body, []byte("password_change_required")) {
		t.Fatalf("post-change GET /users still carries the confinement code: %s", body)
	}

	// The new password is the one that works; the old one is dead.
	if st, _ = postJSON(t, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "confined_user", "password": "AdminPass-0123456789",
	}, nil); st != http.StatusUnauthorized {
		t.Fatalf("login with the old password = %d, want 401", st)
	}
	if st, _ = postJSON(t, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "confined_user", "password": "NewPass-9876543210ab",
	}, nil); st != http.StatusOK {
		t.Fatalf("login with the new password = %d, want 200", st)
	}
}

// Disabling a user kills their LIVE sessions on the next request — sessions
// live for days, so the per-request re-read of the account row is what makes
// "disable this user" take effect immediately (the middleware's own comment
// names this). The account row is not deleted: re-enabling revives the old
// session, and logins are refused while disabled.
func TestDisablingAUserKillsTheirLiveSessionsImmediately(t *testing.T) {
	srv := authCoverageFixture(t)
	admin := bootstrapSession(t, srv.URL)

	adminCall := func(method, path string, v interface{}) (int, []byte) {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(method, srv.URL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+admin.token)
		req.Header.Set("X-CSRF-Token", admin.csrfToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, raw
	}

	st, body := adminCall(http.MethodPost, "/api/v1/users", map[string]interface{}{
		"username": "victim_user", "password": "AdminPass-0123456789",
		"must_change_password": false,
	})
	if st != http.StatusCreated {
		t.Fatalf("POST /users = %d: %s", st, body)
	}

	// The user logs in and the session works.
	st, body = postJSON(t, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "victim_user", "password": "AdminPass-0123456789",
	}, nil)
	if st != http.StatusOK {
		t.Fatalf("victim login = %d: %s", st, body)
	}
	var sess struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &sess); err != nil {
		t.Fatal(err)
	}
	if st, _ := getJSON(t, srv.URL+"/api/v1/auth/me", sess.Data.Token); st != http.StatusOK {
		t.Fatalf("victim session before disable = %d, want 200", st)
	}

	// Disable the account.
	st, body = adminCall(http.MethodPut, "/api/v1/users/"+
		func() string {
			var ids struct {
				Data []struct {
					ID       string `json:"id"`
					Username string `json:"username"`
				} `json:"data"`
			}
			st, b := getJSON(t, srv.URL+"/api/v1/users", admin.token)
			if st != http.StatusOK {
				t.Fatalf("GET /users = %d: %s", st, b)
			}
			if err := json.Unmarshal(b, &ids); err != nil {
				t.Fatal(err)
			}
			for _, u := range ids.Data {
				if u.Username == "victim_user" {
					return u.ID
				}
			}
			t.Fatal("victim user not found")
			return ""
		}(), map[string]interface{}{"enabled": false})
	if st != http.StatusOK {
		t.Fatalf("PUT disable = %d: %s", st, body)
	}

	// The LIVE session is dead on the next request — no waiting for expiry.
	if st, body := getJSON(t, srv.URL+"/api/v1/auth/me", sess.Data.Token); st != http.StatusUnauthorized {
		t.Fatalf("victim session after disable = %d, want 401 (%s)", st, body)
	}
	// And a fresh login is refused too.
	if st, _ = postJSON(t, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "victim_user", "password": "AdminPass-0123456789",
	}, nil); st != http.StatusUnauthorized {
		t.Fatalf("disabled login = %d, want 401", st)
	}

	// Re-enabling revives the old session (the row was re-read, not deleted):
	// this is the documented semantics, so pin it as such.
	st, body = adminCall(http.MethodPut, "/api/v1/users/"+
		func() string {
			var ids struct {
				Data []struct {
					ID       string `json:"id"`
					Username string `json:"username"`
				} `json:"data"`
			}
			st, b := getJSON(t, srv.URL+"/api/v1/users", admin.token)
			if st != http.StatusOK {
				t.Fatal("GET /users failed")
			}
			if err := json.Unmarshal(b, &ids); err != nil {
				t.Fatal(err)
			}
			for _, u := range ids.Data {
				if u.Username == "victim_user" {
					return u.ID
				}
			}
			t.Fatal("victim user not found")
			return ""
		}(), map[string]interface{}{"enabled": true})
	if st != http.StatusOK {
		t.Fatalf("PUT re-enable = %d: %s", st, body)
	}
	if st, _ := getJSON(t, srv.URL+"/api/v1/auth/me", sess.Data.Token); st != http.StatusOK {
		t.Fatalf("victim session after re-enable = %d, want 200 (row re-read, not deleted)", st)
	}
}

// The refresh flow's security properties through the real routes: a valid
// refresh re-issues a working JWT+CSRF pair (deterministic per session by
// design — the pair must work on a mutation), a refresh whose session was
// revoked is refused (SEC-05), a disabled account cannot refresh, and a
// missing token is 401.
func TestAuthRefreshFlowIssuesAWorkingPairAndHonorsRevocation(t *testing.T) {
	srv := authCoverageFixture(t)
	admin := bootstrapSession(t, srv.URL)

	adminCall := func(method, path string, v interface{}) (int, []byte) {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(method, srv.URL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+admin.token)
		req.Header.Set("X-CSRF-Token", admin.csrfToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, raw
	}

	// A user with a working mutation grant, so the rotated pair can be proven
	// as a pair (a JWT without its matching CSRF is refused).
	st, body := adminCall(http.MethodPost, "/api/v1/users", map[string]interface{}{
		"username": "refresh_user", "password": "AdminPass-0123456789",
		"must_change_password": false,
	})
	if st != http.StatusCreated {
		t.Fatalf("POST /users = %d: %s", st, body)
	}
	// Find the user id and grant user:read+write so the pair proof is a write.
	var users struct {
		Data []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"data"`
	}
	st, body = getJSON(t, srv.URL+"/api/v1/users", admin.token)
	if st != http.StatusOK {
		t.Fatalf("GET /users = %d", st)
	}
	if err := json.Unmarshal(body, &users); err != nil {
		t.Fatal(err)
	}
	var uid string
	for _, u := range users.Data {
		if u.Username == "refresh_user" {
			uid = u.ID
		}
	}
	_, body = adminCall(http.MethodGet, "/api/v1/permissions", nil)
	var perms struct {
		Data []struct {
			ID       string `json:"id"`
			Resource string `json:"resource"`
			Action   string `json:"action"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &perms); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, p := range perms.Data {
		if p.Resource == "user" && (p.Action == "read" || p.Action == "write") {
			ids = append(ids, p.ID)
		}
	}
	_, body = adminCall(http.MethodPost, "/api/v1/roles", map[string]string{
		"name": "refresh_writer_role", "description": "refresh flow",
	})
	var roleOut struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &roleOut); err != nil {
		t.Fatal(err)
	}
	_, _ = adminCall(http.MethodPut, "/api/v1/roles/"+roleOut.Data.ID+"/permissions", map[string]interface{}{
		"permission_ids": ids,
	})
	_, _ = adminCall(http.MethodPost, "/api/v1/users/"+uid+"/roles", map[string]interface{}{
		"role_ids": []string{roleOut.Data.ID},
	})

	// Login and refresh.
	st, body = postJSON(t, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "refresh_user", "password": "AdminPass-0123456789",
	}, nil)
	if st != http.StatusOK {
		t.Fatalf("login = %d: %s", st, body)
	}
	var login1 struct {
		Data struct {
			Token     string `json:"token"`
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &login1); err != nil {
		t.Fatal(err)
	}

	st, body = postJSON(t, srv.URL+"/api/v1/auth/refresh", map[string]string{}, map[string]string{
		"Authorization": "Bearer " + login1.Data.Token,
	})
	if st != http.StatusOK {
		t.Fatalf("refresh = %d, want 200: %s", st, body)
	}
	var refreshed struct {
		Data struct {
			Token     string `json:"token"`
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshed.Data.Token == "" || refreshed.Data.CSRFToken == "" {
		t.Fatal("refresh returned an empty token or CSRF token")
	}
	// The strings may equal the originals when the re-issue lands in the same
	// second: the JWT carries second-granularity iat and the CSRF token is an
	// HMAC bound to the session (deliberately deterministic per session). What
	// must hold is the functional property below — the refreshed pair works —
	// not a string difference.

	// The rotated pair works AS A PAIR on a mutation.
	st, body = postJSON(t, srv.URL+"/api/v1/users", map[string]interface{}{
		"username": "created_by_refreshed", "password": "AdminPass-0123456789",
		"must_change_password": false,
	}, map[string]string{
		"Authorization": "Bearer " + refreshed.Data.Token,
		"X-CSRF-Token":  refreshed.Data.CSRFToken,
	})
	if st != http.StatusCreated {
		t.Fatalf("mutation with the rotated pair = %d, want 201: %s", st, body)
	}

	// Without a token at all: 401.
	if st, _ = postJSON(t, srv.URL+"/api/v1/auth/refresh", map[string]string{}, nil); st != http.StatusUnauthorized {
		t.Fatalf("refresh without token = %d, want 401", st)
	}

	// Revoke the session: the (otherwise valid) refresh is refused — SEC-05.
	// /auth/sessions lists only the CALLER's sessions, so the refresh user
	// lists and revokes its own (the login and the refreshed token share one
	// session id).
	st, body = getJSON(t, srv.URL+"/api/v1/auth/sessions", refreshed.Data.Token)
	if st != http.StatusOK {
		t.Fatalf("GET /auth/sessions (refresh user) = %d: %s", st, body)
	}
	var sessions struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions.Data) != 1 {
		t.Fatalf("the refresh user should own exactly one session, got %d", len(sessions.Data))
	}
	victimSession := sessions.Data[0].ID
	delReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/auth/sessions/"+victimSession, nil)
	if err != nil {
		t.Fatal(err)
	}
	delReq.Header.Set("Authorization", "Bearer "+refreshed.Data.Token)
	delReq.Header.Set("X-CSRF-Token", refreshed.Data.CSRFToken)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	rawDel, _ := io.ReadAll(delResp.Body)
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("self-revoke = %d: %s", delResp.StatusCode, rawDel)
	}
	if st, _ := postJSON(t, srv.URL+"/api/v1/auth/refresh", map[string]string{}, map[string]string{
		"Authorization": "Bearer " + refreshed.Data.Token,
	}); st != http.StatusUnauthorized {
		t.Fatalf("refresh after session revocation = %d, want 401 (SEC-05)", st)
	}

	// A disabled account cannot refresh either.
	st, _ = adminCall(http.MethodPut, "/api/v1/users/"+uid, map[string]interface{}{"enabled": false})
	if st != http.StatusOK {
		t.Fatalf("disable = %d", st)
	}
	// Re-login is refused while disabled, so refresh is proven through a token
	// issued while enabled: re-enable, log in, disable again, then refresh.
	if st, _ = adminCall(http.MethodPut, "/api/v1/users/"+uid, map[string]interface{}{"enabled": true}); st != http.StatusOK {
		t.Fatalf("re-enable = %d", st)
	}
	st, body = postJSON(t, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "refresh_user", "password": "AdminPass-0123456789",
	}, nil)
	if st != http.StatusOK {
		t.Fatalf("re-login = %d: %s", st, body)
	}
	var login2 struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &login2); err != nil {
		t.Fatal(err)
	}
	if st, _ = adminCall(http.MethodPut, "/api/v1/users/"+uid, map[string]interface{}{"enabled": false}); st != http.StatusOK {
		t.Fatalf("disable again = %d", st)
	}
	if st, _ = postJSON(t, srv.URL+"/api/v1/auth/refresh", map[string]string{}, map[string]string{
		"Authorization": "Bearer " + login2.Data.Token,
	}); st != http.StatusUnauthorized {
		t.Fatalf("refresh with a disabled account = %d, want 401", st)
	}
}

// Once TOTP is enabled, login REQUIRES a code: a missing code is refused
// (and counted against the rate limiter), a wrong code is refused, the right
// TOTP code works, and recovery codes work exactly once — the second use of
// the same recovery code is refused.
func TestLoginRequiresTOTPAfterEnablingAndRecoveryCodesAreSingleUse(t *testing.T) {
	srv := authCoverageFixture(t)
	admin := bootstrapSession(t, srv.URL)
	auth := map[string]string{
		"Authorization": "Bearer " + admin.token,
		"X-CSRF-Token":  admin.csrfToken,
	}

	// Enable TOTP and capture the recovery codes.
	st, body := postJSON(t, srv.URL+"/api/v1/auth/totp/setup", map[string]string{}, auth)
	if st != http.StatusOK {
		t.Fatalf("setup = %d: %s", st, body)
	}
	var secretOut struct {
		Data struct {
			Secret string `json:"secret"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &secretOut); err != nil || secretOut.Data.Secret == "" {
		t.Fatalf("setup returned no secret: %s", body)
	}
	code, err := totp.GenerateCode(secretOut.Data.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	st, body = postJSON(t, srv.URL+"/api/v1/auth/totp/verify", map[string]string{
		"secret": secretOut.Data.Secret, "code": code,
	}, auth)
	if st != http.StatusOK {
		t.Fatalf("verify = %d: %s", st, body)
	}
	var enabled struct {
		Data struct {
			RecoveryCodes []string `json:"recovery_codes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &enabled); err != nil || len(enabled.Data.RecoveryCodes) == 0 {
		t.Fatalf("enable returned no recovery codes: %s", body)
	}
	recovery := enabled.Data.RecoveryCodes

	login := func(totpCode string) int {
		payload := map[string]string{
			"username": "csrf-coverage-admin", "password": "AdminPass-0123456789",
		}
		if totpCode != "" {
			payload["totp_code"] = totpCode
		}
		st, _ := postJSON(t, srv.URL+"/api/v1/auth/login", payload, nil)
		return st
	}

	// Missing code: refused even though the password is correct.
	if got := login(""); got != http.StatusUnauthorized {
		t.Fatalf("login without TOTP code = %d, want 401", got)
	}
	// Wrong code: refused.
	if got := login("000000"); got != http.StatusUnauthorized {
		t.Fatalf("login with wrong code = %d, want 401", got)
	}
	// The right code works.
	current, err := totp.GenerateCode(secretOut.Data.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := login(current); got != http.StatusOK {
		t.Fatalf("login with the right TOTP code = %d, want 200", got)
	}
	// A recovery code works — exactly once.
	if got := login(recovery[0]); got != http.StatusOK {
		t.Fatalf("login with a recovery code = %d, want 200", got)
	}
	if got := login(recovery[0]); got != http.StatusUnauthorized {
		t.Fatalf("reuse of a recovery code = %d, want 401 (single-use)", got)
	}
	// The next recovery code still works.
	if got := login(recovery[1]); got != http.StatusOK {
		t.Fatalf("login with a second recovery code = %d, want 200", got)
	}
}

// The lockout→unlock loop end to end (the R2 fix): an account locked out by
// repeated failures is stuck at 429, an admin's POST /auth/unlock resets it,
// and the account can attempt logins again — without restarting anything.
func TestAdminUnlockReleasesALockedAccount(t *testing.T) {
	srv := authFlowFixture(t, 2)
	admin := bootstrapSession(t, srv.URL)

	login := func(username string) int {
		st, _ := postJSON(t, srv.URL+"/api/v1/auth/login", map[string]string{
			"username": username, "password": "WrongPass-000000000000",
		}, nil)
		return st
	}

	// Lock the account out: 2 attempts pass the limiter, the 3rd hits 429.
	login("locked_victim")
	login("locked_victim")
	if got := login("locked_victim"); got != http.StatusTooManyRequests {
		t.Fatalf("attempt 3 = %d, want 429 (locked out)", got)
	}

	// A non-admin cannot call the unlock endpoint at all (user:write gate —
	// the RBAC matrix pins that; here the admin path is what matters).

	// The admin unlocks the account.
	st, body := postJSON(t, srv.URL+"/api/v1/auth/unlock", map[string]string{
		"username": "locked_victim",
	}, map[string]string{
		"Authorization": "Bearer " + admin.token,
		"X-CSRF-Token":  admin.csrfToken,
	})
	if st != http.StatusOK {
		t.Fatalf("unlock = %d, want 200: %s", st, body)
	}

	// The account can attempt logins again — 401 for the wrong password, not
	// 429 from the limiter.
	if got := login("locked_victim"); got != http.StatusUnauthorized {
		t.Fatalf("post-unlock attempt = %d, want 401 (limiter reset, wrong password)", got)
	}
	// The account was never created in this fixture — the point is that the
	// LIMITER no longer refuses: 401 (unknown user) instead of 429 proves the
	// reset took effect.
	if got := login("locked_victim"); got != http.StatusUnauthorized {
		t.Fatalf("post-unlock attempt = %d, want 401 (not 429)", got)
	}
	// Distinguish: a locked account answers 429, an unlocked one with an
	// unknown username answers 401. Re-verify the limiter fires again for a
	// fresh lockout so the reset is proven to have been consumed.
	login("locked_victim")
	login("locked_victim")
	if got := login("locked_victim"); got != http.StatusTooManyRequests {
		t.Fatalf("re-lock attempt 3 = %d, want 429 (the limiter counts again after unlock)", got)
	}
}

// The API-token lifecycle through the real routes: creation returns the
// secret once, the scoped token works on its scope and is refused outside it,
// and revoking it kills it immediately — a security-relevant loop that the
// router-only fixture can drive end to end (tokens are DB-backed).
func TestAPITokenRevocationKillsTheTokenImmediately(t *testing.T) {
	srv := authCoverageFixture(t)
	admin := bootstrapSession(t, srv.URL)

	adminCall := func(method, path string, v interface{}) (int, []byte) {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(method, srv.URL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+admin.token)
		req.Header.Set("X-CSRF-Token", admin.csrfToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, raw
	}

	// Create a dns:read-scoped token.
	st, body := adminCall(http.MethodPost, "/api/v1/tokens", map[string]interface{}{
		"name":  fmt.Sprintf("revoke-test-%d", time.Now().UnixNano()),
		"scope": "dns:read",
	})
	if st != http.StatusCreated {
		t.Fatalf("POST /tokens = %d, want 201: %s", st, body)
	}
	var created struct {
		Data struct {
			ID    string `json:"id"`
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.Data.Token == "" {
		t.Fatalf("create returned no token secret: %s", body)
	}

	// The scoped token passes authentication AND the dns:read gate: on the
	// router-only fixture the DNS handler then 500s on its nil container —
	// anything but 401/403 proves the grant held. (500 here is the fixture,
	// not the permission layer; api.spec proves the 200 against a full
	// instance.)
	if st, _ := getJSON(t, srv.URL+"/api/v1/dns/zones", created.Data.Token); st == http.StatusUnauthorized || st == http.StatusForbidden {
		t.Fatalf("scoped token GET /dns/zones = %d, want 500 (past RBAC — the grant held) or 200", st)
	}
	// ...and is refused outside its scope.
	if st, _ := getJSON(t, srv.URL+"/api/v1/users", created.Data.Token); st != http.StatusForbidden {
		t.Fatalf("scoped token GET /users = %d, want 403 (outside the dns:read scope)", st)
	}

	// Revoke it.
	st, body = adminCall(http.MethodDelete, "/api/v1/tokens/"+created.Data.ID, nil)
	if st != http.StatusOK {
		t.Fatalf("DELETE token = %d, want 200: %s", st, body)
	}

	// The revoked token is dead immediately.
	if st, _ := getJSON(t, srv.URL+"/api/v1/dns/zones", created.Data.Token); st != http.StatusUnauthorized {
		t.Fatalf("revoked token GET /dns/zones = %d, want 401", st)
	}
}
