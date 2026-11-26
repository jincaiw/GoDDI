package snapshotstage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotipam"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotverify"
)

func ip(a, b, c, d byte) string {
	x := make([]byte, 16)
	copy(x[12:], []byte{a, b, c, d})
	return base64.RawURLEncoding.EncodeToString(x)
}

type line struct {
	Table string         `json:"table"`
	Row   map[string]any `json:"row"`
}

func ledger(kind, tenant, space, id string) line {
	return line{"ipam_resource_ids", map[string]any{"kind": kind, "tenant_id": tenant, "space_id": space, "id": id}}
}
func sampleLines() []line {
	return []line{
		{"ipam_tenants", map[string]any{"id": "t", "slug": "t", "display_name": "租户", "revision": "2"}},
		{"ipam_spaces", map[string]any{"tenant_id": "t", "id": "s", "slug": "s", "display_name": "Space", "revision": "3"}},
		{"ipam_prefixes", map[string]any{"tenant_id": "t", "space_id": "s", "id": "a-subnet", "parent_id": "z-parent", "family": "4", "prefix_len": "24", "first_ip": ip(192, 0, 2, 0), "last_ip": ip(192, 0, 2, 255), "kind": "SUBNET", "usage": "STANDARD", "site_ref": nil, "label": "subnet", "revision": "4"}},
		{"ipam_prefixes", map[string]any{"tenant_id": "t", "space_id": "s", "id": "z-parent", "parent_id": nil, "family": "4", "prefix_len": "16", "first_ip": ip(192, 0, 0, 0), "last_ip": ip(192, 0, 255, 255), "kind": "CONTAINER", "usage": "AGGREGATE", "site_ref": "site", "label": "parent", "revision": "5"}},
		{"ipam_occupancies", map[string]any{"tenant_id": "t", "space_id": "s", "id": "o", "subnet_id": "a-subnet", "family": "4", "address": ip(192, 0, 2, 10), "kind": "STATIC", "owner_ref": "host", "expires_at_ms": nil, "created_at_ms": "1", "revision": "6"}},
		ledger("OCCUPANCY", "t", "s", "o"),
		ledger("PREFIX", "t", "s", "a-subnet"),
		ledger("PREFIX", "t", "s", "z-parent"),
		ledger("SPACE", "t", "", "s"),
		ledger("TENANT", "", "", "t"),
	}
}
func snapshot(t *testing.T, lines []line) (snapshotverify.Manifest, snapshotverify.Trust, []chunk) {
	t.Helper()
	var raw []byte
	counts := map[string]uint64{}
	for _, l := range lines {
		b, e := json.Marshal(l)
		if e != nil {
			t.Fatal(e)
		}
		raw = append(raw, b...)
		raw = append(raw, '\n')
		counts[l.Table]++
	}
	m := snapshotverify.Manifest{Cluster: [16]byte{1}, Leader: [16]byte{2}, Snapshot: [16]byte{3}, Schema: 1, Anchor: snapshotverify.Position{Epoch: 1}, Encoding: 1, RetentionUntilMS: 200, TotalBytes: uint64(len(raw)), Tables: []snapshotverify.TableCount{{Name: "ipam_tenants", Rows: counts["ipam_tenants"]}, {Name: "ipam_spaces", Rows: counts["ipam_spaces"]}, {Name: "ipam_prefixes", Rows: counts["ipam_prefixes"]}, {Name: "ipam_pools", Rows: counts["ipam_pools"]}, {Name: "ipam_occupancies", Rows: counts["ipam_occupancies"]}, {Name: "ipam_resource_ids", Rows: counts["ipam_resource_ids"]}, {Name: "static_occupancy_audit", Rows: counts["static_occupancy_audit"]}, {Name: "idempotency_receipt", Rows: counts["idempotency_receipt"]}}}
	var parts []chunk
	if len(raw) > 0 {
		cut := len(raw) / 2
		parts = []chunk{{0, append([]byte(nil), raw[:cut]...)}, {1, append([]byte(nil), raw[cut:]...)}}
	}
	for _, c := range parts {
		m.Chunks = append(m.Chunks, snapshotverify.ChunkSpec{Index: c.index, Size: uint64(len(c.data)), SHA256: sha256.Sum256(c.data)})
	}
	tr := snapshotverify.Trust{Authenticated: true, Cluster: m.Cluster, Leader: m.Leader, Snapshot: m.Snapshot, Anchor: m.Anchor}
	return m, tr, parts
}
func node() [16]byte { return [16]byte{4} }
func TestFixtureParentAfterChildAndReadOnly(t *testing.T) {
	m, tr, chunks := snapshot(t, sampleLines())
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err = s.conn.QueryRowContext(context.Background(), `SELECT count(*) FROM ipam_resource_ids`).Scan(&n); err != nil || n != 5 {
		t.Fatalf("ledger=%d %v", n, err)
	}
	if _, err = s.conn.ExecContext(context.Background(), `INSERT INTO ipam_tenants(id,slug,display_name) VALUES('x','x','x')`); err == nil {
		t.Fatal("read-only stage accepted write")
	}
	if err = s.close(); err != nil {
		t.Fatal(err)
	}
	if err = s.close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(s.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("not removed: %v", err)
	}
}
func TestEmptyEightTableCandidate(t *testing.T) {
	m, tr, chunks := snapshot(t, nil)
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err = s.conn.QueryRowContext(context.Background(), `SELECT count(*) FROM ipam_resource_ids`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("empty count=%d err=%v", n, err)
	}
}

func TestPublicImportInstallStayClosed(t *testing.T) {
	if !errors.Is(Import(context.Background()), ErrNoAuthority) || !errors.Is(Install(context.Background()), ErrNoAuthority) {
		t.Fatal("authority exposed")
	}
}
func TestRejectAndDeleteCandidate(t *testing.T) {
	cases := []string{"missing chunk", "duplicate chunk", "wrong order", "bad hash", "bad size", "wrong count", "unknown table", "bad table order", "out of order key", "bad parent", "bad occupancy", "parent cycle", "bad identity", "sink panic", "sink nil panic", "sink error", "sqlite error", "ignored row", "ignored ledger", "cancelled", "before commit uncertain", "after commit uncertain"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			lines := sampleLines()
			switch name {
			case "bad parent":
				lines[2].Row["parent_id"] = "missing"
			case "bad occupancy":
				lines[4].Row["address"] = ip(198, 51, 100, 1)
			case "parent cycle":
				lines[3].Row["parent_id"] = "a-subnet"
			case "out of order key":
				lines[2], lines[3] = lines[3], lines[2]
			}
			m, tr, chunks := snapshot(t, lines)
			var path string
			h := fixtureHooks{seePath: func(p string) { path = p }}
			ctx := context.Background()
			switch name {
			case "missing chunk":
				chunks = chunks[:1]
			case "duplicate chunk":
				chunks[1] = chunks[0]
			case "wrong order":
				chunks[0], chunks[1] = chunks[1], chunks[0]
			case "bad hash":
				chunks[0].data[0] ^= 1
			case "bad size":
				chunks[0].data = chunks[0].data[:len(chunks[0].data)-1]
			case "wrong count":
				m.Tables[3].Rows++
			case "unknown table":
				m.Tables[3].Name = "unknown"
			case "bad table order":
				m.Tables[0], m.Tables[1] = m.Tables[1], m.Tables[0]
			case "bad identity":
				tr.Leader = [16]byte{9}
			case "sink panic":
				h.sink = func(snapshotipam.Row) error { panic("test") }
			case "sink nil panic":
				h.sink = func(snapshotipam.Row) error { panic(nil) }
			case "sink error":
				h.sink = func(snapshotipam.Row) error { return errors.New("sink") }
			case "sqlite error":
				h.beforeInsert = func(c *sql.Conn) error { _, e := c.ExecContext(ctx, `DROP TABLE ipam_occupancies`); return e }
			case "ignored row":
				h.beforeInsert = func(c *sql.Conn) error {
					_, e := c.ExecContext(ctx, `CREATE TEMP TRIGGER suppress BEFORE INSERT ON main.ipam_tenants BEGIN SELECT RAISE(IGNORE); END`)
					return e
				}
			case "ignored ledger":
				h.beforeInsert = func(c *sql.Conn) error {
					_, e := c.ExecContext(ctx, `CREATE TEMP TRIGGER suppress_ledger BEFORE INSERT ON main.ipam_resource_ids BEGIN SELECT RAISE(IGNORE); END`)
					return e
				}
			case "cancelled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			case "before commit uncertain":
				h.beforeCommit = func() error { return errors.New("simulated pre-commit uncertainty") }
			case "after commit uncertain":
				h.afterCommit = func() error { return errors.New("simulated post-commit uncertainty") }
			}
			s, err := importFixture(t, ctx, m, tr, node(), chunks, h)
			if err == nil || s != nil {
				t.Fatalf("accepted %s", name)
			}
			if path != "" {
				for _, p := range []string{path, path + "-wal", path + "-shm"} {
					if _, e := os.Stat(p); !errors.Is(e, os.ErrNotExist) {
						t.Fatalf("untrusted file survived %s: %v", p, e)
					}
				}
			}
		})
	}
}
func TestManifestHardLimits(t *testing.T) {
	m, tr, chunks := snapshot(t, sampleLines())
	m.TotalBytes = fixtureBytes + 1
	if s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{}); err == nil || s != nil {
		t.Fatal("oversize accepted")
	}
	m, tr, chunks = snapshot(t, sampleLines())
	m.Tables[0].Rows = fixtureRows + 1
	if s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{}); err == nil || s != nil {
		t.Fatal("too many rows")
	}
}
func TestMismatchAfterCommitDiscarded(t *testing.T) {
	m, tr, chunks := snapshot(t, sampleLines())
	var conn *sql.Conn
	changed := false
	h := fixtureHooks{beforeInsert: func(c *sql.Conn) error { conn = c; return nil }, afterCommit: func() error {
		result, e := conn.ExecContext(context.Background(), `UPDATE ipam_tenants SET display_name='altered', revision=revision+1 WHERE id='t'`)
		if e != nil {
			return e
		}
		n, e := result.RowsAffected()
		changed = n == 1
		return e
	}}
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, h)
	if !changed || !errors.Is(err, errFixture) || s != nil {
		t.Fatal("changed content accepted")
	}
}
