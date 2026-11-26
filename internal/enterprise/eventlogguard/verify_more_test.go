package eventlogguard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamstore"
	"github.com/jincaiw/GoDDI/internal/sqlitepolicy"
)

func TestInspectLegacyPhysicalReadOnlyConnection(t *testing.T) {
	db := legacy(t)
	var id int
	var name, path string
	if err := db.QueryRow(`PRAGMA database_list`).Scan(&id, &name, &path); err != nil {
		t.Fatal(err)
	}
	u := url.URL{Scheme: "file", Path: path}
	reader, err := sql.Open("sqlite", u.String()+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	c, err := reader.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, q := range []string{`PRAGMA foreign_keys=ON`, `PRAGMA busy_timeout=5000`, `PRAGMA synchronous=FULL`, `PRAGMA query_only=ON`} {
		if _, err = c.ExecContext(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	v, err := Inspect(context.Background(), c)
	if err != nil || v != LegacyReference {
		t.Fatalf("version=%v err=%v", v, err)
	}
	if err = Verify(context.Background(), c); !errors.Is(err, ErrInvalid) {
		t.Fatalf("legacy Verify %v", err)
	}
}

func TestVerifyPhysicalReadOnlyConnection(t *testing.T) {
	db := legacy(t)
	if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
		t.Fatal(err)
	}
	var id int
	var name, path string
	if err := db.QueryRow(`PRAGMA database_list`).Scan(&id, &name, &path); err != nil {
		t.Fatal(err)
	}
	u := url.URL{Scheme: "file", Path: path}
	reader, err := sql.Open("sqlite", u.String()+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	c, err := reader.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, q := range []string{`PRAGMA foreign_keys=ON`, `PRAGMA busy_timeout=5000`, `PRAGMA synchronous=FULL`, `PRAGMA query_only=ON`} {
		if _, err = c.ExecContext(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	if err = Verify(context.Background(), c); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRejectsUTF16GuardedReference(t *testing.T) {
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "utf16.sqlite"))
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA main.encoding='UTF-16'`); err != nil {
		t.Fatal(err)
	}
	if err := ipamstore.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := eventlog.Init(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{guardSQL, markerDDL, `INSERT INTO eventlog_reference_guard_migrations VALUES(1,'` + baseDigest + `','` + guardDigest + `')`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	c := connQueryOnly(t, db)
	var enc string
	if err := c.QueryRowContext(context.Background(), `PRAGMA main.encoding`).Scan(&enc); err != nil || !strings.HasPrefix(enc, "UTF-16") {
		t.Fatalf("fixture encoding %q %v", enc, err)
	}
	if err := Verify(context.Background(), c); !errors.Is(err, ErrInvalid) {
		t.Fatalf("UTF16 accepted %v", err)
	}
}
func TestVerifyRejectsInvalidRowsEvenWithExactDDL(t *testing.T) {
	cases := []struct {
		name, trigger, insert string
		id                    any
	}{
		{"nonpositive", "replica_head_rowid_positive_guard", tableCases[1].first, int64(0)},
		{"head key text", "replica_head_cluster_type_guard", strings.Replace(tableCases[1].first, "x'01000000000000000000000000000000'", "'abcdefghijklmnop'", 1), int64(10)},
		{"checkpoint source text", "replica_checkpoint_cluster_type_guard", strings.Replace(tableCases[3].first, "x'02000000000000000000000000000000'", "'abcdefghijklmnop'", 1), int64(10)},
		{"receipt blob principal", "idempotency_receipt_key_type_guard", strings.Replace(tableCases[4].first, "'a','POST','/a','a'", "x'61','POST','/a','a'", 1), int64(10)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := legacy(t)
			if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
				t.Fatal(err)
			}
			var ddl string
			if err := db.QueryRow(`SELECT sql FROM main.sqlite_schema WHERE type='trigger' AND name=?`, tc.trigger).Scan(&ddl); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`DROP TRIGGER ` + tc.trigger); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.insert, tc.id); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(ddl); err != nil {
				t.Fatal(err)
			}
			c := connQueryOnly(t, db)
			if err := Verify(context.Background(), c); !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid existing data accepted: %v", err)
			}
		})
	}
}
func TestBoundedSchemaObjectCount(t *testing.T) {
	db := legacy(t)
	if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := 0; i < maxObjects+1; i++ {
		fmt.Fprintf(&b, "CREATE TABLE overlimit_%d(x);", i)
	}
	if _, err := db.Exec(b.String()); err != nil {
		t.Fatal(err)
	}
	c := connQueryOnly(t, db)
	if err := Verify(context.Background(), c); !errors.Is(err, ErrInvalid) {
		t.Fatalf("too many objects accepted %v", err)
	}
}
func TestVerifyPreservesUnderlyingSQLCause(t *testing.T) {
	db := legacy(t)
	if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
		t.Fatal(err)
	}
	c := connQueryOnly(t, db)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	err := Verify(context.Background(), c)
	if !errors.Is(err, ErrInvalid) || !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("underlying connection error lost %v", err)
	}
}
