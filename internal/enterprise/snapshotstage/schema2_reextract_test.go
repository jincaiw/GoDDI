package snapshotstage

// The pipeline-level re-extract drill: from a composed twelve-table candidate,
// export BOTH halves (eight IPAM tables through snapshotipam.ExportRows, four
// lease tables through dhcplease.ExportLeaseRows), rebuild the NDJSON stream
// and its manifest, and validate the whole thing through a FRESH pipeline —
// without the original database. This is the "re-extract from the trusted
// source" step of the snapshot migration path, proven end to end.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/dhcplease"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotipam"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotpipeline"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotrows"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotverify"
)

func TestReextractFromTheComposedCandidateValidates(t *testing.T) {
	leaseLines := leaseFixtureLines()
	m, tr, chunks := snapshot2(t, leaseLines)
	var captured []dhcplease.LeaseRow
	var hookErr error
	var candidatePath string
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{
		hardened:      true,
		schema2:       true,
		LeaseSinkRows: &captured,
		seePath:       func(p string) { candidatePath = p },
		afterVerify: func() error {
			db, e := sql.Open("sqlite", candidatePath)
			if e != nil {
				return e
			}
			defer db.Close()
			tx, e := db.Begin()
			if e != nil {
				return e
			}
			if e := dhcplease.InstallFrozenDDL(tx); e != nil {
				_ = tx.Rollback()
				hookErr = e
				return e
			}
			if e := dhcplease.StageLeaseRows(tx, captured); e != nil {
				_ = tx.Rollback()
				hookErr = e
				return e
			}
			if e := dhcplease.Verify(tx); e != nil {
				_ = tx.Rollback()
				hookErr = e
				return e
			}
			return tx.Commit()
		},
	})
	if hookErr != nil {
		t.Fatalf("staging the lease rows: %v", hookErr)
	}
	if err != nil {
		t.Fatalf("the composed staging failed: %v", err)
	}

	// 1. Export both halves from the composed candidate.
	ipamRows, err := snapshotipam.ExportRows(context.Background(), s.conn)
	if err != nil {
		t.Fatalf("exporting the eight IPAM tables: %v", err)
	}
	var leaseRows []dhcplease.LeaseRow
	tx, err := s.conn.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	leaseRows, err = dhcplease.ExportLeaseRows(context.Background(), tx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if len(leaseRows) == 0 {
		t.Fatal("the lease export is empty")
	}

	// 2. Rebuild the NDJSON stream from the exported rows.
	type wireRow struct {
		Table string         `json:"table"`
		Row   map[string]any `json:"row"`
	}
	var lines []line
	counts := map[string]uint64{}
	appendRow := func(table string, payload map[string]any) {
		lines = append(lines, line{table, payload})
		counts[table]++
	}
	for _, r := range ipamRows {
		var obj map[string]any
		if err := json.Unmarshal(r.Payload, &obj); err != nil {
			t.Fatal(err)
		}
		appendRow(r.Table, obj)
	}
	for _, r := range leaseRows {
		appendRow(r.Table, r.Values)
	}
	var raw []byte
	for _, l := range lines {
		b, e := json.Marshal(wireRow(l))
		if e != nil {
			t.Fatal(e)
		}
		raw = append(raw, b...)
		raw = append(raw, '\n')
	}

	// 3. The fresh pipeline: twelve tables in frozen order, Schema 2.
	schema := snapshotipam.New().Tables(nil)
	schema = append(schema, dhcplease.SchemaTables()...)
	tables := make([]snapshotverify.TableCount, 0, 12)
	for _, name := range []string{
		"ipam_tenants", "ipam_spaces", "ipam_prefixes", "ipam_pools",
		"ipam_occupancies", "ipam_resource_ids", "static_occupancy_audit",
		"idempotency_receipt", "dhcp_lease", "dhcp_declined", "dhcp_outbox", "dhcp_receipt",
	} {
		tables = append(tables, snapshotverify.TableCount{Name: name, Rows: counts[name]})
	}
	fresh := snapshotverify.Manifest{
		Cluster: [16]byte{1}, Leader: [16]byte{2}, Snapshot: [16]byte{5},
		Schema: 2, Anchor: snapshotverify.Position{Epoch: 1}, Encoding: 1,
		RetentionUntilMS: 200, TotalBytes: uint64(len(raw)), Tables: tables,
	}
	var parts []chunk
	if len(raw) > 0 {
		cut := len(raw) / 2
		parts = []chunk{{0, append([]byte(nil), raw[:cut]...)}, {1, append([]byte(nil), raw[cut:]...)}}
	}
	for _, c := range parts {
		fresh.Chunks = append(fresh.Chunks, snapshotverify.ChunkSpec{Index: c.index, Size: uint64(len(c.data)), SHA256: sha256.Sum256(c.data)})
	}
	tr2 := snapshotverify.Trust{Authenticated: true, Cluster: fresh.Cluster, Leader: fresh.Leader, Snapshot: fresh.Snapshot, Anchor: fresh.Anchor}
	pipeline, err := snapshotpipeline.New(
		fresh, tr2,
		snapshotverify.Limits{MaxBytes: fixtureBytes, MaxChunks: 1024, MaxRows: fixtureRows, Schema: 2},
		snapshotrows.Limits{LineBytes: 2 << 20, TotalBytes: fixtureBytes, Rows: fixtureRows, Tables: 12},
		schema, 100)
	if err != nil {
		t.Fatalf("building the fresh pipeline: %v", err)
	}
	for _, c := range parts {
		if err := pipeline.Feed(c.index, c.data, 101); err != nil {
			t.Fatalf("feeding chunk %d: %v", c.index, err)
		}
	}
	result, err := pipeline.Finish(102)
	if err != nil {
		t.Fatalf("finishing the re-extracted stream: %v", err)
	}
	if !result.Rows.StructureAndCountsValid || !result.Rows.AllRowCallbacksPassed {
		t.Fatalf("the re-extracted stream did not validate: structure=%v callbacks=%v",
			result.Rows.StructureAndCountsValid, result.Rows.AllRowCallbacksPassed)
	}

	// Sanity: the re-extracted stream carries the lease rows.
	var leaseCount int
	for _, t := range tables {
		if t.Name == "dhcp_lease" {
			leaseCount = int(t.Rows)
		}
	}
	if leaseCount != 2 {
		t.Fatalf("the re-extracted stream carries %d lease rows, want 2", leaseCount)
	}
}
