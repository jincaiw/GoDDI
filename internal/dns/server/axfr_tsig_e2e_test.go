package server

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/jincaiw/GoDDI/internal/dns/transfer"
	"github.com/miekg/dns"
)

// TestAXFRTSIGOverRealWire drives a real miekg/dns server and a real transfer
// client over TCP. The unit test with a fake ResponseWriter cannot prove the
// MAC chain: only a real server signs, and only a real client verifies. This is
// the test that catches "no signature found" / "bad signature" on the wire.
func TestAXFRTSIGOverRealWire(t *testing.T) {
	db := newAXFRTestDB(t)
	// Extra records so the transfer spans more than one message.
	for i := 0; i < 150; i++ {
		if _, err := db.Exec(`INSERT INTO dns_records (id,zone_id,name,type,ttl,value,enabled)
			VALUES (?,?,?,?,?,?,1)`, fmt.Sprintf("r%03d", i), "z1", fmt.Sprintf("h%03d", i), "A", 300, "192.0.2.10"); err != nil {
			t.Fatal(err)
		}
	}
	keyName := "xfr-key."
	secret := "c2VjcmV0LXNlY3JldC1zZWNyZXQtc2VjcmV0LXNlY3JldA==" // base64

	h := &DNSHandler{server: &Server{axfrHandler: transfer.NewAXFRHandler(db)}}

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := pc.LocalAddr().String()
	_ = pc.Close()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &dns.Server{
		Listener:   ln,
		Net:        "tcp",
		TsigSecret: map[string]string{keyName: secret},
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
			// Mirror the product wiring: the handler checks TsigStatus, which the
			// real server sets from the MAC it verified against TsigSecret.
			h.serveZoneTransfer(w, req, false, "127.0.0.1")
		}),
	}
	go func() { _ = srv.ActivateAndServe() }()
	defer func() { _ = srv.Shutdown() }()
	time.Sleep(100 * time.Millisecond)

	m := new(dns.Msg)
	m.SetAxfr("z.test.")
	m.SetTsig(keyName, dns.HmacSHA256, 300, time.Now().Unix())
	tr := &dns.Transfer{TsigSecret: map[string]string{keyName: secret}, DialTimeout: 3 * time.Second}
	env, err := tr.In(m, addr)
	if err != nil {
		t.Fatalf("AXFR request: %v", err)
	}
	records := 0
	messages := 0
	for e := range env {
		if e.Error != nil {
			t.Fatalf("AXFR failed on the wire: %v", e.Error)
		}
		messages++
		for _, rr := range e.RR {
			if _, ok := rr.(*dns.SOA); !ok {
				records++
			}
		}
	}
	if records != 151 {
		t.Fatalf("received %d records, want 151 (1 original + 150 added)", records)
	}
	if messages < 2 {
		t.Fatalf("received %d message(s); the multi-message signing path was not exercised", messages)
	}
}
