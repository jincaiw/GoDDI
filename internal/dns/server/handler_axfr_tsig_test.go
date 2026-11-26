package server

import (
	"database/sql"
	"net"
	"testing"
	"time"

	"github.com/jincaiw/GoDDI/internal/dns/transfer"
	"github.com/miekg/dns"
	_ "modernc.org/sqlite"
)

type captureWriter struct {
	msgs   []*dns.Msg
	timers bool
}

func (w *captureWriter) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53}
}
func (w *captureWriter) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53000}
}
func (w *captureWriter) WriteMsg(m *dns.Msg) error {
	w.msgs = append(w.msgs, m.Copy())
	return nil
}
func (w *captureWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *captureWriter) Close() error                { return nil }
func (w *captureWriter) TsigStatus() error           { return nil }
func (w *captureWriter) TsigTimersOnly(b bool)       { w.timers = b }
func (w *captureWriter) Hijack()                     {}

func newAXFRTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`
	CREATE TABLE dns_zones (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, type TEXT NOT NULL, enabled BOOLEAN DEFAULT TRUE,
	  soa_mname TEXT NOT NULL, soa_rname TEXT NOT NULL, serial INTEGER NOT NULL, refresh INTEGER DEFAULT 3600, retry INTEGER DEFAULT 600,
	  expire INTEGER DEFAULT 86400, minimum INTEGER DEFAULT 300, default_ttl INTEGER DEFAULT 3600, transfer_policy TEXT);
	CREATE TABLE dns_zone_transfer (id TEXT PRIMARY KEY, zone_id TEXT NOT NULL, allowed_cidr TEXT NOT NULL, tsig_key_name TEXT);
	CREATE TABLE dns_records (id TEXT PRIMARY KEY, zone_id TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL, ttl INTEGER NOT NULL,
	  value TEXT NOT NULL, priority INTEGER, weight INTEGER, port INTEGER, tag TEXT, flag INTEGER, expires_at DATETIME,
	  enabled BOOLEAN NOT NULL DEFAULT TRUE);`); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO dns_zones (id,name,type,enabled,soa_mname,soa_rname,serial,default_ttl) VALUES ('z1','z.test.','primary',1,'ns.z.test.','host.z.test.',5,300)`,
		`INSERT INTO dns_records (id,zone_id,name,type,ttl,value,enabled) VALUES ('r1','z1','www','A',300,'192.0.2.5',1)`,
		`INSERT INTO dns_zone_transfer (id,zone_id,allowed_cidr,tsig_key_name) VALUES ('t1','z1','0.0.0.0/0','xfr-key.')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// TestServeZoneTransferSignsAXFRResponse proves the product handler attaches the
// TSIG placeholder to every streamed AXFR/IXFR message (miekg only signs a
// message that already carries a TSIG RR). Without it a strict secondary gets
// "no signature found".
func TestServeZoneTransferSignsAXFRResponse(t *testing.T) {
	db := newAXFRTestDB(t)
	h := &DNSHandler{server: &Server{axfrHandler: transfer.NewAXFRHandler(db)}}

	req := new(dns.Msg)
	req.SetAxfr("z.test.")
	req.SetTsig("xfr-key.", dns.HmacSHA256, 300, time.Now().Unix())

	w := &captureWriter{}
	h.serveZoneTransfer(w, req, false, "127.0.0.1")

	if len(w.msgs) == 0 {
		t.Fatal("no AXFR response messages written")
	}
	for i, m := range w.msgs {
		if m.IsTsig() == nil {
			t.Fatalf("AXFR message %d is not TSIG-signed (would fail a strict secondary)", i)
		}
	}
	if len(w.msgs[0].Answer) == 0 || w.msgs[0].Answer[0].Header().Rrtype != dns.TypeSOA {
		t.Fatalf("AXFR response does not start with SOA: %+v", w.msgs[0].Answer)
	}
	if !w.timers {
		t.Fatal("handler did not switch subsequent transfer messages to TSIG timers-only")
	}
}
