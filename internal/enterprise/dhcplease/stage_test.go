package dhcplease

import (
	"encoding/base64"
	"strings"
	"testing"
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func leaseRow(addr []byte, family int, kind string, expiry any) LeaseRow {
	return leaseRowFor(string(addr[len(addr)-1]), addr, family, kind, expiry, "ck")
}

func leaseRowFor(id string, addr []byte, family int, kind string, expiry any, client string) LeaseRow {
	return LeaseRow{Table: "dhcp_lease", Values: map[string]any{
		"tenant": "t", "space": "s", "address": b64(addr), "family": fmtInt(family),
		"lease_id": "lease-" + id, "client_key": b64([]byte(client)),
		"kind": kind, "offer_xid": "1", "issued_ms": "100", "expires_ms": expiry,
		"revision": "1",
	}}
}

func fmtInt(n int) string {
	digits := []byte("0123456789")
	if n == 0 {
		return "0"
	}
	out := []byte{}
	for n > 0 {
		out = append([]byte{digits[n%10]}, out...)
		n /= 10
	}
	return string(out)
}

func TestStageLeaseRowsInstallsAndVerifies(t *testing.T) {
	tx := openFrozen(t)
	// Distinct clients: the frozen partial unique index refuses two ACTIVE
	// leases for one client in one space — the test rows must respect that.
	rows := []LeaseRow{
		leaseRowFor("241", v4(241), 4, "BOUND", "200", "client-a"),
		leaseRowFor("242", v4(242), 4, "OFFER", "250", "client-b"),
	}
	if err := StageLeaseRows(tx, rows); err != nil {
		t.Fatalf("staging two lease rows: %v", err)
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM dhcp_lease`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("dhcp_lease has %d rows, want 2", n)
	}
}

func TestStageLeaseRowsAcceptsPermanentAndDeclinedAndReceipt(t *testing.T) {
	tx := openFrozen(t)
	rows := []LeaseRow{
		leaseRow(v6(9), 6, "BOUND", nil), // permanent (decision 3)
		{Table: "dhcp_declined", Values: map[string]any{
			"tenant": "t", "space": "s", "address": b64(v4(250)), "family": "4",
			"reason": "client DECLINE", "declined_ms": "100", "revision": "1",
		}},
		{Table: "dhcp_receipt", Values: map[string]any{
			"tenant": "t", "space": "s", "client_key": b64([]byte("ck")), "xid": "5",
			"kind": "OFFER", "digest": b64([]byte("dg")), "reply_kind": "OFFER",
			"address": b64(v4(250)), "expires_ms": nil, "revision": "1", "seq": "1",
		}},
	}
	if err := StageLeaseRows(tx, rows); err != nil {
		t.Fatalf("staging permanent + declined + receipt: %v", err)
	}
}

func TestStageLeaseRowsRefusesAnUnknownTable(t *testing.T) {
	tx := openFrozen(t)
	err := StageLeaseRows(tx, []LeaseRow{{Table: "dhcp_else", Values: map[string]any{}}})
	if err == nil || !strings.Contains(err.Error(), "not in the frozen lease set") {
		t.Fatalf("an unknown table must be refused, got %v", err)
	}
}

func TestStageLeaseRowsRefusesAnUnknownColumn(t *testing.T) {
	tx := openFrozen(t)
	r := leaseRow(v4(241), 4, "BOUND", "200")
	r.Values["scope_extra"] = "x"
	err := StageLeaseRows(tx, []LeaseRow{r})
	if err == nil || !strings.Contains(err.Error(), "unknown column") {
		t.Fatalf("an unknown column must be refused, got %v", err)
	}
}

func TestStageLeaseRowsRefusesANonDecimalInteger(t *testing.T) {
	tx := openFrozen(t)
	r := leaseRow(v4(241), 4, "BOUND", "200")
	r.Values["revision"] = "not-a-number"
	err := StageLeaseRows(tx, []LeaseRow{r})
	if err == nil || !strings.Contains(err.Error(), "not a decimal string") {
		t.Fatalf("a non-decimal integer must be refused, got %v", err)
	}
}

func TestStageLeaseRowsRejectsTheStaleOverlapViaVerify(t *testing.T) {
	tx := openFrozen(t)
	rows := []LeaseRow{
		leaseRow(v4(244), 4, "BOUND", "200"),
		{Table: "dhcp_declined", Values: map[string]any{
			"tenant": "t", "space": "s", "address": b64(v4(244)), "family": "4",
			"reason": "client DECLINE", "declined_ms": "100", "revision": "1",
		}},
	}
	err := StageLeaseRows(tx, rows)
	if err == nil || !strings.Contains(err.Error(), "both actively leased and declined") {
		t.Fatalf("the staged pair must fail Verify on the overlap, got %v", err)
	}
}
