package eventlog

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"

	"github.com/google/uuid"
	"github.com/jincaiw/GoDDI/internal/enterprise/auditwire"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamrequest"
	"github.com/jincaiw/GoDDI/internal/enterprise/receiptwire"
)

var ErrStaticOccupancy = errors.New("eventlog: invalid static occupancy transaction")

// StaticOccupancy is an already authorized, explicitly selected address.
// CreatedAtMS must come from the caller's verified trusted-time source; this
// method does not establish clock health, authority, or DHCP lease safety.
type StaticOccupancy struct {
	TenantID, SpaceID, SubnetID, ID string
	Family                          int
	Address                         [16]byte
	OwnerRef                        string
	CreatedAtMS                     int64
}

func (o StaticOccupancy) addressString() (string, error) {
	switch o.Family {
	case 4:
		if !bytes.Equal(o.Address[:12], make([]byte, 12)) {
			return "", ErrStaticOccupancy
		}
		return netip.AddrFrom4([4]byte(o.Address[12:16])).String(), nil
	case 6:
		addr := netip.AddrFrom16(o.Address)
		if addr.Is4In6() {
			return "", ErrStaticOccupancy
		}
		return addr.String(), nil
	default:
		return "", ErrStaticOccupancy
	}
}
func (o StaticOccupancy) validate() error {
	for _, s := range []string{o.TenantID, o.SpaceID, o.SubnetID, o.ID} {
		id, err := uuid.Parse(s)
		if err != nil || id == uuid.Nil || id.String() != s {
			return ErrStaticOccupancy
		}
	}
	if _, err := o.addressString(); err != nil {
		return err
	}
	if o.CreatedAtMS < 0 || len(o.OwnerRef) == 0 || len(o.OwnerRef) > 1024 {
		return ErrStaticOccupancy
	}
	for i := range o.OwnerRef {
		if o.OwnerRef[i] < 0x20 || o.OwnerRef[i] > 0x7e {
			return ErrStaticOccupancy
		}
	}
	return nil
}

// InsertStatic inserts exactly one STATIC occupancy after a bound receipt miss.
// Database constraints enforce scope, prefix containment and address uniqueness.
// Production audit, authorization, time and active lease checks are not supplied.
func (t *WriteTx) InsertStatic(ctx context.Context, o StaticOccupancy) error {
	if t.dirty {
		return sql.ErrTxDone
	}
	if t.receipt == nil || t.receipt.replay || t.receipt.inserted || t.appended != nil || t.static != nil {
		t.dirty = true
		return ErrStaticOccupancy
	}
	if err := o.validate(); err != nil {
		t.dirty = true
		return err
	}
	tenant, _ := uuid.Parse(o.TenantID)
	expectedPath := "/api/v1/ddi/tenants/" + o.TenantID + "/ipam/spaces/" + o.SpaceID + "/allocations"
	if [16]byte(tenant) != t.receipt.scope.Tenant || t.receipt.scope.Method != "POST" || t.receipt.scope.Path != expectedPath {
		t.dirty = true
		return ErrStaticOccupancy
	}
	address, _ := o.addressString()
	// This minimum create path supports no If-Match. Recompute from the exact
	// stored semantics; conditional digests cannot match this unconditional form.
	_, digest, err := ipamrequest.Normalize(ipamrequest.Request{TenantID: o.TenantID, SpaceID: o.SpaceID, SubnetID: o.SubnetID, Address: address, OwnerRef: o.OwnerRef})
	if err != nil || digest != t.receipt.digest {
		t.dirty = true
		return ErrStaticOccupancy
	}
	result, err := t.conn.ExecContext(ctx, `INSERT INTO ipam_occupancies (tenant_id,space_id,id,subnet_id,family,address,kind,owner_ref,created_at_ms) VALUES (?,?,?,?,?,?,'STATIC',?,?)`, o.TenantID, o.SpaceID, o.ID, o.SubnetID, o.Family, o.Address[:], o.OwnerRef, o.CreatedAtMS)
	if err != nil {
		t.dirty = true
		return fmt.Errorf("eventlog: insert static occupancy: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		t.dirty = true
		return fmt.Errorf("eventlog: static insert did not affect exactly one row: %w", ErrStaticOccupancy)
	}
	copy := o
	t.static = &copy
	return nil
}

// StaticChange produces the occupancy item of the three-change candidate set.
// It does not write or replicate the required audit and receipt items.
func (t *WriteTx) StaticChange() (Change, error) {
	if t.static == nil || t.dirty {
		t.dirty = true
		return Change{}, ErrStaticOccupancy
	}
	return t.static.Change()
}

// Change constructs the candidate replication event without writing or granting authority.
func (o StaticOccupancy) Change() (Change, error) {
	if err := o.validate(); err != nil {
		return Change{}, err
	}
	address, err := o.addressString()
	if err != nil {
		return Change{}, err
	}
	body, err := json.Marshal(struct {
		TenantID    string  `json:"tenantId"`
		SpaceID     string  `json:"spaceId"`
		ID          string  `json:"id"`
		SubnetID    string  `json:"subnetId"`
		Family      int     `json:"family"`
		Address     string  `json:"address"`
		Kind        string  `json:"kind"`
		OwnerRef    string  `json:"ownerRef"`
		CreatedAtMS string  `json:"createdAtMs"`
		ExpiresAtMS *string `json:"expiresAtMs"`
		Revision    string  `json:"revision"`
	}{o.TenantID, o.SpaceID, o.ID, o.SubnetID, o.Family, address, "STATIC", o.OwnerRef, strconv.FormatInt(o.CreatedAtMS, 10), nil, "1"})
	if err != nil {
		return Change{}, err
	}
	return Change{Kind: "ipam.occupancy.set", ObjectID: o.ID, Revision: "1", Body: body}, nil
}

// StaticChanges builds candidate occupancy, audit and receipt changes from
// transaction-local copies. Their encoding is not an apply implementation.
func (t *WriteTx) StaticChanges() ([]Change, error) {
	if t.dirty || t.static == nil || !t.audited || t.audit == nil || t.receipt == nil || !t.receipt.inserted || t.receipt.replay {
		t.dirty = true
		return nil, ErrStaticOccupancy
	}
	occupancy, err := t.StaticChange()
	if err != nil {
		t.dirty = true
		return nil, err
	}
	a := t.audit
	audit, err := auditwire.Encode(auditwire.Audit{ClusterID: uuid.UUID(t.log.cluster).String(), Epoch: int64(t.epoch), TenantID: a.TenantID, SpaceID: a.SpaceID, ResourceID: a.ResourceID, Principal: a.Principal, RequestID: a.RequestID, OccurredAtMS: a.OccurredAtMS, RequestDigest: t.receipt.digest})
	if err != nil {
		t.dirty = true
		return nil, err
	}
	receipt, err := receiptwire.Encode(receiptwire.Value{Scope: t.receipt.scope, RequestDigest: t.receipt.digest, Receipt: t.receipt.result})
	if err != nil {
		t.dirty = true
		return nil, err
	}
	return []Change{occupancy, {Kind: "ipam.occupancy.audit", ObjectID: a.RequestID, Revision: "1", Body: audit}, {Kind: "idempotency.receipt.set", ObjectID: t.receipt.scope.Key, Revision: "1", Body: receipt}}, nil
}

func (t *WriteTx) checkStaticChange(changes []Change) error {
	expected, err := t.StaticChanges()
	if err != nil {
		return err
	}
	if len(changes) != len(expected) {
		return ErrStaticOccupancy
	}
	for i := range expected {
		if changes[i].Kind != expected[i].Kind || changes[i].ObjectID != expected[i].ObjectID || changes[i].Revision != expected[i].Revision || !bytes.Equal(changes[i].Body, expected[i].Body) {
			return ErrStaticOccupancy
		}
	}
	return nil
}
