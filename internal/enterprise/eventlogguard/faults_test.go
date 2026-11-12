package eventlogguard

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestPreexistingPoolPollutionRetiresConnection(t *testing.T) {
	for _, kind := range []string{"temp", "attach", "policy"} {
		t.Run(kind, func(t *testing.T) {
			db := legacy(t)
			ctx := context.Background()
			old, err := db.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = old.ExecContext(ctx, `CREATE TEMP TABLE retired_sentinel(x)`); err != nil {
				t.Fatal(err)
			}
			if kind == "attach" {
				_, err = old.ExecContext(ctx, `ATTACH DATABASE ':memory:' AS intruder`)
			}
			if kind == "policy" {
				_, err = old.ExecContext(ctx, `PRAGMA foreign_keys=OFF`)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = old.Close(); err != nil {
				t.Fatal(err)
			}
			if err = migrateFixture(ctx, db, hooks{}); err == nil {
				t.Fatal("polluted connection accepted")
			}
			fresh, err := db.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			var temp, fk int
			if err = fresh.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_temp_schema`).Scan(&temp); err != nil {
				t.Fatal(err)
			}
			if err = fresh.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
				t.Fatal(err)
			}
			rows, err := fresh.QueryContext(ctx, `PRAGMA database_list`)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var id int
				var name, path string
				if err = rows.Scan(&id, &name, &path); err != nil {
					t.Fatal(err)
				}
				if name != "main" && name != "temp" {
					t.Fatalf("attached database remained: %s", name)
				}
			}
			if err = rows.Err(); err != nil {
				t.Fatal(err)
			}
			if err = rows.Close(); err != nil {
				t.Fatal(err)
			}
			if temp != 0 || fk != 1 {
				t.Fatalf("old physical connection reused temp=%d fk=%d", temp, fk)
			}
			if err = fresh.Close(); err != nil {
				t.Fatal(err)
			}
			if err = migrateFixture(ctx, db, hooks{}); err != nil {
				t.Fatalf("fresh retry failed: %v", err)
			}
		})
	}
}

func TestInjectedEnvironmentDriftRollsBack(t *testing.T) {
	for _, kind := range []string{"temp trigger", "attach"} {
		t.Run(kind, func(t *testing.T) {
			db := legacy(t)
			fired := false
			h := hooks{afterDDL: func(c *sql.Conn) error {
				q := `CREATE TEMP TRIGGER intruder BEFORE INSERT ON main.replica_head BEGIN SELECT RAISE(IGNORE); END`
				if kind == "attach" {
					q = `ATTACH DATABASE ':memory:' AS intruder`
				}
				_, err := c.ExecContext(context.Background(), q)
				fired = err == nil
				return err
			}}
			err := migrateFixture(context.Background(), db, h)
			if !fired || err == nil {
				t.Fatalf("injection fired=%v err=%v", fired, err)
			}
			schemaVersion(t, db, 0)
			if err = migrateFixture(context.Background(), db, hooks{}); err != nil {
				t.Fatalf("connection not safely reusable: %v", err)
			}
		})
	}
}
func TestPanicNilAndGoexitRollback(t *testing.T) {
	if os.Getenv("EVENTLOGGUARD_NILPANIC") == "1" {
		db := legacy(t)
		db.SetMaxOpenConns(2)
		fired := false
		func() {
			defer func() { _ = recover() }()
			_ = migrateFixture(context.Background(), db, hooks{afterDDL: func(*sql.Conn) error { fired = true; panic(nil) }})
			t.Error("panic incorrectly returned")
		}()
		if !fired {
			t.Fatal("hook not reached")
		}
		schemaVersion(t, db, 0)
		if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
			t.Fatalf("rollback lock retained: %v", err)
		}
		return
	}
	t.Run("panicnil legacy runtime", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPanicNilAndGoexitRollback$")
		cmd.Env = append(os.Environ(), "GODEBUG=panicnil=1", "EVENTLOGGUARD_NILPANIC=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child %v %s", err, out)
		}
	})
	t.Run("Goexit", func(t *testing.T) {
		db := legacy(t)
		db.SetMaxOpenConns(2)
		done := make(chan struct{})
		fired, returned := false, false
		go func() {
			defer close(done)
			_ = migrateFixture(context.Background(), db, hooks{afterDDL: func(*sql.Conn) error { fired = true; runtime.Goexit(); return nil }})
			returned = true
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("goroutine stuck")
		}
		if !fired || returned {
			t.Fatalf("fired=%v returned=%v", fired, returned)
		}
		schemaVersion(t, db, 0)
		if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
			t.Fatalf("rollback lock retained: %v", err)
		}
	})
}
func TestSchemaAndMarkerDriftRejected(t *testing.T) {
	for name, q := range map[string]string{"extra trigger": `CREATE TRIGGER rogue BEFORE INSERT ON replica_head BEGIN SELECT 1; END`, "extra index": `CREATE INDEX rogue ON replica_head(seq)`, "extra table": `CREATE TABLE rogue(x)`, "ipam marker": `UPDATE enterprise_schema_migrations SET digest='wrong' WHERE version=1`, "missing trigger": `DROP TRIGGER replica_event_no_update`} {
		t.Run(name, func(t *testing.T) {
			db := legacy(t)
			if _, err := db.Exec(q); err != nil {
				t.Fatal(err)
			}
			if err := migrateFixture(context.Background(), db, hooks{}); err == nil {
				t.Fatal("old schema drift accepted")
			}
			schemaVersion(t, db, 0)
		})
	}
	for name, q := range map[string]string{"guard missing": `DROP TRIGGER replica_head_rowid_insert_guard`, "marker forged": `UPDATE eventlog_reference_guard_migrations SET guard_digest='wrong'`, "marker missing": `DELETE FROM eventlog_reference_guard_migrations`, "marker extra": `INSERT INTO eventlog_reference_guard_migrations VALUES(2,'x','y')`} {
		t.Run(name, func(t *testing.T) {
			db := legacy(t)
			if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(q); err != nil {
				t.Fatal(err)
			}
			if err := migrateFixture(context.Background(), db, hooks{}); err == nil {
				t.Fatal("new schema drift accepted")
			}
		})
	}
}
func TestClosedContext(t *testing.T) {
	db := legacy(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := migrateFixture(ctx, db, hooks{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel %v", err)
	}
	schemaVersion(t, db, 0)
}
