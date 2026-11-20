package staticbundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/enterprise/auditwire"
	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/idempotency"
	"github.com/jasonwa/goddi/internal/enterprise/ipamevent"
	"github.com/jasonwa/goddi/internal/enterprise/ipamrequest"
	"github.com/jasonwa/goddi/internal/enterprise/ipamresponse"
	"github.com/jasonwa/goddi/internal/enterprise/receiptwire"
)

const tenant = "11111111-1111-4111-8111-111111111111"
const space = "22222222-2222-4222-8222-222222222222"
const id = "33333333-3333-4333-8333-333333333333"
const subnet = "44444444-4444-4444-8444-444444444444"
const reqID = "55555555-5555-4555-8555-555555555555"
const keyID = "66666666-6666-4666-8666-666666666666"
const cluster = "77777777-7777-4777-8777-777777777777"
const leader = "88888888-8888-4888-8888-888888888888"

func fixture(t *testing.T) (eventlog.Event, []eventlog.Change) {
	t.Helper()
	o := ipamevent.Occupancy{TenantID: tenant, SpaceID: space, ID: id, SubnetID: subnet, Family: 4, Address: "192.0.2.10", Kind: "STATIC", OwnerRef: "host-1", CreatedAtMS: "100", Revision: "1"}
	oc, err := ipamevent.EncodeChange(ipamevent.Set, o)
	if err != nil {
		t.Fatal(err)
	}
	_, digest, err := ipamrequest.Normalize(ipamrequest.Request{TenantID: tenant, SpaceID: space, SubnetID: subnet, Address: o.Address, OwnerRef: o.OwnerRef})
	if err != nil {
		t.Fatal(err)
	}
	a, err := auditwire.Encode(auditwire.Audit{ClusterID: cluster, TenantID: tenant, SpaceID: space, ResourceID: id, Principal: "operator", RequestID: reqID, Epoch: 1, OccurredAtMS: 100, RequestDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	response, err := ipamresponse.Static(id, subnet, o.Address, o.OwnerRef)
	if err != nil {
		t.Fatal(err)
	}
	u := uuid.MustParse(tenant)
	r, err := receiptwire.Encode(receiptwire.Value{Scope: idempotency.Scope{Tenant: [16]byte(u), Principal: "operator", Method: "POST", Path: "/api/v1/ddi/tenants/" + tenant + "/ipam/spaces/" + space + "/allocations", Key: keyID}, RequestDigest: digest, Receipt: idempotency.Receipt{Response: response, CreatedAtMS: 100, ExpiresAtMS: 604800100}})
	if err != nil {
		t.Fatal(err)
	}
	c := []eventlog.Change{oc, {Kind: "ipam.occupancy.audit", ObjectID: reqID, Revision: "1", Body: a}, {Kind: "idempotency.receipt.set", ObjectID: keyID, Revision: "1", Body: r}}
	cl, _ := eventlog.ParseID(cluster)
	le, _ := eventlog.ParseID(leader)
	e := eventlog.Event{ClusterID: cl, LeaderID: le, Position: eventlog.Position{Epoch: 1, Seq: 1}, Schema: 1, CommitMs: 100}
	return rehash(t, e, c), c
}
func rehash(t *testing.T, e eventlog.Event, c []eventlog.Change) eventlog.Event {
	t.Helper()
	var err error
	e.Payload, err = eventlog.MarshalChanges(c...)
	if err != nil {
		t.Fatal(err)
	}
	e.Position.Hash, err = e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestValidAndImmutable(t *testing.T) {
	e, _ := fixture(t)
	if fmt.Sprintf("%x", e.Position.Hash) != "024ce872a7b0761af96d4255ec8a791b08ae6f36fe228c633300417efc4869bb" {
		t.Fatal("golden event hash changed")
	}
	result, err := Validate(e)
	if err != nil {
		t.Fatal(err)
	}
	if result.Occupancy().ID != id || result.Audit().RequestID != reqID || result.Receipt().Scope.Key != keyID {
		t.Fatal("incorrect result")
	}
	original := result.Event().Payload
	e.Payload[0] ^= 1
	copyEvent := result.Event()
	copyEvent.Payload[0] ^= 1
	copyReceipt := result.Receipt()
	copyReceipt.Receipt.Response.Body[0] ^= 1
	if !bytes.Equal(original, result.Event().Payload) || result.Receipt().Receipt.Response.Body[0] != '{' {
		t.Fatal("result aliases external bytes")
	}
}

func TestRehashedCrossFieldTampering(t *testing.T) {
	cases := map[string]func([]eventlog.Change){
		"order":          func(c []eventlog.Change) { c[0], c[1] = c[1], c[0] },
		"audit object":   func(c []eventlog.Change) { c[1].ObjectID = keyID },
		"receipt object": func(c []eventlog.Change) { c[2].ObjectID = reqID },
		"revision":       func(c []eventlog.Change) { c[2].Revision = "2" },
		"occupancy id":   func(c []eventlog.Change) { c[0].ObjectID = keyID },
		"audit principal": func(c []eventlog.Change) {
			a, _ := auditwire.Decode(c[1].Body)
			a.Principal = "other"
			c[1].Body, _ = auditwire.Encode(a)
		},
		"audit tenant": func(c []eventlog.Change) {
			a, _ := auditwire.Decode(c[1].Body)
			a.TenantID = space
			c[1].Body, _ = auditwire.Encode(a)
		},
		"audit space": func(c []eventlog.Change) {
			a, _ := auditwire.Decode(c[1].Body)
			a.SpaceID = tenant
			c[1].Body, _ = auditwire.Encode(a)
		},
		"audit cluster": func(c []eventlog.Change) {
			a, _ := auditwire.Decode(c[1].Body)
			a.ClusterID = leader
			c[1].Body, _ = auditwire.Encode(a)
		},
		"audit epoch": func(c []eventlog.Change) {
			a, _ := auditwire.Decode(c[1].Body)
			a.Epoch = 2
			c[1].Body, _ = auditwire.Encode(a)
		},
		"audit resource": func(c []eventlog.Change) {
			a, _ := auditwire.Decode(c[1].Body)
			a.ResourceID = keyID
			c[1].Body, _ = auditwire.Encode(a)
		},
		"audit digest": func(c []eventlog.Change) {
			a, _ := auditwire.Decode(c[1].Body)
			a.RequestDigest[0] ^= 1
			c[1].Body, _ = auditwire.Encode(a)
		},
		"audit time": func(c []eventlog.Change) {
			a, _ := auditwire.Decode(c[1].Body)
			a.OccurredAtMS++
			c[1].Body, _ = auditwire.Encode(a)
		},
		"receipt tenant": func(c []eventlog.Change) {
			r, _ := receiptwire.Decode(c[2].Body)
			r.Scope.Tenant = [16]byte(uuid.MustParse(space))
			c[2].Body, _ = receiptwire.Encode(r)
		},
		"receipt path": func(c []eventlog.Change) {
			r, _ := receiptwire.Decode(c[2].Body)
			r.Scope.Path = "/api/other"
			c[2].Body, _ = receiptwire.Encode(r)
		},
		"receipt method": func(c []eventlog.Change) {
			r, _ := receiptwire.Decode(c[2].Body)
			r.Scope.Method = "PUT"
			c[2].Body, _ = receiptwire.Encode(r)
		},
		"receipt digest": func(c []eventlog.Change) {
			r, _ := receiptwire.Decode(c[2].Body)
			r.RequestDigest[0] ^= 1
			c[2].Body, _ = receiptwire.Encode(r)
		},
		"receipt status": func(c []eventlog.Change) {
			r, _ := receiptwire.Decode(c[2].Body)
			r.Receipt.Response.Status = 200
			c[2].Body, _ = receiptwire.Encode(r)
		},
		"receipt etag": func(c []eventlog.Change) {
			r, _ := receiptwire.Decode(c[2].Body)
			r.Receipt.Response.ETag = `"v2"`
			c[2].Body, _ = receiptwire.Encode(r)
		},
		"receipt body": func(c []eventlog.Change) {
			r, _ := receiptwire.Decode(c[2].Body)
			r.Receipt.Response.Body = []byte(`{}`)
			c[2].Body, _ = receiptwire.Encode(r)
		},
		"receipt time": func(c []eventlog.Change) {
			r, _ := receiptwire.Decode(c[2].Body)
			r.Receipt.CreatedAtMS++
			c[2].Body, _ = receiptwire.Encode(r)
		},
		"receipt window short": func(c []eventlog.Change) {
			r, _ := receiptwire.Decode(c[2].Body)
			r.Receipt.ExpiresAtMS--
			c[2].Body, _ = receiptwire.Encode(r)
		},
		"occupancy address": func(c []eventlog.Change) {
			c[0].Body = bytes.Replace(c[0].Body, []byte("192.0.2.10"), []byte("192.0.2.11"), 1)
		},
		"occupancy kind": func(c []eventlog.Change) {
			c[0].Body = bytes.Replace(c[0].Body, []byte("STATIC"), []byte("EXCLUDED"), 1)
		},
		"invalid audit body": func(c []eventlog.Change) { c[1].Body = json.RawMessage(`{}`) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			e, c := fixture(t)
			mutate(c)
			e = rehash(t, e, c)
			if _, err := Validate(e); err == nil {
				t.Fatal("accepted tamper")
			}
		})
	}
}

func TestEnvelopeFailures(t *testing.T) {
	e, _ := fixture(t)
	e.Position.Hash[0] ^= 1
	if _, err := Validate(e); err == nil {
		t.Fatal("bad hash")
	}
	e, _ = fixture(t)
	e.Payload = e.Payload[:len(e.Payload)-1]
	if _, err := Validate(e); err == nil {
		t.Fatal("truncated")
	}
	e, _ = fixture(t)
	e.Payload = []byte(strings.Repeat("x", eventlog.MaxPayloadBytes+1))
	if _, err := Validate(e); err == nil {
		t.Fatal("oversized")
	}
	e, c := fixture(t)
	e = rehash(t, e, c[:2])
	if _, err := Validate(e); err == nil {
		t.Fatal("missing")
	}
	e, c = fixture(t)
	c = append(c, c[0])
	e = rehash(t, e, c)
	if _, err := Validate(e); err == nil {
		t.Fatal("extra")
	}
	e, c = fixture(t)
	e.CommitMs = 99
	e = rehash(t, e, c)
	if _, err := Validate(e); err == nil {
		t.Fatal("time before create")
	}
	e, c = fixture(t)
	e.CommitMs = 1000
	e = rehash(t, e, c)
	if _, err := Validate(e); err == nil {
		t.Fatal("expired at commit")
	}
}
