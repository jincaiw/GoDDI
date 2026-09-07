package dnsutil

import (
	"strings"
	"testing"
)

func TestFQDN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", "."},
		{"already FQDN", "example.com.", "example.com."},
		{"without trailing dot", "example.com", "example.com."},
		{"with spaces", "  example.com  ", "example.com."},
		{"single label", "localhost", "localhost."},
		{"single label with dot", "localhost.", "localhost."},
		{"root domain", ".", "."},
		{"whitespace only", "   ", "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := FQDN(tt.input)
			if result != tt.expected {
				t.Errorf("FQDN(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestUnFQDN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"FQDN input", "example.com.", "example.com"},
		{"non-FQDN input", "example.com", "example.com"},
		{"root domain", ".", ""},
		{"empty string", "", ""},
		{"single label FQDN", "localhost.", "localhost"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := UnFQDN(tt.input)
			if result != tt.expected {
				t.Errorf("UnFQDN(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestJoinDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		parts    []string
		expected string
	}{
		{"three parts", []string{"www", "example", "com"}, "www.example.com."},
		{"two parts", []string{"example", "com"}, "example.com."},
		{"single part", []string{"localhost"}, "localhost."},
		{"empty parts", []string{}, "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := JoinDomain(tt.parts...)
			if result != tt.expected {
				t.Errorf("JoinDomain(%v) = %q, want %q", tt.parts, result, tt.expected)
			}
		})
	}
}

func TestReverseIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"standard IPv4", "192.168.1.1", "1.1.168.192.in-addr.arpa."},
		{"loopback", "127.0.0.1", "1.0.0.127.in-addr.arpa."},
		{"class A", "10.0.0.1", "1.0.0.10.in-addr.arpa."},
		{"all zeros", "0.0.0.0", "0.0.0.0.in-addr.arpa."},
		{"broadcast", "255.255.255.255", "255.255.255.255.in-addr.arpa."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ReverseIP(tt.input)
			if result != tt.expected {
				t.Errorf("ReverseIP(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestReverseIPv6(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"full IPv6",
			"2001:0db8:0000:0000:0000:0000:0000:0001",
			"1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa.",
		},
		{
			"loopback",
			"0000:0000:0000:0000:0000:0000:0000:0001",
			"1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.ip6.arpa.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ReverseIPv6(tt.input)
			if result != tt.expected {
				t.Errorf("ReverseIPv6(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateRecordType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"A record", "A", true},
		{"AAAA record", "AAAA", true},
		{"CNAME record", "CNAME", true},
		{"MX record", "MX", true},
		{"NS record", "NS", true},
		{"PTR record", "PTR", true},
		{"SOA record", "SOA", true},
		{"SRV record", "SRV", true},
		{"TXT record", "TXT", true},
		{"CAA record", "CAA", true},
		{"TLSA record", "TLSA", true},
		{"DS record", "DS", true},
		{"DNSKEY record", "DNSKEY", true},
		{"lowercase a", "a", true},
		{"lowercase aaaa", "aaaa", true},
		{"mixed case", "Cname", true},
		{"invalid type", "INVALID", false},
		{"empty string", "", false},
		{"AXFR type", "AXFR", false},
		{"ANY type", "ANY", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ValidateRecordType(tt.input)
			if result != tt.expected {
				t.Errorf("ValidateRecordType(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatRecordValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		recordType     string
		value          string
		priority       int
		weight         int
		port           int
		expectedPrefix string
	}{
		{"A record", "A", "192.168.1.1", 0, 0, 0, "192.168.1.1"},
		{"AAAA record", "AAAA", "::1", 0, 0, 0, "::1"},
		{"MX record", "MX", "mail.example.com", 10, 0, 0, "10 mail.example.com."},
		{"SRV record", "SRV", "server.example.com", 10, 20, 5060, "10 20 5060 server.example.com."},
		{"CNAME record", "CNAME", "alias.example.com", 0, 0, 0, "alias.example.com."},
		{"NS record", "NS", "ns1.example.com", 0, 0, 0, "ns1.example.com."},
		{"PTR record", "PTR", "host.example.com", 0, 0, 0, "host.example.com."},
		{"TXT record", "TXT", "v=spf1 include:example.com ~all", 0, 0, 0, "v=spf1 include:example.com ~all"},
		{"lowercase type", "mx", "mail.example.com", 10, 0, 0, "10 mail.example.com."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := FormatRecordValue(tt.recordType, tt.value, tt.priority, tt.weight, tt.port)
			if result != tt.expectedPrefix {
				t.Errorf("FormatRecordValue() = %q, want %q", result, tt.expectedPrefix)
			}
		})
	}
}

func TestFQDNAndUnFQDNRoundTrip(t *testing.T) {
	t.Parallel()

	domains := []string{"example.com", "www.example.com", "sub.domain.example.com"}
	for _, d := range domains {
		fqdn := FQDN(d)
		unfqdn := UnFQDN(fqdn)
		if unfqdn != d {
			t.Errorf("UnFQDN(FQDN(%q)) = %q, want %q", d, unfqdn, d)
		}
	}
}

func TestValidateZoneName(t *testing.T) {
	t.Parallel()

	valid := []string{
		"example.com", "example.com.", "local", "sub.example.com",
		"1.168.192.in-addr.arpa", "my-zone.example", "_tcp.example.com",
	}
	for _, name := range valid {
		if err := ValidateZoneName(name); err != nil {
			t.Errorf("ValidateZoneName(%q) unexpected error: %v", name, err)
		}
	}

	invalid := map[string]string{
		"":                                 "空名称",
		".":                                "根作为区域",
		"..":                               "空标签",
		"a..b":                             "空标签",
		"-bad.test":                        "前导连字符",
		"bad-.test":                        "尾部连字符",
		"含中文.test":                         "非 ASCII",
		"a b.test":                         "含空格",
		strings.Repeat("a", 64) + ".test":  "超长标签",
		strings.Repeat("a", 250) + ".test": "超长名称",
	}
	for name, why := range invalid {
		if err := ValidateZoneName(name); err == nil {
			t.Errorf("ValidateZoneName(%q) should reject (%s)", name, why)
		}
	}
}

func TestValidateRecordName(t *testing.T) {
	t.Parallel()

	valid := []string{"@", "*", "*.example.com", "www", "www.example.com", "www.example.com.", "_sip._tcp.example.com"}
	for _, name := range valid {
		if err := ValidateRecordName(name); err != nil {
			t.Errorf("ValidateRecordName(%q) unexpected error: %v", name, err)
		}
	}

	invalid := map[string]string{
		"":                                 "空名称",
		"a..b":                             "空标签",
		"*.":                               "不完整的通配符",
		"-a.test":                          "前导连字符",
		"中文.test":                          "非 ASCII",
		strings.Repeat("a", 300) + ".test": "超长名称",
	}
	for name, why := range invalid {
		if err := ValidateRecordName(name); err == nil {
			t.Errorf("ValidateRecordName(%q) should reject (%s)", name, why)
		}
	}
}
