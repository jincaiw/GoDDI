package snapshotstage

// The guarded schema-2 staging end-to-end: the twelve-table NDJSON stream
// (eight IPAM tables + the four frozen lease tables) through the guarded
// staging machinery with h.schema2 — pipeline validation, the lease rows
// captured through the stream, staged into the candidate via
// dhcplease.StageLeaseRows in the afterVerify hook, and the whole
// twelve-table candidate verified in one database.
//
// This closes the gap between the standalone pipeline test (schema2_pipeline_test.go)
// and the DB-level composition (schema2_test.go): the guarded staging now runs
// the full schema-2 flow natively.

import (
	"context"
	"database/sql"
	"encoding/base64"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/dhcplease"
)

func leaseFixtureLines() []line {
	addr := func(n byte) string { return base64.RawURLEncoding.EncodeToString(v4mapped(192, 0, 2, n)) }
	ck := base64.RawURLEncoding.EncodeToString([]byte("client-x"))
	rows := []line{
		{"dhcp_lease", map[string]any{
			"tenant": "t", "space": "s", "address": addr(105), "family": "4",
			"lease_id": "lease-schema2-1", "client_key": ck, "kind": "BOUND",
			"offer_xid": "1", "issued_ms": "100", "expires_ms": "900", "revision": "1",
		}},
		{"dhcp_lease", map[string]any{
			"tenant": "t", "space": "s", "address": addr(106), "family": "4",
			"lease_id":   "lease-schema2-2",
			"client_key": base64.RawURLEncoding.EncodeToString([]byte("client-y")),
			"kind":       "OFFER", "offer_xid": "2", "issued_ms": "100", "expires_ms": "900",
			"revision": "1",
		}},
		{"dhcp_declined", map[string]any{
			"tenant": "t", "space": "s", "address": addr(107), "family": "4",
			"reason": "client DECLINE", "declined_ms": "100", "revision": "1",
		}},
		{"dhcp_outbox", map[string]any{
			"cluster_id": base64.RawURLEncoding.EncodeToString([]byte("cluster-1")),
			"epoch":      "1", "seq": "1",
			"hash":    base64.RawURLEncoding.EncodeToString([]byte("hash-1")),
			"payload": base64.RawURLEncoding.EncodeToString([]byte("payload-1")),
		}},
		{"dhcp_receipt", map[string]any{
			"tenant": "t", "space": "s", "client_key": ck, "xid": "7", "kind": "OFFER",
			"digest":     base64.RawURLEncoding.EncodeToString([]byte("digest-1")),
			"reply_kind": "OFFER", "address": addr(105), "expires_ms": nil,
			"revision": "1", "seq": "1",
		}},
	}
	return rows
}

func TestGuardedSchema2StagingTwelveTables(t *testing.T) {
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
			// The lease table set stages AFTER the hardened-structure
			// verification: eventlogguard's reference match is strict about
			// extra tables, so schema-2 lease tables land post-verify. The
			// captured rows apply here and the composed candidate verifies.
			db, e := sql.Open("sqlite", candidatePath)
			if e != nil {
				return e
			}
			defer db.Close()
			var tableNames []string
			tr2, e := db.Begin()
			if e != nil {
				return e
			}
			trows, e := tr2.Query(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
			if e != nil {
				return e
			}
			for trows.Next() {
				var nm string
				if e := trows.Scan(&nm); e != nil {
					trows.Close()
					return e
				}
				tableNames = append(tableNames, nm)
			}
			trows.Close()
			tr2.Rollback()
			t.Logf("DEBUG tables before install: %v", tableNames)
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
		t.Fatalf("the guarded staging failed: %v", err)
	}
	// DEBUG
	for _, tc := range m.Tables {
		t.Logf("DEBUG manifest table %s rows=%d", tc.Name, tc.Rows)
	}

	// The composed database holds both halves.
	var ipamSpaces int
	if err = s.conn.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM ipam_spaces`).Scan(&ipamSpaces); err != nil {
		t.Fatalf("reading ipam_spaces: %v", err)
	}
	if ipamSpaces == 0 {
		t.Fatal("the eight-table half is missing from the composed candidate")
	}
	var leases int
	if err = s.conn.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM dhcp_lease`).Scan(&leases); err != nil {
		t.Fatalf("reading dhcp_lease: %v", err)
	}
	if leases != 2 {
		t.Fatalf("dhcp_lease has %d rows, want 2", leases)
	}
	var outbox, receipts, declined int
	if err = s.conn.QueryRowContext(context.Background(),
		`SELECT (SELECT COUNT(*) FROM dhcp_outbox),(SELECT COUNT(*) FROM dhcp_receipt),(SELECT COUNT(*) FROM dhcp_declined)`).
		Scan(&outbox, &receipts, &declined); err != nil {
		t.Fatal(err)
	}
	if outbox != 1 || receipts != 1 || declined != 1 {
		t.Fatalf("outbox=%d receipts=%d declined=%d, want 1/1/1", outbox, receipts, declined)
	}
}

func TestGuardedSchema2StagingRejectsAnAlienLeaseRow(t *testing.T) {
	bad := leaseFixtureLines()[0]
	bad.Row["kind"] = "NAK" // outside the frozen enum
	leaseLines := append([]line{bad}, leaseFixtureLines()[1:]...)
	m, tr, chunks := snapshot2(t, leaseLines)

	var captured []dhcplease.LeaseRow
	_, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{
		hardened:      true,
		schema2:       true,
		LeaseSinkRows: &captured,
	})
	if err == nil {
		t.Fatal("an alien lease kind must fail the guarded schema-2 staging")
	}
}
