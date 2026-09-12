package api

// Privilege escalation, exercised as requests.
//
// router_permission_consistency_test.go compares the permission *declarations*
// written in three places, and it issues no request at all -- it cannot show
// that a real session holding real roles is actually stopped at the route it
// has no business using. These tests do, against the real router, with a real
// viewer account created through the real API.
//
// Three things make a 403 in this file mean what it says:
//
//   - the session is shown to be real and to hold a real permission, so the
//     denials are not the sound of a broken token;
//   - each route the viewer is refused on is shown to work for an
//     administrator, so the refusal is about the caller and not the route;
//   - the refusal is shown to be the RBAC one. The per-zone gate sits behind
//     the DNS routes and answers with a different message, so "the request was
//     stopped before it got that far" is checkable rather than assumed.
//
// The routes are deliberately ones that need no process-wide container. The
// DNS manager cache in internal/api/handler is guarded by a sync.Once, so only
// one server per process can use it; a test that reached the handlers here
// would decide, by file order, whether release_flow_test.go still works.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/database"
)

const (
	authzAdmin     = "w14_admin"
	authzAdminPass = "W14-Admin!Pass42"
	authzViewer    = "w14_viewer"
	authzViewerPwd = "W14-Viewer!Pass42"
	authzSpare     = "w14_spare"
	authzSparePwd  = "W14-Spare!Pass42"

	// The refusal the RBAC middleware writes. The per-zone gate behind it
	// writes a different one, which is why these can be compared exactly.
	permissionRefusal = "权限不足"
	zoneRefusal       = "区域权限不足"
)

func TestALowPrivilegeSessionIsStoppedAtTheRouteItCannotUse(t *testing.T) {
	srv, dbh := newSessionTestServer(t)

	adminToken, adminCSRF := initialiseAndLogIn(t, srv, authzAdmin, authzAdminPass)

	// must_change_password is passed as false deliberately. It defaults to
	// true, and a session that owes a password change is refused with a 403 of
	// its own -- leaving the default on would make every denial below
	// indistinguishable from that gate.
	viewerID := createUser(t, srv, adminToken, adminCSRF, authzViewer, authzViewerPwd)
	assignRole(t, srv, adminToken, adminCSRF, viewerID, roleIDByName(t, dbh, "viewer"))
	spareID := createUser(t, srv, adminToken, adminCSRF, authzSpare, authzSparePwd)

	viewerToken, viewerCSRF := logIn(t, srv, authzViewer, authzViewerPwd)

	// Controls 1 and 2: the session is real, and the viewer really does hold
	// the read permission it is about to be allowed to use. Without these the
	// denials below would pass on a session that was simply broken.
	if code, body, _ := doJSON(t, "GET", srv.URL+"/api/v1/auth/me", viewerToken, "", nil); code != http.StatusOK {
		t.Fatalf("viewer GET /auth/me = %d, want 200 (message=%q)", code, body.Message)
	}
	if code, body, _ := doJSON(t, "GET", srv.URL+"/api/v1/users", viewerToken, "", nil); code != http.StatusOK {
		t.Fatalf("viewer GET /users = %d, want 200 (message=%q): the viewer holds user:read, so a refusal here means the state under test is wrong",
			code, body.Message)
	}

	// Control 3: the refusal has to have had no effect, and the route has to
	// work for someone who does hold the permission.
	code, body, _ := doJSON(t, "DELETE", srv.URL+"/api/v1/users/"+spareID, viewerToken, viewerCSRF, nil)
	if code != http.StatusForbidden {
		t.Fatalf("viewer DELETE /users/{id} = %d, want 403 (message=%q)", code, body.Message)
	}
	if code, _, _ := doJSON(t, "GET", srv.URL+"/api/v1/users/"+spareID, adminToken, "", nil); code != http.StatusOK {
		t.Fatalf("the account the viewer was refused permission to delete is gone: the refusal was not a refusal")
	}
	if code, body, _ := doJSON(t, "DELETE", srv.URL+"/api/v1/users/"+spareID, adminToken, adminCSRF, nil); code != http.StatusOK {
		t.Fatalf("admin DELETE /users/{id} = %d, want 200 (message=%q): the route is broken, so the 403 above proves nothing",
			code, body.Message)
	}

	adminRoleID := roleIDByName(t, dbh, "admin")

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"create a user", "POST", "/api/v1/users", map[string]any{
			"username": "w14_refused", "password": "W14-Refused!Pass42"}},
		{"update a user", "PUT", "/api/v1/users/" + viewerID, map[string]any{"display_name": "not mine"}},
		{"delete a user", "DELETE", "/api/v1/users/" + viewerID, nil},
		{"grant itself the administrator role", "POST", "/api/v1/users/" + viewerID + "/roles",
			map[string]any{"role_ids": []string{adminRoleID}}},
		// The resources below are only ever reached through the middleware here,
		// which is the point: the denial happens before any handler runs.
		{"create a DNS zone", "POST", "/api/v1/dns/zones", map[string]any{"name": "w14-refused.example.test", "type": "primary"}},
		{"delete a DNS zone", "DELETE", "/api/v1/dns/zones/00000000-0000-0000-0000-000000000000", nil},
		{"create an IPAM space", "POST", "/api/v1/ipam/spaces", map[string]any{"name": "w14-refused"}},
		{"delete an IPAM space", "DELETE", "/api/v1/ipam/spaces/00000000-0000-0000-0000-000000000000", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body, _ := doJSON(t, tc.method, srv.URL+tc.path, viewerToken, viewerCSRF, tc.body)
			if code != http.StatusForbidden {
				t.Fatalf("%s as a viewer = %d, want 403 (message=%q)", tc.name, code, body.Message)
			}
			if body.Message == zoneRefusal {
				t.Errorf("%s was stopped by the per-zone gate, which means it got past the permission it does not hold", tc.name)
			}
			if body.Message != permissionRefusal {
				t.Errorf("%s refused with %q, want %q", tc.name, body.Message, permissionRefusal)
			}
			// A refusal that names the missing permission hands the caller a map
			// of the surface it has not been given.
			lowered := strings.ToLower(body.Message)
			for _, leak := range []string{"dns", "ipam", "user", "role", "write", "delete", "zone", "create"} {
				if strings.Contains(lowered, leak) {
					t.Errorf("the refusal names %q: %q", leak, body.Message)
				}
			}
		})
	}

	// The escalation attempt above has to have failed in fact, not only in the
	// response: the viewer ended the test holding exactly the one role it was
	// given, and the account it tried to create does not exist.
	if n := rolesHeldBy(t, dbh, viewerID); n != 1 {
		t.Errorf("the viewer holds %d roles, want 1: a refused escalation appears to have taken effect", n)
	}
	if n := usersNamed(t, dbh, "w14_refused"); n != 0 {
		t.Errorf("%d account(s) named w14_refused exist: a refused creation appears to have taken effect", n)
	}
}

// ---- helpers ---------------------------------------------------------------

// initialiseAndLogIn walks the first-run bootstrap, which is the only way to
// get an administrator on a fresh database.
func initialiseAndLogIn(t *testing.T, srv *httptest.Server, username, password string) (string, string) {
	t.Helper()
	if code, body, _ := doJSON(t, "POST", srv.URL+"/api/v1/auth/init", "", "", map[string]string{
		"username": username, "password": password, "email": username + "@example.test",
	}); code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("POST /auth/init = %d (message=%q data=%+v)", code, body.Message, body.Data)
	}
	return logIn(t, srv, username, password)
}

func logIn(t *testing.T, srv *httptest.Server, username, password string) (string, string) {
	t.Helper()
	code, body, _ := doJSON(t, "POST", srv.URL+"/api/v1/auth/login", "", "", map[string]string{
		"username": username, "password": password,
	})
	if code != http.StatusOK {
		t.Fatalf("POST /auth/login as %s = %d (message=%q)", username, code, body.Message)
	}
	token, _ := body.Data["token"].(string)
	csrf, _ := body.Data["csrf_token"].(string)
	if token == "" || csrf == "" {
		t.Fatalf("login as %s returned no token/csrf: %+v", username, body.Data)
	}
	return token, csrf
}

func createUser(t *testing.T, srv *httptest.Server, token, csrf, username, password string) string {
	t.Helper()
	code, body, _ := doJSON(t, "POST", srv.URL+"/api/v1/users", token, csrf, map[string]any{
		"username": username, "password": password,
		"must_change_password": false,
	})
	if code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("POST /users = %d (message=%q)", code, body.Message)
	}
	id, _ := body.Data["id"].(string)
	if id == "" {
		t.Fatalf("user id missing in response: %+v", body.Data)
	}
	return id
}

func assignRole(t *testing.T, srv *httptest.Server, token, csrf, userID, roleID string) {
	t.Helper()
	code, body, _ := doJSON(t, "POST", srv.URL+"/api/v1/users/"+userID+"/roles", token, csrf,
		map[string]any{"role_ids": []string{roleID}})
	if code != http.StatusOK {
		t.Fatalf("POST /users/{id}/roles = %d (message=%q)", code, body.Message)
	}
}

// roleIDByName reads a role's identifier out of the database rather than the
// API: the identifier is what the assignment call needs, and the roles list
// endpoint answers with an array the apiResp shape does not describe.
func roleIDByName(t *testing.T, dbh *database.DB, name string) string {
	t.Helper()
	var id string
	if err := dbh.DB.QueryRow("SELECT id FROM roles WHERE name = ?", name).Scan(&id); err != nil {
		t.Fatalf("read the %s role: %v", name, err)
	}
	return id
}

func rolesHeldBy(t *testing.T, dbh *database.DB, userID string) int {
	t.Helper()
	var n int
	if err := dbh.DB.QueryRow("SELECT COUNT(*) FROM user_roles WHERE user_id = ?", userID).Scan(&n); err != nil {
		t.Fatalf("count roles for %s: %v", userID, err)
	}
	return n
}

func usersNamed(t *testing.T, dbh *database.DB, username string) int {
	t.Helper()
	var n int
	if err := dbh.DB.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&n); err != nil {
		t.Fatalf("count users named %s: %v", username, err)
	}
	return n
}
