package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// allowListProbe builds the middleware under test together with the flag that
// says whether the wrapped handler ever ran.
func allowListProbe(cidrs []string) (func(http.Handler) http.Handler, *bool) {
	reached := false
	return AdminAllowList(cidrs), &reached
}

func serveFrom(mw func(http.Handler) http.Handler, remoteAddr string, reached *bool) *httptest.ResponseRecorder {
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAnEmptyAllowListChangesNothing(t *testing.T) {
	mw, reached := allowListProbe(nil)

	// The middleware must be transparent, not merely permissive: a deployment
	// that never configured an allowlist must not have its request pipeline
	// wrapped in a check that happens to return true.
	if rec := serveFrom(mw, "203.0.113.9:41234", reached); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !*reached {
		t.Error("the wrapped handler was not called")
	}
}

func TestTheAllowListAdmitsOnlyTheNetworksItNames(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		wantCode   int
	}{
		{"inside the /24", "10.20.30.40:5000", http.StatusOK},
		{"the network address itself", "10.20.30.0:5000", http.StatusOK},
		{"the broadcast address", "10.20.30.255:5000", http.StatusOK},
		{"outside the /24", "10.20.31.40:5000", http.StatusForbidden},
		{"a private-range neighbour", "10.20.29.40:5000", http.StatusForbidden},
		{"the loopback", "127.0.0.1:5000", http.StatusForbidden},
		{"a host route inside the /24", "10.20.30.7:5000", http.StatusOK},
		{"an IPv6 client against an IPv4-only list", "[2001:db8::1]:5000", http.StatusForbidden},
	}
	mw, reached := allowListProbe([]string{"10.20.30.0/24", "10.20.30.7/32"})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			*reached = false
			rec := serveFrom(mw, tc.remoteAddr, reached)
			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			if want := tc.wantCode == http.StatusOK; *reached != want {
				t.Errorf("handler reached = %v, want %v", *reached, want)
			}
		})
	}
}

func TestTheAllowListHandlesIPv6(t *testing.T) {
	mw, reached := allowListProbe([]string{"2001:db8:1234::/48"})

	if rec := serveFrom(mw, "[2001:db8:1234::5]:5000", reached); rec.Code != http.StatusOK {
		t.Errorf("in-range IPv6 client got %d, want 200", rec.Code)
	}
	*reached = false
	if rec := serveFrom(mw, "[2001:db8:1235::5]:5000", reached); rec.Code != http.StatusForbidden {
		t.Errorf("out-of-range IPv6 client got %d, want 403", rec.Code)
	}
}

// TestTheAllowListJudgesTheClientNotTheProxy pins the interaction with
// TrustedRealIP. A console behind a proxy sees every request arrive from the
// proxy; if the allowlist judged that address, an operator who listed their own
// workstation would be locked out and an operator who listed the proxy would
// have protected nothing.
func TestTheAllowListJudgesTheClientNotTheProxy(t *testing.T) {
	chain := func(next http.Handler) http.Handler {
		return TrustedRealIP([]string{"10.0.0.0/8"})(AdminAllowList([]string{"192.0.2.0/24"})(next))
	}
	reached := false
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.RemoteAddr = "10.0.0.1:33333" // the proxy, inside the trusted range
	req.Header.Set("X-Forwarded-For", "192.0.2.77")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !reached {
		t.Fatalf("a listed client behind a trusted proxy got %d (reached=%v), want 200",
			rec.Code, reached)
	}

	// The same proxy forwarding for a client that is not listed.
	reached = false
	req = httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.RemoteAddr = "10.0.0.1:33334"
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden || reached {
		t.Fatalf("an unlisted client got %d (reached=%v), want 403", rec.Code, reached)
	}

	// And from an untrusted peer the header means nothing, so the peer itself
	// is judged -- otherwise anyone could put a listed address in a header.
	reached = false
	req = httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.RemoteAddr = "198.51.100.9:55555"
	req.Header.Set("X-Forwarded-For", "192.0.2.77")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden || reached {
		t.Fatalf("an untrusted peer spoofed itself into the allowlist: %d (reached=%v)", rec.Code, reached)
	}
}

// TestAnUnparsableEntryRefusesEverything is the backstop. Config validation
// rejects such an entry at startup, so this path is a backstop rather than
// something an operator walks -- but if it is ever reached, the two available
// behaviours are "ignore the entry" (the allowlist is quietly wider than it
// says) and "refuse everything" (the console is down and the log says why).
// Only one of those is a failure anybody can act on.
func TestAnUnparsableEntryRefusesEverything(t *testing.T) {
	for _, entry := range []string{"10.0.0.5", "not-a-network", "10.0.0.0/33"} {
		t.Run(entry, func(t *testing.T) {
			mw, reached := allowListProbe([]string{"192.0.2.0/24", entry})
			rec := serveFrom(mw, "192.0.2.77:5000", reached)
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403: a broken entry must not leave the list wider than it reads", rec.Code)
			}
			if *reached {
				t.Error("the handler was reached despite an unparsable entry")
			}
		})
	}
}

func TestTheRefusalDoesNotSayWhatWouldHaveWorked(t *testing.T) {
	mw, reached := allowListProbe([]string{"192.0.2.0/24"})
	rec := serveFrom(mw, "198.51.100.9:5000", reached)

	if rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Errorf("content type = %q", rec.Header().Get("Content-Type"))
	}
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Code != http.StatusForbidden {
		t.Errorf("body code = %d, want 403", body.Code)
	}
	// Nothing about credentials, tokens or permissions: the answer is the same
	// whether or not the caller holds a valid one.
	for _, leak := range []string{"token", "password", "credential", "permission", "unauthorized"} {
		if containsFold(body.Message, leak) {
			t.Errorf("refusal message %q mentions %q", body.Message, leak)
		}
	}
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
