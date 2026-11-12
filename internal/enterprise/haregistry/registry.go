// Package haregistry is an isolated operations-registry state-machine prototype.
// It does not validate signatures, authorize issuance, fence, or change roles.
// Public mutation methods are closed until a trusted evidence protocol exists.
package haregistry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

var ErrNoAuthority = errors.New("haregistry: external authority not implemented")
var ErrInvalid = errors.New("haregistry: invalid input or state")

// IssueRequest records an issued permit, not a proven active role. ActiveEpoch
// is an externally established input; the prototype itself cannot establish it.
type IssueRequest struct {
	ClusterID, PredecessorID, CandidateID, Nonce string
	ActiveEpoch, NewEpoch                        int64
	PermitDigest                                 [32]byte
	Requester                                    string
}

// Init installs the schema in the caller's transaction into an empty namespace.
// It is intentionally not an upgrade or production migration. Caller rolls
// back the entire transaction on any returned error, including commit errors.
func Init(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return ErrInvalid
	}
	_, err := tx.ExecContext(ctx, schema)
	return err
}

// Issue is deliberately closed: validated signed approval, registry ownership,
// fencing and exclusive operator workflow are not yet implemented.
func Issue(_ context.Context, _ *sql.Tx, _ IssueRequest) error { return ErrNoAuthority }

// Cancel and Complete are also deliberately closed. A boolean or caller claim
// cannot prove revocation or safe closure of an issued permit.
func Cancel(_ context.Context, _ *sql.Tx, _ string) error   { return ErrNoAuthority }
func Complete(_ context.Context, _ *sql.Tx, _ string) error { return ErrNoAuthority }

func canonical(s string) bool {
	u, e := uuid.Parse(s)
	return e == nil && u != uuid.Nil && u.String() == s
}
func valid(r IssueRequest) bool {
	if !canonical(r.ClusterID) || !canonical(r.PredecessorID) || !canonical(r.CandidateID) || !canonical(r.Nonce) || r.ClusterID == r.PredecessorID || r.ClusterID == r.CandidateID || r.PredecessorID == r.CandidateID || r.ActiveEpoch <= 0 || r.NewEpoch <= r.ActiveEpoch || r.PermitDigest == ([32]byte{}) {
		return false
	}
	return len(r.Requester) > 0 && len(r.Requester) <= 256 && utf8.ValidString(r.Requester) && strings.TrimSpace(r.Requester) == r.Requester && !strings.ContainsAny(r.Requester, "\x00\r\n")
}

// issue is package-private and exercised exclusively by fixtures. The SQL
// triggers are defense in depth, not proof of an authorized external workflow.
func issue(ctx context.Context, tx *sql.Tx, r IssueRequest) error {
	if tx == nil || !valid(r) {
		return ErrInvalid
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO ha_issued(cluster_id,predecessor_id,candidate_id,active_epoch,new_epoch,nonce,permit_digest,requester) VALUES(?,?,?,?,?,?,?,?)`, r.ClusterID, r.PredecessorID, r.CandidateID, r.ActiveEpoch, r.NewEpoch, r.Nonce, r.PermitDigest[:], r.Requester)
	if err != nil {
		return fmt.Errorf("%w: issue: %v", ErrInvalid, err)
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return ErrInvalid
	}
	return nil
}

// resolve is a package-private state-machine fixture, not evidence validation.
func resolve(ctx context.Context, tx *sql.Tx, nonce, kind string) error {
	if tx == nil || !canonical(nonce) || (kind != "CANCELLED" && kind != "COMPLETED") {
		return ErrInvalid
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO ha_resolution(nonce,kind) VALUES(?,?)`, nonce, kind)
	if err != nil {
		return fmt.Errorf("%w: resolve: %v", ErrInvalid, err)
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return ErrInvalid
	}
	return nil
}

// No INSERT OR REPLACE can bypass append-only guards: BEFORE INSERT checks the
// existing identities before SQLite's implicit delete. All users must enable FK.
const schema = `
CREATE TABLE ha_issued (
 cluster_id TEXT NOT NULL, predecessor_id TEXT NOT NULL, candidate_id TEXT NOT NULL,
 active_epoch INTEGER NOT NULL CHECK(active_epoch>0),
 new_epoch INTEGER NOT NULL CHECK(new_epoch>active_epoch),
 nonce TEXT NOT NULL PRIMARY KEY, permit_digest BLOB NOT NULL CHECK(length(permit_digest)=32 AND permit_digest<>zeroblob(32)),
 requester TEXT NOT NULL CHECK(length(requester)>0 AND length(requester)<=256),
 UNIQUE(cluster_id,new_epoch),
 CHECK(cluster_id<>predecessor_id AND cluster_id<>candidate_id AND predecessor_id<>candidate_id),
 CHECK(length(cluster_id)=36 AND substr(cluster_id,9,1)='-' AND substr(cluster_id,14,1)='-' AND substr(cluster_id,19,1)='-' AND substr(cluster_id,24,1)='-' AND length(replace(cluster_id,'-',''))=32 AND replace(cluster_id,'-','') NOT GLOB '*[^0-9a-f]*' AND replace(cluster_id,'-','')<>printf('%032d',0)),
 CHECK(length(predecessor_id)=36 AND substr(predecessor_id,9,1)='-' AND substr(predecessor_id,14,1)='-' AND substr(predecessor_id,19,1)='-' AND substr(predecessor_id,24,1)='-' AND length(replace(predecessor_id,'-',''))=32 AND replace(predecessor_id,'-','') NOT GLOB '*[^0-9a-f]*' AND replace(predecessor_id,'-','')<>printf('%032d',0)),
 CHECK(length(candidate_id)=36 AND substr(candidate_id,9,1)='-' AND substr(candidate_id,14,1)='-' AND substr(candidate_id,19,1)='-' AND substr(candidate_id,24,1)='-' AND length(replace(candidate_id,'-',''))=32 AND replace(candidate_id,'-','') NOT GLOB '*[^0-9a-f]*' AND replace(candidate_id,'-','')<>printf('%032d',0)),
 CHECK(length(nonce)=36 AND substr(nonce,9,1)='-' AND substr(nonce,14,1)='-' AND substr(nonce,19,1)='-' AND substr(nonce,24,1)='-' AND length(replace(nonce,'-',''))=32 AND replace(nonce,'-','') NOT GLOB '*[^0-9a-f]*' AND replace(nonce,'-','')<>printf('%032d',0))
) STRICT, WITHOUT ROWID;
CREATE TABLE ha_resolution (
 nonce TEXT NOT NULL PRIMARY KEY REFERENCES ha_issued(nonce) ON DELETE RESTRICT,
 kind TEXT NOT NULL CHECK(kind IN ('CANCELLED','COMPLETED'))
) STRICT, WITHOUT ROWID;
CREATE TRIGGER ha_issued_guard BEFORE INSERT ON ha_issued BEGIN
 SELECT RAISE(ABORT,'HA_NONCE_USED') WHERE EXISTS(SELECT 1 FROM ha_issued WHERE nonce=NEW.nonce);
 SELECT RAISE(ABORT,'HA_EPOCH_REUSED') WHERE EXISTS(SELECT 1 FROM ha_issued WHERE cluster_id=NEW.cluster_id AND new_epoch>=NEW.new_epoch);
 SELECT RAISE(ABORT,'HA_ACTIVE_REGRESSION') WHERE EXISTS(SELECT 1 FROM ha_issued WHERE cluster_id=NEW.cluster_id AND active_epoch>NEW.active_epoch);
 -- Closure is not a role grant, but a completed permit can have taken effect.
 SELECT RAISE(ABORT,'HA_COMPLETED_EPOCH_REGRESSION') WHERE EXISTS(SELECT 1 FROM ha_issued i JOIN ha_resolution r ON r.nonce=i.nonce WHERE i.cluster_id=NEW.cluster_id AND r.kind='COMPLETED' AND i.new_epoch>NEW.active_epoch);
 SELECT RAISE(ABORT,'HA_UNRESOLVED') WHERE EXISTS(SELECT 1 FROM ha_issued i WHERE i.cluster_id=NEW.cluster_id AND NOT EXISTS(SELECT 1 FROM ha_resolution r WHERE r.nonce=i.nonce));
END;
CREATE TRIGGER ha_issued_no_update BEFORE UPDATE ON ha_issued BEGIN SELECT RAISE(ABORT,'HA_IMMUTABLE'); END;
CREATE TRIGGER ha_issued_no_delete BEFORE DELETE ON ha_issued BEGIN SELECT RAISE(ABORT,'HA_IMMUTABLE'); END;
CREATE TRIGGER ha_resolution_guard BEFORE INSERT ON ha_resolution BEGIN
 SELECT RAISE(ABORT,'HA_UNKNOWN_NONCE') WHERE NOT EXISTS(SELECT 1 FROM ha_issued WHERE nonce=NEW.nonce);
 SELECT RAISE(ABORT,'HA_ALREADY_RESOLVED') WHERE EXISTS(SELECT 1 FROM ha_resolution WHERE nonce=NEW.nonce);
END;
CREATE TRIGGER ha_resolution_no_update BEFORE UPDATE ON ha_resolution BEGIN SELECT RAISE(ABORT,'HA_IMMUTABLE'); END;
CREATE TRIGGER ha_resolution_no_delete BEFORE DELETE ON ha_resolution BEGIN SELECT RAISE(ABORT,'HA_IMMUTABLE'); END;
`
