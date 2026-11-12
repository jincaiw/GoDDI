package dhcplease

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
)

func v4mapped(a, b, c, d byte) []byte {
	out := make([]byte, 16)
	out[10], out[11] = 0xff, 0xff
	out[12], out[13], out[14], out[15] = a, b, c, d
	return out
}

// Export and import are the same contract from two sides: rows ExportLeaseRows
// emits must pass validateRow, re-staging them into a fresh database must
// reproduce the same data, and every table must come out in strict BINARY
// primary-key order.
func TestExportRoundTripsThroughStaging(t *testing.T) {
	source := openFrozen(t)
	rows := []LeaseRow{
		leaseRowFor("105", v4mapped(192, 0, 2, 105), 4, "BOUND", "900", "client-a"),
		leaseRowFor("106", v4mapped(192, 0, 2, 106), 4, "OFFER", "900", "client-b"),
	}
	if err := StageLeaseRows(source, rows); err != nil {
		t.Fatalf("staging the source rows: %v", err)
	}
	// The receipt/outbox tables stay empty here; the export must still emit
	// their (zero-row) tables in frozen order.

	exported, err := ExportLeaseRows(context.Background(), source)
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}
	if len(exported) == 0 {
		t.Fatal("the export is empty")
	}

	// 1. Every exported row passes the import-side validator unchanged.
	for i, r := range exported {
		if r.Table == "dhcp_lease" || r.Table == "dhcp_declined" || r.Table == "dhcp_receipt" {
			if _, err := validateRow(r.Table, mustJSON(t, r)); err != nil {
				t.Fatalf("exported row %d (%s) must pass validateRow: %v", i, r.Table, err)
			}
		}
	}

	// 2. Re-staging into a fresh database reproduces the same data.
	target := openFrozen(t)
	if err := StageLeaseRows(target, exported); err != nil {
		t.Fatalf("re-staging the exported rows: %v", err)
	}
	for _, table := range []string{"dhcp_lease", "dhcp_declined", "dhcp_outbox", "dhcp_receipt"} {
		var a, b int
		if err := source.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&a); err != nil {
			t.Fatal(err)
		}
		if err := target.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&b); err != nil {
			t.Fatal(err)
		}
		if a != b {
			t.Fatalf("%s row count differs after round-trip: %d vs %d", table, a, b)
		}
	}
	var addr1, addr2 string
	if err := source.QueryRow(`SELECT hex(address) FROM dhcp_lease WHERE lease_id='lease-105'`).Scan(&addr1); err != nil {
		t.Fatal(err)
	}
	if err := target.QueryRow(`SELECT hex(address) FROM dhcp_lease WHERE lease_id='lease-105'`).Scan(&addr2); err != nil {
		t.Fatal(err)
	}
	if addr1 != addr2 {
		t.Fatalf("the lease address changed across the round-trip: %s vs %s", addr1, addr2)
	}
}

func TestExportIsInStrictBinaryPKOrder(t *testing.T) {
	source := openFrozen(t)
	rows := []LeaseRow{
		leaseRowFor("105", v4mapped(192, 0, 2, 105), 4, "BOUND", "900", "client-a"),
		leaseRowFor("106", v4mapped(192, 0, 2, 106), 4, "OFFER", "900", "client-b"),
	}
	if err := StageLeaseRows(source, rows); err != nil {
		t.Fatal(err)
	}
	// The export must not error: an error here means the ORDER BY collation
	// does not match the frozen BINARY order.
	if _, err := ExportLeaseRows(context.Background(), source); err != nil {
		t.Fatalf("export must be in strict BINARY PK order: %v", err)
	}
}

func mustJSON(t *testing.T, r LeaseRow) []byte {
	t.Helper()
	raw, err := json.Marshal(r.Values)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The exported rows are a valid schema-2 candidate stream: every row passes
// the pipeline feed validators (SchemaTables) unchanged — the export and the
// feed are the same contract seen from the writer and the reader side.
func TestExportedRowsPassTheFeedValidators(t *testing.T) {
	source := openFrozen(t)
	rows := []LeaseRow{
		leaseRowFor("105", v4mapped(192, 0, 2, 105), 4, "BOUND", "900", "client-a"),
		leaseRowFor("106", v4mapped(192, 0, 2, 106), 4, "OFFER", "900", "client-b"),
	}
	if err := StageLeaseRows(source, rows); err != nil {
		t.Fatal(err)
	}
	exported, err := ExportLeaseRows(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	feed := SchemaTables()
	byName := map[string]func([]byte) error{}
	for _, e := range feed {
		byName[e.Name] = e.ValidateRow
	}
	checked := 0
	for _, r := range exported {
		if _, ok := byName[r.Table]; !ok {
			t.Fatalf("exported row names unknown table %q", r.Table)
		}
		if err := byName[r.Table](mustJSON(t, r)); err != nil {
			t.Fatalf("the exported %s row must pass the feed validator: %v", r.Table, err)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no rows were checked")
	}
}

// A scale sanity check: a large lease set exports in strict BINARY order and
// every exported row passes the feed validators. The strict-increase
// assertion inside ExportLeaseRows is the collation proof at scale.
func TestExportHandlesALargeLeaseSet(t *testing.T) {
	source := openFrozen(t)
	const n = 5000
	rows := make([]LeaseRow, 0, n)
	// Addresses 10.0.0.0 + i (bytes 12-15 of the v4-mapped form) — PKs build
	// in numeric order, which is also BINARY byte order for the last octet.
	for i := 0; i < n; i++ {
		a := byte(10)
		b := byte(i >> 8 & 0xff)
		c := byte(i & 0xff)
		// Use distinct last octets across a wide range: bytes 13,14,15 vary.
		addr := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, a, b, c, byte(i%251 + 1)}
		rows = append(rows, LeaseRow{
			Table: "dhcp_lease",
			Values: map[string]any{
				"tenant": "t", "space": "s",
				"address": base64.RawURLEncoding.EncodeToString(addr), "family": "4",
				"lease_id":   fmt.Sprintf("lease-%05d", i),
				"client_key": base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("client-%05d", i))),
				"kind":       "BOUND", "offer_xid": "1", "issued_ms": "100",
				"expires_ms": "900", "revision": "1",
			},
		})
	}
	if err := StageLeaseRows(source, rows); err != nil {
		t.Fatalf("staging %d rows: %v", n, err)
	}
	exported, err := ExportLeaseRows(context.Background(), source)
	if err != nil {
		t.Fatalf("exporting %d rows: %v", n, err)
	}
	var leaseExported int
	feed := SchemaTables()
	byName := map[string]func([]byte) error{}
	for _, e := range feed {
		byName[e.Name] = e.ValidateRow
	}
	for _, r := range exported {
		if r.Table != "dhcp_lease" {
			continue
		}
		leaseExported++
		if err := byName[r.Table](mustJSON(t, r)); err != nil {
			t.Fatalf("exported lease row must pass the feed validator: %v", err)
		}
	}
	if leaseExported != n {
		t.Fatalf("exported %d lease rows, want %d", leaseExported, n)
	}
}
