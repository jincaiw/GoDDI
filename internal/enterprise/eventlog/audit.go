package eventlog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var ErrStaticAudit = errors.New("eventlog: invalid static occupancy audit")

// StaticAudit describes the success of a STATIC create. The caller supplies an
// authenticated principal and trusted timestamp; this method checks their
// consistency with this transaction, not their independent authenticity.
// Audit replication apply and export are not implemented; this grants no role.
type StaticAudit struct {
	TenantID, SpaceID, ResourceID, Principal string
	Action, RequestID, Result                string
	PriorRevision, NewRevision               int64
	OccurredAtMS                             int64
}

// InsertStaticAudit records one local audit in the same transaction. Only a
// successfully inserted STATIC row and its bound receipt may be audited.
func (t *WriteTx) InsertStaticAudit(ctx context.Context, a StaticAudit) error {
	if t.dirty {
		return sql.ErrTxDone
	}
	if t.static == nil || t.receipt == nil || t.receipt.replay || t.audited {
		t.dirty = true
		return ErrStaticAudit
	}
	o := t.static
	req, err := uuid.Parse(a.RequestID)
	if err != nil || req == uuid.Nil || req.String() != a.RequestID ||
		a.TenantID != o.TenantID || a.SpaceID != o.SpaceID || a.ResourceID != o.ID ||
		a.Principal != t.receipt.scope.Principal || a.Action != "ipam.occupancy.create" ||
		a.Result != "SUCCESS" || a.PriorRevision != 0 || a.NewRevision != 1 || a.OccurredAtMS != o.CreatedAtMS {
		t.dirty = true
		return ErrStaticAudit
	}
	res, err := t.conn.ExecContext(ctx, `INSERT INTO static_occupancy_audit(cluster_id,epoch,tenant_id,space_id,resource_id,principal,action,request_id,prior_revision,new_revision,result,occurred_at_ms,request_digest) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.log.cluster[:], int64(t.epoch), a.TenantID, a.SpaceID, a.ResourceID, a.Principal, a.Action, a.RequestID, a.PriorRevision, a.NewRevision, a.Result, a.OccurredAtMS, t.receipt.digest[:])
	if err != nil {
		t.dirty = true
		return fmt.Errorf("eventlog: insert local audit: %w", err)
	}
	if err := requireOneInserted(res); err != nil {
		t.dirty = true
		return fmt.Errorf("eventlog: insert local audit: %w", err)
	}
	t.audited = true
	copy := a
	t.audit = &copy
	return nil
}
