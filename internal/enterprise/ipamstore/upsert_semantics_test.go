package ipamstore

// The hardened (v2) IPAM reference deliberately changes the write contract:
// logical-key guards refuse a same-key UPSERT, and a revision guard refuses an
// UPDATE that does not advance the revision. Together they make "replace the
// row" impossible -- replacing a row is replacing history, and the whole point
// of the guards is that an ID is never reused or silently rewritten.
//
// These tests pin the observable behaviour so a future integration cannot
// quietly reintroduce INSERT OR REPLACE. They do not authorise any production
// write: eventlogguard.Migrate and the protected write entrypoints remain
// ErrNoAuthority. See docs/audit/UPSERT-WRITE-PATH-COMPAT.md.

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func upsertFixture(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_tenants(id,slug,display_name) VALUES('old','old','Old')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestHardenedReferenceRejectsSameKeyUPSERT(t *testing.T) {
	for _, tc := range []struct {
		name, query, want string
	}{
		{
			"INSERT OR REPLACE",
			`INSERT OR REPLACE INTO ipam_tenants(id,slug,display_name) VALUES('old','replaced','Replaced')`,
			"IPAM_ID_ALREADY_USED",
		},
		{
			"ON CONFLICT DO UPDATE",
			`INSERT INTO ipam_tenants(id,slug,display_name) VALUES('old','upsert','Upsert') ON CONFLICT(id) DO UPDATE SET slug=excluded.slug`,
			"IPAM_ID_ALREADY_USED",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := upsertFixture(t)
			_, err := db.Exec(tc.query)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("same-key UPSERT err = %v, want %q", err, tc.want)
			}
			// The original row must survive untouched.
			var slug string
			if err := db.QueryRow(`SELECT slug FROM ipam_tenants WHERE id='old'`).Scan(&slug); err != nil || slug != "old" {
				t.Fatalf("original row changed: slug=%q err=%v", slug, err)
			}
		})
	}
}

func TestHardenedReferenceRequiresARevisionedUpdate(t *testing.T) {
	db := upsertFixture(t)
	if _, err := db.Exec(`UPDATE ipam_tenants SET slug='updated' WHERE id='old'`); err == nil ||
		!strings.Contains(err.Error(), "IPAM_REVISION_INVALID") {
		t.Fatalf("unrevisioned UPDATE err = %v, want IPAM_REVISION_INVALID", err)
	}
	if _, err := db.Exec(`UPDATE ipam_tenants SET slug='updated', revision=revision+1 WHERE id='old'`); err != nil {
		t.Fatalf("revisioned UPDATE was refused: %v", err)
	}
	var slug string
	var revision int
	if err := db.QueryRow(`SELECT slug, revision FROM ipam_tenants WHERE id='old'`).Scan(&slug, &revision); err != nil {
		t.Fatal(err)
	}
	if slug != "updated" || revision != 2 {
		t.Fatalf("revisioned UPDATE did not take effect: slug=%q revision=%d", slug, revision)
	}
}

// A genuinely new key may still be inserted, including through OR REPLACE
// (which finds no conflict). This is the positive control: the guards reject
// replacement, not all writes.
func TestHardenedReferenceAllowsInsertOfANewKey(t *testing.T) {
	db := upsertFixture(t)
	if _, err := db.Exec(`INSERT OR REPLACE INTO ipam_tenants(id,slug,display_name) VALUES('fresh','fresh','Fresh')`); err != nil {
		t.Fatalf("inserting a new key was refused: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM ipam_tenants WHERE id IN ('old','fresh')`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("expected both rows, got %d (%v)", count, err)
	}
}
