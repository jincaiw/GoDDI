package snapshotipam

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/snapshotpipeline"
	"github.com/jasonwa/goddi/internal/enterprise/snapshotrows"
	"github.com/jasonwa/goddi/internal/enterprise/snapshotverify"
)

func ledger(kind, tenant, space, id string) string {
	return fmt.Sprintf(`{"kind":%q,"tenant_id":%q,"space_id":%q,"id":%q}`, kind, tenant, space, id)
}
func TestLedgerAllKindsAndCopies(t *testing.T) {
	v := New()
	for _, s := range []string{ledger("OCCUPANCY", "t", "s", "id"), ledger("POOL", "t", "s", "id"), ledger("PREFIX", "t", "s", "id"), ledger("SPACE", "t", "", "id"), ledger("TENANT", "", "", "id")} {
		input := []byte(s)
		got, err := v.Validate("ipam_resource_ids", input)
		if err != nil {
			t.Fatal(err)
		}
		input[0] = 'x'
		if string(got.Payload) != s {
			t.Fatal("aliased source")
		}
		got.Payload[0] = 'x'
	}
}
func TestLedgerRejectsInvalid(t *testing.T) {
	tests := []string{ledger("SIXTH", "t", "s", "id"), ledger("TENANT", "t", "", "id"), ledger("TENANT", "", "s", "id"), ledger("SPACE", "", "", "id"), ledger("SPACE", "t", "s", "id"), ledger("PREFIX", "", "s", "id"), ledger("POOL", "t", "", "id"), ledger("OCCUPANCY", "t", "s", ""), ledger("TENANT", "", "", strings.Repeat("x", 4097)),
		`{"kind":"TENANT","tenant_id":"","space_id":"","id":null}`, `{"kind":"TENANT","tenant_id":"","space_id":"","id":"x","id":"y"}`, `{"kind":"TENANT","tenant_id":"","space_id":"","id":"x","unexpected":1}`, `{"kind":"TENANT","tenant_id":"","space_id":"","id":"\ud800"}`, "{\"kind\":\"TENANT\",\"tenant_id\":\"\",\"space_id\":\"\",\"id\":\"\xff\"}"}
	for i, s := range tests {
		v := New()
		if _, err := v.Validate("ipam_resource_ids", []byte(s)); err == nil {
			t.Errorf("accepted case %d", i)
		}
		if _, err := v.Validate("ipam_resource_ids", []byte(ledger("TENANT", "", "", "x"))); !errors.Is(err, ErrDirty) {
			t.Errorf("not dirty case %d: %v", i, err)
		}
	}
}
func TestLedgerPrimaryKeyOrdering(t *testing.T) {
	for _, pair := range [][2]string{{ledger("PREFIX", "t", "s", "x"), ledger("PREFIX", "t", "s", "x")}, {ledger("SPACE", "t", "", "z"), ledger("PREFIX", "t", "s", "a")}, {ledger("PREFIX", "t", "s2", "a"), ledger("PREFIX", "t", "s1", "z")}} {
		v := New()
		if _, err := v.Validate("ipam_resource_ids", []byte(pair[0])); err != nil {
			t.Fatal(err)
		}
		if _, err := v.Validate("ipam_resource_ids", []byte(pair[1])); err == nil {
			t.Fatal("duplicate or descending key accepted")
		}
	}
}
func TestLedgerPipelineAcrossChunks(t *testing.T) {
	payload := []byte(`{"table":"ipam_resource_ids","row":` + ledger("TENANT", "", "", "历史") + "}\n")
	split := bytes.Index(payload, []byte("历史")) + 1
	chunks := [][]byte{payload[:split], payload[split:]}
	m := snapshotverify.Manifest{Cluster: [16]byte{1}, Leader: [16]byte{2}, Snapshot: [16]byte{3}, Schema: 1, Anchor: snapshotverify.Position{Epoch: 1}, Encoding: snapshotverify.EncodingNDJSONV1, RetentionUntilMS: 200, TotalBytes: uint64(len(payload))}
	for i, c := range chunks {
		m.Chunks = append(m.Chunks, snapshotverify.ChunkSpec{Index: uint32(i), Size: uint64(len(c)), SHA256: sha256.Sum256(c)})
	}
	for _, name := range []string{"ipam_tenants", "ipam_spaces", "ipam_prefixes", "ipam_pools", "ipam_occupancies", "ipam_resource_ids", "static_occupancy_audit", "idempotency_receipt"} {
		count := uint64(0)
		if name == "ipam_resource_ids" {
			count = 1
		}
		m.Tables = append(m.Tables, snapshotverify.TableCount{Name: name, Rows: count})
	}
	trust := snapshotverify.Trust{Authenticated: true, Cluster: m.Cluster, Leader: m.Leader, Snapshot: m.Snapshot, Anchor: m.Anchor}
	var got []Row
	p, err := snapshotpipeline.New(m, trust, snapshotverify.Limits{Schema: 1, MaxBytes: 1 << 20, MaxChunks: 4, MaxRows: 10}, snapshotrows.Limits{LineBytes: 1 << 20, TotalBytes: 1 << 20, Rows: 10, Tables: 8}, New().Tables(func(r Row) error { got = append(got, r); return nil }), 100)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range chunks {
		if err = p.Feed(uint32(i), c, 101); err != nil {
			t.Fatal(err)
		}
	}
	result, err := p.Finish(102)
	if err != nil || len(got) != 1 || got[0].Table != "ipam_resource_ids" || !result.Rows.AllRowCallbacksPassed || result.BusinessComplete {
		t.Fatalf("result %+v %v", result, err)
	}
}
func TestLedgerSinkPanicDoesNotRepeat(t *testing.T) {
	v := New()
	effects := 0
	sentinel := &struct{}{}
	callbacks := v.Tables(func(Row) error { effects++; panic(sentinel) })
	raw := []byte(ledger("TENANT", "", "", "a"))
	func() {
		defer func() {
			if got := recover(); got != sentinel {
				t.Errorf("panic changed: %v", got)
			}
		}()
		_ = callbacks[5].ValidateRow(raw)
	}()
	if err := callbacks[5].ValidateRow(raw); !errors.Is(err, ErrDirty) || effects != 1 {
		t.Fatalf("retry: %v effects=%d", err, effects)
	}
}

func TestLedgerSinkCancellationAndConcurrentRetry(t *testing.T) {
	v := New()
	effects := 0
	callbacks := v.Tables(func(Row) error { effects++; return context.Canceled })
	raw := []byte(ledger("TENANT", "", "", "a"))
	if err := callbacks[5].ValidateRow(raw); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := callbacks[5].ValidateRow(raw); !errors.Is(err, ErrDirty) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if effects != 1 {
		t.Fatalf("effects %d", effects)
	}
}
