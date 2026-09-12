package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	baseURL    = integrationBaseURL()
	jwtToken   string
	csrfToken  string
	runID      = fmt.Sprintf("%d", time.Now().UnixNano())
	httpClient = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
)

func integrationBaseURL() string {
	if value := strings.TrimRight(os.Getenv("GODDI_TEST_BASE_URL"), "/"); value != "" {
		return value
	}
	return "http://127.0.0.1:6080"
}

func main() {
	passed := 0
	failed := 0
	total := 0

	test := func(name string, fn func() error) {
		total++
		fmt.Printf("  [%d] %s ... ", total, name)
		if err := fn(); err != nil {
			fmt.Printf("FAIL\n       %v\n", err)
			failed++
		} else {
			fmt.Println("PASS")
			passed++
		}
	}

	fmt.Println("=== GoDDI Integration Test Suite ===")
	fmt.Println()

	// ---- Phase 1: Health & Unauthenticated ----
	fmt.Println("--- Phase 1: Health & Unauthenticated ---")
	test("GET /health", testHealth)
	test("GET /health returns valid JSON", testHealthJSON)
	test("GET /ready is served and has probes registered", testReady)
	test("Unauthenticated GET /api/v1/users returns 401", testUnauthUsers)
	test("Unauthenticated POST /api/v1/auth/login with bad creds returns 401", testBadLogin)
	test("GET /metrics without a credential returns 401", testMetricsRequiresAuth)

	// ---- Phase 2: Authentication ----
	fmt.Println("\n--- Phase 2: Authentication ---")
	test("POST /api/v1/auth/init - initialize admin", testInitAdmin)
	test("POST /api/v1/auth/login - admin login", testLogin)
	test("GET /api/v1/auth/me - get current user", testGetMe)
	test("POST /api/v1/auth/refresh - refresh token", testRefreshToken)
	test("GET /api/v1/auth/sessions - list sessions", testListSessions)

	// ---- Phase 3: RBAC & Users ----
	fmt.Println("\n--- Phase 3: RBAC & Users ---")
	test("GET /api/v1/users - list users", testListUsers)
	test("POST /api/v1/roles - create role", testCreateRole)
	test("GET /api/v1/roles - list roles", testListRoles)
	test("POST /api/v1/groups - create group", testCreateGroup)
	test("GET /api/v1/groups - list groups", testListGroups)
	test("GET /api/v1/permissions - list permissions", testListPermissions)

	// ---- Phase 4: DNS Zones ----
	fmt.Println("\n--- Phase 4: DNS Zones ---")
	test("POST /api/v1/dns/zones - create zone", func() error {
		return testCreateZoneWithType("primary")
	})
	test("GET /api/v1/dns/zones - list zones", testListZones)
	test("GET /api/v1/dns/zones/:id - get zone", testGetZone)

	// ---- Phase 5: DNS Records ----
	fmt.Println("\n--- Phase 5: DNS Records ---")
	test("POST /api/v1/dns/zones/:zoneId/records - create record", testCreateRecord)
	test("GET /api/v1/dns/zones/:zoneId/records - list records", testListRecords)

	// ---- Phase 6: DNS Forwarders ----
	fmt.Println("\n--- Phase 6: DNS Forwarders ---")
	test("POST /api/v1/dns/forwarders - create forwarder", testCreateForwarder)
	test("GET /api/v1/dns/forwarders - list forwarders", testListForwarders)

	// ---- Phase 7: DNS Security ----
	fmt.Println("\n--- Phase 7: DNS Security ---")
	test("POST /api/v1/dns/security/policies - create policy", testCreateClientPolicy)
	test("GET /api/v1/dns/security/policies - list policies", testListClientPolicies)
	test("GET /api/v1/dns/security/blocklists - list blocklists", testListBlocklists)
	test("GET /api/v1/dns/security/allowlists - list allowlists", testListAllowlists)

	// ---- Phase 8: DHCP Scopes ----
	fmt.Println("\n--- Phase 8: DHCP Scopes ---")
	test("POST /api/v1/dhcp/scopes - create scope", testCreateScope)
	test("GET /api/v1/dhcp/scopes - list scopes", testListScopes)
	test("GET /api/v1/dhcp/leases - list leases", testListLeases)

	// ---- Phase 9: IPAM ----
	fmt.Println("\n--- Phase 9: IPAM ---")
	test("POST /api/v1/ipam/spaces - create space", testCreateSpace)
	test("GET /api/v1/ipam/spaces - list spaces", testListSpaces)
	test("POST /api/v1/ipam/subnets - create subnet", testCreateSubnet)
	test("GET /api/v1/ipam/subnets - list subnets", testListSubnets)
	test("GET /api/v1/ipam/addresses - list addresses", testListAddresses)

	// ---- Phase 10: System ----
	fmt.Println("\n--- Phase 10: System ---")
	test("GET /api/v1/settings - list settings", testListSettings)
	test("GET /api/v1/dashboard - dashboard", testDashboard)
	test("GET /api/v1/audit-logs - audit logs", testAuditLogs)
	test("GET /api/v1/tasks - task list", testTaskList)
	test("GET /api/v1/dns/cache - DNS cache stats", testDNSCache)
	test("GET /metrics with an admin credential - Prometheus payload", testMetricsWithToken)

	// ---- Phase 11: Extensions (501) ----
	fmt.Println("\n--- Phase 11: Extensions (501 Not Implemented) ---")
	test("GET /api/v1/sso - returns 501", testExtSSO)
	test("GET /api/v1/cluster - returns 501", testExtCluster)
	test("GET /api/v1/dhcp/ha - returns 501", testExtDHCPHA)

	// ---- Phase 12: Cleanup ----
	fmt.Println("\n--- Phase 12: Cleanup & Session ---")
	test("DELETE /api/v1/dns/zones/:id - delete zone", testDeleteZone)
	test("DELETE /api/v1/dhcp/scopes/:id - delete scope", testDeleteScope)
	test("DELETE /api/v1/ipam/subnets (cleanup before space)", testDeleteSubnets)
	test("DELETE /api/v1/ipam/spaces/:id - delete space", testDeleteSpace)
	test("POST /api/v1/auth/logout - logout", testLogout)

	fmt.Println()
	fmt.Printf("=== Results: %d/%d passed, %d failed ===\n", passed, total, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// ========== Helpers ==========

func doRequest(method, path string, body interface{}) (*http.Response, map[string]interface{}) {
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, baseURL+path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if jwtToken != "" {
		req.Header.Set("Authorization", "Bearer "+jwtToken)
	}
	// CSRF protection now applies to all mutating methods, so include
	// the token issued at login/refresh time.
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	return resp, result
}

func doRequestRaw(method, path string, body interface{}) (*http.Response, []byte) {
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, baseURL+path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if jwtToken != "" {
		req.Header.Set("Authorization", "Bearer "+jwtToken)
	}
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

// ========== Phase 1: Health & Unauthenticated ==========

func testHealth() error {
	resp, _ := doRequest("GET", "/health", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d", resp.StatusCode)
	}
	return nil
}

func testHealthJSON() error {
	resp, result := doRequest("GET", "/health", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if status, _ := result["status"].(string); status != "ok" {
		return fmt.Errorf("expected status=ok, got %v", result["status"])
	}
	return nil
}

// testReady checks the readiness endpoint is served and that something is
// being watched. A process with no probes registered answers 503 by design,
// so this also catches a wiring change that drops the registration.
func testReady() error {
	resp, result := doRequest("GET", "/ready", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d (body %v)", resp.StatusCode, result)
	}
	status, _ := result["status"].(string)
	if status != "ok" && status != "degraded" {
		return fmt.Errorf("expected status ok or degraded, got %v", result["status"])
	}
	// The route is unauthenticated, so it must stay coarse: the level and
	// nothing else. The planes, reasons and counters live on the authenticated
	// route and would be reconnaissance here.
	if len(result) != 1 {
		return fmt.Errorf("expected only a status field on the unauthenticated route, got %v", result)
	}
	return nil
}

func testUnauthUsers() error {
	savedToken := jwtToken
	jwtToken = ""
	resp, _ := doRequest("GET", "/api/v1/users", nil)
	jwtToken = savedToken
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 401 {
		return fmt.Errorf("expected 401, got %d", resp.StatusCode)
	}
	return nil
}

func testBadLogin() error {
	resp, _ := doRequest("POST", "/api/v1/auth/login", map[string]string{
		"username": "nonexistent",
		"password": "wrongpassword",
	})
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 401 {
		return fmt.Errorf("expected 401, got %d", resp.StatusCode)
	}
	return nil
}

// testMetricsRequiresAuth pins the fact that /metrics is part of the
// management surface, not a probe. Its payload exposes request volumes, route
// patterns and status distributions, so an unauthenticated caller must not
// read it. This used to expect 200 with no credential; that assertion went
// stale when the endpoint was hardened, and a stale pass here is worse than
// no test because it reports the old contract as verified.
func testMetricsRequiresAuth() error {
	savedToken := jwtToken
	jwtToken = ""
	resp, data := doRequestRaw("GET", "/metrics", nil)
	jwtToken = savedToken
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 401 {
		return fmt.Errorf("expected 401 without a credential, got %d (body %s)", resp.StatusCode, string(data[:min(200, len(data))]))
	}
	if strings.Contains(string(data), "goddi_") {
		return fmt.Errorf("metrics payload leaked to an unauthenticated caller")
	}
	return nil
}

// testMetricsWithToken checks the other half: a credential produces the
// payload it is supposed to. Both halves are needed -- 401 alone is also what
// a broken registry or a disabled endpoint returns.
func testMetricsWithToken() error {
	resp, data := doRequestRaw("GET", "/metrics", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200 with a credential, got %d (body %s)", resp.StatusCode, string(data[:min(200, len(data))]))
	}
	if !strings.Contains(string(data), "goddi_") {
		return fmt.Errorf("expected prometheus metrics, got: %s", string(data[:min(200, len(data))]))
	}
	return nil
}

// ========== Phase 2: Authentication ==========

func testInitAdmin() error {
	resp, result := doRequest("POST", "/api/v1/auth/init", map[string]string{
		"username": "admin",
		"password": "Admin@123456",
		"email":    "admin@goddi.local",
	})
	if resp == nil {
		return fmt.Errorf("no response")
	}
	// May be 200 (first init) or 409 (already initialized)
	if resp.StatusCode != 200 && resp.StatusCode != 409 {
		return fmt.Errorf("expected 200 or 409, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testLogin() error {
	resp, result := doRequest("POST", "/api/v1/auth/login", map[string]string{
		"username": "admin",
		"password": "Admin@123456",
	})
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing data field in login response: %v", result)
	}
	token, ok := data["token"].(string)
	if !ok || token == "" {
		return fmt.Errorf("missing token in login response data: %v", data)
	}
	jwtToken = token
	if csrf, ok := data["csrf_token"].(string); ok {
		csrfToken = csrf
	}
	return nil
}

func testGetMe() error {
	resp, result := doRequest("GET", "/api/v1/auth/me", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing data in me response: %v", result)
	}
	username, _ := data["username"].(string)
	if username != "admin" {
		return fmt.Errorf("expected username=admin, got %s", username)
	}
	return nil
}

func testRefreshToken() error {
	resp, result := doRequest("POST", "/api/v1/auth/refresh", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	data, ok := result["data"].(map[string]interface{})
	if ok {
		if newToken, ok := data["token"].(string); ok && newToken != "" {
			jwtToken = newToken
		}
		if newCSRF, ok := data["csrf_token"].(string); ok && newCSRF != "" {
			csrfToken = newCSRF
		}
	}
	return nil
}

func testListSessions() error {
	resp, result := doRequest("GET", "/api/v1/auth/sessions", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

// ========== Phase 3: RBAC & Users ==========

func testListUsers() error {
	resp, result := doRequest("GET", "/api/v1/users", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testCreateRole() error {
	resp, result := doRequest("POST", "/api/v1/roles", map[string]interface{}{
		"name":        "test-role-" + runID,
		"description": "Test role for integration testing",
		"permissions": []string{"dns:read", "dns:write"},
	})
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("expected 200/201, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testListRoles() error {
	resp, result := doRequest("GET", "/api/v1/roles", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testCreateGroup() error {
	resp, result := doRequest("POST", "/api/v1/groups", map[string]interface{}{
		"name":        "test-group-" + runID,
		"description": "Test group for integration testing",
	})
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("expected 200/201, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testListGroups() error {
	resp, result := doRequest("GET", "/api/v1/groups", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testListPermissions() error {
	resp, result := doRequest("GET", "/api/v1/permissions", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

// ========== Phase 4: DNS Zones ==========

var createdZoneID string

func testCreateZoneWithType(zoneType string) error {
	resp, result := doRequest("POST", "/api/v1/dns/zones", map[string]interface{}{
		"name":   "test.example.com",
		"type":   zoneType,
		"ttl":    3600,
		"admin":  "admin.test.example.com",
		"serial": 2024010101,
	})
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("expected 200/201, got %d, body: %v", resp.StatusCode, result)
	}
	data, ok := result["data"].(map[string]interface{})
	if ok {
		if id, ok := data["id"]; ok {
			createdZoneID = fmt.Sprintf("%v", id)
		}
	}
	return nil
}

func testListZones() error {
	resp, result := doRequest("GET", "/api/v1/dns/zones", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testGetZone() error {
	if createdZoneID == "" {
		return fmt.Errorf("no zone ID available (create zone first)")
	}
	resp, result := doRequest("GET", "/api/v1/dns/zones/"+createdZoneID, nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

// ========== Phase 5: DNS Records ==========

func testCreateRecord() error {
	if createdZoneID == "" {
		return fmt.Errorf("no zone ID available")
	}
	resp, result := doRequest("POST", "/api/v1/dns/zones/"+createdZoneID+"/records", map[string]interface{}{
		"name":     "www",
		"type":     "A",
		"value":    "192.168.1.100",
		"ttl":      3600,
		"priority": 0,
	})
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("expected 200/201, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testListRecords() error {
	if createdZoneID == "" {
		return fmt.Errorf("no zone ID available")
	}
	resp, result := doRequest("GET", "/api/v1/dns/zones/"+createdZoneID+"/records", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

// ========== Phase 6: DNS Forwarders ==========

func testCreateForwarder() error {
	resp, result := doRequest("POST", "/api/v1/dns/forwarders", map[string]interface{}{
		"name":     "test-forwarder",
		"address":  "8.8.8.8:53",
		"protocol": "udp",
		"priority": 10,
	})
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("expected 200/201, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testListForwarders() error {
	resp, result := doRequest("GET", "/api/v1/dns/forwarders", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

// ========== Phase 7: DNS Security ==========

func testCreateClientPolicy() error {
	resp, result := doRequest("POST", "/api/v1/dns/security/policies", map[string]interface{}{
		"name":          "test-policy",
		"source_cidr":   "192.168.0.0/16",
		"match_type":    "cidr",
		"response_type": "allow",
		"action":        "allow",
		"priority":      10,
	})
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("expected 200/201, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testListClientPolicies() error {
	resp, result := doRequest("GET", "/api/v1/dns/security/policies", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testListBlocklists() error {
	resp, result := doRequest("GET", "/api/v1/dns/security/blocklists", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testListAllowlists() error {
	resp, result := doRequest("GET", "/api/v1/dns/security/allowlists", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

// ========== Phase 8: DHCP Scopes ==========

var createdScopeID string

func testCreateScope() error {
	resp, result := doRequest("POST", "/api/v1/dhcp/scopes", map[string]interface{}{
		"name":        "test-scope",
		"subnet":      "192.168.100.0/24",
		"start_ip":    "192.168.100.10",
		"end_ip":      "192.168.100.200",
		"router":      "192.168.100.1",
		"dns_servers": "8.8.8.8",
		"lease_time":  86400,
	})
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("expected 200/201, got %d, body: %v", resp.StatusCode, result)
	}
	data, ok := result["data"].(map[string]interface{})
	if ok {
		if id, ok := data["id"]; ok {
			createdScopeID = fmt.Sprintf("%v", id)
		}
	}
	return nil
}

func testListScopes() error {
	resp, result := doRequest("GET", "/api/v1/dhcp/scopes", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testListLeases() error {
	resp, result := doRequest("GET", "/api/v1/dhcp/leases", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

// ========== Phase 9: IPAM ==========

var createdSpaceID string

func testCreateSpace() error {
	resp, result := doRequest("POST", "/api/v1/ipam/spaces", map[string]interface{}{
		"name":        "test-space",
		"description": "Test IPAM space",
	})
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("expected 200/201, got %d, body: %v", resp.StatusCode, result)
	}
	data, ok := result["data"].(map[string]interface{})
	if ok {
		if id, ok := data["id"]; ok {
			createdSpaceID = fmt.Sprintf("%v", id)
		}
	}
	return nil
}

func testListSpaces() error {
	resp, result := doRequest("GET", "/api/v1/ipam/spaces", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testCreateSubnet() error {
	resp, result := doRequest("POST", "/api/v1/ipam/subnets", map[string]interface{}{
		"space_id":    createdSpaceID,
		"name":        "test-subnet",
		"cidr":        "10.0.0.0/24",
		"description": "Test subnet",
	})
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("expected 200/201, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testListSubnets() error {
	resp, result := doRequest("GET", "/api/v1/ipam/subnets", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testListAddresses() error {
	resp, result := doRequest("GET", "/api/v1/ipam/addresses", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

// ========== Phase 10: System ==========

func testListSettings() error {
	resp, result := doRequest("GET", "/api/v1/settings", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testDashboard() error {
	resp, result := doRequest("GET", "/api/v1/dashboard", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testAuditLogs() error {
	resp, result := doRequest("GET", "/api/v1/audit-logs", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testTaskList() error {
	resp, result := doRequest("GET", "/api/v1/tasks", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testDNSCache() error {
	resp, result := doRequest("GET", "/api/v1/dns/cache", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

// ========== Phase 11: Extensions (501) ==========

func testExtSSO() error {
	resp, result := doRequest("GET", "/api/v1/sso", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 501 {
		return fmt.Errorf("expected 501, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testExtCluster() error {
	resp, result := doRequest("GET", "/api/v1/cluster", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 501 {
		return fmt.Errorf("expected 501, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testExtDHCPHA() error {
	resp, result := doRequest("GET", "/api/v1/dhcp/ha", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 501 {
		return fmt.Errorf("expected 501, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

// ========== Phase 12: Cleanup ==========

func testDeleteZone() error {
	if createdZoneID == "" {
		return fmt.Errorf("no zone ID to delete")
	}
	resp, result := doRequest("DELETE", "/api/v1/dns/zones/"+createdZoneID, nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return fmt.Errorf("expected 200/204, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testDeleteScope() error {
	if createdScopeID == "" {
		return fmt.Errorf("no scope ID to delete")
	}
	resp, result := doRequest("DELETE", "/api/v1/dhcp/scopes/"+createdScopeID, nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return fmt.Errorf("expected 200/204, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testDeleteSpace() error {
	if createdSpaceID == "" {
		return fmt.Errorf("no space ID to delete")
	}
	resp, result := doRequest("DELETE", "/api/v1/ipam/spaces/"+createdSpaceID, nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return fmt.Errorf("expected 200/204, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func testDeleteSubnets() error {
	// List subnets and delete them all before deleting the space
	resp, result := doRequest("GET", "/api/v1/ipam/subnets", nil)
	if resp == nil || resp.StatusCode != 200 {
		return fmt.Errorf("failed to list subnets for cleanup")
	}
	data, ok := result["data"].([]interface{})
	if !ok {
		return nil // no subnets to delete
	}
	for _, item := range data {
		if subnet, ok := item.(map[string]interface{}); ok {
			if id, ok := subnet["id"].(string); ok && id != "" {
				doRequest("DELETE", "/api/v1/ipam/subnets/"+id, nil)
			}
		}
	}
	return nil
}

func testLogout() error {
	resp, result := doRequest("POST", "/api/v1/auth/logout", nil)
	if resp == nil {
		return fmt.Errorf("no response")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("expected 200, got %d, body: %v", resp.StatusCode, result)
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
