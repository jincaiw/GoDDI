package dnssec

import (
	"testing"

	"github.com/miekg/dns"

	"github.com/jasonwa/goddi/internal/dns/zone"
)

// responderFixture builds a signed primary zone with a KSK and ZSK and
// returns a responder over the same database.
func responderFixture(t *testing.T, algorithm DNSSECAlgorithm) (*Responder, *DNSSECManager, zone.Zone) {
	t.Helper()
	mgr, zm := setupDSSECTest(t)
	z, err := zm.CreateZone(zone.ZoneOptions{Name: "signed.example.com", Type: string(zone.ZoneTypePrimary)})
	if err != nil {
		t.Fatalf("create zone: %v", err)
	}
	if _, err := mgr.GenerateKSK(z.ID, string(algorithm)); err != nil {
		t.Fatalf("generate KSK: %v", err)
	}
	if _, err := mgr.GenerateZSK(z.ID, string(algorithm)); err != nil {
		t.Fatalf("generate ZSK: %v", err)
	}
	if err := mgr.SignZone(z.ID); err != nil {
		t.Fatalf("sign zone: %v", err)
	}
	return NewResponder(mgr.db, "test-secret"), mgr, *z
}

// dnskeysIn extracts DNSKEY RRs and the RRSIG covering them.
func dnskeysIn(rrs []dns.RR) ([]dns.RR, *dns.RRSIG) {
	var rrset []dns.RR
	var sig *dns.RRSIG
	for _, rr := range rrs {
		switch v := rr.(type) {
		case *dns.DNSKEY:
			rrset = append(rrset, rr)
		case *dns.RRSIG:
			sig = v
		}
	}
	return rrset, sig
}

func findDNSKEY(rrs []dns.RR, tag uint16) *dns.DNSKEY {
	for _, rr := range rrs {
		if k, ok := rr.(*dns.DNSKEY); ok && k.KeyTag() == tag {
			return k
		}
	}
	return nil
}

func TestResponderDNSKEYRRsetIsVerifiable(t *testing.T) {
	r, _, z := responderFixture(t, AlgECDSAP256SHA256)
	if !r.SignedZone(z.Name) {
		t.Fatal("SignedZone = false for a zone with active KSK+ZSK")
	}

	rrs, err := r.DNSKEYRRset(z.Name)
	if err != nil {
		t.Fatalf("DNSKEYRRset: %v", err)
	}
	rrset, sig := dnskeysIn(rrs)
	if len(rrset) != 2 || sig == nil {
		t.Fatalf("got %d DNSKEYs and sig=%v, want 2 DNSKEYs + 1 RRSIG", len(rrset), sig != nil)
	}
	if sig.TypeCovered != dns.TypeDNSKEY {
		t.Errorf("RRSIG TypeCovered = %d, want DNSKEY", sig.TypeCovered)
	}
	ksk := findDNSKEY(rrset, sig.KeyTag)
	if ksk == nil {
		t.Fatalf("RRSIG key tag %d has no matching DNSKEY in the RRset", sig.KeyTag)
	}
	if ksk.Flags != 257 {
		t.Errorf("DNSKEY signing the DNSKEY RRset has flags %d, want 257 (KSK)", ksk.Flags)
	}
	if err := sig.Verify(ksk, rrset); err != nil {
		t.Fatalf("RRSIG over DNSKEY RRset does not verify: %v", err)
	}
	if sig.SignerName != dns.Fqdn(z.Name) {
		t.Errorf("SignerName = %q, want %q", sig.SignerName, dns.Fqdn(z.Name))
	}
}

func TestResponderSignRRsetsIsVerifiablePerRRset(t *testing.T) {
	r, _, z := responderFixture(t, AlgECDSAP256SHA256)

	// Two A records form one RRset; the TXT forms a second. A single RRSIG
	// over all three would not validate, so the responder must group them.
	rrs := []dns.RR{
		&dns.A{Hdr: dns.RR_Header{Name: "www." + z.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: []byte{192, 0, 2, 1}},
		&dns.A{Hdr: dns.RR_Header{Name: "www." + z.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: []byte{192, 0, 2, 2}},
		&dns.TXT{Hdr: dns.RR_Header{Name: "www." + z.Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 300}, Txt: []string{"hello"}},
	}
	signed := r.SignRRsets(z.Name, rrs)
	if len(signed) != len(rrs)+2 {
		t.Fatalf("signed answer has %d records, want %d (3 RRs + 2 RRSIGs)", len(signed), len(rrs)+2)
	}

	// Rebuild the ZSK DNSKEY to verify with.
	keys, err := r.enabledKeys(z.Name)
	if err != nil {
		t.Fatal(err)
	}
	var zsk *dns.DNSKEY
	for i := range keys {
		if keys[i].KeyType == "ZSK" {
			zsk = buildDNSKEYRecord(&keys[i], z.Name)
		}
	}
	if zsk == nil {
		t.Fatal("no ZSK DNSKEY")
	}

	verified := 0
	for _, rr := range signed {
		sig, ok := rr.(*dns.RRSIG)
		if !ok {
			continue
		}
		var covered []dns.RR
		for _, c := range rrs {
			if c.Header().Rrtype == sig.TypeCovered {
				covered = append(covered, c)
			}
		}
		if len(covered) == 0 {
			t.Errorf("RRSIG covers type %d with no matching RRs", sig.TypeCovered)
			continue
		}
		if sig.KeyTag != zsk.KeyTag() {
			t.Errorf("RRSIG key tag %d != ZSK tag %d", sig.KeyTag, zsk.KeyTag())
		}
		if err := sig.Verify(zsk, covered); err != nil {
			t.Errorf("RRSIG over type %d does not verify: %v", sig.TypeCovered, err)
			continue
		}
		verified++
	}
	if verified != 2 {
		t.Errorf("verified %d RRSIGs, want 2", verified)
	}
}

func TestResponderSignatureRejectsTampering(t *testing.T) {
	r, _, z := responderFixture(t, AlgED25519)
	rrs := []dns.RR{
		&dns.A{Hdr: dns.RR_Header{Name: "host." + z.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: []byte{192, 0, 2, 9}},
	}
	signed := r.SignRRsets(z.Name, rrs)
	rrset, sig := dnskeysIn(signed)
	_ = rrset
	if sig == nil {
		// dnskeysIn only collects DNSKEYs; find the RRSIG directly.
		for _, rr := range signed {
			if s, ok := rr.(*dns.RRSIG); ok {
				sig = s
			}
		}
	}
	if sig == nil {
		t.Fatal("no RRSIG produced")
	}
	keys, _ := r.enabledKeys(z.Name)
	var zsk *dns.DNSKEY
	for i := range keys {
		if keys[i].KeyType == "ZSK" {
			zsk = buildDNSKEYRecord(&keys[i], z.Name)
		}
	}
	tampered := []dns.RR{
		&dns.A{Hdr: dns.RR_Header{Name: "host." + z.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: []byte{192, 0, 2, 10}},
	}
	if err := sig.Verify(zsk, tampered); err == nil {
		t.Error("RRSIG verified over a tampered RRset")
	}
	if err := sig.Verify(zsk, rrs); err != nil {
		t.Errorf("RRSIG failed over the original RRset: %v", err)
	}
}

func TestResponderIsFailClosedWithoutUsableKeys(t *testing.T) {
	mgr, zm := setupDSSECTest(t)
	z, err := zm.CreateZone(zone.ZoneOptions{Name: "unsigned.example.com", Type: string(zone.ZoneTypePrimary)})
	if err != nil {
		t.Fatal(err)
	}
	r := NewResponder(mgr.db, "test-secret")
	rrs := []dns.RR{
		&dns.A{Hdr: dns.RR_Header{Name: "host." + z.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: []byte{192, 0, 2, 1}},
	}

	if r.SignedZone(z.Name) {
		t.Error("SignedZone = true for an unsigned zone")
	}
	if _, err := r.DNSKEYRRset(z.Name); err == nil {
		t.Error("DNSKEYRRset succeeded for an unsigned zone")
	}
	if got := r.SignRRsets(z.Name, rrs); len(got) != len(rrs) {
		t.Errorf("unsigned zone gained %d records", len(got)-len(rrs))
	}

	// A zone flagged dnssec_enabled but with no keys must also stay unsigned.
	if _, err := mgr.db.Exec(`UPDATE dns_zones SET dnssec_enabled = 1 WHERE id = ?`, z.ID); err != nil {
		t.Fatal(err)
	}
	if r.SignedZone(z.Name) {
		t.Error("SignedZone = true with no keys")
	}
	if got := r.SignRRsets(z.Name, rrs); len(got) != len(rrs) {
		t.Errorf("keyless zone gained %d records", len(got)-len(rrs))
	}
}

func TestResponderFailClosedOnUndecryptableKey(t *testing.T) {
	r, mgr, z := responderFixture(t, AlgECDSAP256SHA256)
	rrs := []dns.RR{
		&dns.A{Hdr: dns.RR_Header{Name: "host." + z.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: []byte{192, 0, 2, 1}},
	}
	// A responder with the wrong secret cannot decrypt the private key. It
	// must return the answer untouched rather than a half-signed one.
	wrong := NewResponder(mgr.db, "wrong-secret")
	if got := wrong.SignRRsets(z.Name, rrs); len(got) != len(rrs) {
		t.Errorf("wrong-secret responder added %d records", len(got)-len(rrs))
	}
	if _, err := wrong.DNSKEYRRset(z.Name); err == nil {
		t.Error("DNSKEYRRset succeeded with an undecryptable KSK")
	}

	// Corrupt the ZSK ciphertext in place. Some keys still sign, so fail
	// closed must fall back to the entire original answer.
	if _, err := mgr.db.Exec(`UPDATE dns_dnssec_keys SET private_key_ciphertext = 'not-base64!!' WHERE zone_id = ? AND key_type = 'ZSK'`, z.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.SignRRsets(z.Name, rrs); len(got) != len(rrs) {
		t.Errorf("corrupted ZSK produced %d records, want the original %d", len(got), len(rrs))
	}
}

func TestResponderSupportsEveryAlgorithm(t *testing.T) {
	for _, algorithm := range []DNSSECAlgorithm{
		AlgECDSAP256SHA256, AlgECDSAP384SHA384, AlgED25519, AlgRSASHA256, AlgRSASHA512,
	} {
		t.Run(string(algorithm), func(t *testing.T) {
			r, _, z := responderFixture(t, algorithm)
			rrs, err := r.DNSKEYRRset(z.Name)
			if err != nil {
				t.Fatalf("DNSKEYRRset: %v", err)
			}
			rrset, sig := dnskeysIn(rrs)
			ksk := findDNSKEY(rrset, sig.KeyTag)
			if ksk == nil {
				t.Fatal("no DNSKEY matches the RRSIG key tag")
			}
			if err := sig.Verify(ksk, rrset); err != nil {
				t.Fatalf("RRSIG does not verify: %v", err)
			}
			answer := []dns.RR{
				&dns.A{Hdr: dns.RR_Header{Name: "host." + z.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: []byte{192, 0, 2, 1}},
			}
			if got := r.SignRRsets(z.Name, answer); len(got) != 2 {
				t.Fatalf("signed answer = %d records, want 2", len(got))
			}
		})
	}
}
