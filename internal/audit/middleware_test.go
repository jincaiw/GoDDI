package audit

// The HTTP audit middleware.
//
// The middleware sits in front of every write route, so its mistakes are
// systemic rather than local. Two of them matter most:
//
//   - it reads the request body to summarise it. If it does not put the body
//     back, every write route in the product starts seeing an empty body --
//     which is a total outage caused by a logging change.
//   - it decides what to keep by a whitelist. A field that is not on the list
//     must not reach the table, because the bodies it sees include passwords,
//     tokens and API keys.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestASecretInTheBodyNeverReachesTheTrail(t *testing.T) {
	am := newAuditStore(t)
	srv := httptest.NewServer(AuditMiddleware(am)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})))
	defer srv.Close()

	// A body of the shape a real write route accepts, plus the credentials a
	// few of them also accept in the same object.
	const body = `{
		"name": "example.test",
		"description": "a zone",
		"password": "hunter2-should-never-be-logged",
		"token": "tok-should-never-be-logged",
		"api_key": "key-should-never-be-logged",
		"secret": "secret-should-never-be-logged",
		"private_key": "pk-should-never-be-logged",
		"csrf_token": "csrf-should-never-be-logged"
	}`
	send(t, srv, http.MethodPost, "/api/v1/dns/zones", body)

	logs, _, err := am.QueryLogs(AuditFilter{})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("recorded %d entries, want 1", len(logs))
	}
	detail := logs[0].Detail

	for _, secret := range []string{
		"hunter2-should-never-be-logged", "tok-should-never-be-logged",
		"key-should-never-be-logged", "secret-should-never-be-logged",
		"pk-should-never-be-logged", "csrf-should-never-be-logged",
		"password", "token", "api_key", "secret", "private_key", "csrf_token",
	} {
		if strings.Contains(detail, secret) {
			t.Errorf("the detail carries %q: %q", secret, detail)
		}
	}
	// And the fields that are on the list are still there, so the assertions
	// above are not passing because the detail is empty.
	for _, kept := range []string{"name=example.test", "description=a zone"} {
		if !strings.Contains(detail, kept) {
			t.Errorf("the detail lost %q: %q", kept, detail)
		}
	}
}

func TestTheBodyReachesTheHandlerIntact(t *testing.T) {
	am := newAuditStore(t)
	const body = `{"name":"example.test","subnets":["192.0.2.0/24"],"nested":{"a":1}}`

	var seen string
	srv := httptest.NewServer(AuditMiddleware(am)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("the handler could not read the body: %v", err)
		}
		seen = string(raw)
		w.WriteHeader(http.StatusOK)
	})))
	defer srv.Close()

	send(t, srv, http.MethodPost, "/api/v1/ipam/subnets", body)

	// Summarising the body for the trail must not consume it. A middleware that
	// forgets to restore it turns every write route in the product into one
	// that sees nothing.
	if seen != body {
		t.Errorf("the handler saw %q, want %q", seen, body)
	}
}

func TestOnlyWriteMethodsAreAudited(t *testing.T) {
	am := newAuditStore(t)
	srv := httptest.NewServer(AuditMiddleware(am)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	defer srv.Close()

	send(t, srv, http.MethodGet, "/api/v1/dns/zones", "")
	send(t, srv, http.MethodHead, "/api/v1/dns/zones", "")
	send(t, srv, http.MethodOptions, "/api/v1/dns/zones", "")
	if logs, _, err := am.QueryLogs(AuditFilter{}); err != nil || len(logs) != 0 {
		t.Fatalf("read-only requests produced %d entries (err %v), want 0", len(logs), err)
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		send(t, srv, method, "/api/v1/dns/zones", "")
	}
	logs, _, err := am.QueryLogs(AuditFilter{})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	if len(logs) != 4 {
		t.Fatalf("the four write methods produced %d entries, want 4", len(logs))
	}
}

func TestTheEntryRecordsWhatActuallyHappened(t *testing.T) {
	am := newAuditStore(t)
	srv := httptest.NewServer(AuditMiddleware(am)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/refused") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})))
	defer srv.Close()

	send(t, srv, http.MethodPost, "/api/v1/dns/zones/allowed", `{"name":"a"}`)
	send(t, srv, http.MethodPost, "/api/v1/dns/zones/refused", `{"name":"b"}`)

	logs, _, err := am.QueryLogs(AuditFilter{})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("recorded %d entries, want 2", len(logs))
	}
	// A refusal is the entry an operator goes looking for; recording it as a
	// success would make the trail say the request worked.
	outcomes := map[string]bool{}
	for _, l := range logs {
		outcomes[l.Detail] = l.Success
	}
	if !outcomes["name=a"] {
		t.Error("the request that succeeded was recorded as a failure")
	}
	if outcomes["name=b"] {
		t.Error("the request that was refused was recorded as a success")
	}
}

func TestTheSameRequestProducesTheSameDetail(t *testing.T) {
	am := newAuditStore(t)
	srv := httptest.NewServer(AuditMiddleware(am)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	defer srv.Close()

	// The same object with its keys in a different order. A detail string that
	// follows Go's map iteration order would differ between two identical
	// requests, which makes the trail unusable for comparison.
	send(t, srv, http.MethodPut, "/api/v1/dns/zones/z1", `{"name":"a","description":"b","ttl":300}`)
	send(t, srv, http.MethodPut, "/api/v1/dns/zones/z2", `{"ttl":300,"description":"b","name":"a"}`)

	logs, _, err := am.QueryLogs(AuditFilter{})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("recorded %d entries, want 2", len(logs))
	}
	if logs[0].Detail != logs[1].Detail {
		t.Errorf("the same object produced %q and %q", logs[0].Detail, logs[1].Detail)
	}
	if want := "description=b name=a ttl=300"; logs[0].Detail != want {
		t.Errorf("detail = %q, want %q", logs[0].Detail, want)
	}
}

func TestABodyThatIsNotJSONIsKeptAsASnippet(t *testing.T) {
	am := newAuditStore(t)
	srv := httptest.NewServer(AuditMiddleware(am)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	defer srv.Close()

	// Not JSON, so there is no whitelist to apply. The prefix is kept so the
	// entry is not blank, and it is bounded so one request cannot fill the
	// table.
	long := strings.Repeat("A", 400)
	send(t, srv, http.MethodPost, "/api/v1/dns/zones/import", long)

	logs, _, err := am.QueryLogs(AuditFilter{})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("recorded %d entries, want 1", len(logs))
	}
	detail := logs[0].Detail
	if !strings.HasPrefix(detail, strings.Repeat("A", 256)) || !strings.HasSuffix(detail, "...") {
		t.Errorf("a 400-byte non-JSON body was recorded as %d characters: %q", len(detail), detail)
	}
}

func TestThePathBecomesAResourceAndAnID(t *testing.T) {
	for _, tc := range []struct {
		path     string
		resource string
		id       string
	}{
		{"/api/v1/dns/zones", "dns", ""},
		{"/api/v1/dns/zones/zone-1", "dns", "zone-1"},
		{"/api/v1/dns/zones/zone-1/records", "dns", "zone-1"},
		{"/api/v1/ipam/subnets/sub-1/dhcp-scope-plan", "ipam", "sub-1"},
		{"/api/v1/users", "user", ""},
		{"/api/v1/users/user-1/roles", "user", "user-1"},
		{"/api/v1/leases/lease-1", "lease", "lease-1"},
		// An unknown prefix falls back to dropping one trailing 's', so a route
		// added without touching the map still gets a usable resource name.
		{"/api/v1/widgets", "widget", ""},
		{"/api/v1/widget/thing", "widget", "thing"},
		{"/api/v1/", "", ""},
		{"/health", "", ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			resource, id := parseResourceFromPath(tc.path)
			if resource != tc.resource || id != tc.id {
				t.Errorf("parseResourceFromPath(%q) = (%q, %q), want (%q, %q)",
					tc.path, resource, id, tc.resource, tc.id)
			}
		})
	}
}

func TestTheMethodBecomesAnAction(t *testing.T) {
	for method, want := range map[string]string{
		http.MethodPost:   "create",
		http.MethodPut:    "update",
		http.MethodPatch:  "update",
		http.MethodDelete: "delete",
	} {
		if got := methodToAction(method); got != want {
			t.Errorf("methodToAction(%s) = %q, want %q", method, got, want)
		}
	}
	// Anything else falls through to its own name rather than to a write verb:
	// guessing "create" for an unknown method would put a write in the trail
	// that never happened.
	if got := methodToAction(http.MethodGet); got != "get" {
		t.Errorf("methodToAction(GET) = %q, want %q", got, "get")
	}
}

func TestTheClientIsRecordedWhole(t *testing.T) {
	for _, tc := range []struct {
		remoteAddr string
		want       string
	}{
		// The regression this replaced produced "[" or "" for an IPv6 client.
		{"[2001:db8::1]:54321", "2001:db8::1"},
		{"192.0.2.10:54321", "192.0.2.10"},
		// RemoteAddr is not always host:port; a bare value is kept as it is
		// rather than truncated to nothing.
		{"192.0.2.10", "192.0.2.10"},
		{"", ""},
	} {
		t.Run(tc.remoteAddr, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			if got := getClientIP(r); got != tc.want {
				t.Errorf("getClientIP(%q) = %q, want %q", tc.remoteAddr, got, tc.want)
			}
		})
	}
}

// ---- helpers ---------------------------------------------------------------

func send(t *testing.T, srv *httptest.Server, method, path, body string) int {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	if body != "" {
		req.ContentLength = int64(len(body))
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}
