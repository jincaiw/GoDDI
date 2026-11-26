package dnsanswer

import (
	"fmt"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/dnswire"
	"github.com/miekg/dns"
)

func TestEDNSAndBudgets(t *testing.T) {
	r := resolver(t, fixture())
	for _, tc := range []struct {
		advertised uint16
		present    bool
		want       uint16
	}{{0, false, 512}, {128, true, 512}, {512, true, 512}, {900, true, 900}, {4096, true, 1232}} {
		q := query("www.example.test.", dns.TypeA)
		if tc.present {
			q.SetEdns0(tc.advertised, false)
		}
		resp := r.Answer("internal", q)
		if resp.Rcode != dns.RcodeSuccess {
			t.Fatal(resp.Rcode)
		}
		if tc.present {
			o := resp.IsEdns0()
			if o == nil || o.UDPSize() != tc.want || len(o.Option) != 0 {
				t.Fatalf("OPT %v", o)
			}
		} else if resp.IsEdns0() != nil {
			t.Fatal("unexpected OPT")
		}
		wire, err := PackUDP(q, resp)
		if err != nil || len(wire) > int(tc.want) {
			t.Fatalf("pack %d: %v", len(wire), err)
		}
		parsed := new(dns.Msg)
		if err = parsed.Unpack(wire); err != nil || parsed.Id != q.Id || len(parsed.Question) != 1 || len(parsed.Answer) != 1 {
			t.Fatalf("unpack %v %v", parsed, err)
		}
	}
}
func TestEDNSRejectsUnsupported(t *testing.T) {
	r := resolver(t, fixture())
	for _, tc := range []struct {
		name   string
		mutate func(*dns.Msg)
		rcode  int
	}{
		{"version", func(q *dns.Msg) { q.SetEdns0(1232, false); q.IsEdns0().SetVersion(1) }, dns.RcodeBadVers},
		{"do", func(q *dns.Msg) { q.SetEdns0(1232, true) }, dns.RcodeNotImplemented},
		{"unknown flag", func(q *dns.Msg) { q.SetEdns0(1232, false); q.IsEdns0().Hdr.Ttl |= 1 }, dns.RcodeNotImplemented},
		{"option", func(q *dns.Msg) {
			q.SetEdns0(1232, false)
			q.IsEdns0().Option = append(q.IsEdns0().Option, &dns.EDNS0_SUBNET{Code: dns.EDNS0SUBNET, Family: 1, SourceNetmask: 24})
		}, dns.RcodeNotImplemented},
		{"duplicate opt", func(q *dns.Msg) { q.SetEdns0(1232, false); q.Extra = append(q.Extra, dns.Copy(q.Extra[0])) }, dns.RcodeFormatError},
		{"non-opt", func(q *dns.Msg) {
			q.Extra = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: "example.test.", Rrtype: dns.TypeA, Class: dns.ClassINET}}}
		}, dns.RcodeFormatError},
		{"opt owner", func(q *dns.Msg) { q.SetEdns0(1232, false); q.IsEdns0().Hdr.Name = "example.test." }, dns.RcodeFormatError},
		{"extended rcode", func(q *dns.Msg) { q.SetEdns0(1232, false); q.IsEdns0().Hdr.Ttl |= 1 << 24 }, dns.RcodeFormatError},
		{"opt in answer", func(q *dns.Msg) { q.SetEdns0(1232, false); q.Answer = q.Extra; q.Extra = nil }, dns.RcodeFormatError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := query("www.example.test.", dns.TypeA)
			tc.mutate(q)
			resp := r.Answer("internal", q)
			if resp.Rcode != tc.rcode || len(resp.Answer) != 0 {
				t.Fatalf("got %v", resp)
			}
			if tc.rcode == dns.RcodeBadVers {
				wire, err := PackUDP(q, resp)
				if err != nil {
					t.Fatal(err)
				}
				parsed := new(dns.Msg)
				if err = parsed.Unpack(wire); err != nil || parsed.Rcode != dns.RcodeBadVers || parsed.IsEdns0() == nil {
					t.Fatalf("BADVERS unpack %v %v", parsed, err)
				}
			}
		})
	}
}
func TestUDPTruncatesWholeRRSetAndTCPRemainsFull(t *testing.T) {
	d := fixture()
	for i := 1; i <= 150; i++ {
		d.Zones[0].Records = append(d.Zones[0].Records, dnswire.Record{Owner: "big.example.test.", Type: "A", RData: fmt.Sprintf("192.0.2.%d", i), TTL: 60})
	}
	r := resolver(t, d)
	for _, advertised := range []uint16{0, 200, 900, 4096} {
		q := query("big.example.test.", dns.TypeA)
		if advertised != 0 {
			q.SetEdns0(advertised, false)
		}
		resp := r.Answer("internal", q)
		if len(resp.Answer) != 150 {
			t.Fatal(len(resp.Answer))
		}
		tcp, err := resp.Pack()
		if err != nil || len(tcp) <= 1232 {
			t.Fatalf("TCP %d %v", len(tcp), err)
		}
		wire, err := PackUDP(q, resp)
		if err != nil {
			t.Fatal(err)
		}
		limit, _, _ := inspectEDNS(q)
		if len(wire) > limit {
			t.Fatalf("wire=%d limit=%d", len(wire), limit)
		}
		parsed := new(dns.Msg)
		if err = parsed.Unpack(wire); err != nil || !parsed.Truncated || len(parsed.Answer) != 0 || len(parsed.Question) != 1 || parsed.Id != q.Id {
			t.Fatalf("truncation %v %v", parsed, err)
		}
		if len(resp.Answer) != 150 || resp.Truncated {
			t.Fatal("PackUDP mutated TCP response")
		}
	}
}
func TestOversizedAuthorityDropsCompleteRRSet(t *testing.T) {
	q := query("missing.example.test.", dns.TypeA)
	resp := new(dns.Msg)
	resp.SetReply(q)
	resp.Authoritative = true
	resp.Rcode = dns.RcodeNameError
	for i := 0; i < 40; i++ {
		resp.Ns = append(resp.Ns, &dns.SOA{Hdr: dns.RR_Header{Name: "example.test.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 60}, Ns: "ns.example.test.", Mbox: "hostmaster.example.test.", Serial: uint32(i), Minttl: 60})
	}
	wire, err := PackUDP(q, resp)
	if err != nil {
		t.Fatal(err)
	}
	out := new(dns.Msg)
	if err = out.Unpack(wire); err != nil || !out.Truncated || out.Rcode != dns.RcodeNameError || len(out.Ns) != 0 || len(out.Question) != 1 || len(wire) > 512 {
		t.Fatalf("authority overflow: %v %v", out, err)
	}
	if len(resp.Ns) != 40 || resp.Truncated {
		t.Fatal("modified response")
	}
}

func TestNegativeSOAWithEDNS(t *testing.T) {
	r := resolver(t, fixture())
	for _, name := range []string{"missing.example.test.", "branch.example.test."} {
		q := query(name, dns.TypeA)
		q.SetEdns0(1232, false)
		resp := r.Answer("internal", q)
		wire, err := PackUDP(q, resp)
		if err != nil {
			t.Fatal(err)
		}
		out := new(dns.Msg)
		if err = out.Unpack(wire); err != nil || len(out.Ns) != 1 || out.Ns[0].Header().Ttl != 60 || out.IsEdns0() == nil {
			t.Fatalf("negative %v %v", out, err)
		}
	}
}
