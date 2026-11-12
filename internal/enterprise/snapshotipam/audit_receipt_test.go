package snapshotipam

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/idempotency"
	"github.com/jasonwa/goddi/internal/enterprise/snapshotpipeline"
	"github.com/jasonwa/goddi/internal/enterprise/snapshotrows"
	"github.com/jasonwa/goddi/internal/enterprise/snapshotverify"
)

func encoded(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
func receiptRow(tenant []byte, body []byte) map[string]any {
	response := make([]byte, 8+len(body))
	copy(response, "GIR1")
	binary.BigEndian.PutUint16(response[4:6], 201)
	copy(response[8:], body)
	return map[string]any{"tenant_id": encoded(tenant), "principal": "operator", "method": "POST", "path": "/api/v1/ipam", "idempotency_key": "11111111-1111-4111-8111-111111111111", "request_digest": encoded(make([]byte, 32)), "response": encoded(response), "created_at_unix_ms": "1", "expires_at_unix_ms": "2"}
}
func auditRow() map[string]any {
	return map[string]any{"cluster_id": encoded(bytes.Repeat([]byte{1}, 16)), "epoch": "1", "tenant_id": "11111111-1111-4111-8111-111111111111", "space_id": "22222222-2222-4222-8222-222222222222", "resource_id": "33333333-3333-4333-8333-333333333333", "principal": "operator", "action": "ipam.occupancy.create", "request_id": "44444444-4444-4444-8444-444444444444", "prior_revision": "0", "new_revision": "1", "result": "SUCCESS", "occurred_at_ms": "0", "request_digest": encoded(make([]byte, 32))}
}
func TestAuditAndReceiptCandidates(t *testing.T) {
	a := auditRow()
	raw := row(t, a)
	r, err := New().Validate("static_occupancy_audit", raw)
	if err != nil {
		t.Fatal(err)
	}
	original := string(r.Payload)
	raw[0] = 'x'
	if string(r.Payload) != original {
		t.Fatal("audit alias")
	}
	tenant := bytes.Repeat([]byte{1}, 16)
	data := receiptRow(tenant, []byte{0, 1, 255})
	raw = row(t, data)
	r, err = New().Validate("idempotency_receipt", raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, r.Payload) {
		t.Fatal("receipt altered")
	}
	raw[0] = 'x'
	if r.Payload[0] != '{' {
		t.Fatal("receipt alias")
	}
}
func TestReceiptBinaryPrimaryKeyOrder(t *testing.T) {
	// In base64url the 0xd0 prefix sorts before the zero prefix, the opposite of BLOB order.
	a := make([]byte, 16)
	a[15] = 1
	b := make([]byte, 16)
	b[0] = 0xd0
	if !(encoded(a) > encoded(b)) {
		t.Fatal("fixture does not invert encoding order")
	}
	v := New()
	if _, err := v.Validate("idempotency_receipt", row(t, receiptRow(a, nil))); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Validate("idempotency_receipt", row(t, receiptRow(b, nil))); err != nil {
		t.Fatal(err)
	}
	v = New()
	if _, err := v.Validate("idempotency_receipt", row(t, receiptRow(b, nil))); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Validate("idempotency_receipt", row(t, receiptRow(a, nil))); err == nil {
		t.Fatal("accepted reverse BLOB order")
	}
}
func TestAuditReceiptRejectMalformed(t *testing.T) {
	tenant := bytes.Repeat([]byte{1}, 16)
	cases := []struct {
		name, table string
		raw         []byte
	}{
		{"digest", "static_occupancy_audit", row(t, func() map[string]any { x := auditRow(); x["request_digest"] = encoded([]byte{1}); return x }())},
		{"revision", "static_occupancy_audit", row(t, func() map[string]any { x := auditRow(); x["new_revision"] = "2"; return x }())},
		{"cluster", "static_occupancy_audit", row(t, func() map[string]any { x := auditRow(); x["cluster_id"] = encoded(make([]byte, 15)); return x }())},
		{"expired shape", "idempotency_receipt", row(t, func() map[string]any { x := receiptRow(tenant, nil); x["expires_at_unix_ms"] = "1"; return x }())},
		{"path", "idempotency_receipt", row(t, func() map[string]any { x := receiptRow(tenant, nil); x["path"] = "/a/../b"; return x }())},
		{"short response", "idempotency_receipt", row(t, func() map[string]any { x := receiptRow(tenant, nil); x["response"] = encoded([]byte("GIR1")); return x }())},
		{"bad magic", "idempotency_receipt", row(t, func() map[string]any {
			x := receiptRow(tenant, nil)
			x["response"] = encoded([]byte("BAD!\x00\xc8\x00\x00"))
			return x
		}())},
		{"wrong etag length", "idempotency_receipt", row(t, func() map[string]any {
			x := receiptRow(tenant, nil)
			x["response"] = encoded([]byte("GIR1\x00\xc8\x00\x01"))
			return x
		}())},
		{"noncanonical blob", "idempotency_receipt", row(t, func() map[string]any {
			x := receiptRow(tenant, nil)
			x["tenant_id"] = x["tenant_id"].(string) + "="
			return x
		}())},
		{"unknown", "idempotency_receipt", row(t, func() map[string]any { x := receiptRow(tenant, nil); x["extra"] = "x"; return x }())},
		{"null", "idempotency_receipt", row(t, func() map[string]any { x := receiptRow(tenant, nil); x["principal"] = nil; return x }())},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New().Validate(tc.table, tc.raw); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	raw := row(t, receiptRow(tenant, nil))
	dupe := strings.Replace(string(raw), `"principal":"operator"`, `"principal":"operator","principal":"operator"`, 1)
	if _, err := New().Validate("idempotency_receipt", []byte(dupe)); err == nil {
		t.Fatal("duplicate field accepted")
	}
}
func TestLargeReceiptCrossChunkPreservesRaw(t *testing.T) {
	tenant := bytes.Repeat([]byte{1}, 16)
	body := bytes.Repeat([]byte{0xa5}, idempotency.MaxBodyBytes)
	data := receiptRow(tenant, body)
	etag := bytes.Repeat([]byte{'e'}, idempotency.MaxETagBytes)
	response := make([]byte, 8+len(etag)+len(body))
	copy(response, "GIR1")
	binary.BigEndian.PutUint16(response[4:6], 201)
	binary.BigEndian.PutUint16(response[6:8], uint16(len(etag)))
	copy(response[8:], etag)
	copy(response[8+len(etag):], body)
	data["response"] = encoded(response)
	raw := row(t, data)
	if len(raw) <= 1<<20 || len(raw) >= 2<<20 {
		t.Fatalf("row size %d", len(raw))
	}
	input := append([]byte(`{"table":"idempotency_receipt","row":`), raw...)
	input = append(input, []byte("}\n")...)
	chunks := [][]byte{input[:1<<20], input[1<<20:]}
	m := snapshotverify.Manifest{Cluster: [16]byte{1}, Leader: [16]byte{2}, Snapshot: [16]byte{3}, Schema: 1, Anchor: snapshotverify.Position{Epoch: 1}, Encoding: snapshotverify.EncodingNDJSONV1, RetentionUntilMS: 200, TotalBytes: uint64(len(input))}
	for i, c := range chunks {
		m.Chunks = append(m.Chunks, snapshotverify.ChunkSpec{Index: uint32(i), Size: uint64(len(c)), SHA256: sha256.Sum256(c)})
	}
	for _, name := range order {
		n := uint64(0)
		if name == "idempotency_receipt" {
			n = 1
		}
		m.Tables = append(m.Tables, snapshotverify.TableCount{Name: name, Rows: n})
	}
	trust := snapshotverify.Trust{Authenticated: true, Cluster: m.Cluster, Leader: m.Leader, Snapshot: m.Snapshot, Anchor: m.Anchor}
	var observed Row
	p, err := snapshotpipeline.New(m, trust, snapshotverify.Limits{Schema: 1, MaxBytes: 3 << 20, MaxChunks: 3, MaxRows: 4}, snapshotrows.Limits{LineBytes: 2 << 20, TotalBytes: 3 << 20, Rows: 4, Tables: 8}, New().Tables(func(r Row) error { observed = r; return nil }), 100)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range chunks {
		if err = p.Feed(uint32(i), c, 101); err != nil {
			t.Fatal(err)
		}
	}
	result, err := p.Finish(102)
	if err != nil || result.BusinessComplete || !bytes.Equal(observed.Payload, raw) {
		t.Fatalf("finish %v", err)
	}
	var fields map[string]string
	if err = json.Unmarshal(observed.Payload, &fields); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(fields["response"])
	if err != nil || !bytes.Equal(decoded, response) {
		t.Fatal("body altered", err)
	}
}
func TestAuditReceiptOrderAndDuplicate(t *testing.T) {
	for _, tc := range []struct {
		table string
		data  map[string]any
	}{{"static_occupancy_audit", auditRow()}, {"idempotency_receipt", receiptRow(bytes.Repeat([]byte{1}, 16), nil)}} {
		t.Run(tc.table, func(t *testing.T) {
			v := New()
			raw := row(t, tc.data)
			if _, err := v.Validate(tc.table, raw); err != nil {
				t.Fatal(err)
			}
			if _, err := v.Validate(tc.table, raw); err == nil {
				t.Fatal("duplicate accepted")
			}
			if _, err := v.Validate(tc.table, raw); !errors.Is(err, ErrDirty) {
				t.Fatal(fmt.Sprint(err))
			}
		})
	}
}
