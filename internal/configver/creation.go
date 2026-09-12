package configver

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jasonwa/goddi/internal/dhcp/scope"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/jasonwa/goddi/internal/ipam/subnet"
)

// The content a creation is recorded with is built from the domain type the
// create path just wrote, through the same struct the adapter publishes.
//
// Two properties follow from that and both matter. The shape cannot drift: the
// constructor and the adapter name the same Go type, so a field added to one is
// a compile error in the other rather than a diff that reports the whole
// resource as changed. And no database read is involved, so the creation is
// recorded from what was actually created rather than from a re-read that could
// observe a later edit.

// DNSZoneContentFromZone builds the publishable content of a zone.
func DNSZoneContentFromZone(z *zone.Zone) DNSZoneContent {
	if z == nil {
		return DNSZoneContent{}
	}
	return DNSZoneContent{
		Name:           z.Name,
		Type:           z.Type,
		Enabled:        z.Enabled,
		DNSSECEnabled:  z.DNSSECEnabled,
		DefaultTTL:     z.DefaultTTL,
		SOAMName:       z.SOA_MName,
		SOARName:       z.SOA_RName,
		Refresh:        z.Refresh,
		Retry:          z.Retry,
		Expire:         z.Expire,
		Minimum:        z.Minimum,
		TransferPolicy: z.TransferPolicy,
		UpdatePolicy:   z.UpdatePolicy,
	}
}

// DHCPScopeContentFromScope builds the publishable content of a scope.
func DHCPScopeContentFromScope(s *scope.Scope) DHCPScopeContent {
	if s == nil {
		return DHCPScopeContent{}
	}
	return DHCPScopeContent{
		Name:             s.Name,
		Interface:        s.Interface,
		Subnet:           s.Subnet,
		StartIP:          s.StartIP,
		EndIP:            s.EndIP,
		SubnetMask:       s.SubnetMask,
		Router:           s.Router,
		DNSServers:       s.DNSServers,
		NTPServers:       s.NTPServers,
		DomainName:       s.DomainName,
		LeaseTime:        s.LeaseTime,
		MaxLeaseTime:     s.MaxLeaseTime,
		Enabled:          s.Enabled,
		PingCheckEnabled: s.PingCheckEnabled,
		DNSUpdates:       s.DNSUpdates,
		Comment:          s.Comment,
	}
}

// IPAMSubnetContentFromSubnet builds the publishable content of a subnet.
//
// space_id is deliberately not carried: the adapter excludes it, because moving
// a subnet between spaces is a re-parenting operation rather than a field edit.
// A creation records where the subnet was made, and the space is not part of
// what the type governs.
func IPAMSubnetContentFromSubnet(s *subnet.Subnet) IPAMSubnetContent {
	if s == nil {
		return IPAMSubnetContent{}
	}
	return IPAMSubnetContent{
		Name:        s.Name,
		CIDR:        s.CIDR,
		VLANID:      s.VLANID,
		Location:    s.Location,
		Description: s.Description,
	}
}

// RecordCreationRequest asks for a creation to be recorded as a revision.
type RecordCreationRequest struct {
	ResourceType ResourceType
	ResourceID   string
	Content      json.RawMessage
	Actor        string
	Note         string
}

// RecordCreation writes the first revision of a resource whose rows were just
// created.
//
// The publish path refuses a resource that does not exist yet, and it has to --
// a revision pointing at nothing could never be applied. That left creation
// outside the log: the first revision recorded the first *edit*, so the state
// the resource was born in was unanswerable, and a rollback could not reach it.
// This closes that hole from the side the caller is on: the rows exist, so the
// revision is written after them.
//
// Two things it deliberately does not do:
//
//   - It writes no release. A release exists to carry a *change* to a data
//     plane that is already serving something else. At creation there is
//     nothing to carry: the write that created the rows is the write that made
//     them visible, through each resource's own mechanism (the revision counter
//     the console path moves, the store's own notification). A release row here
//     would announce a change that never happened.
//   - It writes no `applied` status, for the same reason. `persisted` is the
//     claim it can support: the content is durably recorded. The revision is
//     restorable either way, which is the point of recording it.
//
// Recording a creation twice is refused rather than ignored: the second call
// means the caller has lost track of what it has already written, and silently
// returning the existing revision would hide that.
func (s *Service) RecordCreation(req RecordCreationRequest) (*Revision, error) {
	if !req.ResourceType.Valid() {
		return nil, fmt.Errorf("configver: unknown resource type %q", req.ResourceType)
	}
	if strings.TrimSpace(req.ResourceID) == "" {
		return nil, fmt.Errorf("configver: resource id is required")
	}
	adapter, ok := s.adapters[req.ResourceType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoAdapter, req.ResourceType)
	}

	content, hash, err := normaliseContent(req.Content)
	if err != nil {
		return nil, &ValidationError{
			ResourceType: req.ResourceType,
			ResourceID:   req.ResourceID,
			Reason:       err.Error(),
		}
	}
	// The same validation the governed path applies. A resource created through
	// an endpoint that validates less than the adapter does would otherwise be
	// recorded with content that no later publish could carry.
	if err := adapter.Validate(content); err != nil {
		return nil, &ValidationError{
			ResourceType: req.ResourceType,
			ResourceID:   req.ResourceID,
			Reason:       err.Error(),
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("configver: begin recording the creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	exists, err := adapter.Exists(tx, req.ResourceID)
	if err != nil {
		return nil, fmt.Errorf("configver: check resource: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: %s %s", ErrResourceNotFound, req.ResourceType, req.ResourceID)
	}

	current, err := currentRevision(tx, req.ResourceType, req.ResourceID)
	if err != nil {
		return nil, err
	}
	if current != 0 {
		return nil, &ConflictError{
			ResourceType:     req.ResourceType,
			ResourceID:       req.ResourceID,
			ExpectedRevision: 0,
			ActualRevision:   current,
		}
	}

	rev := &Revision{
		ID:           uuid.New().String(),
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		Revision:     1,
		BaseRevision: 0,
		Status:       StatusPersisted,
		ContentHash:  hash,
		Actor:        req.Actor,
		Note:         req.Note,
		Content:      content,
	}
	if rev.Note == "" {
		rev.Note = "created"
	}
	if err := insertRevision(tx, rev); err != nil {
		// Two callers recording the same creation at once. The unique index on
		// (resource_type, resource_id, revision) is what decides it, and the
		// loser is told it lost rather than being handed a revision.
		if isUniqueViolation(err) {
			return nil, &ConflictError{
				ResourceType:     req.ResourceType,
				ResourceID:       req.ResourceID,
				ExpectedRevision: 0,
				ActualRevision:   1,
			}
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("configver: commit the creation: %w", err)
	}
	return rev, nil
}

// ContentBuilder reads a resource back in the shape its adapter publishes.
type ContentBuilder func(q Queryer, id string) (json.RawMessage, error)

// builders is keyed by type rather than looked up by type assertion so that a
// missing entry is a nil check at the call site rather than a panic.
//
// Only the record set has a reader, and that is not an oversight: the other
// three types are single rows whose owning endpoints already serve them in the
// publishable shape (the zone and scope responses carry every governed field),
// so a second reader would be a second place for the shape to be spelled. The
// record set has no such endpoint -- it is a list, and the endpoint that serves
// it serves rows with timestamps rather than a document.
var builders = map[ResourceType]ContentBuilder{
	ResourceDNSRecords: ReadDNSRecordsContent,
}

// ReadContent returns a resource's current publishable content.
func ReadContent(q Queryer, rt ResourceType, id string) (json.RawMessage, error) {
	builder, ok := builders[rt]
	if !ok {
		return nil, fmt.Errorf("configver: no content reader for %s", rt)
	}
	return builder(q, id)
}
