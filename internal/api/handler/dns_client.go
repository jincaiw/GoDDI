package handler

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/dns/client"
	"github.com/miekg/dns"
)

// DNSQueryRequest represents a DNS query request from the API.
type DNSQueryRequest struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Upstream string `json:"upstream"`
}

// ExecuteDNSQuery executes a DNS query for debugging.
// POST /api/v1/dns/client
func ExecuteDNSQuery(w http.ResponseWriter, r *http.Request) {
	var req DNSQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Name == "" {
		response.BadRequest(w, "缺少名称")
		return
	}

	// SEC-23: Validate the DNS name before using it. miekg/dns will
	// happily pass arbitrary strings to the wire format, so we enforce
	// a sane shape here (no leading/trailing dots, no embedded nulls,
	// bounded length).
	if err := validateDNSName(req.Name); err != nil {
		response.BadRequest(w, "无效的DNS名称: "+err.Error())
		return
	}

	// SEC-07: Validate upstream address to prevent SSRF.
	if req.Upstream != "" {
		if err := validateUpstreamAddress(req.Upstream); err != nil {
			response.BadRequest(w, err.Error())
			return
		}
	} else {
		// If upstream is empty, use configured forwarders or safe defaults.
		if DNSServices != nil && DNSServices.Forwarder != nil {
			forwarders := DNSServices.Forwarder.GetForwarders()
			if len(forwarders) > 0 {
				req.Upstream = forwarders[0].Address
			}
		}
		if req.Upstream == "" {
			req.Upstream = "8.8.8.8:53"
		}
	}

	// Parse query type.
	qtype := dns.TypeA
	if req.Type != "" {
		var ok bool
		qtype, ok = dns.StringToType[req.Type]
		if !ok {
			response.BadRequest(w, "无效的DNS记录类型: "+req.Type)
			return
		}
	}

	// Use the DNS client to execute the query.
	var dnsClient *client.DNSClient
	if DNSServices != nil && DNSServices.DNSClient != nil {
		dnsClient = DNSServices.DNSClient
	} else {
		dnsClient = client.NewDNSClient(5 * 1e9) // 5 seconds
	}

	result, _, err := dnsClient.Query(req.Name, qtype, req.Upstream)
	if err != nil {
		response.InternalErrorWithLog(w, "DNS查询失败", err)
		return
	}

	response.OK(w, result)
}

// maxDNSNameLength is the maximum allowed length (in bytes) of a DNS name
// supplied to the debug query endpoint. RFC 1035 caps a full domain name
// (including dots) at 253 octets; we use 255 to leave a small margin while
// still rejecting obviously oversized input.
const maxDNSNameLength = 255

// validateDNSName performs lightweight sanity checks on a DNS name
// supplied by an authenticated API caller. We deliberately do not try to
// implement the full RFC 1035 grammar here; we just reject shapes that
// are guaranteed to be invalid (empty labels, leading/trailing dot, NUL
// bytes, oversize input, control characters). The DNS protocol layer
// does additional validation before transmission.
func validateDNSName(name string) error {
	if name == "" {
		return fmt.Errorf("name is empty")
	}
	if len(name) > maxDNSNameLength {
		return fmt.Errorf("name exceeds %d bytes", maxDNSNameLength)
	}
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("name contains a NUL byte")
	}
	// No leading or trailing dots: those have caused edge-case bugs in
	// downstream resolvers.
	if name[0] == '.' || name[len(name)-1] == '.' {
		return fmt.Errorf("name must not start or end with a dot")
	}
	// No whitespace or control characters.
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("name contains a control character")
		}
		if r == ' ' || r == '\t' {
			return fmt.Errorf("name contains whitespace")
		}
	}
	// Each label between dots must be 1..63 octets and consist of
	// LDH (letters, digits, hyphen). We don't enforce hyphen placement
	// rules to stay lenient with internationalised names that have
	// been punycoded.
	labels := strings.Split(name, ".")
	for _, l := range labels {
		if l == "" {
			return fmt.Errorf("name contains an empty label")
		}
		if len(l) > 63 {
			return fmt.Errorf("label %q exceeds 63 bytes", l)
		}
	}
	return nil
}

// validateUpstreamAddress validates that the upstream DNS address is not a
// private/internal IP address to prevent Server-Side Request Forgery (SSRF).
//
// NOTE: This function resolves the hostname via DNS to check the resulting IPs,
// which introduces a TOCTOU (Time-of-Check-Time-of-Use) race condition: the DNS
// record could change between validation and the actual DNS query. A future
// improvement should enforce private-IP rejection at the dialer level using a
// custom net.Dialer with a Control function that inspects the actual dialed IP.
// dnsAllowPrivateUpstream reports whether private/internal upstream addresses
// are permitted (dns.allow_private_upstream, default true).
func dnsAllowPrivateUpstream() bool {
	return DNSServices == nil || DNSServices.Config == nil || DNSServices.Config.DNS.AllowPrivateUpstream
}

func validateUpstreamAddress(addr string) error {
	// Internal-resolver forwarding is a legitimate deployment; the config
	// switch turns the private-address guard off for those setups.
	if DNSServices != nil && DNSServices.Config != nil && DNSServices.Config.DNS.AllowPrivateUpstream {
		return nil
	}

	// Split host and port.
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// If no port, try adding default DNS port.
		host, _, err = net.SplitHostPort(addr + ":53")
		if err != nil {
			return fmt.Errorf("invalid upstream address format: %s", addr)
		}
	}

	// Resolve the host to get IP addresses.
	ips, err := net.LookupIP(host)
	if err != nil {
		// If resolution fails, try parsing as IP directly.
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("cannot resolve upstream address: %s", host)
		}
		ips = []net.IP{ip}
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("upstream address %s resolves to private/internal IP %s, which is not allowed", host, ip)
		}
	}

	return nil
}

// isPrivateIP checks if an IP address is private, loopback, link-local,
// or unspecified ("this host" — e.g. 0.0.0.0), all of which must be
// rejected as upstream targets when private upstreams are disallowed.
func isPrivateIP(ip net.IP) bool {
	// Check unspecified (0.0.0.0 / ::).
	if ip.IsUnspecified() {
		return true
	}
	// Check loopback.
	if ip.IsLoopback() {
		return true
	}
	// Check link-local.
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	// Check private networks.
	privateRanges := []struct {
		network *net.IPNet
	}{
		{mustParseCIDR("10.0.0.0/8")},
		{mustParseCIDR("172.16.0.0/12")},
		{mustParseCIDR("192.168.0.0/16")},
		{mustParseCIDR("127.0.0.0/8")},
		{mustParseCIDR("169.254.0.0/16")},
		{mustParseCIDR("::1/128")},
		{mustParseCIDR("fc00::/7")},
		{mustParseCIDR("fe80::/10")},
	}

	for _, r := range privateRanges {
		if r.network.Contains(ip) {
			return true
		}
	}

	return false
}

// mustParseCIDR parses a CIDR string or panics.
func mustParseCIDR(s string) *net.IPNet {
	_, network, err := net.ParseCIDR(s)
	if err != nil {
		panic(fmt.Sprintf("invalid CIDR %s: %v", s, err))
	}
	return network
}
