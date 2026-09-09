package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedRealIPIgnoresForwardedHeadersFromDirectPeer(t *testing.T) {
	h := TrustedRealIP(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "198.51.100.20:42000" {
			t.Fatalf("RemoteAddr = %q, want direct peer", r.RemoteAddr)
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.20:42000"
	req.Header.Set("X-Forwarded-For", "203.0.113.99")
	h.ServeHTTP(httptest.NewRecorder(), req)
}

func TestTrustedRealIPAcceptsTrustedProxyHeaders(t *testing.T) {
	h := TrustedRealIP([]string{"10.0.0.0/8"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "203.0.113.99:0" {
			t.Fatalf("RemoteAddr = %q, want proxied client", r.RemoteAddr)
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.1.2.3:42000"
	req.Header.Set("X-Forwarded-For", "203.0.113.99, 10.1.2.3")
	h.ServeHTTP(httptest.NewRecorder(), req)
}

func TestTrustedRealIPRejectsMalformedForwardedHeader(t *testing.T) {
	h := TrustedRealIP([]string{"10.0.0.0/8"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "10.1.2.3:42000" {
			t.Fatalf("RemoteAddr = %q, want proxy peer after malformed header", r.RemoteAddr)
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.1.2.3:42000"
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	h.ServeHTTP(httptest.NewRecorder(), req)
}
