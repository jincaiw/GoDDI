// Package staticbundle validates an isolated candidate replication bundle. It
// neither authenticates a peer nor establishes a chain position or applies data.
package staticbundle

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/enterprise/auditwire"
	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/ipamevent"
	"github.com/jasonwa/goddi/internal/enterprise/ipamrequest"
	"github.com/jasonwa/goddi/internal/enterprise/ipamresponse"
	"github.com/jasonwa/goddi/internal/enterprise/receiptwire"
)

var ErrInvalid = errors.New("staticbundle: invalid STATIC create bundle")

// Result owns all its data. Accessors return values or independent copies.
// Successful validation does not authenticate, authorize or apply the event.
type Result struct {
	event     eventlog.Event
	occupancy ipamevent.Occupancy
	audit     auditwire.Audit
	receipt   receiptwire.Value
}

func (r Result) Event() eventlog.Event {
	e := r.event
	e.Payload = append([]byte(nil), e.Payload...)
	return e
}
func (r Result) Occupancy() ipamevent.Occupancy {
	o := r.occupancy
	if o.ExpiresAtMS != nil {
		v := *o.ExpiresAtMS
		o.ExpiresAtMS = &v
	}
	return o
}
func (r Result) Audit() auditwire.Audit { return r.audit }
func (r Result) Receipt() receiptwire.Value {
	v := r.receipt
	v.Receipt.Response.Body = append([]byte(nil), v.Receipt.Response.Body...)
	return v
}

// Validate copies and verifies the original bytes and the three ordered changes.
// Callers must not mutate e.Payload concurrently while Validate is running.
func Validate(e eventlog.Event) (Result, error) {
	var zero Result
	if len(e.Payload) == 0 || len(e.Payload) > eventlog.MaxPayloadBytes {
		return zero, ErrInvalid
	}
	e.Payload = append([]byte(nil), e.Payload...)
	if err := e.Verify(); err != nil {
		return zero, fmt.Errorf("%w: event verification: %v", ErrInvalid, err)
	}
	if e.ClusterID == (eventlog.ID{}) || e.LeaderID == (eventlog.ID{}) || e.ClusterID == e.LeaderID {
		return zero, fmt.Errorf("%w: identities", ErrInvalid)
	}
	changes, err := eventlog.DecodePayload(e.Payload)
	if err != nil || len(changes) != 3 {
		return zero, fmt.Errorf("%w: expected three changes", ErrInvalid)
	}
	if changes[0].Kind != ipamevent.Set || changes[1].Kind != "ipam.occupancy.audit" || changes[2].Kind != "idempotency.receipt.set" {
		return zero, fmt.Errorf("%w: order or kind", ErrInvalid)
	}
	for _, c := range changes {
		if c.Revision != "1" {
			return zero, fmt.Errorf("%w: revision", ErrInvalid)
		}
	}
	// ipamevent is a single-domain payload validator. This adapter is used
	// solely for semantic validation; event verification above used raw bytes.
	single, err := eventlog.MarshalChanges(changes[0])
	if err != nil {
		return zero, fmt.Errorf("%w: occupancy envelope", ErrInvalid)
	}
	decoded, err := ipamevent.DecodePayload(single)
	if err != nil || len(decoded) != 1 {
		return zero, fmt.Errorf("%w: occupancy", ErrInvalid)
	}
	o := decoded[0].Occupancy
	if o.Kind != "STATIC" || o.Revision != "1" || o.ExpiresAtMS != nil {
		return zero, fmt.Errorf("%w: not STATIC create", ErrInvalid)
	}
	a, err := auditwire.Decode(changes[1].Body)
	if err != nil {
		return zero, fmt.Errorf("%w: audit", ErrInvalid)
	}
	r, err := receiptwire.Decode(changes[2].Body)
	if err != nil {
		return zero, fmt.Errorf("%w: receipt", ErrInvalid)
	}
	tenant, err := uuid.Parse(o.TenantID)
	if err != nil || [16]byte(tenant) != r.Scope.Tenant || a.TenantID != o.TenantID || a.SpaceID != o.SpaceID || a.ResourceID != o.ID ||
		a.ClusterID != e.ClusterID.String() || uint64(a.Epoch) != e.Position.Epoch || a.Principal != r.Scope.Principal ||
		changes[1].ObjectID != a.RequestID || changes[2].ObjectID != r.Scope.Key {
		return zero, fmt.Errorf("%w: identity mismatch", ErrInvalid)
	}
	path := "/api/v1/ddi/tenants/" + o.TenantID + "/ipam/spaces/" + o.SpaceID + "/allocations"
	if r.Scope.Method != "POST" || r.Scope.Path != path {
		return zero, fmt.Errorf("%w: request scope", ErrInvalid)
	}
	_, digest, err := ipamrequest.Normalize(ipamrequest.Request{TenantID: o.TenantID, SpaceID: o.SpaceID, SubnetID: o.SubnetID, Address: o.Address, OwnerRef: o.OwnerRef})
	if err != nil || digest != r.RequestDigest || digest != a.RequestDigest {
		return zero, fmt.Errorf("%w: request digest", ErrInvalid)
	}
	response, err := ipamresponse.Static(o.ID, o.SubnetID, o.Address, o.OwnerRef)
	if err != nil || response.Status != r.Receipt.Response.Status || response.ETag != r.Receipt.Response.ETag || !bytes.Equal(response.Body, r.Receipt.Response.Body) {
		return zero, fmt.Errorf("%w: response", ErrInvalid)
	}
	created, err := strconv.ParseInt(o.CreatedAtMS, 10, 64)
	// One trusted snapshot supplies the row, audit, and receipt creation time.
	// The commit uses that same instant; the receipt lasts at least seven days.
	if err != nil || created <= 0 || a.OccurredAtMS != created || r.Receipt.CreatedAtMS != created || e.CommitMs != created || r.Receipt.ExpiresAtMS-created < eventlog.StaticReceiptMinRetentionMS {
		return zero, fmt.Errorf("%w: inconsistent time", ErrInvalid)
	}
	return Result{event: e, occupancy: o, audit: a, receipt: r}, nil
}
