package transfer

import (
	"database/sql"
	"net"
	"strings"
	"testing"

	"github.com/miekg/dns"
	_ "modernc.org/sqlite"
)

func TestValidateTransferredAXFRRejectsInconsistentRRsetTTLs(t *testing.T) {
	soa := func() *dns.SOA {
		return &dns.SOA{
			Hdr: dns.RR_Header{Name: "example.test.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 3600},
			Ns:  "ns.example.test.", Mbox: "hostmaster.example.test.", Serial: 7,
			Refresh: 3600, Retry: 600, Expire: 86400, Minttl: 300,
		}
	}
	records := []dns.RR{
		soa(),
		&dns.A{Hdr: dns.RR_Header{Name: "www.example.test.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}},
		&dns.A{Hdr: dns.RR_Header{Name: "WWW.EXAMPLE.TEST.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 600}},
		soa(),
	}
	if err := validateTransferredAXFR("example.test.", records); err == nil || !strings.Contains(err.Error(), "inconsistent TTLs") {
		t.Fatalf("AXFR TTL validation error = %v, want inconsistent RRset TTL rejection", err)
	}
}

// TestParsePrimaryAddress covers the transport scheme prefixes for
// XFR-over-TLS (RFC 9103) support in transfer_policy.
func TestParsePrimaryAddress(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantAddr  string
		wantTLS   bool
		wantInsec bool
		wantErr   bool
	}{
		{"plain with port", "192.0.2.1:53", "192.0.2.1:53", false, false, false},
		{"plain default port", "primary.example.com", "primary.example.com:53", false, false, false},
		{"tls with port", "tls://primary.example.com:8530", "primary.example.com:8530", true, false, false},
		{"tls default port", "tls://primary.example.com", "primary.example.com:853", true, false, false},
		{"tls-insecure", "tls-insecure://192.0.2.7", "192.0.2.7:853", true, true, false},
		{"empty after scheme", "tls://", "", false, false, true},
		{"empty", "", "", false, false, true},
		// net.SplitHostPort does not validate the port is numeric, so an
		// unknown scheme is passed through untouched and the dialer will
		// reject it later.
		{"wrong scheme treated as host", "http://x.example", "http://x.example", false, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr, useTLS, insecure, err := parsePrimaryAddress(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got addr=%q", addr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if addr != tc.wantAddr {
				t.Errorf("addr = %q, want %q", addr, tc.wantAddr)
			}
			if useTLS != tc.wantTLS {
				t.Errorf("useTLS = %v, want %v", useTLS, tc.wantTLS)
			}
			if insecure != tc.wantInsec {
				t.Errorf("insecure = %v, want %v", insecure, tc.wantInsec)
			}
			// Recognised TLS schemes must never leak into the address.
			if tc.wantTLS && strings.Contains(addr, "tls") {
				t.Errorf("addr still contains scheme prefix: %q", addr)
			}
		})
	}
}

// TestParseTransferPolicyTLS ensures scheme prefixes coexist with TSIG
// fields in the pipe-delimited policy string.
func TestParseTransferPolicyTLS(t *testing.T) {
	cfg, err := parseTransferPolicy("tls://primary.example.com|tsig-key|c2VjcmV0|hmac-sha512")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.useTLS || cfg.tlsInsecure {
		t.Errorf("useTLS=%v insecure=%v, want true/false", cfg.useTLS, cfg.tlsInsecure)
	}
	if cfg.primaryAddr != "primary.example.com:853" {
		t.Errorf("addr = %q, want primary.example.com:853", cfg.primaryAddr)
	}
	if cfg.tsigName != "tsig-key" || cfg.tsigSecret != "c2VjcmV0" {
		t.Errorf("TSIG fields lost: %q/%q", cfg.tsigName, cfg.tsigSecret)
	}
	if cfg.tsigAlgo != "hmac-sha512" {
		t.Errorf("algo = %q, want hmac-sha512", cfg.tsigAlgo)
	}
}

// TestInsertRRTxStoresFQDNName is the regression for a silent whole-zone outage:
// a transferred zone committed its records under relative names ("www"), the
// zone store indexed them as FQDN("www")="www." (outside the zone), and the
// secondary answered NXDOMAIN for every query while its SOA serial looked
// correct. The stored name must be the absolute FQDN, matching the control-plane
// writer and the zone store's indexing.
func TestInsertRRTxStoresFQDNName(t *testing.T) {
	db := newSecondaryDB(t)
	s := NewSecondarySync(db)
	rr := &dns.A{
		Hdr: dns.RR_Header{Name: "www.example.test.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   net.ParseIP("192.0.2.7"),
	}
	if err := s.insertRRTx(db, "zone-1", rr, "example.test."); err != nil {
		t.Fatalf("insertRRTx: %v", err)
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM dns_records WHERE zone_id='zone-1'`).Scan(&name); err != nil {
		t.Fatalf("reading stored record: %v", err)
	}
	if name != "www.example.test." {
		t.Fatalf("stored name = %q, want the FQDN www.example.test.", name)
	}
}

// newSecondaryDB builds the minimal schema insertRRTx writes to.
func newSecondaryDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE dns_records (
		id TEXT PRIMARY KEY, zone_id TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL,
		value TEXT NOT NULL, ttl INTEGER NOT NULL, priority INTEGER, weight INTEGER,
		port INTEGER, flag INTEGER, tag TEXT, enabled INTEGER NOT NULL DEFAULT 1,
		comment TEXT, tags TEXT, owner TEXT, expires_at DATETIME, authored_locally INTEGER DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func axfrFrameTOKs() (*dns.SOA, *dns.SOA) {
	opening := &dns.SOA{
		Hdr: dns.RR_Header{Name: "example.test.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 3600},
		Ns:  "ns.example.test.", Mbox: "hostmaster.example.test.", Serial: 7,
		Refresh: 3600, Retry: 600, Expire: 86400, Minttl: 300,
	}
	closing := dns.Copy(opening).(*dns.SOA)
	return opening, closing
}

func TestValidateTransferredAXFRRejectsBadFrameTail(t *testing.T) {
	opening, closing := axfrFrameTOKs()
	record := &dns.A{Hdr: dns.RR_Header{Name: "www.example.test.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: net.ParseIP("192.0.2.1")}
	closing.Serial++
	if err := validateTransferredAXFR("example.test.", []dns.RR{opening, record, closing}); err == nil ||
		!strings.Contains(err.Error(), "do not match") {
		t.Fatalf("mismatched closing SOA = %v, want rejection", err)
	}

	opening, _ = axfrFrameTOKs()
	if err := validateTransferredAXFR("example.test.", []dns.RR{opening, record}); err == nil ||
		!strings.Contains(err.Error(), "begin and end with SOA") {
		t.Fatalf("truncated tail = %v, want rejection", err)
	}

	opening, closing = axfrFrameTOKs()
	outside := &dns.A{Hdr: dns.RR_Header{Name: "outside.invalid.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: net.ParseIP("192.0.2.2")}
	if err := validateTransferredAXFR("example.test.", []dns.RR{opening, outside, closing}); err == nil ||
		!strings.Contains(err.Error(), "outside zone") {
		t.Fatalf("out-of-zone record = %v, want rejection", err)
	}
}
