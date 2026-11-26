package dhcplease

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotrows"
)

func TestSchemaTablesReturnsTheFrozenOrderWithValidators(t *testing.T) {
	tables := SchemaTables()
	want := []string{"dhcp_lease", "dhcp_declined", "dhcp_outbox", "dhcp_receipt"}
	if len(tables) != len(want) {
		t.Fatalf("SchemaTables returned %d entries, want %d", len(tables), len(want))
	}
	for i, e := range tables {
		if e.Name != want[i] {
			t.Fatalf("entry %d = %q, want %q (frozen order)", i, e.Name, want[i])
		}
		if e.ValidateRow == nil {
			t.Fatalf("entry %d carries no row validator", i)
		}
	}
}

func TestSchemaTablesValidatorsAcceptGoodAndRejectBadRows(t *testing.T) {
	tables := SchemaTables()
	byName := map[string]snapshotrows.ValidateRow{}
	for _, e := range tables {
		byName[e.Name] = e.ValidateRow
	}
	addr := base64.RawURLEncoding.EncodeToString(v4(105))
	ck := base64.RawURLEncoding.EncodeToString([]byte("ck"))

	good := map[string]string{
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
	for name, raw := range good {
		if err := byName[name]([]byte(raw)); err != nil {
			t.Fatalf("%s: a well-formed row must pass the feed validator: %v", name, err)
		}
	}

	// An alien kind is refused through the same feed surface the real stream
	// would hit.
	bad := strings.Replace(good["dhcp_lease"], `"kind":"BOUND"`, `"kind":"NAK"`, 1)
	if err := byName["dhcp_lease"]([]byte(bad)); err == nil || !strings.Contains(err.Error(), "outside the frozen enum") {
		t.Fatalf("the feed validator must refuse an alien kind, got %v", err)
	}
}
