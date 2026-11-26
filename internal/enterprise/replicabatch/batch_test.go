package replicabatch

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jincaiw/GoDDI/internal/enterprise/auditwire"
	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/idempotency"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamevent"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamrequest"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamresponse"
	"github.com/jincaiw/GoDDI/internal/enterprise/receiptwire"
)

const tenant = "11111111-1111-4111-8111-111111111111"
const space = "22222222-2222-4222-8222-222222222222"
const subnet = "44444444-4444-4444-8444-444444444444"

func fixture(t *testing.T, n int) (Expected, []eventlog.Event) {
	t.Helper()
	cluster, _ := eventlog.ParseID("77777777-7777-4777-8777-777777777777")
	leader, _ := eventlog.ParseID("88888888-8888-4888-8888-888888888888")
	expected := Expected{Cluster: cluster, Leader: leader, Start: eventlog.Position{Epoch: 1}, Schema: 1}
	head := expected.Start
	events := make([]eventlog.Event, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("33333333-3333-4333-8333-%012x", i+1)
		request := fmt.Sprintf("55555555-5555-4555-8555-%012x", i+1)
		key := fmt.Sprintf("66666666-6666-4666-8666-%012x", i+1)
		address := fmt.Sprintf("192.0.%d.%d", 2+i/250, 1+i%250)
		owner := fmt.Sprintf("host-%d", i)
		o := ipamevent.Occupancy{TenantID: tenant, SpaceID: space, ID: id, SubnetID: subnet, Family: 4, Address: address, Kind: "STATIC", OwnerRef: owner, CreatedAtMS: "100", Revision: "1"}
		change, err := ipamevent.EncodeChange(ipamevent.Set, o)
		if err != nil {
			t.Fatal(err)
		}
		_, digest, err := ipamrequest.Normalize(ipamrequest.Request{TenantID: tenant, SpaceID: space, SubnetID: subnet, Address: address, OwnerRef: owner})
		if err != nil {
			t.Fatal(err)
		}
		audit, err := auditwire.Encode(auditwire.Audit{ClusterID: cluster.String(), TenantID: tenant, SpaceID: space, ResourceID: id, Principal: "operator", RequestID: request, Epoch: 1, OccurredAtMS: 100, RequestDigest: digest})
		if err != nil {
			t.Fatal(err)
		}
		response, err := ipamresponse.Static(id, subnet, address, owner)
		if err != nil {
			t.Fatal(err)
		}
		tid := uuid.MustParse(tenant)
		receipt, err := receiptwire.Encode(receiptwire.Value{Scope: idempotency.Scope{Tenant: [16]byte(tid), Principal: "operator", Method: "POST", Path: "/api/v1/ddi/tenants/" + tenant + "/ipam/spaces/" + space + "/allocations", Key: key}, RequestDigest: digest, Receipt: idempotency.Receipt{Response: response, CreatedAtMS: 100, ExpiresAtMS: 604800100}})
		if err != nil {
			t.Fatal(err)
		}
		payload, err := eventlog.MarshalChanges(change, eventlog.Change{Kind: "ipam.occupancy.audit", ObjectID: request, Revision: "1", Body: audit}, eventlog.Change{Kind: "idempotency.receipt.set", ObjectID: key, Revision: "1", Body: receipt})
		if err != nil {
			t.Fatal(err)
		}
		e := eventlog.Event{ClusterID: cluster, LeaderID: leader, Position: eventlog.Position{Epoch: 1, Seq: head.Seq + 1}, Schema: 1, CommitMs: 100, PrevHash: head.Hash, Payload: payload}
		e.Position.Hash, err = e.Hash()
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
		head = e.Position
	}
	return expected, events
}
func TestValidImmutableAndEmpty(t *testing.T) {
	x, events := fixture(t, 3)
	b, err := Verify(x, events)
	if err != nil {
		t.Fatal(err)
	}
	if b.Len() != 3 || b.Head() != events[2].Position || b.Start() != x {
		t.Fatal("incorrect position")
	}
	v, ok := b.At(0)
	if !ok {
		t.Fatal("missing item")
	}
	original := v.Event().Payload
	events[0].Payload[0] ^= 1
	first := v.Event()
	first.Payload[0] ^= 1
	receipt := v.Receipt()
	receipt.Receipt.Response.Body[0] ^= 1
	again, _ := b.At(0)
	if !bytes.Equal(original, again.Event().Payload) || again.Receipt().Receipt.Response.Body[0] != '{' {
		t.Fatal("mutable alias")
	}
	if _, ok := b.At(-1); ok {
		t.Fatal("negative index")
	}
	if _, ok := b.At(3); ok {
		t.Fatal("out of bounds")
	}
	empty, err := Verify(x, nil)
	if err != nil || empty.Len() != 0 || empty.Head() != x.Start {
		t.Fatalf("empty %+v %v", empty, err)
	}
}
func TestMaxBatchAndAggregate(t *testing.T) {
	x, events := fixture(t, 256)
	if b, err := Verify(x, events); err != nil || b.Len() != 256 {
		t.Fatalf("256: %v", err)
	}
	events = append(events, events[0])
	if b, err := Verify(x, events); err == nil || b != nil {
		t.Fatal("257 accepted")
	}
	x, events = fixture(t, 3)
	for i := range events {
		events[i].Payload = make([]byte, eventlog.MaxPayloadBytes)
	}
	if b, err := Verify(x, events); err == nil || b != nil {
		t.Fatal("aggregate accepted")
	}
}
func TestRejectMutatedBatchWithoutPartialResult(t *testing.T) {
	tests := map[string]func(*Expected, []eventlog.Event){
		"cluster": func(_ *Expected, e []eventlog.Event) { e[1].ClusterID[0] ^= 1 }, "leader": func(_ *Expected, e []eventlog.Event) { e[1].LeaderID[0] ^= 1 }, "epoch": func(_ *Expected, e []eventlog.Event) { e[1].Position.Epoch++ }, "schema": func(_ *Expected, e []eventlog.Event) { e[1].Schema++ }, "gap": func(_ *Expected, e []eventlog.Event) { e[1].Position.Seq++ }, "repeat": func(_ *Expected, e []eventlog.Event) { e[1] = e[0] }, "order": func(_ *Expected, e []eventlog.Event) { e[0], e[1] = e[1], e[0] }, "previous": func(_ *Expected, e []eventlog.Event) { e[1].PrevHash[0] ^= 1 }, "hash": func(_ *Expected, e []eventlog.Event) { e[1].Position.Hash[0] ^= 1 }, "payload": func(_ *Expected, e []eventlog.Event) { e[1].Payload[5] ^= 1 }, "time": func(_ *Expected, e []eventlog.Event) { e[1].CommitMs = 0 }, "wrong anchor": func(x *Expected, _ []eventlog.Event) { x.Start.Hash[0] = 1 }, "overflow": func(x *Expected, _ []eventlog.Event) { x.Start.Seq = math.MaxInt64 }, "untrusted epoch zero": func(x *Expected, _ []eventlog.Event) { x.Start.Epoch = 0 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			x, events := fixture(t, 3)
			mutate(&x, events)
			if b, err := Verify(x, events); err == nil || b != nil {
				t.Fatalf("accepted %+v %v", b, err)
			}
		})
	}
}
func TestBusinessValidationRejectsRehashedUnknownOrMissing(t *testing.T) {
	x, events := fixture(t, 1)
	changes, err := eventlog.DecodePayload(events[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"missing", "different kind"} {
		e := events[0]
		c := append([]eventlog.Change(nil), changes...)
		if mode == "missing" {
			c = c[:1]
		} else {
			c[1].Kind = "dhcp.lease.committed"
		}
		e.Payload, err = eventlog.MarshalChanges(c...)
		if err != nil {
			t.Fatal(err)
		}
		e.Position.Hash, err = e.Hash()
		if err != nil {
			t.Fatal(err)
		}
		if b, err := Verify(x, []eventlog.Event{e}); err == nil || b != nil {
			t.Fatal("unsupported body accepted")
		}
	}
}
func TestPublishedVectorHashAndUnsupportedBusiness(t *testing.T) {
	raw, err := os.ReadFile("../eventlog/testdata/replication-vector.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		ClusterID, LeaderID string
		Events              []struct {
			Epoch, Seq                                   string
			SchemaVersion                                uint32
			CommittedUnixMS, PrevHash, PayloadUTF8, Hash string
		}
	}
	if err = json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	cl, _ := eventlog.ParseID(v.ClusterID)
	leader, _ := eventlog.ParseID(v.LeaderID)
	x := Expected{Cluster: cl, Leader: leader, Start: eventlog.Position{Epoch: 1}, Schema: 1}
	var events []eventlog.Event
	for _, v := range v.Events {
		epoch, _ := strconv.ParseUint(v.Epoch, 10, 64)
		seq, _ := strconv.ParseUint(v.Seq, 10, 64)
		ms, _ := strconv.ParseInt(v.CommittedUnixMS, 10, 64)
		p, _ := hex.DecodeString(v.PrevHash)
		h, _ := hex.DecodeString(v.Hash)
		e := eventlog.Event{ClusterID: cl, LeaderID: leader, Position: eventlog.Position{Epoch: epoch, Seq: seq}, Schema: v.SchemaVersion, CommitMs: ms, Payload: []byte(v.PayloadUTF8)}
		copy(e.PrevHash[:], p)
		copy(e.Position.Hash[:], h)
		if err = e.Verify(); err != nil {
			t.Fatalf("published hash vector: %v", err)
		}
		events = append(events, e)
	}
	if b, err := Verify(x, events); err == nil || b != nil {
		t.Fatal("unimplemented DHCP events accepted")
	}
}
func TestConcurrentReaders(t *testing.T) {
	x, events := fixture(t, 2)
	b, err := Verify(x, events)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				v, ok := b.At(0)
				if !ok {
					t.Error("missing")
				}
				e := v.Event()
				e.Payload[0] ^= 1
				_ = b.Head()
				_ = b.Start()
			}
		}()
	}
	wg.Wait()
}
