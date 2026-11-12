package eventlogguard

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/ipamstore"
)

var errDrift = errors.New("eventlogguard: reference drift")

const markerDDL = `CREATE TABLE eventlog_reference_guard_migrations (version INTEGER PRIMARY KEY, base_digest TEXT NOT NULL, guard_digest TEXT NOT NULL) STRICT`
const ipamMarkerDDL = `CREATE TABLE enterprise_schema_migrations (version INTEGER PRIMARY KEY, digest TEXT NOT NULL) STRICT`

var protected = []string{"writer_authority", "replica_head", "replica_event", "replica_checkpoint", "idempotency_receipt", "static_occupancy_audit"}
var ipamTables = []string{"ipam_resource_ids", "ipam_tenants", "ipam_spaces", "ipam_prefixes", "ipam_pools", "ipam_occupancies"}

func sum(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func verifyIPAMPins(paths []string, digest func(string) (string, bool)) error {
	expected := []struct{ path, sha string }{
		{"migrations/001_ipam.sql", "d5b68888951e6edda528cb7c19f6e7a3ae7f5b234fc12657edafb88a12688ad3"},
		{"migrations/002_rowid_guards.sql", "9172d79d81131b13e22d36cd6c16706ecb1500d5b7e141cf16de87d3409734f3"},
	}
	if len(paths) != len(expected) {
		return errDrift
	}
	for i, p := range paths {
		d, ok := digest(p)
		if !ok || p != expected[i].path || d != expected[i].sha {
			return errDrift
		}
	}
	return nil
}
func verifyPins() error {
	if sum(eventlog.Schema) != baseDigest || sum(guardSQL) != guardDigest {
		return errDrift
	}
	if err := verifyIPAMPins(ipamstore.MigrationPaths(), ipamstore.DigestOf); err != nil {
		return err
	}
	return ipamstore.VerifyDigests()
}
