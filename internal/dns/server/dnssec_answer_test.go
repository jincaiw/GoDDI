package server

import (
	"path/filepath"
	"testing"

	"github.com/miekg/dns"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
	"github.com/jasonwa/goddi/internal/dns/dnssec"
	"github.com/jasonwa/goddi/internal/dns/zone"
)

const dnssecTestSecret = "server-dnssec-test-secret"

// newSignedTestServer builds a zone store over a database that has a
// dnssec_enabled zone, one A record and a real KSK+ZSK pair, then wires the
// signing responder exactly as the bootstrap does.
func newSignedTestServer(t *testing.T, signZone bool, secret string) (*Server, *database.DB, string) {
	t.Helper()
	dir := t.TempDir()
	dbh, err := database.New(config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(dir, "dns.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbh.Close() })
	if err := dbh.RunMigrations(filepath.Join("..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}

	const zoneName = "signed.example.test."
	enabled := 0
	if signZone {
		enabled = 1
	}
	if _, err := dbh.Exec(`
		INSERT INTO dns_zones (
			id, name, type, enabled, dnssec_enabled, default_ttl, soa_mname, soa_rname,
			serial, refresh, retry, expire, minimum
		) VALUES (
			'z1', ?, 'primary', 1, ?, 300, 'ns1.signed.example.test.',
			'admin.signed.example.test.', 1, 3600, 600, 86400, 300
		)`, zoneName, enabled); err != nil {
		t.Fatalf("insert zone: %v", err)
	}
	if _, err := dbh.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, enabled)
		VALUES ('r1', 'z1', 'www.signed.example.test.', 'A', '192.0.2.10', 300, 1)`); err != nil {
		t.Fatalf("insert record: %v", err)
	}

	zm := zone.NewZoneManager(dbh.DB, nil)
	mgr := dnssec.NewDNSSECManager(dbh.DB, zm, nil, dnssecTestSecret)
	if _, err := mgr.GenerateKSK("z1", string(dnssec.AlgECDSAP256SHA256)); err != nil {
		t.Fatalf("generate KSK: %v", err)
	}
	if _, err := mgr.GenerateZSK("z1", string(dnssec.AlgECDSAP256SHA256)); err != nil {
		t.Fatalf("generate ZSK: %v", err)
	}
	if signZone {
		if err := mgr.SignZone("z1"); err != nil {
			t.Fatalf("sign zone: %v", err)
		}
	}

	srv := New(&config.Config{}, nil, nil, nil, nil, nil, zone.NewStore(dbh.DB))
	srv.SetDNSSECResponder(dnssec.NewResponder(dbh.DB, secret))
	return srv, dbh, zoneName
}

// zskDNSKEY rebuilds the ZSK DNSKEY from the stored public key so the test can
// verify the RRSIG the server produced.
func zskDNSKEY(t *testing.T, dbh *database.DB, zoneName string) *dns.DNSKEY {
	t.Helper()
	var algorithm, publicKey string
	if err := dbh.QueryRow(`
		SELECT algorithm, public_key FROM dns_dnssec_keys
		WHERE zone_id = 'z1' AND key_type = 'ZSK' AND enabled = 1 LIMIT 1`,
	).Scan(&algorithm, &publicKey); err != nil {
		t.Fatalf("read ZSK: %v", err)
	}
	algo, ok := dnssec.SupportedAlgorithms[dnssec.DNSSECAlgorithm(algorithm)]
	if !ok {
		t.Fatalf("unknown algorithm %q", algorithm)
	}
	return &dns.DNSKEY{
		Hdr:       dns.RR_Header{Name: zoneName, Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
		Flags:     256,
		Protocol:  3,
		Algorithm: algo,
		PublicKey: publicKey,
	}
}

func TestAuthoritativeAnswerIsSignedWhenDOIsSet(t *testing.T) {
	srv, dbh, zoneName := newSignedTestServer(t, true, dnssecTestSecret)

	resp, denied, found := srv.lookupAuthoritative("www.signed.example.test.", dns.TypeA, "192.0.2.1", true)
	if denied || !found || resp == nil {
		t.Fatalf("lookup = (denied=%v, found=%v, resp=%v), want an authoritative answer", denied, found, resp)
	}
	var rrs []dns.RR
	var sig *dns.RRSIG
	for _, rr := range resp.Answer {
		if s, ok := rr.(*dns.RRSIG); ok {
			sig = s
		} else {
			rrs = append(rrs, rr)
		}
	}
	if len(rrs) == 0 || sig == nil {
		t.Fatalf("answer = %d RRs, sig=%v; want the A record plus an RRSIG", len(rrs), sig != nil)
	}
	if sig.TypeCovered != dns.TypeA {
		t.Errorf("RRSIG TypeCovered = %d, want A", sig.TypeCovered)
	}
	if err := sig.Verify(zskDNSKEY(t, dbh, zoneName), rrs); err != nil {
		t.Fatalf("RRSIG over the authoritative A RRset does not verify: %v", err)
	}
}

func TestAuthoritativeAnswerIsUnsignedWithoutDO(t *testing.T) {
	srv, _, _ := newSignedTestServer(t, true, dnssecTestSecret)
	resp, _, _ := srv.lookupAuthoritative("www.signed.example.test.", dns.TypeA, "192.0.2.1", false)
	if resp == nil {
		t.Fatal("no response")
	}
	for _, rr := range resp.Answer {
		if _, ok := rr.(*dns.RRSIG); ok {
			t.Fatal("RRSIG emitted without the EDNS DO bit")
		}
	}
}

func TestDNSKEYQueryReturnsVerifiableKeySet(t *testing.T) {
	srv, _, zoneName := newSignedTestServer(t, true, dnssecTestSecret)
	resp, denied, found := srv.lookupAuthoritative(zoneName, dns.TypeDNSKEY, "192.0.2.1", true)
	if denied || !found || resp == nil {
		t.Fatalf("DNSKEY lookup = (denied=%v, found=%v), want an answer", denied, found)
	}
	var keys []dns.RR
	var sig *dns.RRSIG
	for _, rr := range resp.Answer {
		switch v := rr.(type) {
		case *dns.DNSKEY:
			keys = append(keys, rr)
		case *dns.RRSIG:
			sig = v
		}
	}
	if len(keys) != 2 || sig == nil {
		t.Fatalf("DNSKEY answer = %d keys, sig=%v; want 2 keys + 1 RRSIG", len(keys), sig != nil)
	}
	var ksk *dns.DNSKEY
	for _, rr := range keys {
		if k, ok := rr.(*dns.DNSKEY); ok && k.KeyTag() == sig.KeyTag {
			ksk = k
		}
	}
	if ksk == nil || ksk.Flags != 257 {
		t.Fatalf("RRSIG was not made with the KSK (flags=%v)", ksk)
	}
	if err := sig.Verify(ksk, keys); err != nil {
		t.Fatalf("RRSIG over the DNSKEY RRset does not verify: %v", err)
	}
}

func TestDNSKEYQueryWithoutDOOmitsRRSIG(t *testing.T) {
	srv, _, zoneName := newSignedTestServer(t, true, dnssecTestSecret)
	resp, _, _ := srv.lookupAuthoritative(zoneName, dns.TypeDNSKEY, "192.0.2.1", false)
	if resp == nil {
		t.Fatal("no response")
	}
	keys := 0
	for _, rr := range resp.Answer {
		if _, ok := rr.(*dns.DNSKEY); ok {
			keys++
		}
		if _, ok := rr.(*dns.RRSIG); ok {
			t.Fatal("RRSIG emitted without the EDNS DO bit")
		}
	}
	if keys != 2 {
		t.Errorf("DNSKEY answer has %d keys without DO, want 2", keys)
	}
}

func TestUnsignedAndUnusableRespondersStayUnsigned(t *testing.T) {
	// A zone not marked dnssec_enabled must never gain signatures.
	srv, _, _ := newSignedTestServer(t, false, dnssecTestSecret)
	resp, _, _ := srv.lookupAuthoritative("www.signed.example.test.", dns.TypeA, "192.0.2.1", true)
	if resp == nil {
		t.Fatal("no response")
	}
	for _, rr := range resp.Answer {
		if _, ok := rr.(*dns.RRSIG); ok {
			t.Fatal("unsigned zone gained an RRSIG")
		}
	}

	// A responder with the wrong secret cannot decrypt the private key; it
	// must fail closed rather than emit a partial answer.
	srv, _, _ = newSignedTestServer(t, true, "wrong-secret")
	resp, _, _ = srv.lookupAuthoritative("www.signed.example.test.", dns.TypeA, "192.0.2.1", true)
	if resp == nil {
		t.Fatal("no response")
	}
	for _, rr := range resp.Answer {
		if _, ok := rr.(*dns.RRSIG); ok {
			t.Fatal("undecryptable key produced an RRSIG")
		}
	}
}

func TestWithoutResponderAnswersAreAlwaysPlain(t *testing.T) {
	srv, _, _ := newSignedTestServer(t, true, dnssecTestSecret)
	srv.SetDNSSECResponder(nil)
	resp, _, _ := srv.lookupAuthoritative("www.signed.example.test.", dns.TypeA, "192.0.2.1", true)
	if resp == nil {
		t.Fatal("no response")
	}
	for _, rr := range resp.Answer {
		if _, ok := rr.(*dns.RRSIG); ok {
			t.Fatal("RRSIG emitted with no responder attached (gate off)")
		}
	}
}

func serverFindRRSIG(rrs []dns.RR, covered uint16) *dns.RRSIG {
	for _, rr := range rrs {
		if s, ok := rr.(*dns.RRSIG); ok && s.TypeCovered == covered {
			return s
		}
	}
	return nil
}

func TestSignedNXDOMAINCarriesVerifiableDenialProof(t *testing.T) {
	srv, dbh, zoneName := newSignedTestServer(t, true, dnssecTestSecret)
	qname := "missing." + zoneName

	resp, denied, found := srv.lookupAuthoritative(qname, dns.TypeA, "192.0.2.1", true)
	if denied || !found || resp == nil {
		t.Fatalf("NXDOMAIN lookup = (denied=%v, found=%v), want an authoritative response", denied, found)
	}
	if resp.Rcode != dns.RcodeNameError {
		t.Errorf("Rcode = %d, want NXDOMAIN", resp.Rcode)
	}
	var nsec *dns.NSEC
	var soa *dns.SOA
	for _, rr := range resp.Ns {
		switch v := rr.(type) {
		case *dns.NSEC:
			nsec = v
		case *dns.SOA:
			soa = v
		}
	}
	if nsec == nil || soa == nil {
		t.Fatalf("authority section lacks NSEC/SOA: nsec=%v soa=%v", nsec != nil, soa != nil)
	}
	soaSig := serverFindRRSIG(resp.Ns, dns.TypeSOA)
	nsecSig := serverFindRRSIG(resp.Ns, dns.TypeNSEC)
	if soaSig == nil || nsecSig == nil {
		t.Fatalf("authority section lacks RRSIGs: soa=%v nsec=%v", soaSig != nil, nsecSig != nil)
	}
	zsk := zskDNSKEY(t, dbh, zoneName)
	if err := soaSig.Verify(zsk, []dns.RR{soa}); err != nil {
		t.Fatalf("RRSIG over SOA does not verify: %v", err)
	}
	if err := nsecSig.Verify(zsk, []dns.RR{nsec}); err != nil {
		t.Fatalf("RRSIG over NSEC does not verify: %v", err)
	}
	if len(resp.Answer) != 0 {
		t.Errorf("NXDOMAIN answer section has %d records, want 0", len(resp.Answer))
	}
}

func TestSignedNODATACarriesVerifiableDenialProof(t *testing.T) {
	srv, dbh, zoneName := newSignedTestServer(t, true, dnssecTestSecret)
	qname := "www." + zoneName

	// www has an A record; asking for AAAA is a NODATA response.
	resp, denied, found := srv.lookupAuthoritative(qname, dns.TypeAAAA, "192.0.2.1", true)
	if denied || !found || resp == nil {
		t.Fatalf("NODATA lookup = (denied=%v, found=%v)", denied, found)
	}
	if resp.Rcode == dns.RcodeNameError {
		t.Error("NODATA must not be NXDOMAIN")
	}
	var nsec *dns.NSEC
	for _, rr := range resp.Ns {
		if v, ok := rr.(*dns.NSEC); ok {
			nsec = v
		}
	}
	if nsec == nil {
		t.Fatal("NODATA authority section has no NSEC")
	}
	for _, rt := range nsec.TypeBitMap {
		if rt == dns.TypeAAAA {
			t.Fatal("NODATA NSEC advertises the queried AAAA type")
		}
	}
	nsecSig := serverFindRRSIG(resp.Ns, dns.TypeNSEC)
	if nsecSig == nil {
		t.Fatal("NODATA authority section has no RRSIG over NSEC")
	}
	if err := nsecSig.Verify(zskDNSKEY(t, dbh, zoneName), []dns.RR{nsec}); err != nil {
		t.Fatalf("RRSIG over NODATA NSEC does not verify: %v", err)
	}
}

func TestNegativeAnswerUnsignedWithoutDO(t *testing.T) {
	srv, _, zoneName := newSignedTestServer(t, true, dnssecTestSecret)
	resp, _, _ := srv.lookupAuthoritative("missing."+zoneName, dns.TypeA, "192.0.2.1", false)
	if resp == nil {
		t.Fatal("no response")
	}
	for _, rr := range resp.Ns {
		if _, ok := rr.(*dns.RRSIG); ok {
			t.Fatal("RRSIG emitted in a negative answer without the DO bit")
		}
		if _, ok := rr.(*dns.NSEC); ok {
			t.Fatal("NSEC emitted without the DO bit")
		}
	}
}

func TestUnsignedZoneNegativeAnswerHasNoDenialProof(t *testing.T) {
	srv, _, zoneName := newSignedTestServer(t, false, dnssecTestSecret)
	resp, _, _ := srv.lookupAuthoritative("missing."+zoneName, dns.TypeA, "192.0.2.1", true)
	if resp == nil {
		t.Fatal("no response")
	}
	for _, rr := range resp.Ns {
		if _, ok := rr.(*dns.NSEC); ok {
			t.Fatal("unsigned zone gained an NSEC")
		}
		if _, ok := rr.(*dns.RRSIG); ok {
			t.Fatal("unsigned zone gained an RRSIG")
		}
	}
}
