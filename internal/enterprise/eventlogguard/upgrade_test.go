package eventlogguard

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/ipamstore"
	"github.com/jasonwa/goddi/internal/sqlitepolicy"
)

func legacy(t *testing.T) *sql.DB {
	t.Helper()
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "reference.sqlite"))
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if err := ipamstore.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := eventlog.Init(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}
func schemaVersion(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM main.sqlite_schema WHERE type='table' AND name='eventlog_reference_guard_migrations'`).Scan(&n); err != nil || n != want {
		t.Fatalf("marker=%d want=%d err=%v", n, want, err)
	}
}
func TestFrozenIPAMPathsAndDigests(t *testing.T) {
	paths := ipamstore.MigrationPaths()
	if err := verifyIPAMPins(paths, ipamstore.DigestOf); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{paths[:1], append(append([]string(nil), paths...), "migrations/003.sql"), {paths[1], paths[0]}, {paths[0], "migrations/replaced.sql"}} {
		if err := verifyIPAMPins(bad, ipamstore.DigestOf); err == nil {
			t.Fatalf("accepted paths %v", bad)
		}
	}
	if err := verifyIPAMPins(paths, func(string) (string, bool) { return strings.Repeat("0", 64), true }); err == nil {
		t.Fatal("accepted digest drift")
	}
}

func TestLegacyUpgradeAndRepeat(t *testing.T) {
	db := legacy(t)
	if _, err := db.Exec(`INSERT INTO main.replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(x'01000000000000000000000000000000',1,4,zeroblob(32),1,99)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
			t.Fatal(err)
		}
	}
	schemaVersion(t, db, 1)
	var seq int
	if err := db.QueryRow(`SELECT seq FROM main.replica_head`).Scan(&seq); err != nil || seq != 4 {
		t.Fatalf("data %d %v", seq, err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM main.sqlite_schema WHERE type='trigger' AND name LIKE '%_guard' AND (tbl_name IN ('writer_authority','replica_head','replica_event','replica_checkpoint','idempotency_receipt','static_occupancy_audit'))`).Scan(&n); err != nil || n != 28 {
		t.Fatalf("guards %d %v", n, err)
	}
}
func TestEmptyAndPublicRemainClosed(t *testing.T) {
	if !errors.Is(Migrate(context.Background(), nil), ErrNoAuthority) {
		t.Fatal("public opened")
	}
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "empty.sqlite"))
	defer db.Close()
	if err := migrateFixture(context.Background(), db, hooks{}); err == nil {
		t.Fatal("empty accepted")
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM main.sqlite_schema`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("empty changed: %d %v", n, err)
	}
	if migrateFixture(nil, db, hooks{}) == nil || migrateFixture(context.Background(), nil, hooks{}) == nil {
		t.Fatal("nil accepted")
	}
}
func TestDDLAndMarkerFailureRollBack(t *testing.T) {
	for _, kind := range []string{"injected", "ignored marker", "cancellation", "before commit"} {
		t.Run(kind, func(t *testing.T) {
			db := legacy(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("injected")
			h := hooks{}
			switch kind {
			case "injected":
				h.afterDDL = func(*sql.Conn) error { return failure }
			case "ignored marker":
				h.afterDDL = func(c *sql.Conn) error {
					_, e := c.ExecContext(context.Background(), `CREATE TEMP TRIGGER suppress BEFORE INSERT ON main.eventlog_reference_guard_migrations BEGIN SELECT RAISE(IGNORE); END`)
					return e
				}
			case "cancellation":
				h.afterDDL = func(*sql.Conn) error { cancel(); return nil }
			case "before commit":
				h.beforeCommit = func() error { return failure }
			}
			if err := migrateFixture(ctx, db, h); err == nil {
				t.Fatal("fault accepted")
			}
			schemaVersion(t, db, 0)
			var n int
			if err := db.QueryRow(`SELECT count(*) FROM main.sqlite_schema WHERE type='trigger' AND name='replica_head_rowid_insert_guard'`).Scan(&n); err != nil || n != 0 {
				t.Fatalf("guard survived rollback %d %v", n, err)
			}
			if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
				t.Fatalf("retry after rollback %v", err)
			}
		})
	}
}
func TestSimulatedPostCommitUncertain(t *testing.T) {
	db := legacy(t)
	fail := errors.New("simulated lost acknowledgement")
	err := migrateFixture(context.Background(), db, hooks{afterCommit: func() error { return fail }})
	if !errors.Is(err, fail) {
		t.Fatalf("error %v", err)
	}
	schemaVersion(t, db, 1)
	if err = migrateFixture(context.Background(), db, hooks{}); err != nil {
		t.Fatalf("recheck committed state %v", err)
	}
}
func TestConcurrentLegacyMigration(t *testing.T) {
	db := legacy(t)
	db.SetMaxOpenConns(2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; out <- migrateFixture(context.Background(), db, hooks{}) }()
	}
	close(start)
	wg.Wait()
	close(out)
	for err := range out {
		if err != nil {
			t.Fatal(err)
		}
	}
	schemaVersion(t, db, 1)
}
