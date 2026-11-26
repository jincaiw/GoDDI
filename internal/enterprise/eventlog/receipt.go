package eventlog

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"math"

	"github.com/jincaiw/GoDDI/internal/enterprise/idempotency"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamresponse"
)

// StaticReceiptMinRetentionMS is the candidate seven-day pilot minimum.
// A deployment may require a longer window but must not shorten its declared
// guarantee. The caller remains responsible for providing trusted time.
const StaticReceiptMinRetentionMS int64 = 7 * 24 * 60 * 60 * 1000

type receiptState struct {
	scope    idempotency.Scope
	digest   [32]byte
	replay   bool
	inserted bool
	result   idempotency.Receipt
}

// Receipt methods use fixed SQL on the pinned connection. A lookup binds this
// transaction to one scope and digest; a replay cannot append another event.
func (t *WriteTx) LookupReceipt(ctx context.Context, scope idempotency.Scope, digest [32]byte) (idempotency.Receipt, bool, error) {
	if t.dirty {
		return idempotency.Receipt{}, false, sql.ErrTxDone
	}
	if t.receipt != nil || t.appended != nil {
		t.dirty = true
		return idempotency.Receipt{}, false, fmt.Errorf("eventlog: receipt lookup must occur once before appending")
	}
	r, found, err := idempotency.LookupBackend(ctx, receiptBackend{t}, scope, digest)
	if err != nil {
		t.dirty = true
		return r, found, err
	}
	t.receipt = &receiptState{scope: scope, digest: digest, replay: found}
	return r, found, nil
}
func (t *WriteTx) InsertReceipt(ctx context.Context, scope idempotency.Scope, digest [32]byte, receipt idempotency.Receipt) error {
	if t.dirty {
		return sql.ErrTxDone
	}
	if t.receipt == nil || t.receipt.replay || t.receipt.inserted || t.receipt.scope != scope || t.receipt.digest != digest {
		t.dirty = true
		return fmt.Errorf("eventlog: receipt does not match its initial lookup")
	}
	if t.static != nil {
		created := t.static.CreatedAtMS
		if receipt.CreatedAtMS != created || created <= 0 || created > math.MaxInt64-StaticReceiptMinRetentionMS || receipt.ExpiresAtMS < created+StaticReceiptMinRetentionMS {
			t.dirty = true
			return fmt.Errorf("eventlog: invalid STATIC receipt retention or transaction time")
		}
		address, err := t.static.addressString()
		if err != nil {
			t.dirty = true
			return err
		}
		expected, err := ipamresponse.Static(t.static.ID, t.static.SubnetID, address, t.static.OwnerRef)
		if err != nil || receipt.Response.Status != expected.Status || receipt.Response.ETag != expected.ETag || !bytes.Equal(receipt.Response.Body, expected.Body) {
			t.dirty = true
			return fmt.Errorf("eventlog: receipt response does not match STATIC result")
		}
	}
	err := idempotency.InsertBackend(ctx, receiptBackend{t}, scope, digest, receipt)
	if err != nil {
		t.dirty = true
		return err
	}
	t.receipt.inserted = true
	t.receipt.result = receipt
	t.receipt.result.Response.Body = append([]byte(nil), receipt.Response.Body...)
	return nil
}

type receiptBackend struct{ tx *WriteTx }

func (b receiptBackend) ReadReceipt(ctx context.Context, s idempotency.Scope) (idempotency.Stored, error) {
	var r idempotency.Stored
	var size, created, expires sql.NullInt64
	const limit = 8 + idempotency.MaxETagBytes + idempotency.MaxBodyBytes
	err := b.tx.conn.QueryRowContext(ctx, `SELECT
 CASE WHEN typeof(request_digest)='blob' AND length(request_digest)=32 THEN request_digest END,
 length(response), CASE WHEN typeof(response)='blob' AND length(response)<=? THEN response END,
 CASE WHEN typeof(created_at_unix_ms)='integer' THEN created_at_unix_ms END,
 CASE WHEN typeof(expires_at_unix_ms)='integer' THEN expires_at_unix_ms END
 FROM idempotency_receipt WHERE tenant_id=? AND principal=? AND method=? AND path=? AND idempotency_key=?`, limit, s.Tenant[:], s.Principal, s.Method, s.Path, s.Key).Scan(&r.Digest, &size, &r.Response, &created, &expires)
	if err != nil {
		return r, err
	}
	if len(r.Digest) != 32 || !size.Valid || size.Int64 > limit || int64(len(r.Response)) != size.Int64 || r.Response == nil || !created.Valid || !expires.Valid {
		return r, idempotency.ErrCorrupt
	}
	r.CreatedAtMS, r.ExpiresAtMS = created.Int64, expires.Int64
	return r, nil
}
func (b receiptBackend) CreateReceipt(ctx context.Context, s idempotency.Scope, r idempotency.Stored) error {
	res, err := b.tx.conn.ExecContext(ctx, `INSERT INTO idempotency_receipt(tenant_id,principal,method,path,idempotency_key,request_digest,response,created_at_unix_ms,expires_at_unix_ms) VALUES(?,?,?,?,?,?,?,?,?)`, s.Tenant[:], s.Principal, s.Method, s.Path, s.Key, r.Digest, r.Response, r.CreatedAtMS, r.ExpiresAtMS)
	if err != nil {
		return fmt.Errorf("eventlog: insert receipt: %w", err)
	}
	if err := requireOneInserted(res); err != nil {
		return fmt.Errorf("eventlog: insert receipt: %w", err)
	}
	return nil
}

var _ idempotency.Backend = receiptBackend{}
