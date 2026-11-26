package eventlogguard

import (
	"fmt"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamstore"
)

// ReferenceDDLs returns the ordered DDL statements that build an isolated
// in-memory reference for a local structure version. It is exported so other
// offline verifiers and isolated fixtures can compare against the same frozen
// definitions without duplicating guard SQL.
//
// This is a description of a reference structure, not a migration: it grants
// no role or authority, never touches a real database, and must not be used as
// an upgrade path. The caller owns the database it applies the statements to.
func ReferenceDDLs(version Version) ([]string, error) {
	if version != LegacyReference && version != HardenedReference {
		return nil, errDrift
	}
	paths := ipamstore.MigrationPaths()
	out := make([]string, 0, len(paths)+4)
	for _, p := range paths {
		raw, err := ipamstore.ContentsOf(p)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	out = append(out, ipamMarkerDDL, eventlog.Schema)
	if version == HardenedReference {
		out = append(out, guardSQL, markerDDL)
	}
	return out, nil
}

// ReferenceMarkerStatements returns the INSERT statements that record the
// frozen migration digests a reference of the given version must carry. The
// digests are hex strings, so the statements contain no caller-controlled
// input. Like ReferenceDDLs this is descriptive only: it is not a migration
// of any real database and grants no write authority.
func ReferenceMarkerStatements(version Version) ([]string, error) {
	if version != LegacyReference && version != HardenedReference {
		return nil, errDrift
	}
	paths := ipamstore.MigrationPaths()
	out := make([]string, 0, len(paths)+1)
	for i, p := range paths {
		digest, ok := ipamstore.DigestOf(p)
		if !ok {
			return nil, errDrift
		}
		out = append(out, fmt.Sprintf(
			`INSERT INTO enterprise_schema_migrations (version, digest) VALUES (%d, '%s')`,
			i+1, digest))
	}
	if version == HardenedReference {
		out = append(out, fmt.Sprintf(
			`INSERT INTO eventlog_reference_guard_migrations (version, base_digest, guard_digest) VALUES (1, '%s', '%s')`,
			baseDigest, guardDigest))
	}
	return out, nil
}
