package snapshotipamcheck

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/ipamstore"
	"github.com/jincaiw/GoDDI/internal/sqlitepolicy"
)

func TestAcceptUTF8NonASCIIMain(t *testing.T) {
	db := fixture(t)
	if _, err := db.Exec(`INSERT INTO main.ipam_tenants(id,slug,display_name) VALUES('租户','unicode','租户')`); err != nil {
		t.Fatal(err)
	}
	c := conn(t, db, true)
	ctx := context.Background()
	var encoding string
	if err := c.QueryRowContext(ctx, `PRAGMA main.encoding`).Scan(&encoding); err != nil || encoding != "UTF-8" {
		t.Fatalf("encoding=%q err=%v", encoding, err)
	}
	l := limits()
	l.Expected.Tenants++
	l.Expected.Ledger++
	out, err := Check(ctx, c, l)
	if err != nil || out.Counts != l.Expected {
		t.Fatalf("UTF-8 rejected: %+v %v", out, err)
	}
}

func TestRejectUTF16MainDatabase(t *testing.T) {
	for _, encoding := range []string{"UTF-16le", "UTF-16be"} {
		t.Run(encoding, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "utf16.sqlite")
			seed, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			// The encoding must be set before the database's first table is created.
			for _, q := range []string{`PRAGMA encoding='` + encoding + `'`, `CREATE TABLE bootstrap(id TEXT)`, `DROP TABLE bootstrap`} {
				if _, err = seed.Exec(q); err != nil {
					seed.Close()
					t.Fatal(err)
				}
			}
			if err = seed.Close(); err != nil {
				t.Fatal(err)
			}
			db := sqlitepolicy.OpenWriter(path)
			db.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = db.Close() })
			ctx := context.Background()
			if err = ipamstore.Migrate(ctx, db); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`INSERT INTO main.ipam_tenants(id,slug,display_name) VALUES('租户','tenant','Tenant')`); err != nil {
				t.Fatal(err)
			}
			c := conn(t, db, true)
			var actual string
			if err = c.QueryRowContext(ctx, `PRAGMA main.encoding`).Scan(&actual); err != nil || actual != encoding {
				t.Fatalf("encoding=%q err=%v", actual, err)
			}
			var id string
			if err = c.QueryRowContext(ctx, `SELECT id FROM main.ipam_tenants`).Scan(&id); err != nil || id != "租户" {
				t.Fatalf("non-ASCII row: %q %v", id, err)
			}
			out, err := Check(ctx, c, Limits{Rows: 10, Bytes: 1024, Expected: Counts{Tenants: 1}})
			if !errors.Is(err, ErrInvalid) || out != (Evidence{}) {
				t.Fatalf("UTF-16 accepted: %+v %v", out, err)
			}
		})
	}
}
