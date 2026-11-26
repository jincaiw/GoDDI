package dataplane

import (
	"context"
	"testing"

	"github.com/miekg/dns"

	"github.com/jincaiw/GoDDI/internal/dns/dnssec"
	"github.com/jincaiw/GoDDI/internal/dns/zone"
)

// A DNS data plane mints RRSIGs on the query path without ever opening the
// control database. The encrypted DNSSEC keys therefore have to be replicated
// into the zone store. This test closes the loop: keys generated on control,
// synced down, then used to produce verifiable signatures from the store alone.
func TestDNSSECKeysAreReplicatedAndSignFromTheStore(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()
	const secret = "replication-test-secret"

	zm := zone.NewZoneManager(control, nil)
	z, err := zm.CreateZone(zone.ZoneOptions{Name: "signed.example.test.", Type: string(zone.ZoneTypePrimary)})
	if err != nil {
		t.Fatalf("create zone: %v", err)
	}
	mgr := dnssec.NewDNSSECManager(control, zm, nil, secret)
	if _, err := mgr.GenerateKSK(z.ID, string(dnssec.AlgECDSAP256SHA256)); err != nil {
		t.Fatalf("generate KSK: %v", err)
	}
	if _, err := mgr.GenerateZSK(z.ID, string(dnssec.AlgECDSAP256SHA256)); err != nil {
		t.Fatalf("generate ZSK: %v", err)
	}
	if err := mgr.SignZone(z.ID); err != nil {
		t.Fatalf("sign zone: %v", err)
	}

	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("sync dns domain: %v", err)
	}

	var count int
	if err := store.QueryRow(`SELECT COUNT(*) FROM dns_dnssec_keys`).Scan(&count); err != nil {
		t.Fatalf("the data-plane store has no dns_dnssec_keys table: %v", err)
	}
	if count != 2 {
		t.Fatalf("replicated DNSSEC keys = %d, want 2", count)
	}

	// Build the responder over the data-plane DB only. If it can sign, the
	// data plane no longer needs control to be up.
	responder := dnssec.NewResponder(store.DB, secret)
	if !responder.SignedZone(z.Name) {
		t.Fatal("SignedZone = false on the data-plane copy")
	}

	dnskeyRRs, err := responder.DNSKEYRRset(z.Name)
	if err != nil {
		t.Fatalf("DNSKEYRRset on the data-plane copy: %v", err)
	}
	var dnskeys []dns.RR
	var sig *dns.RRSIG
	for _, rr := range dnskeyRRs {
		switch v := rr.(type) {
		case *dns.DNSKEY:
			dnskeys = append(dnskeys, rr)
		case *dns.RRSIG:
			sig = v
		}
	}
	if len(dnskeys) != 2 || sig == nil {
		t.Fatalf("DNSKEY answer = %d keys, sig=%v; want 2 + 1", len(dnskeys), sig != nil)
	}
	var ksk *dns.DNSKEY
	for _, rr := range dnskeys {
		if k, ok := rr.(*dns.DNSKEY); ok && k.KeyTag() == sig.KeyTag {
			ksk = k
		}
	}
	if ksk == nil {
		t.Fatal("no DNSKEY matches the RRSIG key tag")
	}
	if err := sig.Verify(ksk, dnskeys); err != nil {
		t.Fatalf("RRSIG produced by the data-plane copy does not verify: %v", err)
	}

	// Positive answers can be signed too.
	answer := []dns.RR{
		&dns.A{Hdr: dns.RR_Header{Name: "www." + z.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: []byte{192, 0, 2, 1}},
	}
	signed := responder.SignRRsets(z.Name, answer)
	if len(signed) != 2 {
		t.Fatalf("signed answer = %d records, want 2", len(signed))
	}
}

// Disabling a key on control must invalidate the data-plane copy. A retired
// key that keeps signing is a key-rotation failure, so the revision triggers
// are load-bearing.
func TestDNSSECKeyChangesInvalidateTheDataPlaneCopy(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()
	const secret = "replication-test-secret"

	zm := zone.NewZoneManager(control, nil)
	z, err := zm.CreateZone(zone.ZoneOptions{Name: "rotate.example.test.", Type: string(zone.ZoneTypePrimary)})
	if err != nil {
		t.Fatalf("create zone: %v", err)
	}
	mgr := dnssec.NewDNSSECManager(control, zm, nil, secret)
	if _, err := mgr.GenerateKSK(z.ID, string(dnssec.AlgED25519)); err != nil {
		t.Fatalf("generate KSK: %v", err)
	}
	if _, err := mgr.GenerateZSK(z.ID, string(dnssec.AlgED25519)); err != nil {
		t.Fatalf("generate ZSK: %v", err)
	}
	if err := mgr.SignZone(z.ID); err != nil {
		t.Fatalf("sign zone: %v", err)
	}
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	var before int64
	if err := control.QueryRow(`SELECT revision FROM dataplane_revision WHERE domain = 'dns'`).Scan(&before); err != nil {
		t.Fatalf("read dns revision: %v", err)
	}

	// Disable the ZSK. The data plane must stop signing altogether rather
	// than keep using a retired key.
	if _, err := control.Exec(`UPDATE dns_dnssec_keys SET enabled = 0 WHERE zone_id = ? AND key_type = 'ZSK'`, z.ID); err != nil {
		t.Fatalf("disable ZSK: %v", err)
	}
	var after int64
	if err := control.QueryRow(`SELECT revision FROM dataplane_revision WHERE domain = 'dns'`).Scan(&after); err != nil {
		t.Fatalf("read revision after key change: %v", err)
	}
	if after <= before {
		t.Fatalf("key change did not bump the dns revision: before=%d after=%d", before, after)
	}
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("resync: %v", err)
	}

	var enabled int
	if err := store.QueryRow(`SELECT enabled FROM dns_dnssec_keys WHERE zone_id = ? AND key_type = 'ZSK'`, z.ID).Scan(&enabled); err != nil {
		t.Fatalf("read replicated ZSK: %v", err)
	}
	if enabled != 0 {
		t.Errorf("replicated ZSK enabled = %d, want 0", enabled)
	}
	responder := dnssec.NewResponder(store.DB, secret)
	if responder.SignedZone(z.Name) {
		t.Error("SignedZone = true after the ZSK was retired")
	}
	rrs := []dns.RR{
		&dns.A{Hdr: dns.RR_Header{Name: "www." + z.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: []byte{192, 0, 2, 1}},
	}
	if got := responder.SignRRsets(z.Name, rrs); len(got) != len(rrs) {
		t.Errorf("retired key still produced signatures: %d records", len(got))
	}

	// Deleting all keys must remove them from the store as well.
	if _, err := control.Exec(`DELETE FROM dns_dnssec_keys WHERE zone_id = ?`, z.ID); err != nil {
		t.Fatalf("delete keys: %v", err)
	}
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("resync after delete: %v", err)
	}
	var remaining int
	if err := store.QueryRow(`SELECT COUNT(*) FROM dns_dnssec_keys`).Scan(&remaining); err != nil {
		t.Fatalf("count keys: %v", err)
	}
	if remaining != 0 {
		t.Errorf("deleted keys survived the sync: %d", remaining)
	}
}

func TestDNSSECKeyProjectionColumnsMatchControl(t *testing.T) {
	control, store, _ := newZonePair(t)
	var projection *table
	tables := TablesFor(DomainDNS)
	for i := range tables {
		if tables[i].name == "dns_dnssec_keys" {
			projection = &tables[i]
			break
		}
	}
	if projection == nil {
		t.Fatal("dns_dnssec_keys is not in the DNS replication domain")
	}
	for _, col := range projection.columns {
		var name string
		if err := control.QueryRow(
			`SELECT name FROM pragma_table_info('dns_dnssec_keys') WHERE name = ?`, col,
		).Scan(&name); err != nil {
			t.Errorf("control dns_dnssec_keys has no column %q: %v", col, err)
		}
		if err := store.QueryRow(
			`SELECT name FROM pragma_table_info('dns_dnssec_keys') WHERE name = ?`, col,
		).Scan(&name); err != nil {
			t.Errorf("data-plane dns_dnssec_keys has no column %q: %v", col, err)
		}
	}
}
