package api

// CSRF coverage of the documented API surface.
//
// Authentication (auth_coverage_test.go) is only half of the story: a valid
// session token alone must not be enough to mutate state, or any page the
// operator visits could issue writes on their behalf. CSRFProtection requires
// an HMAC token bound to the session for every mutating request.
//
// The test is deliberately two-sided. "No CSRF -> 401" alone would also pass
// if the session were simply invalid, so the second half proves that the same
// requests DO reach their handlers once a valid CSRF token is supplied.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"testing"
)

type sessionTokens struct {
	token     string
	csrfToken string
}

// bootstrapSession creates the first admin through the public init endpoint
// and logs in, returning the credentials a real browser would hold.
func bootstrapSession(t *testing.T, srvURL string) sessionTokens {
	t.Helper()
	post := func(path string, body interface{}) *http.Response {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.Post(srvURL+path, "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp := post("/api/v1/auth/init", map[string]string{
		"username": "csrf-coverage-admin",
		"password": "AdminPass-0123456789",
	})
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	// 409 means an earlier bootstrap in this test already created the admin;
	// logging in with the same credentials is still valid.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		t.Fatalf("auth/init = %d, want 200 or 409", resp.StatusCode)
	}

	resp = post("/api/v1/auth/login", map[string]string{
		"username": "csrf-coverage-admin",
		"password": "AdminPass-0123456789",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("auth/login = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Data struct {
			Token     string `json:"token"`
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Data.Token == "" || out.Data.CSRFToken == "" {
		t.Fatal("login did not return both session and CSRF tokens")
	}
	return sessionTokens{token: out.Data.Token, csrfToken: out.Data.CSRFToken}
}

func isMutation(method string) bool {
	return method == http.MethodPost || method == http.MethodPut ||
		method == http.MethodDelete || method == http.MethodPatch
}

func csrfRequest(t *testing.T, srvURL, method, path string, tok sessionTokens, withCSRF bool, body string) int {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, srvURL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.token)
	if withCSRF {
		req.Header.Set("X-CSRF-Token", tok.csrfToken)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

func TestEveryMutatingSecureOperationRequiresCSRF(t *testing.T) {
	srv := authCoverageFixture(t)
	tok := bootstrapSession(t, srv.URL)
	// Prove the session is live before attributing refusals to CSRF.
	if status := csrfRequest(t, srv.URL, http.MethodGet, "/api/v1/users", tok, false, ""); status != http.StatusOK {
		t.Fatalf("authenticated GET /users = %d, want 200; the session is not valid", status)
	}
	// /auth/logout and /auth/change-password destroy the session on success.
	// If they were reached early (for example when the CSRF middleware is
	// removed in a regression), every later request would fail with 401 for an
	// unrelated reason and mask the remaining leaks. Handle them last, each
	// with its own fresh session.
	sessionInvalidating := map[string]bool{
		"/auth/logout":          true,
		"/auth/change-password": true,
	}
	count := 0
	var leaks []string
	for _, rd := range routeDocs {
		if !rd.Secure || !isMutation(rd.Method) || sessionInvalidating[rd.Path] {
			continue
		}
		path := regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString("/api/v1"+rd.Path, "csrf-coverage-id")
		count++
		status := csrfRequest(t, srv.URL, rd.Method, path, tok, false, "")
		// CSRF refusal is written through writeAuthError (401). 403 is also an
		// acceptable refusal, but a 2xx/4xx-from-the-handler/5xx is not.
		if status != http.StatusUnauthorized && status != http.StatusForbidden {
			leaks = append(leaks, rd.Method+" "+path+" -> "+http.StatusText(status))
		}
	}
	if count == 0 {
		t.Fatal("no mutating secure operations found")
	}
	for _, rd := range routeDocs {
		if !rd.Secure || !isMutation(rd.Method) || !sessionInvalidating[rd.Path] {
			continue
		}
		fresh := bootstrapSession(t, srv.URL)
		path := "/api/v1" + rd.Path
		status := csrfRequest(t, srv.URL, rd.Method, path, fresh, false, "")
		if status != http.StatusUnauthorized && status != http.StatusForbidden {
			leaks = append(leaks, rd.Method+" "+path+" -> "+http.StatusText(status))
		}
	}
	if len(leaks) > 0 {
		t.Fatalf("%d mutating operations accepted a request without a CSRF token: %v", len(leaks), leaks)
	}

	// The complement: a valid CSRF token must get a mutating request past both
	// the auth and CSRF layers. We use one handler that needs no service
	// container (the router-only fixture leaves DNSServices nil, so hitting
	// every handler would produce unrelated 500s). An empty JSON object makes
	// the handler reject the payload (400), not the credentials.
	status := csrfRequest(t, srv.URL, http.MethodPost, "/api/v1/users", tok, true, "{}")
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		t.Fatalf("a mutating request with a valid CSRF token was refused (status %d)", status)
	}
}
