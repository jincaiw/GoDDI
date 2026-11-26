package snapshotstage

// The schema-2 pipeline end-to-end: the guarded eight-table NDJSON stream
// extended with the frozen lease table set, flowing through ONE unified
// snapshotpipeline whose schema entries are the eight IPAM validators plus
// dhcplease's four lease validators. The manifest declares Schema 2 and
// twelve tables; snapshotpipeline.New enforces the exact schema/manifest
// match, so this test proves the wiring a real schema-2 import needs.

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/dhcplease"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotipam"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotpipeline"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotrows"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotverify"
)

// snapshot2 extends the guarded stream with lease-table rows and declares
// Schema 2 with twelve tables in frozen order.
func snapshot2(t *testing.T, leaseLines []line) (snapshotverify.Manifest, snapshotverify.Trust, []chunk) {
	lines := append(guardedLines(), leaseLines...)
	var raw []byte
	counts := map[string]uint64{}
	for _, l := range lines {
		b, e := jsonMarshalLine(l)
		if e != nil {
			t.Fatal(e)
		}
		raw = append(raw, b...)
		raw = append(raw, '\n')
		counts[l.Table]++
	}
	tables := []snapshotverify.TableCount{}
	for _, name := range []string{
		"ipam_tenants", "ipam_spaces", "ipam_prefixes", "ipam_pools",
		"ipam_occupancies", "ipam_resource_ids", "static_occupancy_audit",
		"idempotency_receipt", "dhcp_lease", "dhcp_declined", "dhcp_outbox", "dhcp_receipt",
	} {
		tables = append(tables, snapshotverify.TableCount{Name: name, Rows: counts[name]})
	}
	m := snapshotverify.Manifest{
		Cluster: [16]byte{1}, Leader: [16]byte{2}, Snapshot: [16]byte{3},
		Schema: 2, Anchor: snapshotverify.Position{Epoch: 1}, Encoding: 1,
		RetentionUntilMS: 200, TotalBytes: uint64(len(raw)), Tables: tables,
	}
	var parts []chunk
	if len(raw) > 0 {
		cut := len(raw) / 2
		parts = []chunk{{0, append([]byte(nil), raw[:cut]...)}, {1, append([]byte(nil), raw[cut:]...)}}
	}
	for _, c := range parts {
		m.Chunks = append(m.Chunks, snapshotverify.ChunkSpec{Index: c.index, Size: uint64(len(c.data)), SHA256: sha256.Sum256(c.data)})
	}
	tr := snapshotverify.Trust{Authenticated: true, Cluster: m.Cluster, Leader: m.Leader, Snapshot: m.Snapshot, Anchor: m.Anchor}
	return m, tr, parts
}

// jsonMarshalLine marshals one fixture line (the same shape the eight-table
// helper uses).
func jsonMarshalLine(l line) ([]byte, error) {
	return json.Marshal(l)
}

func TestSchema2PipelineValidatesTheTwelveTableStream(t *testing.T) {
	// Real lease rows: two leases and one declined entry, all in the tenant
	// and space the pool fixture created, addresses inside the pool range.
	leaseLines := []line{
		leaseLine("dhcp_lease", "l1", 105, "client-a", "BOUND", "900"),
		leaseLine("dhcp_lease", "l2", 106, "client-b", "OFFER", "900"),
		leaseLine("dhcp_declined", "d1", 107, "", "client DECLINE", "100"),
	}
	m, tr, chunks := snapshot2(t, leaseLines)

	schema := snapshotipam.New().Tables(nil)
	schema = append(schema, dhcplease.SchemaTables()...)
	if len(schema) != 12 {
		t.Fatalf("the schema-2 stream has %d tables, want 12", len(schema))
	}

	pipeline, err := snapshotpipeline.New(
		m, tr,
		snapshotverify.Limits{MaxBytes: fixtureBytes, MaxChunks: 1024, MaxRows: fixtureRows, Schema: 2},
		snapshotrows.Limits{LineBytes: 2 << 20, TotalBytes: fixtureBytes, Rows: fixtureRows, Tables: 12},
		schema, 100)
	if err != nil {
		t.Fatalf("building the 12-table pipeline: %v", err)
	}
	for _, c := range chunks {
		if err := pipeline.Feed(c.index, c.data, 101); err != nil {
			t.Fatalf("feeding chunk %d: %v", c.index, err)
		}
	}
	result, err := pipeline.Finish(102)
	if err != nil {
		t.Fatalf("finishing the stream: %v", err)
	}
	if !result.Rows.StructureAndCountsValid {
		t.Fatal("the twelve-table structure/counts did not validate")
	}
	if !result.Rows.AllRowCallbacksPassed {
		t.Fatal("not every row passed its frozen-shape validator")
	}
	if result.BusinessComplete {
		t.Fatal("row callbacks alone must never claim business completeness")
	}
}

func TestSchema2PipelineRefusesAnAlienLeaseRow(t *testing.T) {
	leaseLines := []line{
		leaseLine("dhcp_lease", "bad", 105, "client-a", "NAK", "900"), // kind outside the frozen enum
	}
	m, tr, chunks := snapshot2(t, leaseLines)
	schema := snapshotipam.New().Tables(nil)
	schema = append(schema, dhcplease.SchemaTables()...)
	pipeline, err := snapshotpipeline.New(
		m, tr,
		snapshotverify.Limits{MaxBytes: fixtureBytes, MaxChunks: 1024, MaxRows: fixtureRows, Schema: 2},
		snapshotrows.Limits{LineBytes: 2 << 20, TotalBytes: fixtureBytes, Rows: fixtureRows, Tables: 12},
		schema, 100)
	if err != nil {
		t.Fatalf("building the pipeline: %v", err)
	}
	// The alien kind must fail the stream AT FEED TIME — the row callback
	// runs during the feed, and its refusal is the rejection path.
	for _, c := range chunks {
		if err := pipeline.Feed(c.index, c.data, 101); err != nil {
			return // the expected rejection: the feed refused the alien row
		}
	}
	t.Fatal("an alien lease kind must fail the stream validation at feed time")
}

func leaseLine(table, id string, last byte, client, kind, expiry string) line {
	full := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 192, 0, 2, last}
	addr := base64.RawURLEncoding.EncodeToString(full)
	row := map[string]any{}
	switch table {
	case "dhcp_lease":
		row = map[string]any{
			"tenant": "t", "space": "s", "address": addr, "family": "4",
			"lease_id": "lease-" + id, "client_key": base64.RawURLEncoding.EncodeToString([]byte(client)),
			"kind": kind, "offer_xid": "1", "issued_ms": "100", "expires_ms": expiry, "revision": "1",
		}
	case "dhcp_declined":
		row = map[string]any{
			"tenant": "t", "space": "s", "address": addr, "family": "4",
			"reason": kind, "declined_ms": expiry, "revision": "1",
		}
	}
	return line{table, row}
}
