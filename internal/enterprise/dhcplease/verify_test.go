package dhcplease

import (
	"database/sql"
	"encoding/binary"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func openFrozen(t *testing.T) *sql.Tx {
	tx := openFrozenNoInstall(t)
	if err := install(tx); err != nil {
		t.Fatalf("installing frozen ddl: %v", err)
	}
	return tx
}

// openFrozenNoInstall opens a bare transaction on the frozen schema's
// package-private install path: staging (StageLeaseRows) installs the DDL
// itself, mirroring how the eight-table fixture installs its own schema.
func openFrozenNoInstall(t *testing.T) *sql.Tx {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tx.Rollback() })
	return tx
}

func v4(n byte) []byte {
	b := make([]byte, 16)
	// IPv4-mapped form ::ffff:0:0/96 — what the family check classifies as v4.
	b[10], b[11] = 0xff, 0xff
	binary.BigEndian.PutUint32(b[12:], uint32(n))
	return b
}

func v6(n byte) []byte {
	b := make([]byte, 16)
	b[15] = n
	return b
}

func insertLease(t *testing.T, tx *sql.Tx, addr []byte, family int, kind string) {
	t.Helper()
	_, err := tx.Exec(`INSERT INTO dhcp_lease (tenant,space,address,family,lease_id,client_key,kind,offer_xid,issued_ms,expires_ms,revision)
		VALUES ('t','s',?,?,?,?,?,1,100,200,1)`, addr, family, "lease-"+string(kind)+string(addr[len(addr)-1]), []byte("ck"), kind)
	if err != nil {
		t.Fatalf("insert lease (%v %s): %v", addr, kind, err)
	}
}

func TestFrozenDDLInstallsAndAcceptsALease(t *testing.T) {
	tx := openFrozen(t)
	insertLease(t, tx, v4(241), 4, "BOUND")
	if err := Verify(tx); err != nil {
		t.Fatalf("verify on a clean single lease: %v", err)
	}
}

func TestPartialUniqueIndexStopsASecondActiveLeaseForTheSameClient(t *testing.T) {
	tx := openFrozen(t)
	insertLease(t, tx, v4(240), 4, "BOUND")
	if _, err := tx.Exec(`INSERT INTO dhcp_lease (tenant,space,address,family,lease_id,client_key,kind,offer_xid,issued_ms,expires_ms,revision)
		VALUES ('t','s',?,4,'lease-2',?,'BOUND',1,100,200,1)`, v4(241), []byte("ck")); err == nil {
		t.Fatal("the same client_key must not hold two active leases in one space")
	}
	// A terminal kind for the same client is fine (the history is the point).
	if _, err := tx.Exec(`INSERT INTO dhcp_lease (tenant,space,address,family,lease_id,client_key,kind,offer_xid,issued_ms,expires_ms,revision)
		VALUES ('t','s',?,4,'lease-3',?,'RELEASED',1,100,200,1)`, v4(242), []byte("ck")); err != nil {
		t.Fatalf("a terminal-kind row for the same client must be allowed: %v", err)
	}
}

func TestPermanentLeaseCarriesNullExpiry(t *testing.T) {
	tx := openFrozen(t)
	if _, err := tx.Exec(`INSERT INTO dhcp_lease (tenant,space,address,family,lease_id,client_key,kind,offer_xid,issued_ms,expires_ms,revision)
		VALUES ('t','s',?,6,'lease-perm',?,'BOUND',1,100,NULL,1)`, v6(9), []byte("perm")); err != nil {
		t.Fatalf("a permanent lease (NULL expiry) must be accepted: %v", err)
	}
	if err := Verify(tx); err != nil {
		t.Fatalf("verify with a permanent lease: %v", err)
	}
}

func TestExpiryBeforeIssueIsRefusedByDDL(t *testing.T) {
	tx := openFrozen(t)
	if _, err := tx.Exec(`INSERT INTO dhcp_lease (tenant,space,address,family,lease_id,client_key,kind,offer_xid,issued_ms,expires_ms,revision)
		VALUES ('t','s',?,4,'lease-bad',?,'BOUND',1,200,100,1)`, v4(243), []byte("ck")); err == nil {
		t.Fatal("expires_ms < issued_ms must be refused by the frozen CHECK")
	}
}

func TestVerifyCatchesAFamilyMismatchTheDDLCheckCannot(t *testing.T) {
	// The frozen DDL CHECK can only validate family IN (4,6); the relationship
	// between the 16-byte prefix and the family column is what Verify checks.
	tx := openFrozen(t)
	if _, err := tx.Exec(`INSERT INTO dhcp_lease (tenant,space,address,family,lease_id,client_key,kind,offer_xid,issued_ms,expires_ms,revision)
		VALUES ('t','s',?,4,'lease-v4as6',?,'BOUND',1,100,200,1)`, v6(9), []byte("ck")); err != nil {
		t.Fatal(err)
	}
	err := Verify(tx)
	if err == nil || !strings.Contains(err.Error(), "family mismatch") {
		t.Fatalf("Verify must catch a v6 address carrying family=4, got %v", err)
	}
}

func TestVerifyCatchesActiveAndDeclinedOverlap(t *testing.T) {
	tx := openFrozen(t)
	insertLease(t, tx, v4(244), 4, "BOUND")
	if _, err := tx.Exec(`INSERT INTO dhcp_declined (tenant,space,address,family,reason,declined_ms,revision)
		VALUES ('t','s',?,4,'client DECLINE',100,1)`, v4(244)); err != nil {
		t.Fatal(err)
	}
	err := Verify(tx)
	if err == nil || !strings.Contains(err.Error(), "both actively leased and declined") {
		t.Fatalf("Verify must catch the overlap, got %v", err)
	}
}

func TestVerifyAllowsDeclinedWithoutAnActiveLease(t *testing.T) {
	tx := openFrozen(t)
	if _, err := tx.Exec(`INSERT INTO dhcp_declined (tenant,space,address,family,reason,declined_ms,revision)
		VALUES ('t','s',?,4,'client DECLINE',100,1)`, v4(245)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO dhcp_receipt (tenant,space,client_key,xid,kind,digest,reply_kind,address,expires_ms,revision,seq)
		VALUES ('t','s',?,5,'OFFER',?,'OFFER',?,NULL,1,1)`, []byte("ck"), []byte("dg"), v4(245)); err != nil {
		t.Fatal(err)
	}
	if err := Verify(tx); err != nil {
		t.Fatalf("a declined-only address with a receipt is consistent: %v", err)
	}
}

func TestVerifyCatchesNonPositiveOutboxSequence(t *testing.T) {
	tx := openFrozen(t)
	if _, err := tx.Exec(`INSERT INTO dhcp_outbox (cluster_id,epoch,seq,hash,payload)
		VALUES (?,1,0,?,?)`, []byte("cluster"), []byte("h"), []byte("p")); err != nil {
		t.Fatal(err)
	}
	err := Verify(tx)
	if err == nil || !strings.Contains(err.Error(), "non-positive sequence") {
		t.Fatalf("Verify must catch the non-positive outbox seq, got %v", err)
	}
}

func TestVerifyCatchesReceiptKindOutsideTheFrozenEnum(t *testing.T) {
	tx := openFrozen(t)
	if _, err := tx.Exec(`INSERT INTO dhcp_receipt (tenant,space,client_key,xid,kind,digest,reply_kind,address,expires_ms,revision,seq)
		VALUES ('t','s',?,5,'NAK',?,'NAK',?,NULL,1,1)`, []byte("ck"), []byte("dg"), v4(246)); err != nil {
		t.Fatal(err)
	}
	err := Verify(tx)
	if err == nil || !strings.Contains(err.Error(), "outside the frozen enum") {
		t.Fatalf("Verify must catch the alien receipt kind, got %v", err)
	}
}

func TestSchemaVersion2IsDistinctFromTheEightTableCandidate(t *testing.T) {
	if SchemaVersion2 != 2 {
		t.Fatalf("SchemaVersion2 = %d, want 2", SchemaVersion2)
	}
}
