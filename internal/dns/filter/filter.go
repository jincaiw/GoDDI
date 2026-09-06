package filter

import (
	"log/slog"
	"net"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// FilterEngine is the main DNS security filter engine.
type FilterEngine struct {
	BlockListMgr *BlockListManager
	AllowListMgr *AllowListManager
	PolicyMgr    *ClientPolicyManager

	rebindingProtection atomic.Bool
	blockingEnabled     atomic.Bool

	// blockingDisabledUntil is the deadline until which block lists and
	// client policies are suspended (temporary disable, 0 = never).
	blockingDisabledUntil atomic.Int64
}

// NewFilterEngine creates a new filter engine.
func NewFilterEngine(rebindingProtection bool) *FilterEngine {
	fe := &FilterEngine{
		BlockListMgr: NewBlockListManager(),
		AllowListMgr: NewAllowListManager(),
		PolicyMgr:    NewClientPolicyManager(),
	}
	fe.rebindingProtection.Store(rebindingProtection)
	fe.blockingEnabled.Store(true)
	return fe
}

// SetRebindingProtection enables or disables DNS rebinding protection at
// runtime (hot setting).
func (fe *FilterEngine) SetRebindingProtection(enabled bool) {
	fe.rebindingProtection.Store(enabled)
}

// SetBlockingEnabled globally enables or disables the block list / client
// policy pipeline (master switch, hot setting).
func (fe *FilterEngine) SetBlockingEnabled(enabled bool) {
	fe.blockingEnabled.Store(enabled)
	slog.Info("filter: blocking pipeline", "enabled", enabled)
}

// TemporaryDisableBlocking suspends blocking for the given duration
// (1 minute to 24 hours). A zero duration re-enables blocking immediately.
func (fe *FilterEngine) TemporaryDisableBlocking(d time.Duration) {
	if d <= 0 {
		fe.blockingDisabledUntil.Store(0)
		slog.Info("filter: temporary blocking suspension cleared")
		return
	}
	fe.blockingDisabledUntil.Store(time.Now().Add(d).Unix())
	slog.Info("filter: blocking temporarily disabled", "minutes", int(d.Minutes()))
}

// BlockingStatus reports whether blocking is currently active and, when
// temporarily disabled, until when.
func (fe *FilterEngine) BlockingStatus() (enabled bool, disabledUntil time.Time) {
	if !fe.blockingEnabled.Load() {
		return false, time.Time{}
	}
	untilUnix := fe.blockingDisabledUntil.Load()
	if untilUnix == 0 {
		return true, time.Time{}
	}
	until := time.Unix(untilUnix, 0)
	if time.Now().After(until) {
		return true, time.Time{}
	}
	return false, until
}

// CheckResult contains the result of a filter check.
type CheckResult struct {
	Blocked      bool   `json:"blocked"`
	ResponseType string `json:"response_type,omitempty"` // NXDOMAIN, NODATA, REFUSED, CUSTOM_IP, DROP
	ResponseData string `json:"response_data,omitempty"` // Custom IP for CUSTOM_IP type
	Reason       string `json:"reason,omitempty"`        // Why it was blocked/allowed
}

// Check performs the full filter pipeline for a DNS query.
// Order: client policy -> allow list -> block list.
func (fe *FilterEngine) Check(qname string, clientIP string) CheckResult {
	// Step 0: honour the global blocking switch. When blocking is disabled
	// (or temporarily paused from the console) no list is consulted at all.
	if enabled, _ := fe.BlockingStatus(); !enabled {
		return CheckResult{Blocked: false, Reason: "blocking_disabled"}
	}

	// Step 1: Match client policy.
	policy := fe.PolicyMgr.MatchClientPolicy(clientIP)
	if policy != nil {
		slog.Debug("filter: matched client policy",
			"client_ip", clientIP,
			"policy", policy.Name,
			"action", policy.Action,
		)

		switch policy.Action {
		case "allow":
			return CheckResult{Blocked: false, Reason: "client_policy_allow"}
		case "block":
			return CheckResult{Blocked: true, ResponseType: "REFUSED", Reason: "client_policy_block"}
		case "apply_lists":
			// Continue to check allow/block lists with the policy's specific lists.
			return fe.checkWithPolicyLists(qname, clientIP, policy)
		}
	}

	// Step 2: Check allow list (whitelist overrides block list).
	if fe.AllowListMgr.CheckAllowList(qname) {
		return CheckResult{Blocked: false, Reason: "allow_list"}
	}

	// Step 3: Check block list.
	blocked, responseType, responseData := fe.BlockListMgr.CheckBlockList(qname, clientIP)
	if blocked {
		return CheckResult{
			Blocked:      true,
			ResponseType: responseType,
			ResponseData: responseData,
			Reason:       "block_list",
		}
	}

	return CheckResult{Blocked: false}
}

// checkWithPolicyLists checks allow/block lists scoped to a specific policy.
func (fe *FilterEngine) checkWithPolicyLists(qname, clientIP string, policy *ClientPolicy) CheckResult {
	// Check policy-specific allow rules first.
	for _, ruleID := range policy.AllowRuleIDs {
		rules := fe.AllowListMgr.GetRules()
		for _, r := range rules {
			if r.ID == ruleID && r.Enabled {
				// Simple check: if the allow rule matches, allow.
				if matchRulePattern(r.Pattern, qname, r.MatchType, r.compiledRegex) {
					return CheckResult{Blocked: false, Reason: "policy_allow_rule"}
				}
			}
		}
	}

	// Check policy-specific block lists.
	for _, listID := range policy.BlockListIDs {
		rules := fe.BlockListMgr.GetRules(listID)
		for _, r := range rules {
			if r.Enabled && matchRulePattern(r.Pattern, qname, r.MatchType, r.compiledRegex) {
				return CheckResult{
					Blocked:      true,
					ResponseType: r.ResponseType,
					ResponseData: r.ResponseData,
					Reason:       "policy_block_list",
				}
			}
		}
	}

	return CheckResult{Blocked: false}
}

// matchRulePattern checks if a domain matches a rule pattern.
func matchRulePattern(pattern, qname, matchType string, compiledRegex *regexp.Regexp) bool {
	pattern = stripDot(pattern)
	qname = stripDot(qname)

	switch matchType {
	case "exact":
		return pattern == qname
	case "suffix":
		return isSuffixMatch(pattern, qname)
	case "wildcard":
		return isWildcardMatch(pattern, qname)
	case "regex":
		return isRegexMatch(pattern, qname, compiledRegex)
	default:
		return pattern == qname
	}
}

// CheckRebindingProtection checks if DNS response contains private IPs
// when the query came from a public client (DNS rebinding attack prevention).
func (fe *FilterEngine) CheckRebindingProtection(msg *dns.Msg, clientIP string) bool {
	if !fe.rebindingProtection.Load() {
		return false // Not blocked.
	}

	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}

	// Only apply rebinding protection for public clients.
	if IsPrivateIP(ip) {
		return false
	}

	// Check all answer records for private IPs.
	for _, rr := range msg.Answer {
		switch rr := rr.(type) {
		case *dns.A:
			if IsPrivateIP(rr.A) {
				slog.Warn("filter: DNS rebinding protection triggered",
					"client_ip", clientIP,
					"answer_ip", rr.A.String(),
				)
				return true // Blocked.
			}
		case *dns.AAAA:
			if IsPrivateIP(rr.AAAA) {
				slog.Warn("filter: DNS rebinding protection triggered",
					"client_ip", clientIP,
					"answer_ip", rr.AAAA.String(),
				)
				return true // Blocked.
			}
		}
	}

	return false
}

// CheckCNAMECloaking detects CNAME chains to tracking or other block-list
// domains. A naive implementation that only inspects the immediate CNAME
// target can be bypassed by an attacker that publishes
//
//	tracker.example ->  cdn.tracker.example
//
// because the resolver would then be asked to chase the second hop and
// discover the tracker only on the second query. To close this gap, we
// follow every CNAME in the response (and any subsequent chain that the
// message happens to contain) up to a small bound, with explicit loop
// detection so a cyclic CNAME configuration cannot hang the filter.
//
// Returns true if any link in the chain matches a block-list rule.
func (fe *FilterEngine) CheckCNAMECloaking(msg *dns.Msg) bool {
	// Build a set of names we have already examined so a CNAME cycle
	// (a -> b -> a) terminates quickly.
	seen := make(map[string]struct{})
	const maxHops = 16

	for hop := 0; hop < maxHops; hop++ {
		// Pick the first CNAME whose target we have not already
		// inspected. Inspecting in the order they appear in the
		// response is fine for blocking purposes: the filter cares
		// whether *any* link is on the block list, not which one.
		var nextTarget string
		found := false
		for _, rr := range msg.Answer {
			cname, ok := rr.(*dns.CNAME)
			if !ok {
				continue
			}
			target := stripDot(cname.Target)
			if _, dup := seen[target]; dup {
				continue
			}
			nextTarget = target
			found = true
			break
		}
		if !found {
			return false
		}
		seen[nextTarget] = struct{}{}

		// Resolve the chain hop against the block list. A name that
		// matches a block rule is treated as cloaking: the response
		// was steered towards a destination the policy says we
		// should not have looked up.
		blocked, _, _ := fe.BlockListMgr.CheckBlockList(nextTarget, "")
		if blocked {
			slog.Debug("filter: CNAME cloaking detected",
				"cname_target", nextTarget,
				"hop", hop,
			)
			return true
		}

		// See if the same response also contains a CNAME for this
		// target (multi-hop answer that was already chased by the
		// upstream). If not, we have to ask the resolver for the
		// next hop ourselves; that is intentionally out of scope
		// for the filter (which is stateless), so we stop here.
		hasNext := false
		for _, rr := range msg.Answer {
			cname, ok := rr.(*dns.CNAME)
			if !ok {
				continue
			}
			if stripDot(cname.Hdr.Name) == nextTarget {
				hasNext = true
				break
			}
		}
		if !hasNext {
			return false
		}
	}
	return false
}

// GenerateBlockedResponse creates a DNS response for blocked queries.
func GenerateBlockedResponse(req *dns.Msg, responseType, responseData string) *dns.Msg {
	resp := new(dns.Msg)
	resp.SetReply(req)

	switch responseType {
	case "NXDOMAIN":
		resp.SetRcode(req, dns.RcodeNameError)
	case "REFUSED":
		resp.SetRcode(req, dns.RcodeRefused)
	case "NODATA":
		// Return NOERROR with no answers.
		resp.SetRcode(req, dns.RcodeSuccess)
	case "CUSTOM_IP":
		resp.SetRcode(req, dns.RcodeSuccess)
		if len(req.Question) > 0 {
			q := req.Question[0]
			if responseData != "" {
				ip := net.ParseIP(responseData)
				if ip != nil {
					if ip.To4() != nil && q.Qtype == dns.TypeA {
						rr := &dns.A{
							Hdr: dns.RR_Header{
								Name:   q.Name,
								Rrtype: dns.TypeA,
								Class:  dns.ClassINET,
								Ttl:    300,
							},
							A: ip,
						}
						resp.Answer = append(resp.Answer, rr)
					} else if ip.To4() == nil && q.Qtype == dns.TypeAAAA {
						rr := &dns.AAAA{
							Hdr: dns.RR_Header{
								Name:   q.Name,
								Rrtype: dns.TypeAAAA,
								Class:  dns.ClassINET,
								Ttl:    300,
							},
							AAAA: ip,
						}
						resp.Answer = append(resp.Answer, rr)
					}
				}
			} else {
				// Default to 0.0.0.0 for A queries, :: for AAAA.
				if q.Qtype == dns.TypeA {
					rr := &dns.A{
						Hdr: dns.RR_Header{
							Name:   q.Name,
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    300,
						},
						A: net.ParseIP("0.0.0.0"),
					}
					resp.Answer = append(resp.Answer, rr)
				} else if q.Qtype == dns.TypeAAAA {
					rr := &dns.AAAA{
						Hdr: dns.RR_Header{
							Name:   q.Name,
							Rrtype: dns.TypeAAAA,
							Class:  dns.ClassINET,
							Ttl:    300,
						},
						AAAA: net.ParseIP("::"),
					}
					resp.Answer = append(resp.Answer, rr)
				}
			}
		}
	case "DROP":
		// Return nil to signal that the query should be dropped.
		return nil
	default:
		resp.SetRcode(req, dns.RcodeNameError)
	}

	return resp
}

// Helper functions.

func stripDot(s string) string {
	if len(s) > 0 && s[len(s)-1] == '.' {
		return s[:len(s)-1]
	}
	return s
}

func isSuffixMatch(pattern, qname string) bool {
	if pattern == qname {
		return true
	}
	return len(qname) > len(pattern) && qname[len(qname)-len(pattern)-1] == '.' && qname[len(qname)-len(pattern):] == pattern
}

func isWildcardMatch(pattern, qname string) bool {
	// *.example.com matches sub.example.com but not example.com.
	if len(pattern) > 1 && pattern[0] == '*' && pattern[1] == '.' {
		suffix := pattern[2:]
		return isSuffixMatch(suffix, qname) && len(qname) > len(suffix)+1
	}
	return pattern == qname
}

// isRegexMatch matches qname against a pre-compiled regex, falling back to
// on-the-fly compilation if no pre-compiled regex is provided.
// Returns false if the pattern is not a valid regular expression.
func isRegexMatch(pattern, qname string, compiledRegex *regexp.Regexp) bool {
	if compiledRegex != nil {
		return compiledRegex.MatchString(qname)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		slog.Warn("filter: invalid regex pattern", "pattern", pattern, "error", err)
		return false
	}
	return re.MatchString(qname)
}
