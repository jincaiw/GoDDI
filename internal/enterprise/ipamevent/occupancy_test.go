package ipamevent

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
)

const golden = `{"changes":[{"kind":"ipam.occupancy.set","objectId":"33333333-3333-4333-8333-333333333333","revision":"1","body":{"tenantId":"11111111-1111-4111-8111-111111111111","spaceId":"22222222-2222-4222-8222-222222222222","id":"33333333-3333-4333-8333-333333333333","subnetId":"44444444-4444-4444-8444-444444444444","family":4,"address":"192.0.2.10","kind":"STATIC","ownerRef":"host-1","createdAtMs":"100","expiresAtMs":null,"revision":"1"}}]}`

func row() Occupancy {
	return Occupancy{TenantID: "11111111-1111-4111-8111-111111111111", SpaceID: "22222222-2222-4222-8222-222222222222", ID: "33333333-3333-4333-8333-333333333333", SubnetID: "44444444-4444-4444-8444-444444444444", Family: 4, Address: "192.0.2.10", Kind: "STATIC", OwnerRef: "host-1", CreatedAtMS: "100", Revision: "1"}
}

func TestGoldenAndTombstone(t *testing.T) {
	change, err := EncodeChange(Set, row())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := eventlog.MarshalChanges(change)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != golden {
		t.Fatalf("golden mismatch: %s", raw)
	}
	got, err := DecodePayload(raw)
	if err != nil || len(got) != 1 || got[0].Kind != Set || got[0].Occupancy != row() {
		t.Fatalf("decode: %+v %v", got, err)
	}
	change.Kind = Cleared
	raw, err = eventlog.MarshalChanges(change)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != strings.Replace(golden, Set, Cleared, 1) {
		t.Fatalf("tombstone golden mismatch: %s", raw)
	}
	got, err = DecodePayload(raw)
	if err != nil || got[0].Kind != Cleared || got[0].Occupancy.ID != row().ID {
		t.Fatalf("tombstone: %+v %v", got, err)
	}
}

func TestInvalidPayload(t *testing.T) {
	for name, raw := range map[string]string{
		"duplicate":         strings.Replace(golden, `"tenantId":`, `"tenantId":"11111111-1111-4111-8111-111111111111","tenantId":`, 1),
		"escaped duplicate": strings.Replace(golden, `"tenantId":`, `"tenant\u0049d":"11111111-1111-4111-8111-111111111111","tenantId":`, 1),
		"unknown":           strings.Replace(golden, `"ownerRef":`, `"extra":"x","ownerRef":`, 1),
		"case alias":        strings.Replace(golden, `"tenantId":`, `"TenantId":`, 1),
		"missing null":      strings.Replace(golden, `,"expiresAtMs":null`, "", 1),
		"null creation":     strings.Replace(golden, `"createdAtMs":"100"`, `"createdAtMs":null`, 1),
		"wrong revision":    strings.Replace(golden, `"revision":"1","body"`, `"revision":"2","body"`, 1),
		"wrong id":          strings.Replace(golden, `"objectId":"33333333-3333-4333-8333-333333333333"`, `"objectId":"55555555-5555-4555-8555-555555555555"`, 1),
		"overflow":          strings.Replace(golden, `"createdAtMs":"100"`, `"createdAtMs":"9223372036854775808"`, 1),
		"leading zero":      strings.Replace(golden, `"createdAtMs":"100"`, `"createdAtMs":"0100"`, 1),
		"mapped":            strings.Replace(golden, `"address":"192.0.2.10"`, `"address":"::ffff:192.0.2.10"`, 1),
		"family mismatch":   strings.Replace(golden, `"family":4`, `"family":6`, 1),
		"uppercase uuid":    strings.Replace(golden, `"tenantId":"11111111-1111-4111-8111-111111111111"`, `"tenantId":"AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"`, 1),
		"nil uuid":          strings.Replace(golden, `"tenantId":"11111111-1111-4111-8111-111111111111"`, `"tenantId":"00000000-0000-0000-0000-000000000000"`, 1),
		"hold null":         strings.Replace(golden, `"kind":"STATIC"`, `"kind":"HOLD"`, 1),
		"static expiry":     strings.Replace(golden, `"expiresAtMs":null`, `"expiresAtMs":"101"`, 1),
		"unknown kind":      strings.Replace(golden, `ipam.occupancy.set`, `ipam.prefix.created`, 1),
		"invalid utf8":      golden + string([]byte{0xff}),
		"bom":               "\xef\xbb\xbf" + golden,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodePayload([]byte(raw)); err == nil {
				t.Fatal("accepted invalid payload")
			}
		})
	}
}

func TestAddressAndHold(t *testing.T) {
	o := row()
	o.Family = 6
	o.Address = "2001:db8::1"
	o.Kind = "HOLD"
	expiry := "101"
	o.ExpiresAtMS = &expiry
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"2001:0db8::1", "2001:DB8::1", "fe80::1%en0", "::ffff:192.0.2.1"} {
		o.Address = ip
		if o.Validate() == nil {
			t.Fatalf("accepted %s", ip)
		}
	}
	o.Address = "2001:db8::1"
	expiry = "100"
	if o.Validate() == nil {
		t.Fatal("accepted expired at creation")
	}
}

func TestHashUsesOriginalPayload(t *testing.T) {
	cluster, _ := eventlog.ParseID("3a0cc999-92c2-44df-9cc2-06d414a60001")
	leader, _ := eventlog.ParseID("6c0a127e-4d25-42f8-8a4b-e4f1845100a1")
	e := eventlog.Event{ClusterID: cluster, LeaderID: leader, Position: eventlog.Position{Epoch: 1, Seq: 1}, Schema: 1, CommitMs: 1794100000000, Payload: []byte(golden)}
	h, err := Hash(e)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := e.Hash()
	if err != nil || h != h2 {
		t.Fatalf("eventlog digest: %x %x %v", h, h2, err)
	}
	e.Payload = bytes.Replace(e.Payload, []byte(`{"changes":`), []byte(`{ "changes":`), 1)
	h3, err := Hash(e)
	if err != nil || h3 == h {
		t.Fatalf("original bytes not hashed: %x %v", h3, err)
	}
}
