package snapshotstage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

func insertLedger(rows []line, l line) []line {
	out := append([]line(nil), rows...)
	out = append(out, l)
	for i := len(out) - 1; i > 0; i-- {
		if out[i-1].Table != "ipam_resource_ids" {
			break
		}
		a, b := out[i-1].Row, out[i].Row
		left := a["kind"].(string) + "\x00" + a["tenant_id"].(string) + "\x00" + a["space_id"].(string) + "\x00" + a["id"].(string)
		right := b["kind"].(string) + "\x00" + b["tenant_id"].(string) + "\x00" + b["space_id"].(string) + "\x00" + b["id"].(string)
		if left <= right {
			break
		}
		out[i-1], out[i] = out[i], out[i-1]
	}
	return out
}
func TestUTF16DatabaseRejectedBeforeImport(t *testing.T) {
	m, tr, chunks := snapshot(t, sampleLines())
	var path string
	configured, reachedInsert := false, false
	h := fixtureHooks{seePath: func(p string) { path = p }, afterCreate: func(db *sql.DB) error {
		_, e := db.Exec(`PRAGMA main.encoding='UTF-16'`)
		configured = e == nil
		return e
	}, beforeInsert: func(*sql.Conn) error { reachedInsert = true; return nil }}
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, h)
	if !configured || reachedInsert || s != nil || !errors.Is(err, errFixture) {
		t.Fatalf("configured=%v insert=%v err=%v", configured, reachedInsert, err)
	}
	assertRemoved(t, path)
}
func TestHistoricalLedgerPreservesScopedTombstones(t *testing.T) {
	lines := sampleLines()
	for _, l := range []line{ledger("TENANT", "", "", "old"), ledger("PREFIX", "t", "s", "retired"), ledger("POOL", "t", "s", "retired"), ledger("PREFIX", "t", "other", "retired")} {
		lines = insertLedger(lines, l)
	}
	m, tr, chunks := snapshot(t, lines)
	var c *sql.Conn
	tested := false
	h := fixtureHooks{beforeInsert: func(conn *sql.Conn) error { c = conn; return nil }, afterCommit: func() error {
		_, err := c.ExecContext(context.Background(), `INSERT INTO main.ipam_tenants(id,slug,display_name) VALUES('old','old','Old')`)
		if err == nil || !strings.Contains(err.Error(), "IPAM_ID_ALREADY_USED") {
			return errors.New("historic ID reusable")
		}
		tested = true
		return nil
	}}
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, h)
	if err != nil || !tested {
		t.Fatalf("error=%v tested=%v", err, tested)
	}
	var count int
	if err = s.conn.QueryRowContext(context.Background(), `SELECT count(*) FROM main.ipam_resource_ids`).Scan(&count); err != nil || count != 9 {
		t.Fatalf("ledger count %d %v", count, err)
	}
}
func TestRejectBadLedger(t *testing.T) {
	for _, kind := range []string{"missing active", "missing ledger", "wrong scope", "unknown kind", "duplicate", "wrong table order", "silent ignore historical", "cancel after commit"} {
		t.Run(kind, func(t *testing.T) {
			lines := sampleLines()
			switch kind {
			case "missing active":
				lines = append(lines[:5], lines[6:]...)
			case "missing ledger":
				lines = lines[:5]
			case "wrong scope":
				lines[6].Row["space_id"] = "other"
			case "unknown kind":
				lines[5].Row["kind"] = "NOT_A_KIND"
			case "duplicate":
				lines = append(lines[:6], append([]line{lines[5]}, lines[6:]...)...)
			case "wrong table order":
				lines[4], lines[5] = lines[5], lines[4]
			case "silent ignore historical":
				lines = insertLedger(lines, ledger("POOL", "t", "s", "retired"))
			}
			m, tr, chunks := snapshot(t, lines)
			var path string
			fired, committed := false, false
			h := fixtureHooks{seePath: func(p string) { path = p }}
			ctx := context.Background()
			var cancel context.CancelFunc
			switch kind {
			case "silent ignore historical":
				h.beforeInsert = func(c *sql.Conn) error {
					_, e := c.ExecContext(ctx, `CREATE TEMP TRIGGER skip_history BEFORE INSERT ON main.ipam_resource_ids WHEN NEW.kind='POOL' BEGIN SELECT RAISE(IGNORE); END`)
					fired = e == nil
					return e
				}
				h.beforeCommit = func() error { committed = true; return nil }
			case "cancel after commit":
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
				h.afterCommit = func() error { fired = true; cancel(); return nil }
			}
			s, err := importFixture(t, ctx, m, tr, node(), chunks, h)
			if err == nil || s != nil {
				t.Fatal("invalid ledger accepted")
			}
			if kind == "silent ignore historical" && (!fired || committed || !errors.Is(err, errFixture)) {
				t.Fatalf("ignore injection did not stop insertion: %v", err)
			}
			if kind == "cancel after commit" && (!fired || !errors.Is(err, context.Canceled)) {
				t.Fatalf("cancel phase: %v", err)
			}
			assertRemoved(t, path)
		})
	}
}
