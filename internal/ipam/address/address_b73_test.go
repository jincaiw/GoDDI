package address

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jasonwa/goddi"
)

// openB73DB uses two independent SQLite handles against one WAL database. The
// production database currently serialises through one pooled handle; this
// test deliberately exercises the allocation transaction at the boundary that
// a future multi-connection store must preserve.
func openB73DB(t *testing.T) (*sql.DB, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ipam-b73.db")
	open := func() *sql.DB {
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatalf("open database: %v", err)
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		for _, pragma := range []string{
			"PRAGMA busy_timeout = 5000",
			"PRAGMA foreign_keys = ON",
			"PRAGMA journal_mode = WAL",
			"PRAGMA synchronous = FULL",
		} {
			if _, err := db.Exec(pragma); err != nil {
				t.Fatalf("set %s: %v", pragma, err)
			}
		}
		return db
	}

	db1 := open()
	goose.SetBaseFS(goddiassets.Migrations())
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(db1, "."); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	db2 := open()
	t.Cleanup(func() {
		_ = db2.Close()
		_ = db1.Close()
	})
	return db1, db2
}

func TestB73_TwoIndependentConnectionsAllocateUniqueAddresses(t *testing.T) {
	db1, db2 := openB73DB(t)
	seedSubnet(t, db1, "b73-space", "b73-subnet", "192.0.2.0/24")
	managers := []*Manager{NewManager(db1), NewManager(db2)}

	const workers = 64
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]string, workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			var a *Address
			var err error
			for attempt := 0; attempt < 20; attempt++ {
				a, err = managers[i%len(managers)].AutoAllocateIP("b73-subnet", AllocateRequest{
					Actor: fmt.Sprintf("b73-worker-%d", i),
				})
				if err == nil || !isSQLiteBusy(err) {
					break
				}
			}
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = a.IPAddress
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d failed: %v", i, err)
		}
	}
	seen := make(map[string]bool, workers)
	for _, ip := range results {
		if seen[ip] {
			t.Fatalf("duplicate allocated address: %s", ip)
		}
		seen[ip] = true
	}
	if len(seen) != workers {
		t.Fatalf("unique addresses = %d, want %d", len(seen), workers)
	}

	var rows, distinct int
	if err := db1.QueryRow(`SELECT COUNT(*) FROM ipam_addresses WHERE subnet_id = 'b73-subnet'`).Scan(&rows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if err := db1.QueryRow(`SELECT COUNT(DISTINCT ip_address) FROM ipam_addresses WHERE subnet_id = 'b73-subnet'`).Scan(&distinct); err != nil {
		t.Fatalf("count distinct: %v", err)
	}
	if rows != workers || distinct != workers {
		t.Fatalf("stored rows=(%d,%d), want (%d,%d)", rows, distinct, workers, workers)
	}
}

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "sqlite_busy") || strings.Contains(msg, "database is locked")
}

func TestB73_SQLiteUsesWALAndFullSynchronousCommit(t *testing.T) {
	db1, _ := openB73DB(t)
	var journal, synchronous string
	if err := db1.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if err := db1.QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatalf("read synchronous: %v", err)
	}
	if journal != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journal)
	}
	if synchronous != "2" {
		t.Fatalf("synchronous = %q, want 2 (FULL)", synchronous)
	}
}
