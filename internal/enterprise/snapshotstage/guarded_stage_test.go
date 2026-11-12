package snapshotstage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/eventlogguard"
	"github.com/jasonwa/goddi/internal/enterprise/idempotency"
)

func guardedLines() []line {
	lines := addPoolFixture(sampleLines(), pool4("p", "s", 100, 110, "DHCP"), ledger("POOL", "t", "s", "p"))
	return append(lines, auditRow([16]byte{1}), receiptRow([16]byte{7}, []byte{0, 0xff, '\n'}, []byte("tag")))
}
func TestGuardedEightTablesAndExpiredReceipt(t *testing.T) {
	lines := guardedLines()
	m, tr, chunks := snapshot(t, lines)
	for _, tc := range m.Tables {
		if tc.Rows == 0 {
			t.Fatalf("expected all eight tables: %s", tc.Name)
		}
	}
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{hardened: true})
	if err != nil {
		t.Fatal(err)
	}
	var blob []byte
	var created, expires int64
	if err = s.conn.QueryRowContext(context.Background(), `SELECT response,created_at_unix_ms,expires_at_unix_ms FROM main.idempotency_receipt`).Scan(&blob, &created, &expires); err != nil {
		t.Fatal(err)
	}
	want, err := base64.RawURLEncoding.DecodeString(lines[len(lines)-1].Row["response"].(string))
	if err != nil || !bytes.Equal(blob, want) || created != 1 || expires != 2 {
		t.Fatalf("expired receipt changed: %v", err)
	}
}
func TestGuardedVerifierRejectsOldFixture(t *testing.T) {
	m, tr, chunks := snapshot(t, sampleLines())
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{})
	if err != nil {
		t.Fatal(err)
	}
	if err = eventlogguard.Verify(context.Background(), s.conn); !errors.Is(err, eventlogguard.ErrInvalid) {
		t.Fatalf("guard verifier accepted old schema: %v", err)
	}
}
func TestGuardedEmptyAndMaximumReceipt(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		m, tr, chunks := snapshot(t, nil)
		s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{hardened: true})
		if err != nil || s == nil {
			t.Fatalf("empty: %v", err)
		}
	})
	t.Run("maximum", func(t *testing.T) {
		r := receiptRow([16]byte{7}, bytes.Repeat([]byte{0xff}, idempotency.MaxBodyBytes), bytes.Repeat([]byte{'e'}, idempotency.MaxETagBytes))
		lines := guardedLines()
		lines[len(lines)-1] = r
		m, tr, chunks := snapshot(t, lines)
		if len(chunks) != 2 || len(chunks[0].data) > 1<<20 || len(chunks[1].data) > 1<<20 {
			t.Fatal("receipt did not span bounded chunks")
		}
		s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{hardened: true})
		if err != nil {
			t.Fatal(err)
		}
		var got []byte
		if err = s.conn.QueryRowContext(context.Background(), `SELECT response FROM main.idempotency_receipt`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		want, err := base64.RawURLEncoding.DecodeString(r.Row["response"].(string))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("maximum response changed", err)
		}
	})
}
func TestGuardedTamperCancellationAndCleanup(t *testing.T) {
	for _, kind := range []string{"guard", "marker", "temp", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			m, tr, chunks := snapshot(t, guardedLines())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var path string
			fired, committed := false, false
			h := fixtureHooks{hardened: true, seePath: func(p string) { path = p }, afterCommit: func() error { committed = true; return nil }}
			h.beforeInsert = func(c *sql.Conn) error {
				var q string
				switch kind {
				case "guard":
					q = `DROP TRIGGER replica_head_rowid_insert_guard`
				case "marker":
					q = `UPDATE eventlog_reference_guard_migrations SET guard_digest='tampered'`
				case "temp":
					q = `CREATE TEMP TABLE guard_drift(x)`
				case "cancel":
					fired = true
					cancel()
					return nil
				}
				_, err := c.ExecContext(context.Background(), q)
				fired = err == nil
				return err
			}
			s, err := importFixture(t, ctx, m, tr, node(), chunks, h)
			if !fired || err == nil || s != nil {
				t.Fatalf("fired=%v stage=%v err=%v", fired, s, err)
			}
			if kind != "cancel" && (!committed || !errors.Is(err, eventlogguard.ErrInvalid)) {
				t.Fatalf("tamper was not rejected by the post-commit verifier: committed=%v err=%v", committed, err)
			}
			if kind == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel cause: %v", err)
			}
			assertRemoved(t, path)
		})
	}
}
func TestGuardedOldSchemaCancellationPreservesBothErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	oldErr := errors.New("old verifier driver failure")
	err := checkOldSchemaResult(ctx, oldErr)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, oldErr) {
		t.Fatalf("lost verifier or cancellation cause: %v", err)
	}
}

func TestGuardedCancellationAtOldSchemaCheck(t *testing.T) {
	m, tr, chunks := snapshot(t, guardedLines())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var path string
	fired := false
	h := fixtureHooks{hardened: true, seePath: func(p string) { path = p }}
	h.beforeInsert = func(c *sql.Conn) error {
		return verifyFixtureSchema(ctx, c, true, func() { fired = true; cancel() })
	}
	s, err := importFixture(t, ctx, m, tr, node(), chunks, h)
	if !fired || s != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("fired=%v stage=%v error=%v", fired, s, err)
	}
	assertRemoved(t, path)
}

func TestGuardedCASAndUpsertCompatibility(t *testing.T) {
	m, tr, chunks := snapshot(t, guardedLines())
	fired := false
	h := fixtureHooks{hardened: true, beforeInsert: func(c *sql.Conn) error {
		ctx := context.Background()
		if _, err := c.ExecContext(ctx, `SAVEPOINT guard_probe`); err != nil {
			return err
		}
		defer func() {
			_, _ = c.ExecContext(ctx, `ROLLBACK TO guard_probe`)
			_, _ = c.ExecContext(ctx, `RELEASE guard_probe`)
		}()
		if _, err := c.ExecContext(ctx, `INSERT INTO main.replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(zeroblob(16),1,0,zeroblob(32),1,1)`); err != nil {
			return err
		}
		result, err := c.ExecContext(ctx, `UPDATE main.replica_head SET seq=1 WHERE cluster_id=zeroblob(16) AND epoch=1 AND seq=0 AND hash=zeroblob(32)`)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil || n != 1 {
			return errors.New("CAS did not update once")
		}
		result, err = c.ExecContext(ctx, `UPDATE main.replica_head SET seq=2 WHERE cluster_id=zeroblob(16) AND epoch=1 AND seq=0 AND hash=zeroblob(32)`)
		if err != nil {
			return err
		}
		n, err = result.RowsAffected()
		if err != nil || n != 0 {
			return errors.New("stale CAS succeeded")
		}
		_, err = c.ExecContext(ctx, `INSERT INTO main.replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(zeroblob(16),1,2,zeroblob(32),1,2) ON CONFLICT(cluster_id) DO UPDATE SET seq=excluded.seq`)
		if err == nil || !strings.Contains(err.Error(), "EVENTLOG_KEY_CONFLICT") {
			return errors.New("UPSERT was not rejected by logical guard")
		}
		_, err = c.ExecContext(ctx, `INSERT OR REPLACE INTO main.replica_head(rowid,cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) SELECT rowid,x'01010101010101010101010101010101',epoch,seq,hash,schema_version,advanced_at_unix_ms FROM main.replica_head`)
		if err == nil || !strings.Contains(err.Error(), "EVENTLOG_ROWID_CONFLICT") {
			return errors.New("rowid replacement was not rejected")
		}
		fired = true
		return nil
	}}
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, h)
	if err != nil || s == nil || !fired {
		t.Fatalf("fired=%v err=%v", fired, err)
	}
}
