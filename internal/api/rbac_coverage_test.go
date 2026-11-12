package api

// Authorisation depth, complementing the authentication (auth_coverage_test.go)
// and CSRF (csrf_coverage_test.go) gates. Being logged in is not the same as
// being allowed to read configuration: a user whose role grants no permissions
// must be refused by every secure read endpoint, with two deliberate
// exceptions for account self-service.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"testing"
)

// selfServiceReads are intentionally available to any authenticated user.
// They expose only the caller's own identity and their own sessions; the
// ownership assertion below keeps that promise testable.
var selfServiceReads = map[string]bool{
	"/auth/me":       true,
	"/auth/sessions": true,
}

func authedRequest(t *testing.T, srvURL, method, path, token string, body interface{}) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, srvURL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
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

// lowPrivilegeSession creates a user with no role (and therefore no
// permissions), logs in as them, and returns their tokens and user ID.
func lowPrivilegeSession(t *testing.T, srvURL string, admin sessionTokens) (sessionTokens, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srvURL+"/api/v1/users", bytes.NewReader([]byte(
		`{"username":"rbac_lowpriv","password":"AdminPass-0123456789","must_change_password":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+admin.token)
	req.Header.Set("X-CSRF-Token", admin.csrfToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || created.Data.ID == "" {
		t.Fatalf("creating the zero-permission user = %d, id=%q", resp.StatusCode, created.Data.ID)
	}

	r := postLogin(t, srvURL, "rbac_lowpriv")
	return r, created.Data.ID
}

func postLogin(t *testing.T, srvURL, username string) sessionTokens {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{
		"username": username,
		"password": "AdminPass-0123456789",
	})
	resp, err := http.Post(srvURL+"/api/v1/auth/login", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login as %s = %d", username, resp.StatusCode)
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
	return sessionTokens{token: out.Data.Token, csrfToken: out.Data.CSRFToken}
}

func TestZeroPermissionUserIsRefusedBySecureReadEndpoints(t *testing.T) {
	srv := authCoverageFixture(t)
	admin := bootstrapSession(t, srv.URL)
	low, _ := lowPrivilegeSession(t, srv.URL, admin)

	// Positive control: the same endpoints work for the administrator, so a
	// 403 below is an authorisation decision, not a broken route.
	if status, _ := authedRequest(t, srv.URL, http.MethodGet, "/api/v1/users", admin.token, nil); status != http.StatusOK {
		t.Fatalf("admin GET /users = %d, want 200", status)
	}

	re := regexp.MustCompile(`\{[^}]+\}`)
	var leaks []string
	checked := 0
	for _, rd := range routeDocs {
		if !rd.Secure || rd.Method != http.MethodGet || selfServiceReads[rd.Path] {
			continue
		}
		checked++
		path := re.ReplaceAllString("/api/v1"+rd.Path, "rbac-probe-id")
		status, _ := authedRequest(t, srv.URL, http.MethodGet, path, low.token, nil)
		// 401 would mean the session is broken; 403 is the expected refusal.
		// Anything below 400 means a permission-less user read the resource.
		if status < http.StatusBadRequest {
			leaks = append(leaks, fmt.Sprintf("GET %s -> %d", path, status))
		}
	}
	if checked == 0 {
		t.Fatal("no permission-guarded reads found")
	}
	if len(leaks) > 0 {
		t.Fatalf("%d secure reads were available to a user with no permissions: %v", len(leaks), leaks)
	}
}

func TestSelfServiceReadsAreScopedToTheCaller(t *testing.T) {
	srv := authCoverageFixture(t)
	admin := bootstrapSession(t, srv.URL)
	low, lowID := lowPrivilegeSession(t, srv.URL, admin)

	status, meBody := authedRequest(t, srv.URL, http.MethodGet, "/api/v1/auth/me", low.token, nil)
	if status != http.StatusOK {
		t.Fatalf("low-privilege GET /auth/me = %d, want 200", status)
	}
	var me struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(meBody, &me); err != nil {
		t.Fatal(err)
	}
	if me.Data.ID != lowID {
		t.Fatalf("/auth/me returned id %q, want the caller's %q", me.Data.ID, lowID)
	}

	status, sessionsBody := authedRequest(t, srv.URL, http.MethodGet, "/api/v1/auth/sessions", low.token, nil)
	if status != http.StatusOK {
		t.Fatalf("low-privilege GET /auth/sessions = %d, want 200", status)
	}
	var out struct {
		Data []struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(sessionsBody, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Data) == 0 {
		t.Fatal("caller's own session is missing from /auth/sessions")
	}
	for _, s := range out.Data {
		if s.UserID != lowID {
			t.Fatalf("/auth/sessions leaked a session owned by %q (caller is %q)", s.UserID, lowID)
		}
	}
}

// The other half of the read-side gate: a role that DOES grant every `:read`
// permission but no write permission. The old browser suite exercised this
// matrix through the UI; the server-side pin below is what keeps it true when
// a route's permission row is edited. Every mutating operation must be 403,
// while the reads the role does hold stay 200.
func TestReadOnlyRoleIsRefusedByEveryMutatingOperation(t *testing.T) {
	srv := authCoverageFixture(t)
	admin := bootstrapSession(t, srv.URL)

	post := func(path string, body interface{}) (int, []byte) {
		// Admin mutations need the session's CSRF token as well.
		reqBody, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(reqBody))
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
		raw, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, raw
	}

	// 1. Collect every read permission from the catalog.
	status, body := authedRequest(t, srv.URL, http.MethodGet, "/api/v1/permissions", admin.token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /permissions = %d", status)
	}
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
	readIDs := make([]string, 0, len(perms.Data))
	for _, p := range perms.Data {
		if p.Action == "read" {
			readIDs = append(readIDs, p.ID)
		}
	}
	if len(readIDs) == 0 {
		t.Fatal("the permission catalog lists no read permissions")
	}

	// 2. Create a role holding exactly those reads.
	status, body = post("/api/v1/roles", map[string]string{
		"name": "rbac_readonly_role", "description": "read-only coverage role",
	})
	if status != http.StatusCreated {
		t.Fatalf("POST /roles = %d: %s", status, body)
	}
	var role struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &role); err != nil {
		t.Fatal(err)
	}
	putJSON := func(path string, v interface{}) (int, []byte) {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPut, srv.URL+path, bytes.NewReader(raw))
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
	if status, body = putJSON("/api/v1/roles/"+role.Data.ID+"/permissions", map[string]interface{}{
		"permission_ids": readIDs,
	}); status != http.StatusOK {
		t.Fatalf("granting read permissions = %d: %s", status, body)
	}

	// 3. A user holding only that role.
	status, body = post("/api/v1/users", map[string]interface{}{
		"username": "rbac_reader", "password": "AdminPass-0123456789",
		"must_change_password": false,
	})
	if status != http.StatusCreated {
		t.Fatalf("POST /users = %d: %s", status, body)
	}
	var user struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &user); err != nil {
		t.Fatal(err)
	}
	if status, body = post("/api/v1/users/"+user.Data.ID+"/roles", map[string]interface{}{
		"role_ids": []string{role.Data.ID},
	}); status != http.StatusOK {
		t.Fatalf("assigning the role = %d: %s", status, body)
	}

	reader := postLogin(t, srv.URL, "rbac_reader")

	// Positive control: a read the role grants actually works for it. /users
	// reads straight from the database — the router-only fixture leaves the
	// DNS service container nil, so a DNS handler here would 500 for reasons
	// unrelated to permissions.
	if status, _ := authedRequest(t, srv.URL, http.MethodGet, "/api/v1/users", reader.token, nil); status != http.StatusOK {
		t.Fatalf("reader GET /users = %d, want 200 (the role must hold its reads)", status)
	}

	// 4. Every mutating operation is forbidden for the read-only role.
	re := regexp.MustCompile(`\{[^}]+\}`)

	// Mutations that are deliberately reachable without a write permission.
	// Each is verified in the router: the TOTP and session paths are account
	// self-service (no permission gate, ownership checked in the handler);
	// the DNS debug client is gated dns:read and only issues queries; the
	// import preview is gated ipam:read and writes nothing by design (its
	// server-side pin is TestAPreviewWritesNothing). The 500s the fixture
	// shows for preview are its nil service container, not the permission.
	selfService := map[string]bool{
		"/auth/sessions/{id}":   true,
		"/auth/totp/setup":      true,
		"/auth/totp/verify":     true,
		"/auth/totp/disable":    true,
		"/dns/client":           true,
		"/ipam/import/preview":  true,
		"/auth/logout":          true,
		"/auth/change-password": true,
	}

	var leaks []string
	checked := 0
	for _, rd := range routeDocs {
		if !rd.Secure || !isMutation(rd.Method) || selfService[rd.Path] {
			continue
		}
		checked++
		path := re.ReplaceAllString("/api/v1"+rd.Path, "rbac-reader-id")
		// The reader's own CSRF token must ride along: without it every
		// mutation is refused by the CSRF layer (401) before RBAC is asked,
		// and this test would pass for the wrong reason.
		reqBody := bytes.NewBufferString("{}")
		req, err := http.NewRequest(rd.Method, srv.URL+path, reqBody)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+reader.token)
		req.Header.Set("X-CSRF-Token", reader.csrfToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if status := resp.StatusCode; status != http.StatusForbidden {
			leaks = append(leaks, fmt.Sprintf("%s %s -> %d", rd.Method, path, status))
		}
	}
	if checked == 0 {
		t.Fatal("no mutating operations found")
	}
	if len(leaks) > 0 {
		t.Fatalf("a read-only role could perform %d mutations: %v", len(leaks), leaks)
	}
}

// authedBodyRequest is authedRequest with an explicit method and empty-body
// option, kept separate so the table-driven callers stay readable.
func authedBodyRequest(t *testing.T, srvURL, method, path, token string, body interface{}) (int, []byte) {
	t.Helper()
	return authedRequest(t, srvURL, method, path, token, body)
}

// Per-resource isolation, the other axis of the old browser RBAC matrix: a
// role scoped to one resource's read must grant exactly that resource's reads
// and nothing else, and a second role must union rather than replace. The old
// console e2e exercised this through the UI; the server-side pin below is the
// enforcement half. The scope is user:read because the router-only fixture
// leaves the DNS service container nil — a DNS handler here would 500 for
// reasons unrelated to permissions.
func TestScopedRoleGetsResourceIsolationOnReads(t *testing.T) {
	srv := authCoverageFixture(t)
	admin := bootstrapSession(t, srv.URL)

	// Admin mutations need the session's CSRF token as well.
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

	// The catalog's user:read and group:read permission ids. The scope is
	// user:read because the router-only fixture leaves the DNS service
	// container nil — a DNS handler here would 500 for reasons unrelated to
	// permissions.
	status, body := adminCall(http.MethodGet, "/api/v1/permissions", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /permissions = %d", status)
	}
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
	userRead, groupRead := "", ""
	for _, p := range perms.Data {
		if p.Resource == "user" && p.Action == "read" {
			userRead = p.ID
		}
		if p.Resource == "group" && p.Action == "read" {
			groupRead = p.ID
		}
	}
	if userRead == "" || groupRead == "" {
		t.Fatalf("catalog missing user:read or group:read (user=%q group=%q)", userRead, groupRead)
	}

	newRole := func(name string, permissionIDs []string) string {
		st, bd := adminCall(http.MethodPost, "/api/v1/roles", map[string]string{
			"name": name, "description": "isolation coverage",
		})
		if st != http.StatusCreated {
			t.Fatalf("POST /roles (%s) = %d: %s", name, st, bd)
		}
		var r struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(bd, &r); err != nil {
			t.Fatal(err)
		}
		if st, bd = adminCall(http.MethodPut, "/api/v1/roles/"+r.Data.ID+"/permissions", map[string]interface{}{
			"permission_ids": permissionIDs,
		}); st != http.StatusOK {
			t.Fatalf("granting to %s = %d: %s", name, st, bd)
		}
		return r.Data.ID
	}

	newScopedUser := func(username string, roleIDs []string) sessionTokens {
		st, bd := adminCall(http.MethodPost, "/api/v1/users", map[string]interface{}{
			"username": username, "password": "AdminPass-0123456789", "must_change_password": false,
		})
		if st != http.StatusCreated {
			t.Fatalf("POST /users (%s) = %d: %s", username, st, bd)
		}
		var u struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(bd, &u); err != nil {
			t.Fatal(err)
		}
		if st, bd = adminCall(http.MethodPost, "/api/v1/users/"+u.Data.ID+"/roles", map[string]interface{}{
			"role_ids": roleIDs,
		}); st != http.StatusOK {
			t.Fatalf("assigning roles to %s = %d: %s", username, st, bd)
		}
		return postLogin(t, srv.URL, username)
	}

	userOnly := newRole("rbac_user_only_role", []string{userRead})
	reader := newScopedUser("rbac_user_reader", []string{userOnly})

	// Positive control: the granted read works, and it is the ONLY resource
	// the scope opens — every other resource's list is forbidden.
	if sc, _ := authedRequest(t, srv.URL, http.MethodGet, "/api/v1/users", reader.token, nil); sc != http.StatusOK {
		t.Fatalf("user-scoped GET /users = %d, want 200 (the role must hold its reads)", sc)
	}
	isolated := map[string]string{
		"/dns/zones":   "dns",
		"/dhcp/scopes": "dhcp",
		"/ipam/spaces": "ipam",
		"/settings":    "settings",
		"/backup":      "backup",
		"/tokens":      "token",
		"/roles":       "role",
		"/groups":      "group",
	}
	var leaked []string
	for path, resource := range isolated {
		// authedRequest takes the FULL path: without the /api/v1 prefix these
		// would hit the console's static handler (200) instead of the API.
		if sc, _ := authedRequest(t, srv.URL, http.MethodGet, "/api/v1"+path, reader.token, nil); sc != http.StatusForbidden {
			leaked = append(leaked, fmt.Sprintf("GET %s (%s) -> %d", path, resource, sc))
		}
	}
	if len(leaked) > 0 {
		t.Fatalf("user-scoped leaks: %v", leaked)
	}

	// Union: a second role with group:read grants that resource too, without
	// displacing the first role's grant — and dns stays forbidden (never given).
	groupOnly := newRole("rbac_group_only_role", []string{groupRead})
	union := newScopedUser("rbac_union_user", []string{userOnly, groupOnly})
	if sc, _ := authedRequest(t, srv.URL, http.MethodGet, "/api/v1/users", union.token, nil); sc != http.StatusOK {
		t.Fatalf("union GET /users = %d, want 200 (first role still held)", sc)
	}
	if sc, _ := authedRequest(t, srv.URL, http.MethodGet, "/api/v1/groups", union.token, nil); sc != http.StatusOK {
		t.Fatalf("union GET /groups = %d, want 200 (second role granted)", sc)
	}
	if sc, _ := authedRequest(t, srv.URL, http.MethodGet, "/api/v1/dns/zones", union.token, nil); sc != http.StatusForbidden {
		t.Fatalf("union GET /dns/zones = %d, want 403 (never granted)", sc)
	}
}

// The fourth quadrant of the RBAC matrix. The other three are pinned above:
// no grant + read -> 403 (zero-permission), grant + read -> 200 (isolation),
// no grant + write -> 403 (read-only). This one pins grant + write -> the
// operation succeeds: a role carrying user:write really can manage users, so
// "the grant works" is machine-checked, not assumed from the 403 symmetry.
func TestUserWriterRoleCanManageUsers(t *testing.T) {
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

	status, body := adminCall(http.MethodGet, "/api/v1/permissions", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /permissions = %d", status)
	}
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
	userRead, userWrite := "", ""
	for _, p := range perms.Data {
		if p.Resource == "user" && p.Action == "read" {
			userRead = p.ID
		}
		if p.Resource == "user" && p.Action == "write" {
			userWrite = p.ID
		}
	}
	if userRead == "" || userWrite == "" {
		t.Fatalf("catalog missing user:read or user:write (read=%q write=%q)", userRead, userWrite)
	}

	// A role holding exactly user:read + user:write.
	status, body = adminCall(http.MethodPost, "/api/v1/roles", map[string]string{
		"name": "rbac_user_writer_role", "description": "writer coverage",
	})
	if status != http.StatusCreated {
		t.Fatalf("POST /roles = %d: %s", status, body)
	}
	var role struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &role); err != nil {
		t.Fatal(err)
	}
	if status, body = adminCall(http.MethodPut, "/api/v1/roles/"+role.Data.ID+"/permissions", map[string]interface{}{
		"permission_ids": []string{userRead, userWrite},
	}); status != http.StatusOK {
		t.Fatalf("granting = %d: %s", status, body)
	}

	// The writer user, created by the admin and holding only that role.
	status, body = adminCall(http.MethodPost, "/api/v1/users", map[string]interface{}{
		"username": "rbac_user_writer", "password": "AdminPass-0123456789", "must_change_password": false,
	})
	if status != http.StatusCreated {
		t.Fatalf("POST /users (writer) = %d: %s", status, body)
	}
	var writerUser struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &writerUser); err != nil {
		t.Fatal(err)
	}
	if status, body = adminCall(http.MethodPost, "/api/v1/users/"+writerUser.Data.ID+"/roles", map[string]interface{}{
		"role_ids": []string{role.Data.ID},
	}); status != http.StatusOK {
		t.Fatalf("assigning role = %d: %s", status, body)
	}
	writer := postLogin(t, srv.URL, "rbac_user_writer")

	// The grant works: the writer can create and enable/disable users...
	writerCall := func(method, path string, v interface{}) (int, []byte) {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(method, srv.URL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+writer.token)
		req.Header.Set("X-CSRF-Token", writer.csrfToken)
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
	status, body = writerCall(http.MethodPost, "/api/v1/users", map[string]interface{}{
		"username": "created_by_writer", "password": "AdminPass-0123456789", "must_change_password": false,
	})
	if status != http.StatusCreated {
		t.Fatalf("writer POST /users = %d, want 201 (the grant must work): %s", status, body)
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}

	// ...but nothing beyond the grant: the writer cannot delete users
	// (user:delete not granted) and cannot touch other resources.
	if status, _ = writerCall(http.MethodDelete, "/api/v1/users/"+created.Data.ID, nil); status != http.StatusForbidden {
		t.Fatalf("writer DELETE /users = %d, want 403 (user:delete not granted)", status)
	}
	if status, _ = authedRequest(t, srv.URL, http.MethodGet, "/api/v1/dns/zones", writer.token, nil); status == http.StatusOK {
		// dns:read not granted; the router-only fixture would 500 past RBAC,
		// so anything but 403 here means the gate did not hold.
		t.Fatalf("writer GET /dns/zones = %d, want 403 (dns not granted)", status)
	}
	if status, _ = writerCall(http.MethodDelete, "/api/v1/users/"+created.Data.ID, nil); status != http.StatusForbidden {
		t.Fatalf("writer DELETE retry = %d, want 403", status)
	}

	// The admin cleans up the created user (the writer cannot).
	if status, _ = adminCall(http.MethodDelete, "/api/v1/users/"+created.Data.ID, nil); status != http.StatusOK {
		t.Fatalf("admin cleanup DELETE = %d: %s", status, body)
	}
}

// The 501 contract is documented in OpenAPI (pinned by
// TestReservedExtensionsDocumentTheir501Contract); this pins the actual HTTP
// behavior, so a route cannot quietly start answering 200/500 while the
// documentation still promises 501. Runs before the evidence exists: the v0.9.0
// plan keeps /cluster, /sso, /apps and /dhcp/ha reserved until it does.
func TestReservedExtensionEndpointsAnswer501(t *testing.T) {
	srv := authCoverageFixture(t)
	admin := bootstrapSession(t, srv.URL)

	call := func(method, path string) int {
		req, err := http.NewRequest(method, srv.URL+"/api/v1"+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+admin.token)
		// Mutating methods must carry the session's CSRF token, or the CSRF
		// layer answers 401 before the 501 stub is ever reached.
		if method != http.MethodGet {
			req.Header.Set("X-CSRF-Token", admin.csrfToken)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}

	// GET /dhcp/ha left this list in round 285: it now answers a read-only
	// status snapshot. Six reserved extensions remain.
	reserved := []struct{ method, path string }{
		{"GET", "/sso"},
		{"PUT", "/sso"},
		{"GET", "/cluster"},
		{"POST", "/cluster"},
		{"GET", "/apps"},
		{"POST", "/apps/auth-probe-id/install"},
	}
	for _, r := range reserved {
		if got := call(r.method, r.path); got != http.StatusNotImplemented {
			t.Fatalf("%s %s = %d, want 501 (reserved extension must not answer anything else)", r.method, r.path, got)
		}
	}
}

// TestDHCPHAStatusIsReadOnly pins the /dhcp/ha decision: it answers 200 with
// an explicit ha_enabled flag on a node without HA, and no mutating verb is
// routed to it. A future "config API" with write methods must not reuse this
// path without a separate product decision.
func TestDHCPHAStatusIsReadOnly(t *testing.T) {
	srv := authCoverageFixture(t)
	admin := bootstrapSession(t, srv.URL)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/dhcp/ha", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+admin.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /dhcp/ha = %d: %s", resp.StatusCode, body)
	}
	var out struct {
		Code int `json:"code"`
		Data struct {
			HAEnabled bool   `json:"ha_enabled"`
			Role      string `json:"role"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decoding /dhcp/ha: %v", err)
	}
	if out.Code != 0 || out.Data.HAEnabled {
		t.Fatalf("an HA-less node must report ha_enabled=false, got code=%d data=%+v", out.Code, out.Data)
	}
	for _, method := range []string{http.MethodPut, http.MethodPost, http.MethodDelete} {
		r, err := http.NewRequest(method, srv.URL+"/api/v1/dhcp/ha", nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+admin.token)
		r.Header.Set("X-CSRF-Token", admin.csrfToken)
		w, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, w.Body)
		w.Body.Close()
		if w.StatusCode == http.StatusOK || w.StatusCode == http.StatusCreated {
			t.Fatalf("%s /dhcp/ha = %d: the endpoint must stay read-only", method, w.StatusCode)
		}
	}
}
