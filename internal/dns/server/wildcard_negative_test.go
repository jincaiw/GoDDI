package server

import (
	"testing"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/miekg/dns"
)

// A wildcard RRset proves the queried name exists in the zone. A qtype miss
// under that wildcard is NOERROR/NODATA, not NXDOMAIN; a resolver must not
// cache a negative answer for a name the zone publishes.
func TestWildcardQtypeMissIsNotNXDOMAIN(t *testing.T) {
	zs, db := newTestZoneStoreWithDB(t, string(zone.ZoneTypePrimary), nil)
	defer zs.Close()
	if _, err := db.Exec(`INSERT INTO dns_records
		(id, zone_id, name, type, ttl, value, enabled)
		VALUES ('r1', 'z1', '*.example.test.', 'A', 300, '192.0.2.10', 1)`); err != nil {
		t.Fatal(err)
	}
	zs.Load()

	srv := New(&config.Config{}, nil, nil, nil, nil, nil, zs)
	resp, denied, found := srv.lookupAuthoritative("www.example.test.", dns.TypeAAAA, "192.0.2.1", false)
	if denied {
		t.Fatal("wildcard qtype miss was denied")
	}
	if !found || resp == nil {
		t.Fatal("no authoritative answer for wildcard qtype miss")
	}
	if resp.Rcode == dns.RcodeNameError {
		t.Fatal("wildcard qtype miss was reported as NXDOMAIN")
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if len(resp.Answer) != 0 {
		t.Fatalf("qtype miss returned %d answers", len(resp.Answer))
	}
	if len(resp.Ns) == 0 {
		t.Fatal("qtype miss must carry the zone SOA in authority")
	}
}

// A name with no matching record and no wildcard must remain NXDOMAIN.
func TestAbsentNameWithoutWildcardIsNXDOMAIN(t *testing.T) {
	zs, db := newTestZoneStoreWithDB(t, string(zone.ZoneTypePrimary), nil)
	defer zs.Close()
	// Replace the seeded A record with a different concrete name; no wildcard exists.
	if _, err := db.Exec(`UPDATE dns_records SET name = ? WHERE id = ?`, "www.example.test.", "r1"); err != nil {
		t.Fatal(err)
	}
	zs.Load()

	srv := New(&config.Config{}, nil, nil, nil, nil, nil, zs)
	resp, denied, found := srv.lookupAuthoritative("missing.example.test.", dns.TypeA, "192.0.2.1", false)
	if denied {
		t.Fatal("absent name was denied")
	}
	if !found || resp == nil {
		t.Fatal("no authoritative response for absent name")
	}
	if resp.Rcode != dns.RcodeNameError {
		t.Fatalf("rcode = %s, want NXDOMAIN", dns.RcodeToString[resp.Rcode])
	}
}
