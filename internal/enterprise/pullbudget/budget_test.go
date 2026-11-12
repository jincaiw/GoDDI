package pullbudget

import (
	_ "embed"
	"errors"
	"math/rand"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Generated from the supplied contracts/replication.proto with protoc 36.2.
//
//go:embed replication.pbdesc
var descriptor []byte

func schema(t *testing.T) (protoreflect.MessageDescriptor, protoreflect.MessageDescriptor, protoreflect.MessageDescriptor) {
	t.Helper()
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(descriptor, &set); err != nil || len(set.File) != 1 {
		t.Fatalf("descriptor: %v", err)
	}
	fd, err := protodesc.NewFile(set.File[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	return fd.Messages().ByName("Position"), fd.Messages().ByName("Event"), fd.Messages().ByName("PullResponse")
}
func dynamicPosition(d protoreflect.MessageDescriptor, p *Position) *dynamicpb.Message {
	m := dynamicpb.NewMessage(d)
	f := d.Fields()
	m.Set(f.ByNumber(1), protoreflect.ValueOfUint64(p.Epoch))
	m.Set(f.ByNumber(2), protoreflect.ValueOfUint64(p.Seq))
	m.Set(f.ByNumber(3), protoreflect.ValueOfBytes(p.Hash))
	return m
}
func dynamicEvent(d, pd protoreflect.MessageDescriptor, e Event) *dynamicpb.Message {
	m := dynamicpb.NewMessage(d)
	f := d.Fields()
	m.Set(f.ByNumber(1), protoreflect.ValueOfBytes(e.ClusterID))
	if e.Position != nil {
		m.Set(f.ByNumber(2), protoreflect.ValueOfMessage(dynamicPosition(pd, e.Position)))
	}
	m.Set(f.ByNumber(3), protoreflect.ValueOfBytes(e.LeaderID))
	m.Set(f.ByNumber(4), protoreflect.ValueOfUint32(e.SchemaVersion))
	m.Set(f.ByNumber(5), protoreflect.ValueOfInt64(e.CommittedUnixMS))
	m.Set(f.ByNumber(6), protoreflect.ValueOfBytes(e.PrevHash))
	m.Set(f.ByNumber(7), protoreflect.ValueOfBytes(e.Payload))
	return m
}
func dynamicResponse(pd, ed, rd protoreflect.MessageDescriptor, r Response) *dynamicpb.Message {
	m := dynamicpb.NewMessage(rd)
	f := rd.Fields()
	m.Set(f.ByNumber(1), protoreflect.ValueOfEnum(protoreflect.EnumNumber(r.Disposition)))
	l := m.Mutable(f.ByNumber(2)).List()
	for _, e := range r.Events {
		l.Append(protoreflect.ValueOfMessage(dynamicEvent(ed, pd, e)))
	}
	if r.ObservedHead != nil {
		m.Set(f.ByNumber(3), protoreflect.ValueOfMessage(dynamicPosition(pd, r.ObservedHead)))
	}
	m.Set(f.ByNumber(4), protoreflect.ValueOfUint32(r.RequiredBytes))
	return m
}
func TestSizeMatchesDescriptor(t *testing.T) {
	pd, ed, rd := schema(t)
	rng := rand.New(rand.NewSource(321))
	for i := 0; i < 1000; i++ {
		randBytes := func(n int) []byte { return make([]byte, n) }
		p := func() *Position {
			return &Position{Epoch: uint64(rng.Intn(300)), Seq: uint64(rng.Intn(20000)), Hash: randBytes(rng.Intn(35))}
		}
		r := Response{Disposition: int32(rng.Intn(8)), RequiredBytes: uint32(rng.Intn(40000))}
		if i%3 != 0 {
			r.ObservedHead = p()
		}
		for j := 0; j < rng.Intn(5); j++ {
			e := Event{ClusterID: randBytes(rng.Intn(18)), LeaderID: randBytes(rng.Intn(18)), SchemaVersion: uint32(rng.Intn(300)), CommittedUnixMS: int64(rng.Intn(20000)), PrevHash: randBytes(rng.Intn(34)), Payload: randBytes(rng.Intn(300))}
			if i%2 == 0 {
				e.Position = p()
			}
			r.Events = append(r.Events, e)
		}
		got, err := Size(r)
		m := dynamicResponse(pd, ed, rd, r)
		encoded, e := proto.Marshal(m)
		if err != nil || e != nil || got != proto.Size(m) || got != len(encoded) {
			t.Fatalf("iteration %d: got=%d size=%d bytes=%d err=%v/%v", i, got, proto.Size(m), len(encoded), err, e)
		}
	}
	for _, r := range []Response{{}, {ObservedHead: &Position{}}, {Disposition: AtHead, ObservedHead: &Position{Epoch: 128, Seq: 16384, Hash: make([]byte, 32)}}, {Disposition: BudgetTooSmall, RequiredBytes: 128}} {
		got, e := Size(r)
		m := dynamicResponse(pd, ed, rd, r)
		if e != nil || got != proto.Size(m) {
			t.Fatalf("edge %v %d %d", e, got, proto.Size(m))
		}
	}
}
func testEvent(payload int) Event {
	return Event{ClusterID: make([]byte, 16), LeaderID: make([]byte, 16), Position: &Position{Epoch: 1, Seq: 1, Hash: make([]byte, 32)}, SchemaVersion: 1, CommittedUnixMS: 1, PrevHash: make([]byte, 32), Payload: make([]byte, payload)}
}
func TestPlan(t *testing.T) {
	h := Position{Epoch: 1, Seq: 8, Hash: make([]byte, 32)}
	p, e := Plan(h, nil, 0, 0)
	if e != nil || p.Disposition != AtHead || p.Count != 0 {
		t.Fatal(p, e)
	}
	first := testEvent(1100)
	p, e = Plan(h, []Event{first}, 0, 1024)
	if !errors.Is(e, ErrBudgetTooSmall) || p.RequiredBytes <= 1024 {
		t.Fatal(p, e)
	}
	required, _ := Size(Response{Disposition: Batch, ObservedHead: &h, Events: []Event{first}})
	reported, _ := Size(Response{Disposition: BudgetTooSmall, ObservedHead: &h, RequiredBytes: uint32(required)})
	if int(p.RequiredBytes) != required || int(p.ResponseBytes) != reported {
		t.Fatal(p, required, reported)
	}
	// Find the exact 1024-byte boundary, including all response framing.
	boundary := -1
	for size := 0; size < 1024; size++ {
		candidate := testEvent(size)
		n, err := Size(Response{Disposition: Batch, ObservedHead: &h, Events: []Event{candidate}})
		if err != nil {
			t.Fatal(err)
		}
		if n == 1024 {
			boundary = size
			break
		}
	}
	if boundary < 0 {
		t.Fatal("no exact boundary candidate")
	}
	at := testEvent(boundary)
	p, e = Plan(h, []Event{at}, 0, 1024)
	if e != nil || p.Count != 1 || p.ResponseBytes != 1024 {
		t.Fatal(p, e)
	}
	at.Payload = append(at.Payload, 0)
	p, e = Plan(h, []Event{at}, 0, 1024)
	if !errors.Is(e, ErrBudgetTooSmall) || p.RequiredBytes != 1025 {
		t.Fatal(p, e)
	}
	a, b := testEvent(700), testEvent(700)
	p, e = Plan(h, []Event{a, b}, 0, 1024)
	if e != nil || p.Count != 1 || p.Disposition != Batch {
		t.Fatal(p, e)
	}
	actual, _ := Size(Response{Disposition: Batch, ObservedHead: &h, Events: []Event{a}})
	if int(p.ResponseBytes) != actual {
		t.Fatal(p, actual)
	}
	p, e = Plan(h, []Event{a, b}, 1, 0)
	if e != nil || p.Count != 1 {
		t.Fatal(p, e)
	}
	for _, v := range []uint32{1, 1023, MaxBytes + 1} {
		if _, e = Plan(h, nil, 0, v); !errors.Is(e, ErrInvalid) {
			t.Fatal(v, e)
		}
	}
	if _, e = Plan(h, nil, 257, 0); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	first.CommittedUnixMS = -1
	if _, e = Plan(h, []Event{first}, 0, 0); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
