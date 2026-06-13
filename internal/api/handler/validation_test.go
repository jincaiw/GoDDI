package handler

import (
	"net"
	"testing"
)

// --- Email Validation Tests ---

func TestEmailRegex_Valid(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"user@example.com", true},
		{"user.name@example.com", true},
		{"user+tag@example.com", true},
		{"user@sub.example.com", true},
		{"user123@example.co", true},
		{"a@b.cc", true},
		{"test_user@domain.org", true},
		{"user%name@example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			if got := emailRegex.MatchString(tt.email); got != tt.valid {
				t.Errorf("emailRegex.MatchString(%q) = %v, want %v", tt.email, got, tt.valid)
			}
		})
	}
}

func TestEmailRegex_Invalid(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"", false},
		{"user", false},
		{"user@", false},
		{"@example.com", false},
		{"user@.com", false},
		{"user@example", false},
		{"user@example.", false},
		{"user example@example.com", false},
		{"user@exa mple.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			if got := emailRegex.MatchString(tt.email); got != tt.valid {
				t.Errorf("emailRegex.MatchString(%q) = %v, want %v", tt.email, got, tt.valid)
			}
		})
	}
}

// --- Forwarder Address Validation Tests ---

func TestForwarderAddressValidation(t *testing.T) {
	tests := []struct {
		address string
		valid   bool
	}{
		{"8.8.8.8:53", true},
		{"1.1.1.1:853", true},
		{"dns.example.com:53", true},
		{"[::1]:53", true},
		{"8.8.8.8", false}, // missing port
		{"", false},        // empty
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			_, _, err := net.SplitHostPort(tt.address)
			got := err == nil
			if got != tt.valid {
				t.Errorf("net.SplitHostPort(%q) valid = %v, want %v (err: %v)", tt.address, got, tt.valid, err)
			}
		})
	}
}

// --- Forwarder Protocol Validation Tests ---

func TestForwarderProtocolValidation(t *testing.T) {
	validProtoList := []string{"udp", "tcp", "dot", "doh", "doq"}
	for _, proto := range validProtoList {
		t.Run(proto, func(t *testing.T) {
			if !validProtocols[proto] {
				t.Errorf("protocol %q should be valid", proto)
			}
		})
	}

	invalidProtoList := []string{"http", "https", "icmp", "", "DNS"}
	for _, proto := range invalidProtoList {
		t.Run(proto, func(t *testing.T) {
			if validProtocols[proto] {
				t.Errorf("protocol %q should be invalid", proto)
			}
		})
	}
}

// --- Block Rule match_type Validation Tests ---

func TestBlockRuleMatchTypeValidation(t *testing.T) {
	validTypes := []string{"exact", "suffix", "wildcard", "regex"}
	for _, mt := range validTypes {
		t.Run(mt, func(t *testing.T) {
			if !validMatchTypes[mt] {
				t.Errorf("match_type %q should be valid", mt)
			}
		})
	}

	invalidTypes := []string{"prefix", "contains", "", "fuzzy", "glob"}
	for _, mt := range invalidTypes {
		t.Run(mt, func(t *testing.T) {
			if validMatchTypes[mt] {
				t.Errorf("match_type %q should be invalid", mt)
			}
		})
	}
}

// --- Block Rule response_type Validation Tests ---

func TestBlockRuleResponseTypeValidation(t *testing.T) {
	validTypes := []string{"NXDOMAIN", "NODATA", "REFUSED", "CUSTOM_IP", "DROP"}
	for _, rt := range validTypes {
		t.Run(rt, func(t *testing.T) {
			if !validResponseTypes[rt] {
				t.Errorf("response_type %q should be valid", rt)
			}
		})
	}

	invalidTypes := []string{"nxdomain", "refuse", "", "BLOCK", "SINKHOLE"}
	for _, rt := range invalidTypes {
		t.Run(rt, func(t *testing.T) {
			if validResponseTypes[rt] {
				t.Errorf("response_type %q should be invalid", rt)
			}
		})
	}
}

// --- Client Policy action Validation Tests ---

func TestClientPolicyActionValidation(t *testing.T) {
	validActionList := []string{"allow", "block", "apply_lists"}
	for _, action := range validActionList {
		t.Run(action, func(t *testing.T) {
			if !validActions[action] {
				t.Errorf("action %q should be valid", action)
			}
		})
	}

	invalidActionList := []string{"deny", "redirect", "", "ALLOW", "BLOCK"}
	for _, action := range invalidActionList {
		t.Run(action, func(t *testing.T) {
			if validActions[action] {
				t.Errorf("action %q should be invalid", action)
			}
		})
	}
}

// --- Client Policy CIDR Validation Tests ---

func TestClientPolicyCIDRValidation(t *testing.T) {
	tests := []struct {
		cidr  string
		valid bool
	}{
		{"192.168.1.0/24", true},
		{"10.0.0.0/8", true},
		{"172.16.0.0/12", true},
		{"0.0.0.0/0", true},
		{"fd00::/8", true},
		{"192.168.1.1/32", true},
		{"192.168.1.1", false},    // missing mask
		{"192.168.1.0/33", false}, // invalid mask
		{"999.0.0.0/24", false},   // invalid IP
		{"", false},               // empty
		{"not-a-cidr", false},     // not a CIDR
	}

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			_, _, err := net.ParseCIDR(tt.cidr)
			got := err == nil
			if got != tt.valid {
				t.Errorf("net.ParseCIDR(%q) valid = %v, want %v (err: %v)", tt.cidr, got, tt.valid, err)
			}
		})
	}
}

// --- Setting Key Validation Tests ---

func TestSettingKeyValidation(t *testing.T) {
	validKeys := []string{
		"server_name", "server_language", "server_dark_mode",
		"dns_default_ttl", "dns_recursion",
		"dhcp_lease_time",
		"ipam_ping_check", "ipam_auto_scan",
		"security_rebinding",
		"log_retention_days",
		"backup_auto_enabled", "backup_auto_schedule", "backup_retention",
	}
	for _, key := range validKeys {
		t.Run(key, func(t *testing.T) {
			if !validSettingKeys[key] {
				t.Errorf("setting key %q should be valid", key)
			}
		})
	}

	invalidKeys := []string{"unknown_key", "random_setting", "", "server_port", "dns_server"}
	for _, key := range invalidKeys {
		t.Run(key, func(t *testing.T) {
			if validSettingKeys[key] {
				t.Errorf("setting key %q should be invalid", key)
			}
		})
	}
}
