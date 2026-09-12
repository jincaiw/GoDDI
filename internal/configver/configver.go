// Package configver implements versioned publishing for configuration objects.
//
// Before this package, a configuration change was a bare UPDATE against the
// resource row. Nothing recorded what the previous value was, who changed it,
// or whether the data plane ever picked it up. That is fine while the only
// consumer is a human watching the screen; it is not fine once a wrong SOA
// contact or a mistyped pool range can take a site offline, because the only
// recovery path is to remember the old value.
//
// This package makes every governed change a revision:
//
//   - Revisions are numbered per (resource_type, resource_id) and are
//     append-only. A rollback is a NEW revision carrying old content, never a
//     rewrite -- otherwise the log would lose the fact that the bad
//     configuration was ever live, which is the one thing an operator needs to
//     know after an incident.
//   - Each revision stores the full content, so any two revisions can be
//     diffed and any earlier one can be restored.
//   - Two writers racing on the same resource cannot silently clobber each
//     other: the caller states which revision it based its change on, and a
//     mismatch is rejected with ErrConflict.
//   - Recording the configuration and telling the data plane about it are two
//     steps, because an in-memory zone reload cannot join a SQLite
//     transaction. The release event is written in the SAME transaction as the
//     revision, so a crash after commit leaves a durable "a reload is owed"
//     record rather than a configuration the resolver never learned about.
//
// Lifecycle of a revision:
//
//	persisted -> staged -> applied
//	                    \-> failed
//
//	persisted  the content is durably recorded
//	staged     a release event is queued for the data plane (same transaction
//	           as `persisted`; the stored status is the furthest one reached)
//	applied    the data plane confirmed it is serving this revision
//	failed     release exhausted its retries; the last `applied` revision
//	           remains the last-known-good configuration
//
// There is deliberately no stored `draft` or `validated` state. A draft lives
// in the operator's form until it is submitted, and validation is synchronous
// and precedes any write -- so a request either becomes a revision or is
// rejected, and nothing in between needs a row.
//
// A dry run (PublishRequest.DryRun) therefore stores nothing at all: it
// validates and reports the diff, and returns. Storing a preview as a revision
// was tried and rejected: it consumes a revision number that the concurrency
// baseline does not count, so the next real publish collides with it on
// (resource_type, resource_id, revision). Previewing a change must not change
// the version anyone else has to cite.
//
// Scope: this package governs PUBLISHING of an existing resource's
// configuration. Resource creation and deletion still use the existing
// endpoints; a resource gets its first revision on its first publish
// (ExpectedRevision = 0).
package configver

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ResourceType identifies a class of governed configuration object.
//
// The value doubles as the adapter registry key and is stored in the database,
// so it must remain stable across releases.
type ResourceType string

const (
	// ResourceDNSZone governs the zone-level configuration of a DNS zone
	// (SOA timers, defaults, policies). Records are not part of this slice.
	ResourceDNSZone ResourceType = "dns_zone"
	// ResourceDHCPScope governs a DHCP scope's pool and options.
	ResourceDHCPScope ResourceType = "dhcp_scope"
	// ResourceIPAMSubnet governs an IPAM subnet's descriptive metadata.
	ResourceIPAMSubnet ResourceType = "ipam_subnet"
	// ResourceDNSRecords governs a zone's control-plane-authored record set as
	// one unit, keyed by zone id.
	//
	// It is a separate type from dns_zone rather than part of that content: the
	// two change at completely different rates and for different reasons, and
	// folding records into the zone would have changed the shape of every
	// revision already stored under the old type -- which a diff would then
	// report as a change of the zone.
	ResourceDNSRecords ResourceType = "dns_records"
)

// Valid reports whether t is a known resource type.
func (t ResourceType) Valid() bool {
	switch t {
	case ResourceDNSZone, ResourceDHCPScope, ResourceIPAMSubnet, ResourceDNSRecords:
		return true
	}
	return false
}

// Status is the furthest stage a revision has reached.
type Status string

const (
	// StatusPersisted means the content is durably recorded.
	StatusPersisted Status = "persisted"
	// StatusStaged means a release event is queued for the data plane.
	StatusStaged Status = "staged"
	// StatusApplied means the data plane confirmed it serves this revision.
	StatusApplied Status = "applied"
	// StatusFailed means the release exhausted its retries. The previous
	// applied revision remains the last-known-good configuration.
	StatusFailed Status = "failed"
)

// transitions is the complete state machine. Modelling it explicitly means an
// illegal transition (for example marking a draft "applied") is a code error
// that fails a test, not a row that quietly claims something untrue.
var transitions = map[Status][]Status{
	StatusPersisted: {StatusStaged, StatusFailed},
	StatusStaged:    {StatusApplied, StatusFailed},
	StatusApplied:   {},
	StatusFailed:    {},
}

// CanTransitionTo reports whether a revision may move from s to next.
func (s Status) CanTransitionTo(next Status) bool {
	for _, allowed := range transitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	_, ok := transitions[s]
	return ok
}

// restorable reports whether a revision's content can be rolled back to.
//
// `failed` is excluded even though it was written: it never reached the data
// plane, so its content was never a configuration anyone ran. Rolling back to
// it would restore something that never existed in service.
func (s Status) restorable() bool {
	switch s {
	case StatusPersisted, StatusStaged, StatusApplied:
		return true
	}
	return false
}

// Revision is one published (or previewed) configuration of a resource.
type Revision struct {
	ID                string          `json:"id"`
	ResourceType      ResourceType    `json:"resource_type"`
	ResourceID        string          `json:"resource_id"`
	Revision          int64           `json:"revision"`
	Status            Status          `json:"status"`
	Content           json.RawMessage `json:"content"`
	ContentHash       string          `json:"content_hash"`
	BaseRevision      int64           `json:"base_revision"`
	IdempotencyKey    string          `json:"idempotency_key,omitempty"`
	Actor             string          `json:"actor,omitempty"`
	Note              string          `json:"note,omitempty"`
	Error             string          `json:"error,omitempty"`
	AppliedGeneration int64           `json:"applied_generation"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
	AppliedAt         string          `json:"applied_at,omitempty"`
}

// Errors returned by the service. They are sentinel values so the HTTP layer
// can map them to status codes without string matching.
var (
	// ErrNotFound means no such revision exists.
	ErrNotFound = errors.New("configver: revision not found")
	// ErrNoAdapter means no adapter is registered for the resource type.
	ErrNoAdapter = errors.New("configver: no adapter registered for resource type")
	// ErrInvalidRevision means the requested revision number is not usable as
	// a rollback target.
	ErrInvalidRevision = errors.New("configver: invalid rollback target")
)

// ValidationError is returned when content is rejected before anything is
// written. It carries the adapter's reason verbatim: the operator needs to
// know which field is wrong, not just that the publish failed.
type ValidationError struct {
	ResourceType ResourceType
	ResourceID   string
	Reason       string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("configver: %s %s rejected: %s", e.ResourceType, e.ResourceID, e.Reason)
}

// ConflictError reports a lost optimistic-concurrency race.
//
// This is deliberately a distinct error from a validation failure: the content
// may be perfectly valid and still be rejected because someone else published
// in the meantime. The caller is expected to reload and re-apply, not to fix
// the content.
type ConflictError struct {
	ResourceType     ResourceType
	ResourceID       string
	ExpectedRevision int64
	ActualRevision   int64
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf(
		"configver: %s %s was modified concurrently: expected revision %d, found %d",
		e.ResourceType, e.ResourceID, e.ExpectedRevision, e.ActualRevision)
}

// nowFunc is overridable in tests. Production always uses UTC wall clock so
// stored timestamps are comparable with SQLite's datetime('now').
var nowFunc = func() time.Time { return time.Now().UTC() }
