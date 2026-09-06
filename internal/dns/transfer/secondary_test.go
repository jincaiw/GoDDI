package transfer

import (
	"strings"
	"testing"
)

// TestParsePrimaryAddress covers the transport scheme prefixes for
// XFR-over-TLS (RFC 9103) support in transfer_policy.
func TestParsePrimaryAddress(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantAddr  string
		wantTLS   bool
		wantInsec bool
		wantErr   bool
	}{
		{"plain with port", "192.0.2.1:53", "192.0.2.1:53", false, false, false},
		{"plain default port", "primary.example.com", "primary.example.com:53", false, false, false},
		{"tls with port", "tls://primary.example.com:8530", "primary.example.com:8530", true, false, false},
		{"tls default port", "tls://primary.example.com", "primary.example.com:853", true, false, false},
		{"tls-insecure", "tls-insecure://192.0.2.7", "192.0.2.7:853", true, true, false},
		{"empty after scheme", "tls://", "", false, false, true},
		{"empty", "", "", false, false, true},
		// net.SplitHostPort does not validate the port is numeric, so an
		// unknown scheme is passed through untouched and the dialer will
		// reject it later.
		{"wrong scheme treated as host", "http://x.example", "http://x.example", false, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr, useTLS, insecure, err := parsePrimaryAddress(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got addr=%q", addr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if addr != tc.wantAddr {
				t.Errorf("addr = %q, want %q", addr, tc.wantAddr)
			}
			if useTLS != tc.wantTLS {
				t.Errorf("useTLS = %v, want %v", useTLS, tc.wantTLS)
			}
			if insecure != tc.wantInsec {
				t.Errorf("insecure = %v, want %v", insecure, tc.wantInsec)
			}
			// Recognised TLS schemes must never leak into the address.
			if tc.wantTLS && strings.Contains(addr, "tls") {
				t.Errorf("addr still contains scheme prefix: %q", addr)
			}
		})
	}
}

// TestParseTransferPolicyTLS ensures scheme prefixes coexist with TSIG
// fields in the pipe-delimited policy string.
func TestParseTransferPolicyTLS(t *testing.T) {
	cfg, err := parseTransferPolicy("tls://primary.example.com|tsig-key|c2VjcmV0|hmac-sha512")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.useTLS || cfg.tlsInsecure {
		t.Errorf("useTLS=%v insecure=%v, want true/false", cfg.useTLS, cfg.tlsInsecure)
	}
	if cfg.primaryAddr != "primary.example.com:853" {
		t.Errorf("addr = %q, want primary.example.com:853", cfg.primaryAddr)
	}
	if cfg.tsigName != "tsig-key" || cfg.tsigSecret != "c2VjcmV0" {
		t.Errorf("TSIG fields lost: %q/%q", cfg.tsigName, cfg.tsigSecret)
	}
	if cfg.tsigAlgo != "hmac-sha512" {
		t.Errorf("algo = %q, want hmac-sha512", cfg.tsigAlgo)
	}
}
