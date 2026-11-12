package transfer

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
	_ "modernc.org/sqlite"
)

const (
	xfrZone  = "e2e.test."
	xfrKey   = "xfr-key."
	xfrPlain = "test-secret-material-0123456789"
)

func xfrSecretB64() string { return base64.StdEncoding.EncodeToString([]byte(xfrPlain)) }

func newXferDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { db.Close() })
	schema := `
	CREATE TABLE dns_zones (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, type TEXT NOT NULL,
	  enabled BOOLEAN DEFAULT TRUE, soa_mname TEXT NOT NULL, soa_rname TEXT NOT NULL, serial INTEGER NOT NULL,
	  refresh INTEGER DEFAULT 3600, retry INTEGER DEFAULT 600, expire INTEGER DEFAULT 86400, minimum INTEGER DEFAULT 300,
	  default_ttl INTEGER DEFAULT 3600, transfer_policy TEXT, updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
	  last_sync_at DATETIME, last_sync_attempt_at DATETIME, sync_failure_count INTEGER NOT NULL DEFAULT 0);
	CREATE TABLE dns_zone_changes (id TEXT PRIMARY KEY, zone_id TEXT NOT NULL, serial INTEGER NOT NULL,
	  change_type TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL, value TEXT NOT NULL, ttl INTEGER DEFAULT 300,
	  priority INTEGER, weight INTEGER, port INTEGER, flag INTEGER, tag TEXT, created_at DATETIME NOT NULL DEFAULT (datetime('now')));
	CREATE TABLE dns_zone_transfer (id TEXT PRIMARY KEY, zone_id TEXT NOT NULL, allowed_cidr TEXT NOT NULL,
	  tsig_key_name TEXT, created_at DATETIME NOT NULL DEFAULT (datetime('now')));
	CREATE TABLE dns_records (id TEXT PRIMARY KEY, zone_id TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL,
	  ttl INTEGER NOT NULL, value TEXT NOT NULL, priority INTEGER, weight INTEGER, port INTEGER, tag TEXT, flag INTEGER,
	  expires_at DATETIME, enabled BOOLEAN NOT NULL DEFAULT TRUE, authored_locally INTEGER NOT NULL DEFAULT 0);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func seedPrimary(t *testing.T, db *sql.DB, serial int, www string) {
	t.Helper()
	stmts := []string{
		`INSERT OR REPLACE INTO dns_zones (id,name,type,enabled,soa_mname,soa_rname,serial,refresh,retry,expire,minimum,default_ttl)
		 VALUES ('z1','` + xfrZone + `','primary',1,'ns.e2e.test.','hostmaster.e2e.test.',` + fmt.Sprint(serial) + `,3600,600,86400,300,300)`,
		`DELETE FROM dns_records WHERE zone_id='z1'`,
		`INSERT INTO dns_records (id,zone_id,name,type,ttl,value,enabled) VALUES ('r1','z1','www','A',300,'` + www + `',1)`,
		`INSERT OR REPLACE INTO dns_zone_transfer (id,zone_id,allowed_cidr,tsig_key_name) VALUES ('t1','z1','0.0.0.0/0','` + xfrKey + `')`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("seed primary %q: %v", q, err)
		}
	}
}

type primaryHandler struct {
	h    *AXFRHandler
	db   *sql.DB
	zone string
}

func (p *primaryHandler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	if len(r.Question) == 0 {
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeFormatError)
		_ = w.WriteMsg(m)
		return
	}
	q := r.Question[0]
	tsigName := ""
	if t := r.IsTsig(); t != nil {
		tsigName = t.Hdr.Name
	}
	if q.Qtype == dns.TypeA {
		answerA(p.db, p.zone, w, r)
		return
	}
	if q.Qtype != dns.TypeAXFR {
		m := new(dns.Msg)
		m.SetReply(r)
		_ = w.WriteMsg(m)
		return
	}
	rrs, err := p.h.HandleAXFR(q.Name, tsigName)
	if err != nil {
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeRefused)
		_ = w.WriteMsg(m)
		return
	}
	const chunk = 100
	for start := 0; start < len(rrs); start += chunk {
		end := start + chunk
		if end > len(rrs) {
			end = len(rrs)
		}
		m := new(dns.Msg)
		m.SetReply(r)
		m.Authoritative = true
		m.Answer = rrs[start:end]
		if tsig := r.IsTsig(); tsig != nil {
			m.SetTsig(tsig.Hdr.Name, tsig.Algorithm, 300, time.Now().Unix())
		}
		if err := w.WriteMsg(m); err != nil {
			return
		}
		if r.IsTsig() != nil {
			if tw, ok := w.(interface{ TsigTimersOnly(bool) }); ok {
				tw.TsigTimersOnly(true)
			}
		}
	}
}

// startPrimary serves a real AXFR-over-TCP primary and returns its address.
func startPrimary(t *testing.T, db *sql.DB) (string, func()) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	addr := pc.LocalAddr().String()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	handler := &primaryHandler{h: NewAXFRHandler(db), db: db, zone: xfrZone}
	tsig := map[string]string{xfrKey: xfrSecretB64()}
	udp := &dns.Server{PacketConn: pc, Handler: handler, TsigSecret: tsig}
	tcp := &dns.Server{Listener: ln, Handler: handler, TsigSecret: tsig}
	go func() { _ = udp.ActivateAndServe() }()
	go func() { _ = tcp.ActivateAndServe() }()
	stop := func() { _ = udp.Shutdown(); _ = tcp.Shutdown() }
	t.Cleanup(stop)
	return addr, stop
}

// answerA answers a single A query from dns_records for a minimal authoritative
// server used by the drill (the product serves from the in-memory zone store;
// reading the same replicated rows is equivalent for a read-failover check).
func answerA(db *sql.DB, zone string, w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	if len(r.Question) == 1 && r.Question[0].Qtype == dns.TypeA {
		qn := strings.ToLower(dns.Fqdn(r.Question[0].Name))
		rows, err := db.Query(`SELECT name, value, ttl FROM dns_records WHERE type='A' AND enabled=1`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var name, value string
				var ttl int
				if rows.Scan(&name, &value, &ttl) != nil {
					continue
				}
				fqdn := strings.ToLower(dns.Fqdn(name))
				if !strings.HasSuffix(fqdn, strings.ToLower(dns.Fqdn(zone))) {
					fqdn = strings.ToLower(dns.Fqdn(name + "." + zone))
				}
				if fqdn == qn {
					m.Answer = append(m.Answer, &dns.A{
						Hdr: dns.RR_Header{Name: dns.Fqdn(name), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(ttl)},
						A:   net.ParseIP(value),
					})
					break
				}
			}
		}
	}
	_ = w.WriteMsg(m)
}

// startQueryServer serves minimal authoritative A queries from db over UDP+TCP.
func startQueryServer(t *testing.T, db *sql.DB) (string, *dns.Server, *dns.Server) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	ln, err := net.Listen("tcp", pc.LocalAddr().String())
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	h := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) { answerA(db, xfrZone, w, r) })
	udp := &dns.Server{PacketConn: pc, Handler: h}
	tcp := &dns.Server{Listener: ln, Handler: h}
	go func() { _ = udp.ActivateAndServe() }()
	go func() { _ = tcp.ActivateAndServe() }()
	t.Cleanup(func() { _ = udp.Shutdown(); _ = tcp.Shutdown() })
	return pc.LocalAddr().String(), udp, tcp
}

// queryA resolves an A record over UDP, returning "" on any failure/timeout.
func queryA(t *testing.T, addr, name string) string {
	t.Helper()
	c := new(dns.Client)
	c.Timeout = 500 * time.Millisecond
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), dns.TypeA)
	resp, _, err := c.Exchange(m, addr)
	if err != nil || resp == nil {
		return ""
	}
	for _, rr := range resp.Answer {
		if a, ok := rr.(*dns.A); ok {
			return a.A.String()
		}
	}
	return ""
}

func seedSecondary(t *testing.T, db *sql.DB, policy string) {
	t.Helper()
	if _, err := db.Exec(`INSERT OR REPLACE INTO dns_zones (id,name,type,enabled,soa_mname,soa_rname,serial,refresh,retry,expire,minimum,default_ttl,transfer_policy)
		VALUES ('s1','`+xfrZone+`','secondary',1,'ns.e2e.test.','hostmaster.e2e.test.',1,3600,600,86400,300,300,?);`, policy); err != nil {
		t.Fatalf("seed secondary: %v", err)
	}
}

func secondaryRecord(t *testing.T, db *sql.DB) string {
	t.Helper()
	var v string
	err := db.QueryRow(`SELECT value FROM dns_records WHERE zone_id='s1' AND name LIKE 'www%' AND type='A'`).Scan(&v)
	if err != nil {
		return ""
	}
	return v
}

// TestPrimarySecondaryAXFREndToEnd drives the real主从 path over loopback TCP
// with TSIG: a secondary pulls a zone from a primary via AXFR, then re-syncs an
// updated zone, and a NOTIFY triggers a refresh.
func TestPrimarySecondaryAXFREndToEnd(t *testing.T) {
	pdb := newXferDB(t)
	seedPrimary(t, pdb, 10, "192.0.2.10")
	addr, _ := startPrimary(t, pdb)

	sdb := newXferDB(t)
	seedSecondary(t, sdb, fmt.Sprintf("%s|%s|%s|hmac-sha256", addr, xfrKey, xfrSecretB64()))
	sync := NewSecondarySync(sdb)

	if err := sync.SyncFromPrimary("s1"); err != nil {
		t.Fatalf("initial AXFR sync: %v", err)
	}
	if got := secondaryRecord(t, sdb); got != "192.0.2.10" {
		t.Fatalf("after AXFR secondary A = %q, want 192.0.2.10", got)
	}
	var serial uint32
	_ = sdb.QueryRow(`SELECT serial FROM dns_zones WHERE id='s1'`).Scan(&serial)
	if serial != 10 {
		t.Fatalf("secondary serial = %d, want primary 10", serial)
	}

	// Change on primary: new serial + new address → re-sync picks it up.
	seedPrimary(t, pdb, 11, "192.0.2.11")
	if err := sync.SyncFromPrimary("s1"); err != nil {
		t.Fatalf("second AXFR sync: %v", err)
	}
	if got := secondaryRecord(t, sdb); got != "192.0.2.11" {
		t.Fatalf("after re-sync secondary A = %q, want 192.0.2.11", got)
	}

	// NOTIFY-triggered refresh after another change.
	seedPrimary(t, pdb, 12, "192.0.2.12")
	if err := sync.HandleNotify(xfrZone); err != nil {
		t.Fatalf("HandleNotify: %v", err)
	}
	if got := secondaryRecord(t, sdb); got != "192.0.2.12" {
		t.Fatalf("after NOTIFY secondary A = %q, want 192.0.2.12", got)
	}
}

// TestPrimarySecondaryRejectsMissingOrWrongTSIG proves fail-closed: a policy
// without TSIG is refused, and a wrong shared secret fails the signed transfer.
func TestPrimarySecondaryRejectsMissingOrWrongTSIG(t *testing.T) {
	pdb := newXferDB(t)
	seedPrimary(t, pdb, 1, "192.0.2.10")
	addr, _ := startPrimary(t, pdb)

	// Policy without TSIG fields → rejected before any transfer.
	sdb := newXferDB(t)
	seedSecondary(t, sdb, addr)
	if err := NewSecondarySync(sdb).SyncFromPrimary("s1"); err == nil {
		t.Fatal("sync without TSIG was allowed; must be fail-closed")
	}

	// Wrong secret → the primary rejects the signed AXFR.
	sdb2 := newXferDB(t)
	wrong := base64.StdEncoding.EncodeToString([]byte("wrong-secret-xxxxxxxx"))
	seedSecondary(t, sdb2, fmt.Sprintf("%s|%s|%s|hmac-sha256", addr, xfrKey, wrong))
	if err := NewSecondarySync(sdb2).SyncFromPrimary("s1"); err == nil {
		t.Fatal("sync with wrong TSIG secret was allowed; must fail")
	}
}

// TestPrimarySecondaryReadFailover is the runnable两实例演练: a real primary
// and a real secondary serve DNS over loopback; the secondary pulls the zone,
// serves the latest data after a change, and keeps answering after the primary
// is stopped (read failover). Invoked by scripts/dns_primary_secondary_drill.sh.
func TestPrimarySecondaryReadFailover(t *testing.T) {
	pdb := newXferDB(t)
	seedPrimary(t, pdb, 20, "192.0.2.20")
	paddr, pstop := startPrimary(t, pdb)

	sdb := newXferDB(t)
	seedSecondary(t, sdb, fmt.Sprintf("%s|%s|%s|hmac-sha256", paddr, xfrKey, xfrSecretB64()))
	sync := NewSecondarySync(sdb)
	if err := sync.SyncFromPrimary("s1"); err != nil {
		t.Fatalf("initial AXFR: %v", err)
	}
	// Change on primary, re-sync, then start the secondary's serving socket.
	seedPrimary(t, pdb, 21, "192.0.2.21")
	if err := sync.SyncFromPrimary("s1"); err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	saddr, _, _ := startQueryServer(t, sdb)

	name := "www." + xfrZone
	if got := queryA(t, paddr, name); got != "192.0.2.21" {
		t.Fatalf("primary answer = %q, want 192.0.2.21", got)
	}
	t.Logf("PASS | primary serves latest A %s = 192.0.2.21", name)
	if got := queryA(t, saddr, name); got != "192.0.2.21" {
		t.Fatalf("secondary answer before failover = %q, want 192.0.2.21", got)
	}
	t.Logf("PASS | secondary mirrors A %s = 192.0.2.21", name)

	// Take the primary down; the secondary must keep answering from its local copy.
	pstop()
	if got := queryA(t, paddr, name); got != "" {
		t.Fatalf("primary still answering after shutdown: %q", got)
	}
	t.Logf("PASS | primary down: primary no longer answers")
	if got := queryA(t, saddr, name); got != "192.0.2.21" {
		t.Fatalf("secondary stopped answering after primary down: %q", got)
	}
	t.Logf("PASS | secondary still answers after primary down (read failover)")
}
