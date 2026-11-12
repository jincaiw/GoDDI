package eventlog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type vectorEvent struct {
	Epoch           string `json:"epoch"`
	Seq             string `json:"seq"`
	SchemaVersion   uint32 `json:"schemaVersion"`
	CommittedUnixMs string `json:"committedUnixMs"`
	PrevHash        string `json:"prevHash"`
	PayloadUTF8     string `json:"payloadUtf8"`
	Hash            string `json:"hash"`
}

// vector mirrors contracts/replication-vector.json from the development
// package. The file is copied under testdata rather than referenced across
// repositories so this suite stays reproducible from the Go tree alone;
// TestVectorFileIsStillTheContractCopy guards the copy against drift.
type vector struct {
	Description string        `json:"description"`
	ClusterID   string        `json:"clusterId"`
	LeaderID    string        `json:"leaderId"`
	Events      []vectorEvent `json:"events"`
}

func readVector(t *testing.T) vector {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "replication-vector.json"))
	if err != nil {
		t.Fatalf("read vector: %v", err)
	}
	var v vector
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode vector: %v", err)
	}
	if len(v.Events) != 2 {
		t.Fatalf("vector must carry two chained events, got %d", len(v.Events))
	}
	return v
}

// contractVectorSHA256 is the digest of
// goddi/contracts/replication-vector.json as published with the development
// package. The copy under testdata must stay byte-identical to it: an unpinned
// copy would let the interoperability vectors drift and still pass.
const contractVectorSHA256 = "c7aafc701206ec4ffc30c21cb1c051d75d496efe704455e3c25683c06976ef0e"

func TestVectorFileIsStillTheContractCopy(t *testing.T) {
	local, err := os.ReadFile(filepath.Join("testdata", "replication-vector.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(local)); got != contractVectorSHA256 {
		t.Fatalf("testdata/replication-vector.json drifted from the contract package: sha256 %s, want %s", got, contractVectorSHA256)
	}
	// When the contract package is vendored into this repository, compare the
	// two files byte for byte rather than trusting the pin alone.
	if contract, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "replication-vector.json")); err == nil {
		if string(contract) != string(local) {
			t.Fatal("the vendored contract vector and the testdata copy differ")
		}
	}
}

func (v vector) ids(t *testing.T) (ID, ID) {
	t.Helper()
	cluster, err := ParseID(v.ClusterID)
	if err != nil {
		t.Fatalf("cluster id: %v", err)
	}
	leader, err := ParseID(v.LeaderID)
	if err != nil {
		t.Fatalf("leader id: %v", err)
	}
	return cluster, leader
}

func eventFromVector(t *testing.T, cluster, leader ID, ve vectorEvent) Event {
	t.Helper()
	epoch, err := ParseDecUint(ve.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	seq, err := ParseDecUint(ve.Seq)
	if err != nil {
		t.Fatal(err)
	}
	commitMs, err := ParseDecInt64(ve.CommittedUnixMs)
	if err != nil {
		t.Fatal(err)
	}
	prev, err := ParseHash([]byte(ve.PrevHash))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := ParseHash([]byte(ve.Hash))
	if err != nil {
		t.Fatal(err)
	}
	return Event{
		ClusterID: cluster,
		LeaderID:  leader,
		Position:  Position{Epoch: epoch, Seq: seq, Hash: hash},
		Schema:    ve.SchemaVersion,
		CommitMs:  commitMs,
		PrevHash:  prev,
		Payload:   []byte(ve.PayloadUTF8),
	}
}

// TestVectorDigestsMatchByteForByte is the cross-language interoperability
// check: the digest must come out of the documented byte layout, not out of
// any incidental encoding choice.
func TestVectorDigestsMatchByteForByte(t *testing.T) {
	v := readVector(t)
	cluster, leader := v.ids(t)
	var prev *Event
	for i, ve := range v.Events {
		e := eventFromVector(t, cluster, leader, ve)
		got, err := e.Hash()
		if err != nil {
			t.Fatalf("event %d: %v", i, err)
		}
		if hex.EncodeToString(got[:]) != strings.ToLower(ve.Hash) {
			t.Fatalf("event %d digest mismatch:\n got %x\nwant %s", i, got, ve.Hash)
		}
		if prev != nil && e.PrevHash != prev.Position.Hash {
			t.Fatalf("event %d does not chain onto its predecessor", i)
		}
		applied := e
		if err := applied.Verify(); err != nil {
			t.Fatalf("event %d failed self-verification: %v", i, err)
		}
		prev = &applied
	}
}

// Re-encoding a payload before hashing is the failure mode docs/04:44 warns
// about: the chain would break on the standby without anything local noticing.
func TestReEncodedPayloadDoesNotReproduceVectorDigest(t *testing.T) {
	v := readVector(t)
	cluster, leader := v.ids(t)
	// Event 2 is the one whose sample JSON carries whitespace the encoder
	// would not reproduce.
	ve := v.Events[1]
	e := eventFromVector(t, cluster, leader, ve)

	changes, err := DecodePayload(e.Payload)
	if err != nil {
		t.Fatalf("vector payload must decode: %v", err)
	}
	reEncoded, err := MarshalChanges(changes...)
	if err != nil {
		t.Fatal(err)
	}
	if string(reEncoded) == ve.PayloadUTF8 {
		t.Skip("this payload happens to round-trip; the raw-bytes rule still applies")
	}
	tampered := e
	tampered.Payload = reEncoded
	got, err := tampered.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got[:]) == strings.ToLower(ve.Hash) {
		t.Fatal("a re-encoded payload must not reproduce the contract digest")
	}
	if err := tampered.Verify(); err == nil {
		t.Fatal("a re-encoded payload must fail verification against the recorded digest")
	}
}

func TestChainAcceptance(t *testing.T) {
	v := readVector(t)
	cluster, leader := v.ids(t)
	first := eventFromVector(t, cluster, leader, v.Events[0])
	second := eventFromVector(t, cluster, leader, v.Events[1])

	if err := VerifyChain(first, second); err != nil {
		t.Fatalf("the two vector events must chain: %v", err)
	}

	var err error
	foreign := second
	foreign.ClusterID[0] ^= 1
	foreign.Position.Hash, err = foreign.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyChain(first, foreign); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("foreign cluster accepted: %v", err)
	}
	foreign = second
	foreign.LeaderID[0] ^= 1
	foreign.Position.Hash, err = foreign.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Accept(first, foreign); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("foreign leader accepted: %v", err)
	}

	dup, err := Accept(first, first)
	if err != nil || !dup {
		t.Fatalf("an identical repeat must be a no-op, got duplicate=%v err=%v", dup, err)
	}
	dup, err = Accept(first, second)
	if err != nil || dup {
		t.Fatalf("the next event must apply once, got duplicate=%v err=%v", dup, err)
	}

	// A different payload at an already applied position is a divergence, and
	// must quarantine the link rather than overwrite it (docs/04:47).
	tampered := second
	tampered.Payload = []byte(strings.Replace(string(second.Payload), `"revision":"2"`, `"revision":"3"`, 1))
	hash, err := tampered.Hash()
	if err != nil {
		t.Fatalf("the tampered event must still be well formed: %v", err)
	}
	tampered.Position.Hash = hash
	if _, err := Accept(second, tampered); err == nil {
		t.Fatal("same position with a different digest must diverge")
	} else if !errors.Is(err, ErrDiverged) {
		t.Fatalf("expected ErrDiverged, got %v", err)
	}

	// A well-formed event at a position the head has not reached is a gap:
	// the standby parks and repairs rather than skipping (docs/04:47).
	gapped := second
	gapped.Position.Seq = 9
	gappedHash, err := gapped.Hash()
	if err != nil {
		t.Fatal(err)
	}
	gapped.Position.Hash = gappedHash
	if _, err := Accept(first, gapped); !errors.Is(err, ErrChainBreak) {
		t.Fatalf("a sequence gap must be reported, got %v", err)
	}

	crossEpoch := second
	crossEpoch.Position.Epoch = 2
	crossHash, err := crossEpoch.Hash()
	if err != nil {
		t.Fatal(err)
	}
	crossEpoch.Position.Hash = crossHash
	if _, err := Accept(first, crossEpoch); !errors.Is(err, ErrEpochMismatch) {
		t.Fatalf("V1 must not pull across epochs, got %v", err)
	}
}

func TestAnchorPositions(t *testing.T) {
	anchor, err := NewAnchor(1)
	if err != nil {
		t.Fatal(err)
	}
	if anchor.Hash != GenesisHash() || anchor.Seq != 0 {
		t.Fatalf("epoch 1 must anchor on (1,0) with the all-zero hash, got %+v", anchor)
	}
	e := Event{Position: anchor}
	if err := e.Verify(); err != nil {
		t.Fatalf("anchor must verify structurally: %v", err)
	}
	e.Payload = []byte("{}")
	if err := e.Verify(); err == nil {
		t.Fatal("an anchor must not carry a payload")
	}

	// A rebaselined epoch anchors on the predecessor's frozen final digest.
	final := [HashBytes]byte{0xde, 0xad, 0xbe, 0xef}
	rebased, err := AnchorHash(2, final)
	if err != nil {
		t.Fatal(err)
	}
	if rebased != final {
		t.Fatalf("rebased anchor must be the predecessor digest, got %x", rebased)
	}
	if _, err := AnchorHash(1, final); err == nil {
		t.Fatal("epoch 1 must not anchor on a non-zero digest")
	}
	if _, err := AnchorHash(0, GenesisHash()); err == nil {
		t.Fatal("epoch 0 does not exist")
	}
	if _, err := NewAnchor(2); err == nil {
		t.Fatal("a later epoch must not get an implicit zero anchor")
	}
}

func TestPayloadRejections(t *testing.T) {
	base := `{"changes":[{"kind":"dhcp.lease.committed","objectId":"b01dd6c0-81a0-4a8f-b66c-5f28c1f20001","revision":"1","body":{"address":"192.0.2.10"}}]}`
	if _, err := DecodePayload([]byte(base)); err != nil {
		t.Fatalf("baseline payload must decode: %v", err)
	}
	cases := []struct{ name, raw string }{
		{"byte order mark", "\xEF\xBB\xBF" + base},
		{"empty", ""},
		{"not an object", `[]`},
		{"trailing content", base + ` {}`},
		{"unknown top-level field", `{"changes":[{"kind":"dhcp.lease.committed","objectId":"b01dd6c0-81a0-4a8f-b66c-5f28c1f20001","revision":"1","body":{},"surprise":1}]}`},
		{"empty change list", `{"changes":[]}`},
		{"duplicate key", `{"changes":[{"kind":"dhcp.lease.committed","kind":"dhcp.lease.released","objectId":"b01dd6c0-81a0-4a8f-b66c-5f28c1f20001","revision":"1","body":{}}]}`},
		{"unknown kind", `{"changes":[{"kind":"dhcp.lease.melts","objectId":"b01dd6c0-81a0-4a8f-b66c-5f28c1f20001","revision":"1","body":{}}]}`},
		{"numeric revision", `{"changes":[{"kind":"dhcp.lease.committed","objectId":"b01dd6c0-81a0-4a8f-b66c-5f28c1f20001","revision":1,"body":{}}]}`},
		{"negative revision", `{"changes":[{"kind":"dhcp.lease.committed","objectId":"b01dd6c0-81a0-4a8f-b66c-5f28c1f20001","revision":"-1","body":{}}]}`},
		{"leading-zero revision", `{"changes":[{"kind":"dhcp.lease.committed","objectId":"b01dd6c0-81a0-4a8f-b66c-5f28c1f20001","revision":"01","body":{}}]}`},
		{"non-uuid object id", `{"changes":[{"kind":"dhcp.lease.committed","objectId":"lease-1","revision":"1","body":{}}]}`},
		{"missing body", `{"changes":[{"kind":"dhcp.lease.committed","objectId":"b01dd6c0-81a0-4a8f-b66c-5f28c1f20001","revision":"1"}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := DecodePayload([]byte(c.raw)); err == nil {
				t.Fatalf("payload %q must be rejected", c.raw)
			}
		})
	}
}

func TestStrictPayloadValidation(t *testing.T) {
	base := `{"changes":[{"kind":"dhcp.lease.committed","objectId":"b01dd6c0-81a0-4a8f-b66c-5f28c1f20001","revision":"1","body":{"name":"ok"}}]}`
	cases := []string{
		strings.Replace(base, `"ok"`, "\"\xff\"", 1),
		strings.Replace(base, `"changes":`, `"Changes":`, 1),
		strings.Replace(base, `"kind":`, `"Kind":`, 1),
		strings.Replace(base, `"revision":"1"`, `"revision":"1","Revision":"2"`, 1),
		strings.Replace(base, `"revision":"1"`, `"revision":"9223372036854775808"`, 1),
		strings.Replace(base, `"name":"ok"`, `"name":"a","name":"b"`, 1),
	}
	v := readVector(t)
	cluster, leader := v.ids(t)
	for _, raw := range cases {
		if _, err := DecodePayload([]byte(raw)); err == nil {
			t.Errorf("DecodePayload accepted %q", raw)
		}
		e := eventFromVector(t, cluster, leader, v.Events[0])
		e.Payload = []byte(raw)
		if _, err := e.Hash(); err == nil {
			t.Errorf("Hash accepted %q", raw)
		}
	}
	max := strings.Replace(base, `"revision":"1"`, `"revision":"9223372036854775807"`, 1)
	if _, err := DecodePayload([]byte(max)); err != nil {
		t.Fatalf("max revision rejected: %v", err)
	}
	c := Change{Kind: "dhcp.lease.committed", ObjectID: "b01dd6c0-81a0-4a8f-b66c-5f28c1f20001", Revision: "1", Body: []byte("{\"s\":\"\xff\"}")}
	if _, err := MarshalChanges(c); !errors.Is(err, ErrPayloadEncoding) {
		t.Fatalf("invalid UTF-8 body: %v", err)
	}
	c.Body = []byte(`{"k":1,"k":2}`)
	if _, err := MarshalChanges(c); !errors.Is(err, ErrPayloadDuplicate) {
		t.Fatalf("duplicate body key: %v", err)
	}
	c.Body = []byte(`{}`)
	c.Kind = "unknown.kind"
	if _, err := MarshalChanges(c); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("unknown kind: %v", err)
	}
}

func TestFieldWidthAndRangeGuards(t *testing.T) {
	v := readVector(t)
	cluster, leader := v.ids(t)
	base := eventFromVector(t, cluster, leader, v.Events[0])

	cases := []struct {
		name   string
		mutate func(*Event)
	}{
		{"epoch 0", func(e *Event) { e.Position.Epoch = 0 }},
		{"seq past the signed range", func(e *Event) { e.Position.Seq = 1 << 63 }},
		{"schema 0", func(e *Event) { e.Schema = 0 }},
		{"schema 2", func(e *Event) { e.Schema = 2 }},
		{"empty payload", func(e *Event) { e.Payload = nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := base
			c.mutate(&e)
			if _, err := e.Hash(); err == nil {
				t.Fatalf("%s must be rejected", c.name)
			}
		})
	}
}

// The digest layout is specified field by field; this test rebuilds it
// independently so a refactor cannot quietly reorder or resize a field.
func TestDigestLayoutIsTheDocumentedByteString(t *testing.T) {
	v := readVector(t)
	cluster, leader := v.ids(t)
	e := eventFromVector(t, cluster, leader, v.Events[0])

	raw := []byte("goddi.replica.v1")
	raw = append(raw, 0)
	raw = append(raw, cluster[:]...)
	raw = append(raw, be64(e.Position.Epoch)...)
	raw = append(raw, be64(e.Position.Seq)...)
	raw = append(raw, leader[:]...)
	raw = append(raw, be32(e.Schema)...)
	raw = append(raw, be64(uint64(e.CommitMs))...)
	raw = append(raw, e.PrevHash[:]...)
	raw = append(raw, be32(uint32(len(e.Payload)))...)
	raw = append(raw, e.Payload...)
	want := sha256.Sum256(raw)

	got, err := e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("digest layout drifted: got %x want %x", got, want)
	}
}

func TestIDRoundTrip(t *testing.T) {
	v := readVector(t)
	cluster, _ := v.ids(t)
	if cluster.String() != strings.ToLower(v.ClusterID) {
		t.Fatalf("UUID rendering drifted: got %s want %s", cluster.String(), v.ClusterID)
	}
	again, err := ParseID(cluster.String())
	if err != nil {
		t.Fatal(err)
	}
	if again != cluster {
		t.Fatal("ID must round-trip through its canonical text form")
	}
	for _, bad := range []string{"", "not-a-uuid", "3a0cc999-92c2-44df-9cc2-06d414a6000", "3a0cc99992c244df9cc206d414a60001"} {
		if _, err := ParseID(bad); err == nil {
			t.Fatalf("%q is not a canonical UUID and must be rejected", bad)
		}
	}
	if _, err := IDFromBytes(make([]byte, 15)); err == nil {
		t.Fatal("a 15-byte identifier must be rejected")
	}
}

func TestDecimalStringsAreNotJSNumbers(t *testing.T) {
	for _, s := range []string{"9223372036854775807", "1", "0"} {
		n, err := ParseDecUint(s)
		if err != nil {
			t.Fatalf("%s must parse: %v", s, err)
		}
		if DecUint(n) != s {
			t.Fatalf("%s must round-trip, got %s", s, DecUint(n))
		}
	}
	for _, s := range []string{"", " 1", "1.0", "0x10", "1e3", "-1", "9223372036854775808"} {
		if _, err := ParseDecUint(s); err == nil {
			t.Fatalf("%q must be rejected", s)
		}
	}
}
