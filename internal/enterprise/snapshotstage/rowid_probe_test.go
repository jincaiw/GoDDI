package snapshotstage

import (
	"context"
	"database/sql"
	"testing"
)

// This isolated probe documents a limitation of the existing eventlog DDL.
// It does not modify production data or prove a production exploit path.
func TestReceiptRowIDReplaceIsDetectedBySourceComparison(t *testing.T) {
	tenant := [16]byte{7}
	lines := append(sampleLines(), receiptRow(tenant, []byte("old"), []byte("tag")))
	m, tr, chunks := snapshot(t, lines)
	var conn *sql.Conn
	replaced := false
	h := fixtureHooks{beforeInsert: func(c *sql.Conn) error { conn = c; return nil }, afterCommit: func() error {
		if _, err := conn.ExecContext(context.Background(), `PRAGMA recursive_triggers=OFF`); err != nil {
			return err
		}
		result, err := conn.ExecContext(context.Background(), `INSERT OR REPLACE INTO main.idempotency_receipt(rowid,tenant_id,principal,method,path,idempotency_key,request_digest,response,created_at_unix_ms,expires_at_unix_ms) SELECT rowid,tenant_id,principal,method,path,'66666666-6666-4666-8666-666666666666',request_digest,response,created_at_unix_ms,expires_at_unix_ms FROM main.idempotency_receipt`)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		replaced = n == 1
		return err
	}}
	stage, err := importFixture(t, context.Background(), m, tr, node(), chunks, h)
	if !replaced || err == nil || stage != nil {
		t.Fatalf("rowid probe replaced=%v err=%v", replaced, err)
	}
}
