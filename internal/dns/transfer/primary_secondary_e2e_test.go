package transfer

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"net"
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

type primaryHandler struct{ h *AXFRHandler }

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
func startPrimary(t *testing.T, db *sql.DB) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &dns.Server{Listener: ln, Handler: &primaryHandler{h: NewAXFRHandler(db)},
		TsigSecret: map[string]string{xfrKey: xfrSecretB64()}}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.Dial("tcp", ln.Addr().String()); err == nil {
			c.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return ln.Addr().String()
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
	addr := startPrimary(t, pdb)

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
	addr := startPrimary(t, pdb)

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
