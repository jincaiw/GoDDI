package server

import (
	"net"
	"strings"

	"github.com/miekg/dns"
)

// RFC 6303 / RFC 6761 locally served zones. Queries that fall inside
// these zones must never leak to the internet: answering them
// authoritatively (empty NXDOMAIN) is both a privacy win and a
// standards-compliance fix. Technitium shipped the same behaviour in
// v15.3.
var locallyServedZones = []string{
	// RFC 6303 §4: IPv4/IPv6 special-purpose reverse zones.
	"0.in-addr.arpa.",
	"127.in-addr.arpa.",
	"254.169.in-addr.arpa.",
	"2.0.192.in-addr.arpa.",    // TEST-NET-1
	"100.51.198.in-addr.arpa.", // TEST-NET-2
	"113.0.203.in-addr.arpa.",  // TEST-NET-3
	"255.255.255.255.in-addr.arpa.",
	"0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.ip6.arpa.",
	"8.b.d.0.1.0.0.2.ip6.arpa.", // documentation IPv6
	"8.e.f.ip6.arpa.",           // link-local
	"9.e.f.ip6.arpa.",
	"a.e.f.ip6.arpa.",
	"b.e.f.ip6.arpa.",

	// RFC 6761 §6: special-use domain names.
	"test.",
	"invalid.",
	"example.com.",
	"example.net.",
	"example.org.",
	"onion.",
}

// isLocallyServed reports whether qname falls inside one of the locally
// served zones (subtree match).
func isLocallyServed(qname string) bool {
	qname = strings.ToLower(qname)
	if !strings.HasSuffix(qname, ".") {
		qname += "."
	}
	for _, zone := range locallyServedZones {
		if qname == zone || strings.HasSuffix(qname, "."+zone) {
			return true
		}
	}
	return false
}

// answerLocallyServed builds an authoritative empty answer for a query
// inside a locally served zone. Per RFC 6303 the response is NXDOMAIN
// with the SOA of the enclosing special zone in the authority section
// (we emit a minimal synthetic SOA). localhost is answered from
// loopback (RFC 6761 §6.3) for A/AAAA instead.
func answerLocallyServed(req *dns.Msg) *dns.Msg {
	qname := strings.ToLower(req.Question[0].Name)
	qtype := req.Question[0].Qtype

	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Authoritative = true

	if qname == "localhost." && (qtype == dns.TypeA || qtype == dns.TypeAAAA || qtype == dns.TypeANY) {
		if qtype == dns.TypeA || qtype == dns.TypeANY {
			resp.Answer = append(resp.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: qname, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 0},
				A:   []byte{127, 0, 0, 1},
			})
		}
		if qtype == dns.TypeAAAA || qtype == dns.TypeANY {
			resp.Answer = append(resp.Answer, &dns.AAAA{
				Hdr:  dns.RR_Header{Name: qname, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 0},
				AAAA: net.ParseIP("::1"),
			})
		}
		return resp
	}

	// Authoritative NXDOMAIN with a minimal synthetic SOA.
	resp.Rcode = dns.RcodeNameError
	zone := enclosingLocallyServedZone(qname)
	soa := &dns.SOA{
		Hdr:     dns.RR_Header{Name: zone, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 0},
		Ns:      "localhost.",
		Mbox:    "nobody.invalid.",
		Serial:  1,
		Refresh: 28800,
		Retry:   7200,
		Expire:  604800,
		Minttl:  86400,
	}
	resp.Ns = append(resp.Ns, soa)
	return resp
}

// enclosingLocallyServedZone returns the most specific locally served
// zone containing qname.
func enclosingLocallyServedZone(qname string) string {
	qname = strings.ToLower(qname)
	if !strings.HasSuffix(qname, ".") {
		qname += "."
	}
	best := "invalid."
	for _, zone := range locallyServedZones {
		if qname == zone || strings.HasSuffix(qname, "."+zone) {
			if len(zone) > len(best) {
				best = zone
			}
		}
	}
	return best
}
