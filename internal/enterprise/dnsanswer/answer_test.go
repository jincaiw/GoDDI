package dnsanswer

import (
	"errors"
	"sync"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/dnswire"
	"github.com/miekg/dns"
)

func fixture() dnswire.Document {
	return dnswire.Document{Cluster: "11111111-1111-4111-8111-111111111111", Node: "22222222-2222-4222-8222-222222222222", Snapshot: "33333333-3333-4333-8333-333333333333", Version: "1", View: "internal", Zones: []dnswire.Zone{{Name: "example.test.", Serial: 42, Records: []dnswire.Record{
		{Owner: "example.test.", Type: "SOA", TTL: 300, RData: "ns.example.test. hostmaster.example.test. 42 3600 600 86400 60"},
		{Owner: "example.test.", Type: "NS", TTL: 300, RData: "ns.example.test."},
		{Owner: "www.example.test.", Type: "A", TTL: 120, RData: "192.0.2.1"},
		{Owner: "www.example.test.", Type: "AAAA", TTL: 240, RData: "2001:db8::1"},
		{Owner: "leaf.branch.example.test.", Type: "A", TTL: 30, RData: "192.0.2.2"},
	}}}}
}
func resolver(t *testing.T, doc dnswire.Document) *Resolver {
	t.Helper()
	s, e := dnswire.Build(doc)
	if e != nil {
		t.Fatal(e)
	}
	r, e := Compile(s)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func query(name string, qtype uint16) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(name, qtype)
	return m
}
func TestAnswerAuthoritative(t *testing.T) {
	r := resolver(t, fixture())
	for _, tc := range []struct {
		name   string
		typ    uint16
		rcode  int
		answer bool
		ttl    uint32
	}{
		{"WWW.Example.Test.", dns.TypeA, dns.RcodeSuccess, true, 120},
		{"www.example.test.", dns.TypeAAAA, dns.RcodeSuccess, true, 240},
		{"example.test.", dns.TypeNS, dns.RcodeSuccess, true, 300},
		{"example.test.", dns.TypeSOA, dns.RcodeSuccess, true, 300},
		{"www.example.test.", dns.TypeNS, dns.RcodeSuccess, false, 60},
		{"branch.example.test.", dns.TypeA, dns.RcodeSuccess, false, 60},
		{"missing.example.test.", dns.TypeA, dns.RcodeNameError, false, 60},
	} {
		t.Run(tc.name+dns.TypeToString[tc.typ], func(t *testing.T) {
			out := r.Answer("internal", query(tc.name, tc.typ))
			if out.Rcode != tc.rcode || !out.Authoritative || out.RecursionAvailable {
				t.Fatalf("response: %+v", out)
			}
			rrs := out.Ns
			if tc.answer {
				rrs = out.Answer
			}
			if len(rrs) != 1 || rrs[0].Header().Ttl != tc.ttl {
				t.Fatalf("records: %v", rrs)
			}
			if tc.answer && rrs[0].Header().Rrtype != tc.typ {
				t.Fatalf("type: %v", rrs[0])
			}
			if !tc.answer && rrs[0].Header().Rrtype != dns.TypeSOA {
				t.Fatalf("negative: %v", rrs)
			}
		})
	}
}
func TestFailClosed(t *testing.T) {
	r := resolver(t, fixture())
	q := query("www.example.test.", dns.TypeA)
	cases := []struct {
		name  string
		r     *Resolver
		view  string
		req   *dns.Msg
		rcode int
	}{
		{"no resolver", nil, "internal", q, dns.RcodeServerFailure},
		{"no view", r, "", q, dns.RcodeServerFailure},
		{"wrong view", r, "external", q, dns.RcodeServerFailure},
		{"outside", r, "internal", query("other.test.", dns.TypeA), dns.RcodeServerFailure},
		{"wildcard", r, "internal", query("*.example.test.", dns.TypeA), dns.RcodeServerFailure},
		{"unsupported type", r, "internal", query("www.example.test.", dns.TypeCNAME), dns.RcodeServerFailure},
		{"kelvin", r, "internal", query("K.example.test.", dns.TypeA), dns.RcodeServerFailure},
		{"turkish i", r, "internal", query("İ.example.test.", dns.TypeA), dns.RcodeServerFailure},
		{"invalid utf8", r, "internal", query("\xff.example.test.", dns.TypeA), dns.RcodeServerFailure},
		{"transfer", r, "internal", query("example.test.", dns.TypeAXFR), dns.RcodeRefused},
		{"incremental transfer", r, "internal", query("example.test.", dns.TypeIXFR), dns.RcodeRefused},
	}
	update := q.Copy()
	update.Opcode = dns.OpcodeUpdate
	cases = append(cases, struct {
		name  string
		r     *Resolver
		view  string
		req   *dns.Msg
		rcode int
	}{"update", r, "internal", update, dns.RcodeNotImplemented})
	notify := q.Copy()
	notify.Opcode = dns.OpcodeNotify
	cases = append(cases, struct {
		name  string
		r     *Resolver
		view  string
		req   *dns.Msg
		rcode int
	}{"notify", r, "internal", notify, dns.RcodeNotImplemented})
	many := q.Copy()
	many.Question = append(many.Question, many.Question[0])
	cases = append(cases, struct {
		name  string
		r     *Resolver
		view  string
		req   *dns.Msg
		rcode int
	}{"multiple", r, "internal", many, dns.RcodeFormatError})
	response := q.Copy()
	response.Response = true
	cases = append(cases, struct {
		name  string
		r     *Resolver
		view  string
		req   *dns.Msg
		rcode int
	}{"response packet", r, "internal", response, dns.RcodeFormatError})
	ch := q.Copy()
	ch.Question[0].Qclass = dns.ClassCHAOS
	cases = append(cases, struct {
		name  string
		r     *Resolver
		view  string
		req   *dns.Msg
		rcode int
	}{"class", r, "internal", ch, dns.RcodeRefused})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := tc.r.Answer(tc.view, tc.req)
			if out.Rcode != tc.rcode || out.Authoritative || out.RecursionAvailable || len(out.Answer) > 0 {
				t.Fatalf("response: %+v", out)
			}
		})
	}
}
func TestUnsupportedCandidate(t *testing.T) {
	doc := fixture()
	doc.Zones[0].Records = append(doc.Zones[0].Records, dnswire.Record{Owner: "child.example.test.", Type: "NS", TTL: 10, RData: "ns.child.example.test."})
	s, e := dnswire.Build(doc)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Compile(s); !errors.Is(e, ErrUnsupported) {
		t.Fatalf("delegation: %v", e)
	}
	doc = fixture()
	doc.Zones = append(doc.Zones, dnswire.Zone{Name: "child.example.test.", Serial: 1, Records: []dnswire.Record{{Owner: "child.example.test.", Type: "SOA", TTL: 1, RData: "ns.child.example.test. hostmaster.child.example.test. 1 1 1 1 1"}, {Owner: "child.example.test.", Type: "NS", TTL: 1, RData: "ns.child.example.test."}}})
	s, e = dnswire.Build(doc)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Compile(s); !errors.Is(e, ErrUnsupported) {
		t.Fatalf("overlap: %v", e)
	}
	if _, e = Compile(dnswire.Snapshot{}); e == nil {
		t.Fatal("zero candidate accepted")
	}
}
func TestAnswersAreIndependentAndConcurrent(t *testing.T) {
	r := resolver(t, fixture())
	q := query("www.example.test.", dns.TypeA)
	a := r.Answer("internal", q)
	a.Answer[0].(*dns.A).A[0] = 1
	a.Answer[0].Header().Ttl = 0
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				out := r.Answer("internal", q)
				if out.Answer[0].(*dns.A).A.String() != "192.0.2.1" || out.Answer[0].Header().Ttl != 120 {
					t.Error("answer was mutated")
				}
				out.Answer[0].(*dns.A).A[0] = 3
			}
		}()
	}
	wg.Wait()
}
