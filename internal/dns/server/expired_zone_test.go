package server

import (
	"testing"

	"github.com/jincaiw/GoDDI/internal/config"
	"github.com/jincaiw/GoDDI/internal/dns/zone"
	"github.com/miekg/dns"
)

// A secondary zone past its SOA EXPIRE must not answer from the local copy:
// the primary may have reassigned names the stale copy still holds. The
// response must be SERVFAIL so resolvers try another server instead of
// caching a negative answer.
func TestExpiredSecondaryZoneReturnsSERVFAIL(t *testing.T) {
	zs := newTestZoneStore(t, string(zone.ZoneTypeSecondary), nil)
	defer zs.Close()
	zs.MarkZoneExpired("example.test.")
	if !zs.IsZoneExpired("example.test.") {
		t.Fatal("zone was not marked expired")
	}

	srv := New(&config.Config{}, nil, nil, nil, nil, nil, zs)
	resp, denied, found := srv.lookupAuthoritative("www.example.test.", dns.TypeA, "192.0.2.1", false)
	if denied {
		t.Fatal("expired zone returned an ACL denial")
	}
	if !found || resp == nil {
		t.Fatal("expired zone returned no authoritative response")
	}
	if resp.Rcode != dns.RcodeServerFailure {
		t.Fatalf("rcode = %s, want SERVFAIL", dns.RcodeToString[resp.Rcode])
	}
	if len(resp.Answer) != 0 || len(resp.Ns) != 0 {
		t.Fatalf("expired zone leaked records: answer=%d authority=%d", len(resp.Answer), len(resp.Ns))
	}
}
