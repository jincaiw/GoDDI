// Command dns_drill_server is a tiny two-node DNS drill harness for 方案A
// (standard primary/secondary). It reuses the product's real transfer code
// (transfer.AXFRHandler for the primary, transfer.SecondarySync for the
// secondary) over real DNS wire, so scripts/dns_primary_secondary_container_drill.sh
// can run the pair in containers and probe it with dig. It is NOT the full
// goddi server; it exists only to exercise the transfer path end to end.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/jincaiw/GoDDI/internal/dns/transfer"
	"github.com/miekg/dns"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS dns_zones (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, type TEXT NOT NULL,
  enabled BOOLEAN DEFAULT 1, soa_mname TEXT NOT NULL, soa_rname TEXT NOT NULL, serial INTEGER NOT NULL,
  refresh INTEGER DEFAULT 3600, retry INTEGER DEFAULT 600, expire INTEGER DEFAULT 86400, minimum INTEGER DEFAULT 300,
  default_ttl INTEGER DEFAULT 3600, transfer_policy TEXT, updated_at DATETIME DEFAULT (datetime('now')),
  last_sync_at DATETIME, last_sync_attempt_at DATETIME, sync_failure_count INTEGER DEFAULT 0);
CREATE TABLE IF NOT EXISTS dns_zone_changes (id TEXT PRIMARY KEY, zone_id TEXT NOT NULL, serial INTEGER NOT NULL,
  change_type TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL, value TEXT NOT NULL, ttl INTEGER DEFAULT 300,
  priority INTEGER, weight INTEGER, port INTEGER, flag INTEGER, tag TEXT, created_at DATETIME DEFAULT (datetime('now')));
CREATE TABLE IF NOT EXISTS dns_zone_transfer (id TEXT PRIMARY KEY, zone_id TEXT NOT NULL, allowed_cidr TEXT NOT NULL,
  tsig_key_name TEXT, created_at DATETIME DEFAULT (datetime('now')));
CREATE TABLE IF NOT EXISTS dns_records (id TEXT PRIMARY KEY, zone_id TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL,
  ttl INTEGER NOT NULL, value TEXT NOT NULL, priority INTEGER, weight INTEGER, port INTEGER, tag TEXT, flag INTEGER,
  expires_at DATETIME, enabled BOOLEAN NOT NULL DEFAULT 1, authored_locally INTEGER NOT NULL DEFAULT 0);`

func main() {
	mode := flag.String("mode", "primary", "primary|secondary")
	dbPath := flag.String("db", "/data/dns.db", "sqlite path")
	listen := flag.String("listen", "0.0.0.0:53", "listen address")
	zone := flag.String("zone", "drill.test.", "zone")
	record := flag.String("record", "192.0.2.10", "A value for www")
	primary := flag.String("primary", "", "primary addr host:port (secondary)")
	tsigName := flag.String("tsig-name", "xfr-key.", "TSIG key name")
	tsigSecret := flag.String("tsig-secret", "", "TSIG secret (base64)")
	flag.Parse()
	if *tsigSecret == "" {
		log.Fatal("missing -tsig-secret")
	}
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		log.Fatalf("schema: %v", err)
	}
	if *mode == "primary" {
		if err := seedPrimary(db, *zone, *record, *tsigName); err != nil {
			log.Fatalf("seed: %v", err)
		}
		h := &handler{db: db, zone: *zone, axfr: transfer.NewAXFRHandler(db)}
		serve(*listen, h, map[string]string{*tsigName: *tsigSecret}, false)
	} else {
		if err := seedSecondary(db, *zone, *primary, *tsigName, *tsigSecret); err != nil {
			log.Fatalf("seed secondary: %v", err)
		}
		sync := transfer.NewSecondarySync(db)
		go func() {
			for {
				if err := sync.SyncFromPrimary("s1"); err == nil {
					log.Printf("secondary synced from %s", *primary)
					break
				} else {
					log.Printf("secondary sync retry: %v", err)
				}
				time.Sleep(2 * time.Second)
			}
		}()
		h := &handler{db: db, zone: *zone}
		serve(*listen, h, nil, true)
	}
}

func seedPrimary(db *sql.DB, zone, record, tsigName string) error {
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM dns_zones WHERE id='z1'`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for _, q := range []string{
		fmt.Sprintf(`INSERT INTO dns_zones (id,name,type,enabled,soa_mname,soa_rname,serial,default_ttl) VALUES ('z1','%s','primary',1,'ns.%s','hostmaster.%s',1,300)`, zone, zone, zone),
		fmt.Sprintf(`INSERT INTO dns_records (id,zone_id,name,type,ttl,value,enabled) VALUES ('r1','z1','www','A',300,'%s',1)`, record),
		fmt.Sprintf(`INSERT INTO dns_zone_transfer (id,zone_id,allowed_cidr,tsig_key_name) VALUES ('t1','z1','0.0.0.0/0','%s')`, tsigName),
	} {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func seedSecondary(db *sql.DB, zone, primary, tsigName, tsigSecret string) error {
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM dns_zones WHERE id='s1'`).Scan(&n); err != nil {
		return err
	}
	policy := fmt.Sprintf("%s|%s|%s|hmac-sha256", primary, tsigName, tsigSecret)
	_, err := db.Exec(`INSERT OR REPLACE INTO dns_zones (id,name,type,enabled,soa_mname,soa_rname,serial,default_ttl,transfer_policy)
		VALUES ('s1',?,'secondary',1,'ns.'||?,'hostmaster.'||?,1,300,?)`, zone, zone, zone, policy)
	return err
}

type handler struct {
	db   *sql.DB
	zone string
	axfr *transfer.AXFRHandler
}

func (h *handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	if len(r.Question) == 0 {
		return
	}
	q := r.Question[0]
	tsigName := ""
	if t := r.IsTsig(); t != nil {
		tsigName = t.Hdr.Name
	}
	if q.Qtype == dns.TypeAXFR && h.axfr != nil {
		rrs, err := h.axfr.HandleAXFR(q.Name, tsigName)
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
		return
	}
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	if q.Qtype == dns.TypeA {
		h.answerA(m, q.Name)
	}
	_ = w.WriteMsg(m)
}

func (h *handler) answerA(m *dns.Msg, qname string) {
	qn := strings.ToLower(dns.Fqdn(qname))
	rows, err := h.db.Query(`SELECT name,value,ttl FROM dns_records WHERE type='A' AND enabled=1`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var name, value string
		var ttl int
		if rows.Scan(&name, &value, &ttl) != nil {
			continue
		}
		fqdn := strings.ToLower(dns.Fqdn(name))
		if !strings.HasSuffix(fqdn, strings.ToLower(dns.Fqdn(h.zone))) {
			fqdn = strings.ToLower(dns.Fqdn(name + "." + h.zone))
		}
		if fqdn == qn {
			m.Answer = append(m.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: dns.Fqdn(name), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(ttl)},
				A:   net.ParseIP(value),
			})
			return
		}
	}
}

func serve(addr string, h dns.Handler, tsig map[string]string, serveUDPOnly bool) {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Fatalf("udp listen: %v", err)
	}
	udp := &dns.Server{PacketConn: pc, Handler: h, TsigSecret: tsig}
	go func() { log.Printf("dns udp on %s", addr); log.Println(udp.ActivateAndServe()) }()
	if !serveUDPOnly {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatalf("tcp listen: %v", err)
		}
		tcp := &dns.Server{Listener: ln, Handler: h, TsigSecret: tsig}
		go func() { log.Println(tcp.ActivateAndServe()) }()
	}
	log.Printf("dns server (%s) ready", addr)
	select {}
}

var _ = os.Exit
