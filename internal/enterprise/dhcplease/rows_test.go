package dhcplease

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestValidateRowAcceptsABoundLease(t *testing.T) {
	raw := []byte(`{"tenant":"t","space":"s","address":"` + base64.RawURLEncoding.EncodeToString(v4(105)) +
		`","family":"4","lease_id":"lease-1","client_key":"` + base64.RawURLEncoding.EncodeToString([]byte("ck")) +
		`","kind":"BOUND","offer_xid":"1","issued_ms":"100","expires_ms":"900","revision":"1"}`)
	pk, err := validateRow("dhcp_lease", raw)
	if err != nil {
		t.Fatalf("a well-formed BOUND lease must validate: %v", err)
	}
	if len(pk) != 3 { // tenant, space, address
		t.Fatalf("pk has %d parts, want 3", len(pk))
	}
}

func TestValidateRowAcceptsAPermanentLease(t *testing.T) {
	raw := []byte(`{"tenant":"t","space":"s","address":"` + base64.RawURLEncoding.EncodeToString(v6(9)) +
		`","family":"6","lease_id":"lease-perm","client_key":"` + base64.RawURLEncoding.EncodeToString([]byte("pk")) +
		`","kind":"BOUND","offer_xid":"1","issued_ms":"100","expires_ms":null,"revision":"1"}`)
	if _, err := validateRow("dhcp_lease", raw); err != nil {
		t.Fatalf("a permanent lease (null expiry) must validate: %v", err)
	}
}

func TestValidateRowRejectsWrongFieldCounts(t *testing.T) {
	raw := []byte(`{"tenant":"t","space":"s","address":"x","family":"4","lease_id":"l","client_key":"Y2s","kind":"BOUND","offer_xid":"1","issued_ms":"1","expires_ms":"2","revision":"1","extra":1}`)
	if _, err := validateRow("dhcp_lease", raw); err == nil || !strings.Contains(err.Error(), "frozen column set") {
		t.Fatalf("an extra field must be refused, got %v", err)
	}
}

func TestValidateRowRejectsA16ByteAddressViolation(t *testing.T) {
	short := base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3})
	raw := []byte(`{"tenant":"t","space":"s","address":"` + short +
		`","family":"4","lease_id":"l","client_key":"Y2s","kind":"BOUND","offer_xid":"1","issued_ms":"1","expires_ms":"2","revision":"1"}`)
	if _, err := validateRow("dhcp_lease", raw); err == nil || !strings.Contains(err.Error(), "16 bytes") {
		t.Fatalf("a non-16-byte address must be refused, got %v", err)
	}
}

func TestValidateRowRejectsAnAlienKind(t *testing.T) {
	raw := []byte(`{"tenant":"t","space":"s","address":"` + base64.RawURLEncoding.EncodeToString(v4(105)) +
		`","family":"4","lease_id":"l","client_key":"Y2s","kind":"NAK","offer_xid":"1","issued_ms":"1","expires_ms":"2","revision":"1"}`)
	if _, err := validateRow("dhcp_lease", raw); err == nil || !strings.Contains(err.Error(), "outside the frozen enum") {
		t.Fatalf("an alien kind must be refused, got %v", err)
	}
}

func TestValidateRowRejectsANegativeInteger(t *testing.T) {
	raw := []byte(`{"tenant":"t","space":"s","address":"` + base64.RawURLEncoding.EncodeToString(v4(105)) +
		`","family":"4","lease_id":"l","client_key":"Y2s","kind":"BOUND","offer_xid":"-1","issued_ms":"1","expires_ms":"2","revision":"1"}`)
	if _, err := validateRow("dhcp_lease", raw); err == nil || !strings.Contains(err.Error(), "non-negative") {
		t.Fatalf("a negative integer must be refused, got %v", err)
	}
}

func TestValidateRowRejectsNULInText(t *testing.T) {
	raw := []byte(`{"tenant":"t\u0000x","space":"s","address":"` + base64.RawURLEncoding.EncodeToString(v4(105)) +
		`","family":"4","lease_id":"l","client_key":"Y2s","kind":"BOUND","offer_xid":"1","issued_ms":"1","expires_ms":"2","revision":"1"}`)
	if _, err := validateRow("dhcp_lease", raw); err == nil || !strings.Contains(err.Error(), "without NUL") {
		t.Fatalf("a NUL in TEXT must be refused, got %v", err)
	}
}

func TestValidateRowWorksForAllFourLeaseTables(t *testing.T) {
	addr := base64.RawURLEncoding.EncodeToString(v4(105))
	ck := base64.RawURLEncoding.EncodeToString([]byte("ck"))
	rows := map[string]string{
		"dhcp_lease": `{"tenant":"t","space":"s","address":"` + addr + `","family":"4","lease_id":"l1","client_key":"` + ck + `","kind":"BOUND","offer_xid":"1","issued_ms":"100","expires_ms":"900","revision":"1"}`,
		"dhcp_declined": `{"tenant":"t","space":"s","address":"` + base64.RawURLEncoding.EncodeToString(v4(106)) +
			`","family":"4","reason":"client DECLINE","declined_ms":"100","revision":"1"}`,
		"dhcp_outbox": `{"cluster_id":"` + base64.RawURLEncoding.EncodeToString([]byte("cluster")) +
			`","epoch":"1","seq":"1","hash":"` + base64.RawURLEncoding.EncodeToString([]byte("h")) +
			`","payload":"` + base64.RawURLEncoding.EncodeToString([]byte("p")) + `"}`,
		"dhcp_receipt": `{"tenant":"t","space":"s","client_key":"` + ck + `","xid":"5","kind":"OFFER","digest":"` +
			base64.RawURLEncoding.EncodeToString([]byte("dg")) + `","reply_kind":"OFFER","address":"` + addr +
			`","expires_ms":null,"revision":"1","seq":"1"}`,
	}
	for table, raw := range rows {
		if _, err := validateRow(table, []byte(raw)); err != nil {
			t.Fatalf("%s row must validate: %v", table, err)
		}
	}
}
