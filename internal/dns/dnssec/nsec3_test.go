package dnssec

import (
	"testing"

	"github.com/miekg/dns"
)

// nsec3Fixture builds a signed zone with NSEC3 parameters and a name chain
// apex -> mail -> www, matching denialFixture.
func nsec3Fixture(t *testing.T) (*Responder, string, *dns.SOA) {
	t.Helper()
	r, zoneName, soa := denialFixture(t)
	if _, err := r.db.Exec(`
		UPDATE dns_zones SET nsec3_iterations = 5, nsec3_salt = 'aabb', nsec3_optout = 0
		WHERE name = ?`, zoneName); err != nil {
		t.Fatalf("set NSEC3 params: %v", err)
	}
	return r, zoneName, soa
}

func nsec3RRsIn(rrs []dns.RR) ([]*dns.NSEC3, []*dns.RRSIG) {
	var records []*dns.NSEC3
	var sigs []*dns.RRSIG
	for _, rr := range rrs {
		switch v := rr.(type) {
		case *dns.NSEC3:
			records = append(records, v)
		case *dns.RRSIG:
			sigs = append(sigs, v)
		}
	}
	return records, sigs
}

func TestNSEC3ParamsAreDetected(t *testing.T) {
	r, zoneName, _ := nsec3Fixture(t)
	p, used, err := r.nsec3Params(zoneName)
	if err != nil {
		t.Fatalf("nsec3Params: %v", err)
	}
	if !used {
		t.Fatal("NSEC3 parameters were not detected")
	}
	if p.Iterations != 5 || p.Salt != "aabb" || p.OptOut {
		t.Fatalf("params = %+v", p)
	}

	// The default NSEC zone must keep using NSEC.
	plain, plainZone, _ := denialFixture(t)
	_, used, err = plain.nsec3Params(plainZone)
	if err != nil {
		t.Fatal(err)
	}
	if used {
		t.Fatal("a zone with default parameters must not be treated as NSEC3")
	}
}

func TestNSEC3NODATAProducesVerifiableProof(t *testing.T) {
	r, zoneName, soa := nsec3Fixture(t)
	qname := "mail." + zoneName // has an A record; AAAA is NODATA

	ns := r.SignNegative(zoneName, qname, dns.TypeAAAA, true, []dns.RR{soa})
	records, sigs := nsec3RRsIn(ns)
	if len(records) != 1 {
		t.Fatalf("NODATA proof has %d NSEC3 RRs, want 1", len(records))
	}
	nsec3 := records[0]
	if nsec3.Hash != dns.SHA1 || nsec3.Iterations != 5 || nsec3.Salt != "aabb" {
		t.Errorf("NSEC3 parameters on the wire = %+v", nsec3)
	}
	if got := r.nsec3ParamsForTest(zoneName).hashOf(qname); nsec3.Hdr.Name != got+"."+zoneName {
		t.Errorf("NODATA NSEC3 owner = %q, want H(qname).%s = %q", nsec3.Hdr.Name, zoneName, got+"."+zoneName)
	}
	for _, rt := range nsec3.TypeBitMap {
		if rt == dns.TypeAAAA {
			t.Fatal("NODATA NSEC3 bitmap advertises the queried AAAA type")
		}
	}
	found := false
	for _, rt := range nsec3.TypeBitMap {
		if rt == dns.TypeA {
			found = true
		}
	}
	if !found {
		t.Error("NODATA NSEC3 bitmap does not list the A type that exists")
	}

	var nsecSig, soaSig *dns.RRSIG
	for _, s := range sigs {
		switch s.TypeCovered {
		case dns.TypeNSEC3:
			nsecSig = s
		case dns.TypeSOA:
			soaSig = s
		}
	}
	if nsecSig == nil || soaSig == nil {
		t.Fatalf("missing RRSIGs: NSEC3=%v SOA=%v", nsecSig != nil, soaSig != nil)
	}
	zsk := responderZSK(t, r, zoneName)
	if err := nsecSig.Verify(zsk, []dns.RR{nsec3}); err != nil {
		t.Fatalf("RRSIG over NSEC3 does not verify: %v", err)
	}
	if err := soaSig.Verify(zsk, []dns.RR{soa}); err != nil {
		t.Fatalf("RRSIG over SOA does not verify: %v", err)
	}
}

func TestNSEC3NXDOMAINProducesVerifiableClosestEncloserProof(t *testing.T) {
	r, zoneName, soa := nsec3Fixture(t)
	qname := "missing." + zoneName // apex < mail < missing < www

	ns := r.SignNegative(zoneName, qname, dns.TypeA, false, []dns.RR{soa})
	records, sigs := nsec3RRsIn(ns)
	// Closest-encloser exact match + next-closer covering + wildcard covering;
	// some may coincide, so require at least one and no duplicates.
	if len(records) < 1 {
		t.Fatal("NXDOMAIN has no NSEC3 proof")
	}
	seen := make(map[string]struct{}, len(records))
	for _, rr := range records {
		if _, dup := seen[rr.Hdr.Name]; dup {
			t.Fatalf("duplicate NSEC3 owner %q", rr.Hdr.Name)
		}
		seen[rr.Hdr.Name] = struct{}{}
		if rr.Hash != dns.SHA1 || rr.Iterations != 5 || rr.Salt != "aabb" {
			t.Errorf("NSEC3 parameters on the wire = %+v", rr)
		}
	}
	if len(sigs) < len(records)+1 {
		t.Fatalf("got %d RRSIGs for %d NSEC3 RRs + SOA", len(sigs), len(records))
	}

	chain := r.nsec3ParamsForTest(zoneName)
	// The exact-match proof for the closest encloser must be present.
	ceHash := chain.hashOf(zoneName)
	ceFound := false
	for _, rr := range records {
		if rr.Hdr.Name == ceHash+"."+zoneName {
			ceFound = true
		}
	}
	if !ceFound {
		t.Fatal("no NSEC3 RR matches H(closest encloser)")
	}
	// At least one NSEC3 must cover H(qname).
	qHash := chain.hashOf(qname)
	covered := false
	for _, rr := range records {
		if rr.Hdr.Name != ceHash+"."+zoneName && hashIntervalContains(rr, qHash, chain.entries) {
			covered = true
		}
	}
	if !covered {
		t.Fatal("no NSEC3 RR covers H(qname)")
	}

	zsk := responderZSK(t, r, zoneName)
	for _, rr := range records {
		sig := findRRSIGForOwner(ns, rr.Hdr.Name, dns.TypeNSEC3)
		if sig == nil {
			t.Fatalf("no RRSIG for NSEC3 owner %q", rr.Hdr.Name)
		}
		if err := sig.Verify(zsk, []dns.RR{rr}); err != nil {
			t.Fatalf("RRSIG over NSEC3 %s does not verify: %v", rr.Hdr.Name, err)
		}
	}
}

func hashIntervalContains(rr *dns.NSEC3, target string, entries []nsec3Entry) bool {
	owner := ""
	for _, e := range entries {
		if e.rrName == rr.Hdr.Name {
			owner = e.hash
			break
		}
	}
	if owner == "" {
		return false
	}
	if owner == rr.NextDomain { // single-owner chain covers everything
		return true
	}
	if owner < rr.NextDomain {
		return owner < target && target < rr.NextDomain
	}
	// Wrap-around interval.
	return target > owner || target < rr.NextDomain
}

func findRRSIGForOwner(rrs []dns.RR, owner string, covered uint16) *dns.RRSIG {
	for _, rr := range rrs {
		if s, ok := rr.(*dns.RRSIG); ok && s.TypeCovered == covered {
			// RRSIG's own owner name equals the RRset owner.
			if s.Hdr.Name == owner {
				return s
			}
		}
	}
	return nil
}

// A convenience shim so tests can inspect the chain with the same parameters
// the responder used.
func (r *Responder) nsec3ParamsForTest(zoneName string) *nsec3Chain {
	p, _, err := r.nsec3Params(zoneName)
	if err != nil {
		return &nsec3Chain{zoneName: zoneName}
	}
	names, types, err := r.zoneNameMap(zoneName)
	if err != nil {
		return &nsec3Chain{zoneName: zoneName}
	}
	c, err := r.nsec3Chain(zoneName, names, types, p)
	if err != nil {
		return &nsec3Chain{zoneName: zoneName}
	}
	return c
}

func TestNSEC3InvalidParametersFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  string
	}{
		{"non_hex_salt", "nsec3_salt = 'zzzz'"},
		{"excessive_iterations", "nsec3_iterations = 5000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, zoneName, soa := nsec3Fixture(t)
			if _, err := r.db.Exec(`UPDATE dns_zones SET `+tc.set+` WHERE name = ?`, zoneName); err != nil {
				t.Fatal(err)
			}
			ns := []dns.RR{soa}
			got := r.SignNegative(zoneName, "missing."+zoneName, dns.TypeA, false, ns)
			if len(got) != len(ns) {
				t.Errorf("invalid NSEC3 params gained %d records, want unsigned", len(got)-len(ns))
			}
		})
	}
}

func TestNextCloserName(t *testing.T) {
	q := "a.b.c.signed.example.test."
	ce := "signed.example.test."
	if got := nextCloserName(q, ce); got != "c.signed.example.test." {
		t.Errorf("nextCloserName = %q, want c.signed.example.test.", got)
	}
	if got := nextCloserName("missing.signed.example.test.", ce); got != "missing.signed.example.test." {
		t.Errorf("direct child = %q, want the queried name", got)
	}
}

// dns.NSEC3 computes SaltLength/HashLength while packing. A malformed RR
// would still pass RRSIG.Verify in memory but fail on the wire, so pin the
// pack/unpack round trip explicitly.
func TestNSEC3RecordWireRoundTrip(t *testing.T) {
	r, zoneName, soa := nsec3Fixture(t)
	ns := r.SignNegative(zoneName, "mail."+zoneName, dns.TypeAAAA, true, []dns.RR{soa})
	records, _ := nsec3RRsIn(ns)
	if len(records) != 1 {
		t.Fatalf("got %d NSEC3 records", len(records))
	}
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{records[0]}
	wire, err := msg.Pack()
	if err != nil {
		t.Fatalf("pack NSEC3: %v", err)
	}
	var parsed dns.Msg
	if err := parsed.Unpack(wire); err != nil {
		t.Fatalf("unpack NSEC3: %v", err)
	}
	if len(parsed.Answer) != 1 {
		t.Fatalf("unpacked %d answers", len(parsed.Answer))
	}
	got, ok := parsed.Answer[0].(*dns.NSEC3)
	if !ok {
		t.Fatalf("unpacked answer is %T, want *dns.NSEC3", parsed.Answer[0])
	}
	if got.Hash != records[0].Hash || got.Iterations != records[0].Iterations ||
		got.Flags != records[0].Flags || got.NextDomain != records[0].NextDomain ||
		got.Salt != records[0].Salt {
		t.Fatalf("wire round trip lost fields: got %+v, want %+v", got, records[0])
	}
	if len(got.TypeBitMap) != len(records[0].TypeBitMap) {
		t.Fatalf("type bitmap length = %d, want %d", len(got.TypeBitMap), len(records[0].TypeBitMap))
	}
}

// An empty salt is written as "-" on the wire. Verify that variant too,
// using opt-out to keep the zone in NSEC3 mode with otherwise-default params.
func TestNSEC3EmptySaltWireRoundTrip(t *testing.T) {
	r, zoneName, soa := denialFixture(t)
	if _, err := r.db.Exec(`
		UPDATE dns_zones SET nsec3_iterations = 0, nsec3_salt = '', nsec3_optout = 1
		WHERE name = ?`, zoneName); err != nil {
		t.Fatal(err)
	}
	ns := r.SignNegative(zoneName, "mail."+zoneName, dns.TypeAAAA, true, []dns.RR{soa})
	records, _ := nsec3RRsIn(ns)
	if len(records) != 1 {
		t.Fatalf("got %d NSEC3 records, want 1", len(records))
	}
	if records[0].Salt != "-" || records[0].SaltLength != 0 || records[0].Flags != 1 {
		t.Fatalf("empty-salt NSEC3 fields = salt=%q len=%d flags=%d", records[0].Salt, records[0].SaltLength, records[0].Flags)
	}
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{records[0]}
	wire, err := msg.Pack()
	if err != nil {
		t.Fatalf("pack empty-salt NSEC3: %v", err)
	}
	var parsed dns.Msg
	if err := parsed.Unpack(wire); err != nil {
		t.Fatalf("unpack empty-salt NSEC3: %v", err)
	}
	got, ok := parsed.Answer[0].(*dns.NSEC3)
	if !ok {
		t.Fatalf("unpacked %T, want *dns.NSEC3", parsed.Answer[0])
	}
	// The library stores an empty salt as "" internally and renders it as
	// "-"; SaltLength==0 is the authoritative signal on the wire.
	if got.SaltLength != 0 || got.HashLength != 20 || got.NextDomain != records[0].NextDomain ||
		got.Iterations != 0 || got.Flags != 1 {
		t.Fatalf("wire round trip lost fields: %+v", got)
	}
}
