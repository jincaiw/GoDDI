package replicaapply

import (
	"context"
	"database/sql"
	"encoding/binary"
	"strconv"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamaddr"
	"github.com/jincaiw/GoDDI/internal/enterprise/staticbundle"
)

// applyFixture is private and exists only for isolated tests. Validation does
// not establish peer identity or confer write authority.
func (a *testApplier) applyFixture(ctx context.Context, event eventlog.Event) error {
	bundle, err := staticbundle.Validate(event)
	if err != nil {
		a.mu.Lock()
		a.stopped = true
		a.mu.Unlock()
		return err
	}
	return a.run(ctx, bundle.Event(), staticWriter(bundle))
}

func staticWriter(bundle staticbundle.Result) func(context.Context, *sql.Conn) error {
	e := bundle.Event()
	o := bundle.Occupancy()
	audit := bundle.Audit()
	receipt := bundle.Receipt()
	return func(ctx context.Context, conn *sql.Conn) error {
		addr, err := ipamaddr.ParseAddr(o.Address)
		if err != nil {
			return err
		}
		family, encoded, err := ipamaddr.Encode(addr)
		if err != nil {
			return err
		}
		created, err := strconv.ParseInt(o.CreatedAtMS, 10, 64)
		if err != nil {
			return err
		}
		// Identical GIR1 framing to the currently stored idempotency receipt format.
		// This is not a migration or a frozen cross-version persistence contract.
		response := receipt.Receipt.Response
		blob := make([]byte, 8+len(response.ETag)+len(response.Body))
		copy(blob, "GIR1")
		binary.BigEndian.PutUint16(blob[4:6], uint16(response.Status))
		binary.BigEndian.PutUint16(blob[6:8], uint16(len(response.ETag)))
		copy(blob[8:], response.ETag)
		copy(blob[8+len(response.ETag):], response.Body)
		result, err := conn.ExecContext(ctx, `INSERT INTO ipam_occupancies(tenant_id,space_id,id,subnet_id,family,address,kind,owner_ref,created_at_ms) VALUES(?,?,?,?,?,?,'STATIC',?,?)`, o.TenantID, o.SpaceID, o.ID, o.SubnetID, family, encoded[:], o.OwnerRef, created)
		if err != nil {
			return err
		}
		if err := requireOne(result); err != nil {
			return err
		}
		result, err = conn.ExecContext(ctx, `INSERT INTO static_occupancy_audit(cluster_id,epoch,tenant_id,space_id,resource_id,principal,action,request_id,prior_revision,new_revision,result,occurred_at_ms,request_digest) VALUES(?,?,?,?,?,?,'ipam.occupancy.create',?,0,1,'SUCCESS',?,?)`, e.ClusterID[:], int64(e.Position.Epoch), audit.TenantID, audit.SpaceID, audit.ResourceID, audit.Principal, audit.RequestID, audit.OccurredAtMS, audit.RequestDigest[:])
		if err != nil {
			return err
		}
		if err := requireOne(result); err != nil {
			return err
		}
		s := receipt.Scope
		r := receipt.Receipt
		result, err = conn.ExecContext(ctx, `INSERT INTO idempotency_receipt(tenant_id,principal,method,path,idempotency_key,request_digest,response,created_at_unix_ms,expires_at_unix_ms) VALUES(?,?,?,?,?,?,?,?,?)`, s.Tenant[:], s.Principal, s.Method, s.Path, s.Key, receipt.RequestDigest[:], blob, r.CreatedAtMS, r.ExpiresAtMS)
		if err != nil {
			return err
		}
		if err := requireOne(result); err != nil {
			return err
		}
		return nil
	}
}
