package dnssec

import (
	"testing"

	"github.com/miekg/dns"

	"github.com/jasonwa/goddi/internal/dns/zone"
)

// denialFixture builds a signed zone whose name chain is
// apex -> mail -> www (canonical order), with an A at mail and an A + TXT at
// www. It returns the responder, the zone name and the SOA used in responses.
func denialFixture(t *testing.T) (*Responder, string, *dns.SOA) {
	t.Helper()
	mgr, zm := setupDSSECTest(t)
	z, err := zm.CreateZone(zone.ZoneOptions{Name: "signed.example.test.", Type: string(zone.ZoneTypePrimary)})
	if err != nil {
		t.Fatalf("create zone: %v", err)
	}
	if _, err := mgr.GenerateKSK(z.ID, string(AlgECDSAP256SHA256)); err != nil {
		t.Fatalf("generate KSK: %v", err)
	}
	if _, err := mgr.GenerateZSK(z.ID, string(AlgECDSAP256SHA256)); err != nil {
		t.Fatalf("generate ZSK: %v", err)
	}
	if err := mgr.SignZone(z.ID); err != nil {
		t.Fatalf("sign zone: %v", err)
	}
	for _, rec := range []struct{ id, name, typ, value string }{
		{"r1", "mail.signed.example.test.", "A", "192.0.2.1"},
		{"r2", "www.signed.example.test.", "A", "192.0.2.2"},
		{"r3", "www.signed.example.test.", "TXT", "hello"},
	} {
		if _, err := mgr.db.Exec(`
			INSERT INTO dns_records (id, zone_id, name, type, value, ttl, enabled)
			VALUES (?, ?, ?, ?, ?, 300, 1)`, rec.id, z.ID, rec.name, rec.typ, rec.value); err != nil {
			t.Fatalf("insert record %s: %v", rec.id, err)
		}
	}
	soa := &dns.SOA{
		Hdr:    dns.RR_Header{Name: z.Name, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 300},
		Ns:     "ns1." + z.Name,
		Mbox:   "hostmaster." + z.Name,
		Serial: 1, Refresh: 3600, Retry: 600, Expire: 86400, Minttl: 300,
	}
	return NewResponder(mgr.db, "test-secret"), z.Name, soa
}

func responderZSK(t *testing.T, r *Responder, zoneName string) *dns.DNSKEY {
	t.Helper()
	keys, err := r.enabledKeys(zoneName)
	if err != nil {
		t.Fatal(err)
	}
	for i := range keys {
		if keys[i].KeyType == "ZSK" {
			return buildDNSKEYRecord(&keys[i], zoneName)
		}
	}
	t.Fatal("no ZSK")
	return nil
}

func findRRSIG(rrs []dns.RR, covered uint16) *dns.RRSIG {
	for _, rr := range rrs {
		if s, ok := rr.(*dns.RRSIG); ok && s.TypeCovered == covered {
			return s
		}
	}
	return nil
}

func TestCanonicalLessOrdersNamesRightToLeft(t *testing.T) {
	apex := "signed.example.test."
	mail := "mail." + apex
	www := "www." + apex
	zzz := "zzz." + apex
	if !canonicalLess(apex, mail) || !canonicalLess(mail, www) || !canonicalLess(www, zzz) {
		t.Fatalf("canonical order broken: apex/mail/www/zzz")
	}
	if canonicalLess(www, mail) || canonicalLess(apex, apex) {
		t.Fatal("canonical order is not strict/antisymmetric")
	}
	// A deeper name sorts after its parent.
	if !canonicalLess(mail, "sub."+mail) {
		t.Fatal("parent must sort before its descendant")
	}
}

func TestSignNegativeNXDOMAINProducesVerifiableNSEC(t *testing.T) {
	r, zoneName, soa := denialFixture(t)
	qname := "missing." + zoneName // mail < missing < www
	ns := r.SignNegative(zoneName, qname, dns.TypeA, false, []dns.RR{soa})
	zsk := responderZSK(t, r, zoneName)

	var nsecs []*dns.NSEC
	var soaSig *dns.RRSIG
	var nsecSigs []*dns.RRSIG
	for _, rr := range ns {
		switch v := rr.(type) {
		case *dns.NSEC:
			nsecs = append(nsecs, v)
		case *dns.RRSIG:
			if v.TypeCovered == dns.TypeSOA {
				soaSig = v
			} else if v.TypeCovered == dns.TypeNSEC {
				nsecSigs = append(nsecSigs, v)
			}
		}
	}
	if soaSig == nil {
		t.Fatal("no RRSIG over the SOA")
	}
	if err := soaSig.Verify(zsk, []dns.RR{soa}); err != nil {
		t.Fatalf("RRSIG over SOA does not verify: %v", err)
	}
	// NXDOMAIN needs a covering NSEC for the queried name and, when the
	// wildcard falls in a different interval, a second NSEC denying it.
	if len(nsecs) < 1 || len(nsecs) != len(nsecSigs) {
		t.Fatalf("got %d NSECs and %d NSEC RRSIGs, want one signature each", len(nsecs), len(nsecSigs))
	}
	for i, nsec := range nsecs {
		if err := nsecSigs[i].Verify(zsk, []dns.RR{nsec}); err != nil {
			t.Fatalf("RRSIG over NSEC %d does not verify: %v", i, err)
		}
	}
	covered := false
	for _, nsec := range nsecs {
		if canonicalLess(nsec.Hdr.Name, dns.CanonicalName(qname)) &&
			canonicalLess(dns.CanonicalName(qname), nsec.NextDomain) {
			covered = true
		}
	}
	if !covered {
		t.Fatalf("no NSEC covers %s", qname)
	}
}

func TestSignNegativeNODATAOmitsQueriedType(t *testing.T) {
	r, zoneName, soa := denialFixture(t)
	qname := "mail." + zoneName
	// mail has an A record; ask for AAAA -> NODATA.
	ns := r.SignNegative(zoneName, qname, dns.TypeAAAA, true, []dns.RR{soa})
	var nsec *dns.NSEC
	for _, rr := range ns {
		if v, ok := rr.(*dns.NSEC); ok {
			nsec = v
		}
	}
	if nsec == nil {
		t.Fatal("no NSEC in a NODATA answer")
	}
	if nsec.Hdr.Name != dns.CanonicalName(qname) {
		t.Errorf("NODATA NSEC owner = %q, want %q", nsec.Hdr.Name, dns.CanonicalName(qname))
	}
	for _, rt := range nsec.TypeBitMap {
		if rt == dns.TypeAAAA {
			t.Fatal("NODATA NSEC bitmap advertises the queried AAAA type")
		}
	}
	// The A type that does exist must be advertised.
	found := false
	for _, rt := range nsec.TypeBitMap {
		if rt == dns.TypeA {
			found = true
		}
	}
	if !found {
		t.Error("NODATA NSEC bitmap does not list the A type that exists")
	}
	if err := findRRSIG(ns, dns.TypeNSEC).Verify(responderZSK(t, r, zoneName), []dns.RR{nsec}); err != nil {
		t.Fatalf("RRSIG over the NODATA NSEC does not verify: %v", err)
	}
}

func TestSignNegativeIsFailClosed(t *testing.T) {
	t.Run("invalid_nsec3_parameters", func(t *testing.T) {
		r, zoneName, soa := denialFixture(t)
		// A salt that is not hex cannot produce a hash chain; the responder
		// must withhold the whole denial proof rather than emit garbage.
		if _, err := r.db.Exec(`UPDATE dns_zones SET nsec3_salt = 'zzzz' WHERE name = ?`, zoneName); err != nil {
			t.Fatal(err)
		}
		ns := []dns.RR{soa}
		if got := r.SignNegative(zoneName, "missing."+zoneName, dns.TypeA, false, ns); len(got) != len(ns) {
			t.Errorf("invalid NSEC3 parameters gained %d records, want unsigned", len(got)-len(ns))
		}
	})

	t.Run("no_zsk", func(t *testing.T) {
		r, zoneName, soa := denialFixture(t)
		if _, err := r.db.Exec(`UPDATE dns_dnssec_keys SET enabled = 0 WHERE key_type = 'ZSK'`); err != nil {
			t.Fatal(err)
		}
		ns := []dns.RR{soa}
		if got := r.SignNegative(zoneName, "missing."+zoneName, dns.TypeA, false, ns); len(got) != len(ns) {
			t.Errorf("zone without a ZSK gained %d records, want unsigned", len(got)-len(ns))
		}
	})

	t.Run("undecryptable_key", func(t *testing.T) {
		r, zoneName, soa := denialFixture(t)
		wrong := NewResponder(r.db, "wrong-secret")
		ns := []dns.RR{soa}
		if got := wrong.SignNegative(zoneName, "missing."+zoneName, dns.TypeA, false, ns); len(got) != len(ns) {
			t.Errorf("undecryptable key gained %d records, want unsigned", len(got)-len(ns))
		}
	})
}

// A qtype miss under a wildcard is NODATA for a synthesized name: the NSEC
// proof must come from the wildcard owner that produced the answer and must
// be signed, not silently omitted.
func TestSignNegativeWildcardNODATAUsesWildcardOwner(t *testing.T) {
	r, zoneName, soa := denialFixture(t)
	wildcard := "*." + zoneName
	if _, err := r.db.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, enabled)
		SELECT 'r4', id, ?, 'A', '192.0.2.9', 300, 1 FROM dns_zones WHERE name = ?`, wildcard, zoneName); err != nil {
		t.Fatal(err)
	}
	qname := "foo." + zoneName // no exact owner; matched by the wildcard
	ns := r.SignNegative(zoneName, qname, dns.TypeAAAA, true, []dns.RR{soa})
	var nsec *dns.NSEC
	for _, rr := range ns {
		if v, ok := rr.(*dns.NSEC); ok {
			nsec = v
		}
	}
	if nsec == nil {
		t.Fatal("no NSEC in wildcard NODATA answer")
	}
	if nsec.Hdr.Name != dns.CanonicalName(wildcard) {
		t.Fatalf("NSEC owner = %q, want wildcard %q", nsec.Hdr.Name, dns.CanonicalName(wildcard))
	}
	for _, rt := range nsec.TypeBitMap {
		if rt == dns.TypeAAAA {
			t.Fatal("wildcard NODATA NSEC advertises queried AAAA")
		}
	}
	if err := findRRSIG(ns, dns.TypeNSEC).Verify(responderZSK(t, r, zoneName), []dns.RR{nsec}); err != nil {
		t.Fatalf("RRSIG over wildcard NODATA NSEC does not verify: %v", err)
	}
}
