// Package dnsanswer is an isolated, read-only DNS answer prototype. It does
// not authenticate snapshots or views, authorize service, publish releases,
// listen on a network, or implement transfers, updates, forwarding or DNSSEC.
package dnsanswer

import (
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/jincaiw/GoDDI/internal/enterprise/dnswire"
	"github.com/miekg/dns"
)

var ErrUnsupported = errors.New("dnsanswer: unsupported candidate snapshot")

type zone struct {
	name    string
	names   map[string]bool
	records map[string]map[uint16][]dns.RR
	soa     *dns.SOA
}

// Resolver contains immutable copies of one candidate view. Construction does
// not establish that this view or candidate is trusted or authorized to serve.
type Resolver struct {
	view  string
	zones []zone
}

// Compile accepts only dnswire-validated candidates and rejects delegation and
// overlapping zones, whose answer semantics this prototype does not implement.
// The caller must independently verify provenance, activation and permission.
func Compile(snapshot dnswire.Snapshot) (*Resolver, error) {
	doc, err := snapshot.Document()
	if err != nil {
		return nil, err
	}
	r := &Resolver{view: doc.View}
	for _, z := range doc.Zones {
		for _, other := range doc.Zones {
			if z.Name != other.Name && inZone(z.Name, other.Name) {
				return nil, ErrUnsupported
			}
		}
		out := zone{name: z.Name, names: map[string]bool{}, records: map[string]map[uint16][]dns.RR{}}
		for _, rec := range z.Records {
			if rec.Type == "NS" && rec.Owner != z.Name {
				return nil, ErrUnsupported
			}
			rr, e := toRR(rec)
			if e != nil {
				return nil, ErrUnsupported
			}
			if out.records[rec.Owner] == nil {
				out.records[rec.Owner] = map[uint16][]dns.RR{}
			}
			typ := rr.Header().Rrtype
			out.records[rec.Owner][typ] = append(out.records[rec.Owner][typ], rr)
			if soa, ok := rr.(*dns.SOA); ok {
				out.soa = soa
			}
			// Ancestors are empty non-terminals, and therefore exist for NODATA.
			name := rec.Owner
			for {
				out.names[name] = true
				if name == z.Name {
					break
				}
				name = parent(name)
				if name == "" || !inZone(name, z.Name) {
					return nil, ErrUnsupported
				}
			}
		}
		if out.soa == nil {
			return nil, ErrUnsupported
		}
		r.zones = append(r.zones, out)
	}
	return r, nil
}
func parent(name string) string {
	i := strings.IndexByte(name, '.')
	if i < 0 {
		return ""
	}
	if i == len(name)-1 {
		return "."
	}
	return name[i+1:]
}
func inZone(name, zone string) bool {
	return zone == "." || name == zone || strings.HasSuffix(name, "."+zone)
}
func toRR(r dnswire.Record) (dns.RR, error) {
	h := dns.RR_Header{Name: r.Owner, Class: dns.ClassINET, Ttl: r.TTL}
	switch r.Type {
	case "A":
		h.Rrtype = dns.TypeA
		return &dns.A{Hdr: h, A: net.ParseIP(r.RData).To4()}, nil
	case "AAAA":
		h.Rrtype = dns.TypeAAAA
		return &dns.AAAA{Hdr: h, AAAA: net.ParseIP(r.RData).To16()}, nil
	case "NS":
		h.Rrtype = dns.TypeNS
		return &dns.NS{Hdr: h, Ns: r.RData}, nil
	case "SOA":
		p := strings.Split(r.RData, " ")
		if len(p) != 7 {
			return nil, ErrUnsupported
		}
		var v [5]uint32
		for i := range v {
			x, e := strconv.ParseUint(p[i+2], 10, 32)
			if e != nil {
				return nil, e
			}
			v[i] = uint32(x)
		}
		h.Rrtype = dns.TypeSOA
		return &dns.SOA{Hdr: h, Ns: p[0], Mbox: p[1], Serial: v[0], Refresh: v[1], Retry: v[2], Expire: v[3], Minttl: v[4]}, nil
	}
	return nil, ErrUnsupported
}

// Answer uses a caller-supplied trusted view; no DNS field supplies identity.
// Unsupported names/features fail closed. Responses and RR values are private
// copies, so callers may mutate them without changing the resolver.
func (r *Resolver) Answer(view string, req *dns.Msg) *dns.Msg {
	out := new(dns.Msg)
	if req == nil {
		out.Response = true
		out.Rcode = dns.RcodeFormatError
		return out
	}
	out.SetReply(req)
	if req.Response || req.Truncated {
		out.Rcode = dns.RcodeFormatError
		return out
	}
	if len(req.Question) != 1 {
		out.Rcode = dns.RcodeFormatError
		return out
	}
	limit, hasEDNS, ednsCode := inspectEDNS(req)
	if hasEDNS {
		out.SetEdns0(uint16(limit), false)
	}
	if ednsCode != dns.RcodeSuccess {
		out.Rcode = ednsCode
		return out
	}
	if req.Opcode != dns.OpcodeQuery {
		out.Rcode = dns.RcodeNotImplemented
		return out
	}
	q := req.Question[0]
	if q.Qclass != dns.ClassINET {
		out.Rcode = dns.RcodeRefused
		return out
	}
	if q.Qtype == dns.TypeAXFR || q.Qtype == dns.TypeIXFR {
		out.Rcode = dns.RcodeRefused
		return out
	}
	switch q.Qtype {
	case dns.TypeA, dns.TypeAAAA, dns.TypeNS, dns.TypeSOA:
	default:
		out.Rcode = dns.RcodeServerFailure
		return out
	}
	if r == nil || view == "" || view != r.view {
		out.Rcode = dns.RcodeServerFailure
		return out
	}
	if !simpleName(q.Name) {
		out.Rcode = dns.RcodeServerFailure
		return out
	}
	// Validate ASCII before folding: Unicode case folding can map invalid wire
	// names (for example U+212A) onto an unrelated valid ASCII owner.
	name := strings.ToLower(q.Name)
	for _, z := range r.zones {
		if inZone(name, z.name) {
			out.Authoritative = true
			records := z.records[name][q.Qtype]
			if len(records) > 0 {
				for _, rr := range records {
					out.Answer = append(out.Answer, dns.Copy(rr))
				}
				return out
			}
			if !z.names[name] {
				out.Rcode = dns.RcodeNameError
			}
			soa := dns.Copy(z.soa).(*dns.SOA)
			if soa.Minttl < soa.Hdr.Ttl {
				soa.Hdr.Ttl = soa.Minttl
			}
			out.Ns = []dns.RR{soa}
			return out
		}
	}
	out.Rcode = dns.RcodeServerFailure
	return out
}
func simpleName(name string) bool {
	if name == "." {
		return true
	}
	if len(name) < 2 || len(name) > 254 || name[len(name)-1] != '.' {
		return false
	}
	for _, label := range strings.Split(name[:len(name)-1], ".") {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		for i := range label {
			c := label[i]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return false
			}
		}
	}
	return true
}
