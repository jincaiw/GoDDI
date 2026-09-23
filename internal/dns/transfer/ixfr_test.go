package transfer

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/miekg/dns"
)

func TestHandleIXFR_ReturnsContiguousJournalDeltas(t *testing.T) {
	db := newSingleConnDB(t)
	seedIXFRZone(t, db, 12, "192.0.2.3")
	insertSOAChange(t, db, 11, "delete", 10)
	insertRRChange(t, db, 11, "delete", "192.0.2.1")
	insertRRChange(t, db, 11, "add", "192.0.2.2")
	insertSOAChange(t, db, 11, "add", 11)
	insertSOAChange(t, db, 12, "delete", 11)
	insertRRChange(t, db, 12, "delete", "192.0.2.2")
	insertRRChange(t, db, 12, "add", "192.0.2.3")
	insertSOAChange(t, db, 12, "add", 12)

	got, err := NewAXFRHandler(db).HandleIXFR("example.com.", 10, "xfer-key")
	if err != nil {
		t.Fatalf("HandleIXFR: %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("IXFR answer has %d records, want current SOA, two 4-RR deltas, and closing SOA: %v", len(got), got)
	}
	want := []struct {
		rtype  uint16
		serial uint32
		ip     string
	}{
		{rtype: dns.TypeSOA, serial: 12}, {rtype: dns.TypeSOA, serial: 10}, {rtype: dns.TypeA, ip: "192.0.2.1"},
		{rtype: dns.TypeSOA, serial: 11}, {rtype: dns.TypeA, ip: "192.0.2.2"}, {rtype: dns.TypeSOA, serial: 11},
		{rtype: dns.TypeA, ip: "192.0.2.2"}, {rtype: dns.TypeSOA, serial: 12}, {rtype: dns.TypeA, ip: "192.0.2.3"},
		{rtype: dns.TypeSOA, serial: 12},
	}
	for i, rr := range got {
		if rr.Header().Rrtype != want[i].rtype {
			t.Errorf("answer[%d] = %q, want type %s", i, rr, dns.TypeToString[want[i].rtype])
			continue
		}
		if want[i].rtype == dns.TypeSOA && rr.(*dns.SOA).Serial != want[i].serial {
			t.Errorf("answer[%d] SOA serial = %d, want %d", i, rr.(*dns.SOA).Serial, want[i].serial)
		}
		if want[i].rtype == dns.TypeA && rr.(*dns.A).A.String() != want[i].ip {
			t.Errorf("answer[%d] A = %s, want %s", i, rr.(*dns.A).A, want[i].ip)
		}
	}
}

func TestHandleIXFR_FallsBackWhenJournalChainHasGap(t *testing.T) {
	db := newSingleConnDB(t)
	seedIXFRZone(t, db, 12, "192.0.2.3")
	// Serial 11 is missing. A partial IXFR would leave the client stale, so
	// the server must return a complete AXFR.
	insertSOAChange(t, db, 12, "delete", 11)
	insertRRChange(t, db, 12, "add", "192.0.2.3")
	insertSOAChange(t, db, 12, "add", 12)

	got, err := NewAXFRHandler(db).HandleIXFR("example.com.", 10, "xfer-key")
	if err != nil {
		t.Fatalf("HandleIXFR: %v", err)
	}
	if len(got) != 3 || got[0].Header().Rrtype != dns.TypeSOA || got[len(got)-1].Header().Rrtype != dns.TypeSOA {
		t.Fatalf("incomplete history did not fall back to AXFR: %v", got)
	}
}

func TestHandleIXFR_ReturnsCurrentSOAForEqualOrNewerClient(t *testing.T) {
	for _, clientSerial := range []uint32{12, 13} {
		t.Run(fmt.Sprint(clientSerial), func(t *testing.T) {
			db := newSingleConnDB(t)
			seedIXFRZone(t, db, 12, "192.0.2.3")
			got, err := NewAXFRHandler(db).HandleIXFR("example.com.", clientSerial, "xfer-key")
			if err != nil {
				t.Fatalf("HandleIXFR: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("answer = %v, want current SOA only", got)
			}
			soa, ok := got[0].(*dns.SOA)
			if !ok || soa.Serial != 12 {
				t.Fatalf("answer = %v, want SOA serial 12", got[0])
			}
		})
	}
}

func TestHandleIXFR_UsesSerialArithmeticAcrossWrap(t *testing.T) {
	db := newSingleConnDB(t)
	seedIXFRZone(t, db, 1, "192.0.2.3")
	insertSOAChange(t, db, 0, "delete", ^uint32(0))
	insertRRChange(t, db, 0, "add", "192.0.2.2")
	insertSOAChange(t, db, 0, "add", 0)
	insertSOAChange(t, db, 1, "delete", 0)
	insertRRChange(t, db, 1, "delete", "192.0.2.2")
	insertRRChange(t, db, 1, "add", "192.0.2.3")
	insertSOAChange(t, db, 1, "add", 1)

	got, err := NewAXFRHandler(db).HandleIXFR("example.com.", ^uint32(0), "xfer-key")
	if err != nil {
		t.Fatalf("HandleIXFR across serial wrap: %v", err)
	}
	if len(got) != 9 || got[1].(*dns.SOA).Serial != ^uint32(0) || got[2].(*dns.SOA).Serial != 0 ||
		got[4].(*dns.SOA).Serial != 0 || got[6].(*dns.SOA).Serial != 1 {
		t.Fatalf("wrapped IXFR chain = %v", got)
	}
}

func seedIXFRZone(t *testing.T, db *sql.DB, serial uint32, address string) {
	t.Helper()
	mustExec(t, db, `INSERT INTO dns_zones
		(id, name, type, enabled, soa_mname, soa_rname, serial, refresh, retry, expire, minimum, default_ttl)
		VALUES ('z1', 'example.com.', 'primary', 1, 'ns1.example.com.', 'hostmaster.example.com.', ?, 3600, 600, 86400, 300, 3600)`, serial)
	mustExec(t, db, `INSERT INTO dns_zone_transfer (id, zone_id, allowed_cidr, tsig_key_name)
		VALUES ('t1', 'z1', '0.0.0.0/0', 'xfer-key')`)
	mustExec(t, db, `INSERT INTO dns_records (id, zone_id, name, type, ttl, value, enabled)
		VALUES ('r1', 'z1', 'www.example.com.', 'A', 300, ?, 1)`, address)
}

func insertSOAChange(t *testing.T, db *sql.DB, serial uint32, kind string, soaSerial uint32) {
	t.Helper()
	value := fmt.Sprintf("ns1.example.com. hostmaster.example.com. %d 3600 600 86400 300", soaSerial)
	mustExec(t, db, `INSERT INTO dns_zone_changes (id, zone_id, serial, change_type, name, type, value, ttl)
		VALUES (?, 'z1', ?, ?, 'example.com.', 'SOA', ?, 3600)`,
		fmt.Sprintf("soa-%d-%s", serial, kind), serial, kind, value)
}

func insertRRChange(t *testing.T, db *sql.DB, serial uint32, kind, address string) {
	t.Helper()
	mustExec(t, db, `INSERT INTO dns_zone_changes (id, zone_id, serial, change_type, name, type, value, ttl)
		VALUES (?, 'z1', ?, ?, 'www.example.com.', 'A', ?, 300)`,
		fmt.Sprintf("rr-%d-%s", serial, kind), serial, kind, address)
}
