// Package compat records the CANDIDATE production compatibility matrix for the
// replication protocol and the negotiation primitive that must guard it.
//
// It is NOT frozen: independent review and an explicit peer agreement are
// required before any value here is used to accept a peer. Nothing in this
// package grants a role, an epoch, fencing, a trusted anchor or write
// authority. In particular, two axes sharing the value 1 are never evidence
// that one can be used in place of the other (contracts/version-compatibility.md).
package compat

import (
	"errors"
	"fmt"
	"sort"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotverify"
)

// ErrInvalid means a version set was malformed. ErrIncompatible means there is
// no mutually acceptable version; the exchange must be refused, never silently
// downgraded to a default.
var (
	ErrInvalid      = errors.New("compat: invalid version set")
	ErrIncompatible = errors.New("compat: no mutually acceptable version")
)

// Candidate versions this build can emit/read. They are pinned to the frozen
// constants in the owning packages; the tests assert that they stay equal.
const (
	EventWireSchemaV1       uint32 = eventlog.SchemaVersionV1
	SnapshotEncodingV1      uint32 = snapshotverify.EncodingNDJSONV1
	SnapshotRowSchemaV1     uint32 = 1
	RPCMessageVersionV1     uint32 = 1
	LocalDDLVersionLegacy   uint32 = 1
	LocalDDLVersionHardened uint32 = 2
	IPAMMigrationVersionV1  uint32 = 1
)

// acceptedEventWireSchemas is the CANDIDATE acceptance set for the event wire
// schema axis. A pull request declaring any other readable schema must be
// refused. This set is intentionally not derived from any other axis.
var acceptedEventWireSchemas = []uint32{EventWireSchemaV1}

// Axis describes one independent version axis. The descriptive fields exist so
// a reviewer can see that equal numbers on different axes control different
// objects; they are never used for a compatibility decision.
type Axis struct {
	Name     string
	Current  string
	Accepted string
	Controls string
}

// Axes returns the six independent version axes in a stable order. The matrix
// is a candidate, not a frozen production agreement.
func Axes() []Axis {
	return []Axis{
		{
			Name:     "event_wire_schema",
			Current:  fmt.Sprintf("%d", EventWireSchemaV1),
			Accepted: fmt.Sprintf("%d", EventWireSchemaV1),
			Controls: "Event.Schema / replica_head.schema_version / PullRequest.readable_schema_versions",
		},
		{
			Name:     "snapshot_encoding",
			Current:  fmt.Sprintf("%d", SnapshotEncodingV1),
			Accepted: fmt.Sprintf("%d", SnapshotEncodingV1),
			Controls: "SnapshotManifest.encoding (NDJSON)",
		},
		{
			Name:     "snapshot_row_schema",
			Current:  fmt.Sprintf("%d", SnapshotRowSchemaV1),
			Accepted: fmt.Sprintf("%d", SnapshotRowSchemaV1),
			Controls: "SnapshotManifest.schema_version / Limits.Schema (eight-table row shape)",
		},
		{
			Name:     "rpc_message",
			Current:  "v1",
			Accepted: "v1",
			Controls: "proto package goddi.replica.v1 (wire service/messages)",
		},
		{
			Name:     "local_physical_ddl",
			Current:  "legacy|hardened",
			Accepted: "legacy|hardened",
			Controls: "eventlogguard.Version (read-only local structure identification)",
		},
		{
			Name:     "ipam_reference_migrations",
			Current:  "001+002",
			Accepted: "001+002",
			Controls: "ipamstore.MigrationPaths() (isolated reference DDL set)",
		},
	}
}

// Negotiate returns the versions both sides can use, sorted and deduplicated.
// A zero or duplicated version is malformed; an empty intersection is a hard
// refusal. It never falls back to a default and never infers compatibility
// from a different axis.
func Negotiate(offered, accepted []uint32) ([]uint32, error) {
	normalize := func(in []uint32) (map[uint32]struct{}, error) {
		if len(in) == 0 {
			return nil, ErrInvalid
		}
		out := make(map[uint32]struct{}, len(in))
		for _, v := range in {
			if v == 0 {
				return nil, ErrInvalid
			}
			if _, dup := out[v]; dup {
				return nil, ErrInvalid
			}
			out[v] = struct{}{}
		}
		return out, nil
	}
	a, err := normalize(offered)
	if err != nil {
		return nil, err
	}
	b, err := normalize(accepted)
	if err != nil {
		return nil, err
	}
	common := make([]uint32, 0)
	for v := range a {
		if _, ok := b[v]; ok {
			common = append(common, v)
		}
	}
	if len(common) == 0 {
		return nil, ErrIncompatible
	}
	sort.Slice(common, func(i, j int) bool { return common[i] < common[j] })
	return common, nil
}

// ValidateReadableSchemas checks the readable_schema_versions of a pull
// request against the candidate acceptance set. A client that cannot read the
// only wire schema this build emits must be refused rather than served with a
// guess.
func ValidateReadableSchemas(readable []uint32) error {
	_, err := Negotiate(acceptedEventWireSchemas, readable)
	return err
}

// AcceptedEventWireSchemas returns a copy of the candidate acceptance set.
// Callers must not treat it as a frozen production agreement.
func AcceptedEventWireSchemas() []uint32 {
	return append([]uint32(nil), acceptedEventWireSchemas...)
}
