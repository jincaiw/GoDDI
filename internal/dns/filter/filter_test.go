package filter

import (
	"net"
	"regexp"
	"testing"

	"github.com/miekg/dns"
)

func TestNewFilterEngine(t *testing.T) {
	t.Parallel()

	fe := NewFilterEngine(true)
	if fe == nil {
		t.Fatal("NewFilterEngine() returned nil")
	}
	if !fe.rebindingProtection {
		t.Error("rebindingProtection should be true")
	}
}

func TestFilterEngine_Check_BlockList(t *testing.T) {
	fe := NewFilterEngine(false)

	// Add a block list with a rule
	fe.BlockListMgr.AddList(&BlockList{ID: "bl-1", Name: "Test", Enabled: true})
	fe.BlockListMgr.AddRule("bl-1", MatchRule{
		ID: "r1", Pattern: "ads.example.com", MatchType: "exact",
		ResponseType: "NXDOMAIN", Enabled: true,
	})

	result := fe.Check("ads.example.com.", "192.168.1.1")
	if !result.Blocked {
		t.Error("domain in block list should be blocked")
	}
	if result.Reason != "block_list" {
		t.Errorf("reason = %q, want %q", result.Reason, "block_list")
	}
	if result.ResponseType != "NXDOMAIN" {
		t.Errorf("response type = %q, want %q", result.ResponseType, "NXDOMAIN")
	}
}

func TestFilterEngine_Check_AllowList(t *testing.T) {
	fe := NewFilterEngine(false)

	// Add block list
	fe.BlockListMgr.AddList(&BlockList{ID: "bl-1", Name: "Test", Enabled: true})
	fe.BlockListMgr.AddRule("bl-1", MatchRule{
		ID: "r1", Pattern: "safe.example.com", MatchType: "exact",
		ResponseType: "NXDOMAIN", Enabled: true,
	})

	// Add allow list that overrides the block
	fe.AllowListMgr.AddRule(MatchRule{
		ID: "a1", Pattern: "safe.example.com", MatchType: "exact", Enabled: true,
	})

	result := fe.Check("safe.example.com.", "192.168.1.1")
	if result.Blocked {
		t.Error("allow list should override block list")
	}
	if result.Reason != "allow_list" {
		t.Errorf("reason = %q, want %q", result.Reason, "allow_list")
	}
}

func TestFilterEngine_Check_NoMatch(t *testing.T) {
	fe := NewFilterEngine(false)

	result := fe.Check("clean.example.com.", "192.168.1.1")
	if result.Blocked {
		t.Error("domain not in any list should not be blocked")
	}
}

func TestFilterEngine_Check_ClientPolicyAllow(t *testing.T) {
	fe := NewFilterEngine(false)

	policy := &ClientPolicy{
		ID:         "p1",
		Name:       "Allow Internal",
		SourceCIDR: "192.168.0.0/16",
		Action:     "allow",
		Priority:   10,
		Enabled:    true,
	}
	fe.PolicyMgr.AddPolicy(policy)

	result := fe.Check("ads.example.com.", "192.168.1.1")
	if result.Blocked {
		t.Error("client policy allow should override everything")
	}
	if result.Reason != "client_policy_allow" {
		t.Errorf("reason = %q, want %q", result.Reason, "client_policy_allow")
	}
}

func TestFilterEngine_Check_ClientPolicyBlock(t *testing.T) {
	fe := NewFilterEngine(false)

	policy := &ClientPolicy{
		ID:         "p1",
		Name:       "Block External",
		SourceCIDR: "10.0.0.0/8",
		Action:     "block",
		Priority:   10,
		Enabled:    true,
	}
	fe.PolicyMgr.AddPolicy(policy)

	result := fe.Check("any.example.com.", "10.0.0.1")
	if !result.Blocked {
		t.Error("client policy block should block all queries from matching IP")
	}
	if result.Reason != "client_policy_block" {
		t.Errorf("reason = %q, want %q", result.Reason, "client_policy_block")
	}
}

func TestFilterEngine_CheckRebindingProtection_PublicClient(t *testing.T) {
	fe := NewFilterEngine(true)

	// Create a DNS response with a private IP
	msg := new(dns.Msg)
	msg.SetQuestion("evil.example.com.", dns.TypeA)
	msg.Answer = append(msg.Answer, &dns.A{
		Hdr: dns.RR_Header{Name: "evil.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   net.ParseIP("192.168.1.1"),
	})

	// Public client IP
	blocked := fe.CheckRebindingProtection(msg, "8.8.8.8")
	if !blocked {
		t.Error("should block private IP in response for public client (DNS rebinding)")
	}
}

func TestFilterEngine_CheckRebindingProtection_PrivateClient(t *testing.T) {
	fe := NewFilterEngine(true)

	// Create a DNS response with a private IP
	msg := new(dns.Msg)
	msg.SetQuestion("internal.example.com.", dns.TypeA)
	msg.Answer = append(msg.Answer, &dns.A{
		Hdr: dns.RR_Header{Name: "internal.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   net.ParseIP("192.168.1.1"),
	})

	// Private client IP - should NOT be blocked
	blocked := fe.CheckRebindingProtection(msg, "192.168.1.100")
	if blocked {
		t.Error("should NOT block private IP in response for private client")
	}
}

func TestFilterEngine_CheckRebindingProtection_Disabled(t *testing.T) {
	fe := NewFilterEngine(false)

	msg := new(dns.Msg)
	msg.SetQuestion("evil.example.com.", dns.TypeA)
	msg.Answer = append(msg.Answer, &dns.A{
		Hdr: dns.RR_Header{Name: "evil.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   net.ParseIP("192.168.1.1"),
	})

	blocked := fe.CheckRebindingProtection(msg, "8.8.8.8")
	if blocked {
		t.Error("should NOT block when rebinding protection is disabled")
	}
}

func TestFilterEngine_CheckRebindingProtection_PublicIP(t *testing.T) {
	fe := NewFilterEngine(true)

	// Create a DNS response with a public IP
	msg := new(dns.Msg)
	msg.SetQuestion("safe.example.com.", dns.TypeA)
	msg.Answer = append(msg.Answer, &dns.A{
		Hdr: dns.RR_Header{Name: "safe.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   net.ParseIP("1.2.3.4"),
	})

	blocked := fe.CheckRebindingProtection(msg, "8.8.8.8")
	if blocked {
		t.Error("should NOT block public IP in response")
	}
}

func TestFilterEngine_CheckRebindingProtection_IPv6(t *testing.T) {
	fe := NewFilterEngine(true)

	// Create a DNS response with a private IPv6 address
	msg := new(dns.Msg)
	msg.SetQuestion("evil.example.com.", dns.TypeAAAA)
	msg.Answer = append(msg.Answer, &dns.AAAA{
		Hdr:  dns.RR_Header{Name: "evil.example.com.", Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 300},
		AAAA: net.ParseIP("fd00::1"),
	})

	blocked := fe.CheckRebindingProtection(msg, "2001:db8::1")
	if !blocked {
		t.Error("should block private IPv6 in response for public IPv6 client")
	}
}

func TestFilterEngine_CheckCNAMECloaking(t *testing.T) {
	fe := NewFilterEngine(false)

	// Add a block list rule for the tracking domain
	fe.BlockListMgr.AddList(&BlockList{ID: "bl-1", Name: "Trackers", Enabled: true})
	fe.BlockListMgr.AddRule("bl-1", MatchRule{
		ID: "r1", Pattern: "tracker.evil.com", MatchType: "exact",
		ResponseType: "NXDOMAIN", Enabled: true,
	})

	// Create a DNS response with a CNAME pointing to a blocked domain
	msg := new(dns.Msg)
	msg.SetQuestion("www.example.com.", dns.TypeA)
	msg.Answer = append(msg.Answer, &dns.CNAME{
		Hdr:    dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 300},
		Target: "tracker.evil.com.",
	})

	detected := fe.CheckCNAMECloaking(msg)
	if !detected {
		t.Error("should detect CNAME cloaking when CNAME target is in block list")
	}
}

func TestFilterEngine_CheckCNAMECloaking_NoCloaking(t *testing.T) {
	fe := NewFilterEngine(false)

	fe.BlockListMgr.AddList(&BlockList{ID: "bl-1", Name: "Trackers", Enabled: true})
	fe.BlockListMgr.AddRule("bl-1", MatchRule{
		ID: "r1", Pattern: "tracker.evil.com", MatchType: "exact",
		ResponseType: "NXDOMAIN", Enabled: true,
	})

	// Create a DNS response with a CNAME pointing to a clean domain
	msg := new(dns.Msg)
	msg.SetQuestion("www.example.com.", dns.TypeA)
	msg.Answer = append(msg.Answer, &dns.CNAME{
		Hdr:    dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 300},
		Target: "cdn.clean.com.",
	})

	detected := fe.CheckCNAMECloaking(msg)
	if detected {
		t.Error("should NOT detect CNAME cloaking when CNAME target is not in block list")
	}
}

func TestGenerateBlockedResponse_NXDOMAIN(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("blocked.example.com.", dns.TypeA)

	resp := GenerateBlockedResponse(req, "NXDOMAIN", "")
	if resp == nil {
		t.Fatal("response should not be nil")
	}
	if resp.Rcode != dns.RcodeNameError {
		t.Errorf("rcode = %d, want %d", resp.Rcode, dns.RcodeNameError)
	}
}

func TestGenerateBlockedResponse_REFUSED(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("blocked.example.com.", dns.TypeA)

	resp := GenerateBlockedResponse(req, "REFUSED", "")
	if resp == nil {
		t.Fatal("response should not be nil")
	}
	if resp.Rcode != dns.RcodeRefused {
		t.Errorf("rcode = %d, want %d", resp.Rcode, dns.RcodeRefused)
	}
}

func TestGenerateBlockedResponse_NODATA(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("blocked.example.com.", dns.TypeA)

	resp := GenerateBlockedResponse(req, "NODATA", "")
	if resp == nil {
		t.Fatal("response should not be nil")
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Errorf("rcode = %d, want %d (NOERROR)", resp.Rcode, dns.RcodeSuccess)
	}
	if len(resp.Answer) != 0 {
		t.Errorf("NODATA should have 0 answers, got %d", len(resp.Answer))
	}
}

func TestGenerateBlockedResponse_CUSTOM_IP(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("blocked.example.com.", dns.TypeA)

	resp := GenerateBlockedResponse(req, "CUSTOM_IP", "0.0.0.0")
	if resp == nil {
		t.Fatal("response should not be nil")
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Errorf("rcode = %d, want %d", resp.Rcode, dns.RcodeSuccess)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("should have 1 answer, got %d", len(resp.Answer))
	}
	aRecord, ok := resp.Answer[0].(*dns.A)
	if !ok {
		t.Fatal("answer should be A record")
	}
	if !aRecord.A.Equal(net.ParseIP("0.0.0.0")) {
		t.Errorf("A record = %s, want 0.0.0.0", aRecord.A.String())
	}
}

func TestGenerateBlockedResponse_DROP(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("blocked.example.com.", dns.TypeA)

	resp := GenerateBlockedResponse(req, "DROP", "")
	if resp != nil {
		t.Error("DROP response should be nil")
	}
}

func TestMatchRulePattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pattern   string
		qname     string
		matchType string
		want      bool
	}{
		{"exact match", "example.com", "example.com", "exact", true},
		{"exact no match", "example.com", "sub.example.com", "exact", false},
		{"suffix match", "example.com", "sub.example.com", "suffix", true},
		{"suffix no match", "example.com", "example.org", "suffix", false},
		{"suffix same domain", "example.com", "example.com", "suffix", true},
		{"wildcard match", "*.example.com", "sub.example.com", "wildcard", true},
		{"wildcard no match base", "*.example.com", "example.com", "wildcard", false},
		{"regex match", `^ads`, "ads.example.com", "regex", true},
		{"regex no match", `^ads`, "www.example.com", "regex", false},
		{"regex suffix match", `example\.com$`, "ads.example.com", "regex", true},
		{"unknown type defaults exact", "example.com", "example.com", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var compiledRegex *regexp.Regexp
			if tt.matchType == "regex" {
				compiledRegex = regexp.MustCompile(tt.pattern)
			}
			result := matchRulePattern(tt.pattern, tt.qname, tt.matchType, compiledRegex)
			if result != tt.want {
				t.Errorf("matchRulePattern(%q, %q, %q) = %v, want %v",
					tt.pattern, tt.qname, tt.matchType, result, tt.want)
			}
		})
	}
}

func TestIsSuffixMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		qname   string
		want    bool
	}{
		{"exact match", "example.com", "example.com", true},
		{"subdomain match", "example.com", "sub.example.com", true},
		{"deep subdomain", "example.com", "a.b.c.example.com", true},
		{"no match", "example.com", "example.org", false},
		{"partial no match", "xample.com", "example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isSuffixMatch(tt.pattern, tt.qname)
			if result != tt.want {
				t.Errorf("isSuffixMatch(%q, %q) = %v, want %v", tt.pattern, tt.qname, result, tt.want)
			}
		})
	}
}

func TestIsWildcardMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		qname   string
		want    bool
	}{
		{"wildcard match", "*.example.com", "sub.example.com", true},
		{"wildcard no match base", "*.example.com", "example.com", false},
		{"wildcard deep match", "*.example.com", "a.b.example.com", true},
		{"no wildcard", "example.com", "example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isWildcardMatch(tt.pattern, tt.qname)
			if result != tt.want {
				t.Errorf("isWildcardMatch(%q, %q) = %v, want %v", tt.pattern, tt.qname, result, tt.want)
			}
		})
	}
}

func TestStripDot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"example.com.", "example.com"},
		{"example.com", "example.com"},
		{".", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			result := stripDot(tt.input)
			if result != tt.expected {
				t.Errorf("stripDot(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ip       string
		expected bool
	}{
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"127.0.0.1", true},
		{"169.254.1.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"172.15.0.1", false},
		{"172.32.0.1", false},
		{"fd00::1", true},
		{"fc00::1", true},
		{"::1", true},
		{"fe80::1", true},
		{"2001:db8::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP %q", tt.ip)
			}
			result := IsPrivateIP(ip)
			if result != tt.expected {
				t.Errorf("IsPrivateIP(%q) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}
