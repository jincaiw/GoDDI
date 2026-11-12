package dataplane

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LKGKeyPrefix namespaces the last-known-good markers in dataplane_meta.
const LKGKeyPrefix = "lkg_"

func lkgRevisionKey(d Domain) string { return LKGKeyPrefix + "revision:" + string(d) }
func lkgAppliedKey(d Domain) string  { return LKGKeyPrefix + "applied_at:" + string(d) }

// writeLKG records the revision and its commit instant inside the caller's
// transaction, so the stamp and the rows it describes become visible together.
func writeLKG(tx *sql.Tx, d Domain, revision int64) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for k, v := range map[string]string{
		lkgRevisionKey(d): strconv.FormatInt(revision, 10),
		lkgAppliedKey(d):  now,
	} {
		if _, err := tx.Exec(`INSERT INTO dataplane_meta (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return fmt.Errorf("dataplane: writing LKG %s: %w", k, err)
		}
	}
	return nil
}

// LKG reports the last-known-good revision a node has durably applied for a
// domain, and when. ok is false when nothing has been applied yet.
func LKG(db *sql.DB, d Domain) (revision int64, appliedAt time.Time, ok bool, err error) {
	if db == nil {
		return 0, time.Time{}, false, fmt.Errorf("dataplane: nil store for LKG")
	}
	var rawRev, rawAt string
	if e := db.QueryRow(`SELECT value FROM dataplane_meta WHERE key = ?`, lkgRevisionKey(d)).Scan(&rawRev); e == sql.ErrNoRows {
		return 0, time.Time{}, false, nil
	} else if e != nil {
		return 0, time.Time{}, false, fmt.Errorf("dataplane: read LKG revision: %w", e)
	}
	if e := db.QueryRow(`SELECT value FROM dataplane_meta WHERE key = ?`, lkgAppliedKey(d)).Scan(&rawAt); e != nil && e != sql.ErrNoRows {
		return 0, time.Time{}, false, fmt.Errorf("dataplane: read LKG applied_at: %w", e)
	}
	revision, _ = strconv.ParseInt(strings.TrimSpace(rawRev), 10, 64)
	if strings.TrimSpace(rawAt) != "" {
		appliedAt, _ = time.Parse(time.RFC3339Nano, rawAt)
	}
	return revision, appliedAt, true, nil
}
