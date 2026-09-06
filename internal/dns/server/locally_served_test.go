package server

import (
	"testing"

	"github.com/miekg/dns"
)

// TestIsLocallyServed covers the RFC 6303/6761 zone list matching.
func TestIsLocallyServed(t *testing.T) {
	cases := []struct {
		qname string
		want  bool
	}{
		{"localhost.", true},
		{"LOCALHOST.", true},
		{"localhost", true},
		{"test.", true},
		{"foo.test.", true},
		{"invalid.", true},
		{"example.com.", true},
		{"www.example.com.", true},
		{"1.0.0.127.in-addr.arpa.", true},
		{"onion.", true},
		// Not locally served.
		{"github.com.", false},
		{"example.dev.", false},
		{"mylocalhost.", false},
		{"test.example.io.", false},
	}
	for _, tc := range cases {
		if got := isLocallyServed(tc.qname); got != tc.want {
			t.Errorf("isLocallyServed(%q) = %v, want %v", tc.qname, got, tc.want)
		}
	}
}

// TestAnswerLocallyServedLocalhost ensures RFC 6761 §6.3 loopback answers.
func TestAnswerLocallyServedLocalhost(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("localhost.", dns.TypeA)
	resp := answerLocallyServed(req)

	if resp.Rcode != dns.RcodeSuccess {
		t.Errorf("rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if !resp.Authoritative {
		t.Error("response not authoritative")
	}
	foundA := false
	for _, rr := range resp.Answer {
		if a, ok := rr.(*dns.A); ok && a.A.String() == "127.0.0.1" {
			foundA = true
		}
	}
	if !foundA {
		t.Errorf("missing 127.0.0.1 A answer, got %v", resp.Answer)
	}
}

// TestAnswerLocallyServedSpecial covers the NXDOMAIN + synthetic SOA path.
func TestAnswerLocallyServedSpecial(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("www.example.com.", dns.TypeA)
	resp := answerLocallyServed(req)

	if resp.Rcode != dns.RcodeNameError {
		t.Errorf("rcode = %s, want NXDOMAIN", dns.RcodeToString[resp.Rcode])
	}
	if !resp.Authoritative {
		t.Error("response not authoritative")
	}
	if len(resp.Ns) != 1 {
		t.Fatalf("authority = %d records, want 1 synthetic SOA", len(resp.Ns))
	}
	if _, ok := resp.Ns[0].(*dns.SOA); !ok {
		t.Errorf("authority record is %T, want *dns.SOA", resp.Ns[0])
	}
}
