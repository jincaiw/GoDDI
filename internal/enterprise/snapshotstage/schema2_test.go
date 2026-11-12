package snapshotstage

// The schema-2 composition: the eight-table IPAM candidate staged by the
// guarded fixture, then the frozen lease table set staged on the same
// database through dhcplease, and the whole twelve-table candidate verified.
// This is the composition the takeover flow needs: one snapshot carries both
// the IPAM state and the lease state, so the cross-package invariants
// (lease addresses inside DHCP pools, reservation↔occupancy) can be checked
// against one another.

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/dhcplease"
)

// v4mapped builds the 16-byte network-order form of an IPv4 address, the
// frozen lease schema's address encoding.
func v4mapped(a, b, c, d byte) []byte {
	out := make([]byte, 16)
	out[10], out[11] = 0xff, 0xff
	out[12], out[13], out[14], out[15] = a, b, c, d
	return out
}

func leaseRowInSpace(tenant, space string, addr []byte, family int, id, client, kind string, expiry any) dhcplease.LeaseRow {
	return dhcplease.LeaseRow{Table: "dhcp_lease", Values: map[string]any{
		"tenant": tenant, "space": space, "address": b64raw(addr), "family": itoa(family),
		"lease_id": "lease-" + id, "client_key": b64raw([]byte(client)),
		"kind": kind, "offer_xid": "1", "issued_ms": "100", "expires_ms": expiry,
		"revision": "1",
	}}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	out := []byte{}
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

func TestSchema2ComposesTheLeaseTableSetOverTheEightTables(t *testing.T) {
	lines := guardedLines() // eight tables + pool4("p","s",100,110,"DHCP") + audit + receipt
	m, tr, chunks := snapshot(t, lines)

	// The lease rows live in the same tenant/space the pool fixture created,
	// with addresses inside that pool's 192.0.2.100–110 range.
	leaseRows := []dhcplease.LeaseRow{
		leaseRowInSpace("t", "s", v4mapped(192, 0, 2, 105), 4, "n1", "client-a", "BOUND", "900"),
		leaseRowInSpace("t", "s", v4mapped(192, 0, 2, 106), 4, "n2", "client-b", "OFFER", "900"),
	}

	var hookErr error
	var candidatePath string
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{
		hardened: true,
		seePath:  func(p string) { candidatePath = p },
		afterVerify: func() error {
			// The lease table set stages AFTER the hardened-structure
			// verification: eventlogguard's reference match is strict about
			// extra tables, so schema-2 lease tables land post-verify.
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
			if e := dhcplease.StageLeaseRows(tx, leaseRows); e != nil {
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
		t.Fatalf("staging the lease table set: %v", hookErr)
	}
	if err != nil {
		t.Fatalf("the guarded eight-table staging itself failed: %v", err)
	}

	// The composed database holds both halves: the eight IPAM tables are
	// intact and the lease tables carry the staged rows.
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
	if leases != len(leaseRows) {
		t.Fatalf("dhcp_lease has %d rows, want %d", leases, len(leaseRows))
	}
	var declined int
	if err = s.conn.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM dhcp_declined`).Scan(&declined); err != nil {
		t.Fatalf("reading dhcp_declined: %v", err)
	}
	if declined != 0 {
		t.Fatalf("dhcp_declined should be empty in this candidate, got %d", declined)
	}
}

func TestSchema2StagingCatchesAnOverlapAcrossTheLeaseTables(t *testing.T) {
	lines := guardedLines()
	m, tr, chunks := snapshot(t, lines)

	leaseRows := []dhcplease.LeaseRow{
		leaseRowInSpace("t", "s", v4mapped(192, 0, 2, 105), 4, "n1", "client-a", "BOUND", "900"),
	}
	// The same address is also DECLINED: dhcplease.Verify must refuse the
	// composed candidate even though each row alone is well-formed.
	leaseRows = append(leaseRows, dhcplease.LeaseRow{Table: "dhcp_declined", Values: map[string]any{
		"tenant": "t", "space": "s", "address": b64raw(v4mapped(192, 0, 2, 105)), "family": "4",
		"reason": "client DECLINE", "declined_ms": "100", "revision": "1",
	}})

	var hookErr error
	var candidatePath string
	_, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{
		hardened: true,
		seePath:  func(p string) { candidatePath = p },
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
			if e := dhcplease.StageLeaseRows(tx, leaseRows); e != nil {
				_ = tx.Rollback()
				hookErr = e
				return e
			}
			return tx.Commit()
		},
	})
	if hookErr == nil {
		t.Fatal("an active-lease↔declined overlap must fail the composed staging")
	}
	if err == nil && hookErr == nil {
		t.Fatal("expected a failure")
	}
	if !strings.Contains(hookErr.Error(), "both actively leased and declined") {
		t.Fatalf("the overlap is what must be reported, got %v", hookErr)
	}
}
