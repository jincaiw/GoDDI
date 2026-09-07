package handler

import (
	"net"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
)

// --- DNS name validation regression tests (T2) ---

func TestValidateDNSName(t *testing.T) {
	valid := []string{
		"example.com",
		"www.example.com",
		"a.b.c.d.example.com",
		"xn--e1afmkfd.xn--p1ai", // punycode IDN
		"my-host.example.com",   // LDH label
		strings.Repeat("a", 63) + ".example.com", // max label length
	}
	for _, name := range valid {
		if err := validateDNSName(name); err != nil {
			t.Errorf("validateDNSName(%q) = %v, want nil", name, err)
		}
	}

	invalid := []struct {
		name   string
		reason string
	}{
		{"", "empty"},
		{".example.com", "leading dot"},
		{"example.com.", "trailing dot"},
		{"exa..mple.com", "empty label"},
		{"exa mple.com", "space"},
		{"exa\x00mple.com", "NUL byte"},
		{"exa\x07mple.com", "control char"},
		{strings.Repeat("a", 64) + ".example.com", "label too long"},
		{strings.Repeat("a.", 128) + "com", "name too long"},
	}
	for _, tt := range invalid {
		if err := validateDNSName(tt.name); err == nil {
			t.Errorf("validateDNSName(%q) = nil, want error (%s)", tt.name, tt.reason)
		}
	}
}

// --- SSRF / private-address filter regression tests (T2, U2) ---

func TestIsPrivateIP(t *testing.T) {
	private := []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "172.31.255.255",
		"192.168.1.1", "169.254.1.1", "0.0.0.0",
		"::1", "fe80::1", "fd00::1", "::ffff:127.0.0.1",
	}
	for _, s := range private {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("test bug: cannot parse %q", s)
		}
		if !isPrivateIP(ip) {
			t.Errorf("isPrivateIP(%q) = false, want true", s)
		}
	}

	public := []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:4700:4700::1111"}
	for _, s := range public {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("test bug: cannot parse %q", s)
		}
		if isPrivateIP(ip) {
			t.Errorf("isPrivateIP(%q) = true, want false", s)
		}
	}
}

func TestValidateUpstreamAddress(t *testing.T) {
	// Save and restore the global service container.
	orig := DNSServices
	defer func() { DNSServices = orig }()

	// Private upstreams allowed (default): everything passes.
	DNSServices = &DNSServiceContainer{Config: &config.Config{
		DNS: config.DNSConfig{AllowPrivateUpstream: true},
	}}
	if err := validateUpstreamAddress("127.0.0.1:53"); err != nil {
		t.Errorf("allow_private_upstream=true: 127.0.0.1:53 rejected: %v", err)
	}
	if err := validateUpstreamAddress("10.0.0.53:53"); err != nil {
		t.Errorf("allow_private_upstream=true: 10.0.0.53:53 rejected: %v", err)
	}

	// Private upstreams disallowed: loopback/private rejected, public passes.
	DNSServices = &DNSServiceContainer{Config: &config.Config{
		DNS: config.DNSConfig{AllowPrivateUpstream: false},
	}}
	for _, addr := range []string{"127.0.0.1:53", "10.0.0.53:53", "192.168.1.1:53", "localhost:53"} {
		if err := validateUpstreamAddress(addr); err == nil {
			t.Errorf("allow_private_upstream=false: %q accepted, want rejection", addr)
		}
	}
	if err := validateUpstreamAddress("8.8.8.8:53"); err != nil {
		t.Errorf("allow_private_upstream=false: public upstream 8.8.8.8:53 rejected: %v", err)
	}
}
