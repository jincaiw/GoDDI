package server

import (
	"net"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/miekg/dns"
)

// NOTIFY is unauthenticated by design (RFC 1996): it is a hint to refresh,
// not a publication channel. A stale/spoofed message must not change the
// served zone or be echoed back as the server own SOA, and repeated
// messages must be suppressed.
func TestNotifyIsABoundedRefreshHintNotAPublication(t *testing.T) {
	zs, db := newTestZoneStoreWithDB(t, string(zone.ZoneTypePrimary), nil)
	defer zs.Close()
	if _, err := db.Exec(`INSERT INTO dns_records
		(id, zone_id, name, type, ttl, value, enabled)
		VALUES ('r-local', 'z1', 'www.example.test.', 'A', 300, '192.0.2.1', 1)`); err != nil {
		t.Fatal(err)
	}
	zs.Load()

	refreshes := make(chan string, 4)
	srv := New(&config.Config{}, nil, nil, nil, nil, nil, zs)
	srv.SetNotifyHandler(func(zoneName string) { refreshes <- zoneName })
	h := &DNSHandler{server: srv}

	staleSOA := &dns.SOA{
		Hdr: dns.RR_Header{Name: "example.test.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 60},
		Ns:  "attacker.invalid.", Mbox: "attacker.invalid.", Serial: 999,
		Refresh: 60, Retry: 60, Expire: 60, Minttl: 60,
	}
	staleA := &dns.A{Hdr: dns.RR_Header{Name: "www.example.test.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.ParseIP("192.0.2.99")}
	notify := new(dns.Msg)
	notify.SetQuestion("example.test.", dns.TypeSOA)
	notify.Opcode = dns.OpcodeNotify
	notify.Answer = []dns.RR{staleSOA, staleA}

	writer := &memoryResponseWriter{remote: &net.UDPAddr{IP: net.ParseIP("192.0.2.99"), Port: 53000}}
	h.ServeDNS(writer, notify)
	if writer.msg == nil {
		t.Fatal("no NOTIFY response")
	}
	if !writer.msg.Authoritative {
		t.Fatal("NOTIFY response is not authoritative")
	}
	if len(writer.msg.Answer) != 1 {
		t.Fatalf("NOTIFY answer records = %d, want the local SOA only", len(writer.msg.Answer))
	}
	soa, ok := writer.msg.Answer[0].(*dns.SOA)
	if !ok {
		t.Fatalf("NOTIFY answer type %T, want SOA", writer.msg.Answer[0])
	}
	if soa.Serial != 1 || soa.Ns != "ns1.example.test." {
		t.Fatalf("NOTIFY echoed stale SOA: %#v", soa)
	}
	_, answers, found := zs.Lookup("www.example.test.", dns.TypeA)
	if !found || len(answers) != 1 {
		t.Fatalf("stale NOTIFY changed served data: found=%v answers=%d", found, len(answers))
	}
	if a, ok := answers[0].(*dns.A); !ok || a.A.String() != "192.0.2.1" {
		t.Fatalf("served A = %v, want 192.0.2.1", answers[0])
	}

	select {
	case zoneName := <-refreshes:
		if zoneName != "example.test." {
			t.Fatalf("refresh zone = %q", zoneName)
		}
	case <-time.After(time.Second):
		t.Fatal("NOTIFY did not trigger a refresh")
	}
	h.ServeDNS(&memoryResponseWriter{remote: &net.UDPAddr{IP: net.ParseIP("192.0.2.99"), Port: 53000}}, notify)
	select {
	case zoneName := <-refreshes:
		t.Fatalf("duplicate NOTIFY triggered another refresh: %q", zoneName)
	case <-time.After(100 * time.Millisecond):
	}
}
