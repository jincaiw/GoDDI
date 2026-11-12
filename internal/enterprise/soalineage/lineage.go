// Package soalineage is a transaction-scoped SOA lineage prototype. It neither
// authorizes writes nor proves history is complete. Callers must establish
// approval, a single SQLite writer, and historical continuity independently;
// missing history never proves that a production zone is new. Init is only for
// isolated empty test databases, not an existing installation or recovery.
package soalineage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jasonwa/goddi/internal/enterprise/soaserial"
)

var (
	ErrInvalid  = errors.New("soalineage: invalid input")
	ErrExisting = errors.New("soalineage: test database is not empty")
	ErrConflict = errors.New("soalineage: release or serial conflicts with history")
	ErrOrder    = errors.New("soalineage: serial is not newer")
	ErrMissing  = errors.New("soalineage: missing lineage; independent bootstrap proof required")
)

// Key names a stable tenant/view/zone scope. Before integration, the caller
// must ensure view identifiers are globally unique and zone names canonical;
// adding tenant to this key does not establish the required view/zone lineage.
// Tenant authorization and preventing key reuse also belong to the caller.
type Key struct{ Tenant, View, Zone string }

// Entry records the exact result for one release. Deleted marks a tombstone;
// its digest is caller-supplied and participates in identity checks.
type Entry struct {
	Key     Key
	Release [16]byte
	Serial  uint32
	Digest  [32]byte
	Deleted bool
}

func validKey(k Key) bool {
	for _, s := range []string{k.Tenant, k.View, k.Zone} {
		if len(s) == 0 || len(s) > 255 || !utf8.ValidString(s) || strings.IndexByte(s, 0) >= 0 {
			return false
		}
	}
	return true
}
func valid(e Entry) bool { return validKey(e.Key) && e.Release != [16]byte{} }

// Init creates an append-only schema in an empty test database. The caller owns
// the transaction and must roll it back on error; this function never commits.
func Init(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return ErrInvalid
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`).Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return ErrExisting
	}
	_, err := tx.ExecContext(ctx, schema)
	return err
}

const schema = `
CREATE TABLE soa_lineage (
 ordinal INTEGER PRIMARY KEY AUTOINCREMENT CHECK(ordinal > 0),
 tenant TEXT NOT NULL CHECK(length(CAST(tenant AS BLOB)) BETWEEN 1 AND 255 AND instr(tenant,char(0))=0),
 view TEXT NOT NULL CHECK(length(CAST(view AS BLOB)) BETWEEN 1 AND 255 AND instr(view,char(0))=0),
 zone TEXT NOT NULL CHECK(length(CAST(zone AS BLOB)) BETWEEN 1 AND 255 AND instr(zone,char(0))=0),
 release BLOB NOT NULL CHECK(length(release)=16 AND release<>zeroblob(16)),
 serial INTEGER NOT NULL CHECK(serial BETWEEN 0 AND 4294967295),
 digest BLOB NOT NULL CHECK(length(digest)=32),
 deleted INTEGER NOT NULL CHECK(deleted IN (0,1)),
 UNIQUE(tenant,view,zone,release)
) STRICT;
CREATE INDEX soa_lineage_head ON soa_lineage(tenant,view,zone,ordinal DESC);
CREATE INDEX soa_lineage_serial ON soa_lineage(tenant,view,zone,serial);
CREATE TRIGGER soa_lineage_no_replace BEFORE INSERT ON soa_lineage BEGIN
 SELECT RAISE(ABORT,'SOA_RELEASE_EXISTS') WHERE EXISTS (SELECT 1 FROM soa_lineage WHERE tenant=NEW.tenant AND view=NEW.view AND zone=NEW.zone AND release=NEW.release);
 SELECT RAISE(ABORT,'SOA_ORDINAL_EXISTS') WHERE NEW.ordinal > 0 AND EXISTS (SELECT 1 FROM soa_lineage WHERE ordinal=NEW.ordinal);
 SELECT RAISE(ABORT,'SOA_SERIAL_CONFLICT') WHERE EXISTS (SELECT 1 FROM soa_lineage WHERE tenant=NEW.tenant AND view=NEW.view AND zone=NEW.zone AND serial=NEW.serial AND (digest<>NEW.digest OR deleted<>NEW.deleted));
END;
CREATE TRIGGER soa_lineage_no_update BEFORE UPDATE ON soa_lineage BEGIN SELECT RAISE(ABORT,'SOA_IMMUTABLE'); END;
CREATE TRIGGER soa_lineage_no_delete BEFORE DELETE ON soa_lineage BEGIN SELECT RAISE(ABORT,'SOA_IMMUTABLE'); END;
`

// Record fixes a release inside the caller's existing SQLite write transaction.
// bootstrap explicitly permits the first row only; callers must independently
// prove it is truly a new scope, not a truncated or restored history. Callers
// must begin an immediate write transaction, roll back on all errors, and own
// commit/unknown-outcome handling. Concurrent readers cannot establish authority.
// A duplicate release is a no-op only if every recorded field agrees. A new
// release with unchanged content may retain the current serial; changed content
// or tombstone status requires a strictly newer RFC 1982 serial.
func Record(ctx context.Context, tx *sql.Tx, e Entry, bootstrap bool) (bool, error) {
	if tx == nil || !valid(e) {
		return false, ErrInvalid
	}
	var serial int64
	var digest []byte
	var deleted int
	err := tx.QueryRowContext(ctx, `SELECT serial,digest,deleted FROM soa_lineage WHERE tenant=? AND view=? AND zone=? AND release=?`, e.Key.Tenant, e.Key.View, e.Key.Zone, e.Release[:]).Scan(&serial, &digest, &deleted)
	if err == nil {
		if serial == int64(e.Serial) && bytes.Equal(digest, e.Digest[:]) && (deleted == 1) == e.Deleted {
			return true, nil
		}
		return false, ErrConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	err = tx.QueryRowContext(ctx, `SELECT serial,digest,deleted FROM soa_lineage WHERE tenant=? AND view=? AND zone=? ORDER BY ordinal DESC LIMIT 1`, e.Key.Tenant, e.Key.View, e.Key.Zone).Scan(&serial, &digest, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		if !bootstrap {
			return false, ErrMissing
		}
	} else if err != nil {
		return false, err
	} else {
		if serial < 0 || serial > int64(^uint32(0)) || len(digest) != 32 || (deleted != 0 && deleted != 1) {
			return false, ErrInvalid
		}
		cmp, err := soaserial.Compare(uint32(serial), e.Serial)
		if err != nil {
			return false, err
		}
		changed := !bytes.Equal(digest, e.Digest[:]) || (deleted == 1) != e.Deleted
		if cmp > 0 || (changed && cmp == 0) {
			return false, ErrOrder
		}
	}
	var previous []byte
	var previousDeleted int
	err = tx.QueryRowContext(ctx, `SELECT digest,deleted FROM soa_lineage WHERE tenant=? AND view=? AND zone=? AND serial=? LIMIT 1`, e.Key.Tenant, e.Key.View, e.Key.Zone, int64(e.Serial)).Scan(&previous, &previousDeleted)
	if err == nil && (!bytes.Equal(previous, e.Digest[:]) || (previousDeleted == 1) != e.Deleted) {
		return false, ErrConflict
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	flag := 0
	if e.Deleted {
		flag = 1
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO soa_lineage(tenant,view,zone,release,serial,digest,deleted) VALUES (?,?,?,?,?,?,?)`, e.Key.Tenant, e.Key.View, e.Key.Zone, e.Release[:], int64(e.Serial), e.Digest[:], flag)
	if err != nil {
		return false, fmt.Errorf("soalineage: insert: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return false, fmt.Errorf("soalineage: insertion did not affect exactly one row: %w", ErrInvalid)
	}
	return false, nil
}
