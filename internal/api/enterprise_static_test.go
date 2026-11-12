package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestEnterpriseStaticClosedRoute(t *testing.T) {
	srv, db := newSessionTestServer(t)
	admin, csrf := initialiseAndLogIn(t, srv, "enterprise_static_admin", "Strong-Static-Password-42!")
	viewerID := createUser(t, srv, admin, csrf, "enterprise_static_viewer", "Strong-Static-Password-42!")
	assignRole(t, srv, admin, csrf, viewerID, roleIDByName(t, db, "viewer"))
	viewer, viewerCSRF := logIn(t, srv, "enterprise_static_viewer", "Strong-Static-Password-42!")
	path := "/api/v1/ddi/tenants/11111111-1111-4111-8111-111111111111/ipam/spaces/22222222-2222-4222-8222-222222222222/allocations"
	body := `{"subnetId":"33333333-3333-4333-8333-333333333333","address":"192.0.2.1","ownerRef":"host","kind":"STATIC"}`
	key := "44444444-4444-4444-8444-444444444444"
	send := func(token, csrf, path, body string, keys []string, chunked bool) (int, string, []byte) {
		t.Helper()
		req, e := http.NewRequest("POST", srv.URL+path, bytes.NewBufferString(body))
		if e != nil {
			t.Fatal(e)
		}
		if chunked {
			req.ContentLength = -1
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		for _, k := range keys {
			req.Header.Add("Idempotency-Key", k)
		}
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		data, e := io.ReadAll(resp.Body)
		if e != nil {
			t.Fatal(e)
		}
		return resp.StatusCode, resp.Header.Get("Content-Type"), data
	}
	if status, content, _ := send("", "", path, body, []string{key}, false); status != 401 || !strings.HasPrefix(content, "application/problem+json") {
		t.Fatalf("anonymous %d %s", status, content)
	}
	if status, content, _ := send(viewer, viewerCSRF, path, body, []string{key}, false); status != 403 || !strings.HasPrefix(content, "application/problem+json") {
		t.Fatalf("viewer %d %s", status, content)
	}
	for _, tc := range []struct {
		name, path, body string
		keys             []string
		chunked          bool
		status           int
	}{
		{"closed", path, body, []string{key}, false, 503}, {"retry", path, body, []string{key}, false, 503},
		{"uppercase UUIDs", "/api/v1/ddi/tenants/AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA/ipam/spaces/BBBBBBBB-BBBB-4BBB-8BBB-BBBBBBBBBBBB/allocations", `{"subnetId":"CCCCCCCC-CCCC-4CCC-8CCC-CCCCCCCCCCCC","poolId":"DDDDDDDD-DDDD-4DDD-8DDD-DDDDDDDDDDDD","ownerRef":"host","kind":"STATIC"}`, []string{"EEEEEEEE-EEEE-4EEE-8EEE-EEEEEEEEEEEE"}, false, 503},
		{"mixed UUIDs", "/api/v1/ddi/tenants/AaAaAaAa-AaAa-4AaA-8AaA-AaAaAaAaAaAa/ipam/spaces/BbBbBbBb-BbBb-4BbB-8BbB-BbBbBbBbBbBb/allocations", `{"subnetId":"CcCcCcCc-CcCc-4CcC-8CcC-CcCcCcCcCcCc","poolId":"DdDdDdDd-DdDd-4DdD-8DdD-DdDdDdDdDdDd","ownerRef":"host","kind":"STATIC"}`, []string{"EeEeEeEe-EeEe-4EeE-8EeE-EeEeEeEeEeEe"}, false, 503},
		{"nil tenant", strings.Replace(path, "11111111-1111-4111-8111-111111111111", "00000000-0000-0000-0000-000000000000", 1), body, []string{key}, false, 400},
		{"nil subnet", path, strings.Replace(body, "33333333-3333-4333-8333-333333333333", "00000000-0000-0000-0000-000000000000", 1), []string{key}, false, 400},
		{"nil key", path, body, []string{"00000000-0000-0000-0000-000000000000"}, false, 400},
		{"compact subnet", path, strings.Replace(body, "33333333-3333-4333-8333-333333333333", "33333333333343338333333333333333", 1), []string{key}, false, 400},
		{"braced key", path, body, []string{"{" + key + "}"}, false, 400},
		{"duplicate key", path, body, []string{key, key}, false, 400}, {"missing key", path, body, nil, false, 400},
		{"bad tenant", strings.Replace(path, "11111111-1111-4111-8111-111111111111", "bad", 1), body, []string{key}, false, 400},
		{"duplicate json", path, strings.Replace(body, `"kind":"STATIC"`, `"kind":"STATIC","kind":"STATIC"`, 1), []string{key}, false, 400},
		{"unknown", path, strings.Replace(body, `"kind":"STATIC"`, `"kind":"STATIC","extra":1`, 1), []string{key}, false, 400},
		{"hold without seconds", path, strings.Replace(body, `"STATIC"`, `"HOLD"`, 1), []string{key}, false, 400},
		{"static pool", path, strings.Replace(body, `"address":"192.0.2.1"`, `"poolId":"55555555-5555-4555-8555-555555555555"`, 1), []string{key}, false, 503},
		{"hold address", path, strings.Replace(body, `"kind":"STATIC"`, `"kind":"HOLD","holdSeconds":1.0`, 1), []string{key}, false, 503},
		{"hold pool", path, `{"subnetId":"33333333-3333-4333-8333-333333333333","poolId":"55555555-5555-4555-8555-555555555555","ownerRef":"中文😀","kind":"HOLD","holdSeconds":864e2}`, []string{key}, false, 503},
		{"unpaired surrogate", path, strings.Replace(body, `"host"`, `"\uD800"`, 1), []string{key}, false, 400},
		{"paired surrogate", path, strings.Replace(body, `"host"`, `"\uD83D\uDE00"`, 1), []string{key}, false, 503},
		{"hold fractional", path, strings.Replace(body, `"kind":"STATIC"`, `"kind":"HOLD","holdSeconds":1.00000000000000001`, 1), []string{key}, false, 400},
		{"static seconds", path, strings.Replace(body, `"kind":"STATIC"`, `"kind":"STATIC","holdSeconds":1`, 1), []string{key}, false, 400},
		{"both address and pool", path, strings.Replace(body, `"kind":"STATIC"`, `"kind":"STATIC","poolId":"55555555-5555-4555-8555-555555555555"`, 1), []string{key}, false, 400},
		{"bad address", path, strings.Replace(body, "192.0.2.1", "::ffff:192.0.2.1", 1), []string{key}, false, 400},
		{"trailing", path, body + `{}`, []string{key}, false, 400},
		{"oversize chunked", path, body + strings.Repeat(" ", 1<<20), []string{key}, true, 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, content, data := send(admin, csrf, tc.path, tc.body, tc.keys, tc.chunked)
			if status != tc.status {
				t.Fatalf("got %d body %s", status, data)
			}
			if !strings.HasPrefix(content, "application/problem+json") {
				t.Fatalf("content %s", content)
			}
			var problem struct {
				Status    int    `json:"status"`
				Code      string `json:"code"`
				RequestID string `json:"requestId"`
			}
			if e := json.Unmarshal(data, &problem); e != nil || problem.Status != status || problem.Code == "" || problem.RequestID == "" {
				t.Fatalf("bad problem %s %v", data, e)
			}
		})
	}
	for _, auth := range []bool{false, true} {
		request, err := http.NewRequest(http.MethodPut, srv.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if auth {
			request.Header.Set("Authorization", "Bearer "+admin)
			request.Header.Set("X-CSRF-Token", csrf)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusMethodNotAllowed || !strings.HasPrefix(response.Header.Get("Content-Type"), "application/problem+json") {
			t.Fatalf("unsupported method auth=%v: status=%d content=%s", auth, response.StatusCode, response.Header.Get("Content-Type"))
		}
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+admin)
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("If-Match", `"v1"`)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("If-Match changed closed response: %d", resp.StatusCode)
	}
	for _, table := range []string{"ipam_spaces", "ipam_subnets", "ipam_addresses", "ipam_history"} {
		var n int
		if e := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); e != nil || n != 0 {
			t.Fatalf("business table %s has %d %v", table, n, e)
		}
	}
	for _, table := range []string{"ipam_tenants", "ipam_occupancies", "replica_event", "idempotency_receipt"} {
		var n int
		if e := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type='table' AND name=?`, table).Scan(&n); e != nil || n != 0 {
			t.Fatalf("unexpected prototype table %s %d %v", table, n, e)
		}
	}
}
