package snapshotstage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"sort"
	"strings"
	"testing"
)

func pool4(id, space string, first, last byte, kind string) line {
	return line{"ipam_pools", map[string]any{"tenant_id": "t", "space_id": space, "id": id, "subnet_id": "a-subnet", "family": "4", "pool_kind": kind, "first_ip": ip(192, 0, 2, first), "last_ip": ip(192, 0, 2, last), "label": "池", "revision": "7"}}
}
func addPoolFixture(lines []line, extra ...line) []line {
	lines = append(lines, extra...)
	rank := map[string]int{"ipam_tenants": 0, "ipam_spaces": 1, "ipam_prefixes": 2, "ipam_pools": 3, "ipam_occupancies": 4, "ipam_resource_ids": 5}
	key := func(l line) string {
		r := l.Row
		if l.Table == "ipam_resource_ids" {
			return r["kind"].(string) + "\x00" + r["tenant_id"].(string) + "\x00" + r["space_id"].(string) + "\x00" + r["id"].(string)
		}
		var out string
		for _, k := range []string{"tenant_id", "space_id", "id"} {
			if v, ok := r[k].(string); ok {
				out += v + "\x00"
			}
		}
		return out
	}
	sort.SliceStable(lines, func(i, j int) bool {
		a, b := rank[lines[i].Table], rank[lines[j].Table]
		if a != b {
			return a < b
		}
		return key(lines[i]) < key(lines[j])
	})
	return lines
}
func TestPoolsV4V6AndScopedDuplicates(t *testing.T) {
	lines := sampleLines()
	v6start := []byte{0x20, 1, 0xd, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	v6end := append([]byte(nil), v6start...)
	v6end[15] = 255
	v6low := append([]byte(nil), v6start...)
	v6low[15] = 10
	v6high := append([]byte(nil), v6start...)
	v6high[15] = 20
	b64 := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	p6 := line{"ipam_prefixes", map[string]any{"tenant_id": "t", "space_id": "s", "id": "v6", "parent_id": nil, "family": "6", "prefix_len": "120", "first_ip": b64(v6start), "last_ip": b64(v6end), "kind": "SUBNET", "usage": "STANDARD", "site_ref": nil, "label": "v6", "revision": "9"}}
	pool6 := line{"ipam_pools", map[string]any{"tenant_id": "t", "space_id": "s", "id": "v6pool", "subnet_id": "v6", "family": "6", "pool_kind": "IPAM", "first_ip": b64(v6low), "last_ip": b64(v6high), "label": "六", "revision": "8"}}
	s2 := line{"ipam_spaces", map[string]any{"tenant_id": "t", "id": "s2", "slug": "s2", "display_name": "Scope 2", "revision": "1"}}
	p2 := line{"ipam_prefixes", map[string]any{"tenant_id": "t", "space_id": "s2", "id": "a-subnet", "parent_id": nil, "family": "4", "prefix_len": "24", "first_ip": ip(192, 0, 2, 0), "last_ip": ip(192, 0, 2, 255), "kind": "SUBNET", "usage": "STANDARD", "site_ref": nil, "label": "other", "revision": "1"}}
	lines = addPoolFixture(lines, s2, p2, p6, pool4("a-pool", "s", 100, 110, "DHCP"), pool4("a-pool", "s2", 100, 110, "DHCP"), pool6, ledger("SPACE", "t", "", "s2"), ledger("PREFIX", "t", "s", "v6"), ledger("PREFIX", "t", "s2", "a-subnet"), ledger("POOL", "t", "s", "a-pool"), ledger("POOL", "t", "s", "v6pool"), ledger("POOL", "t", "s2", "a-pool"))
	m, tr, chunks := snapshot(t, lines)
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{})
	if err != nil {
		t.Fatal(err)
	}
	// Independent hand-written expectations cover every pool field.
	cases := []struct {
		id, space string
		expected  []any
	}{{"a-pool", "s", []any{"t", "s", "a-pool", "a-subnet", int64(4), "DHCP", []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 192, 0, 2, 100}, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 192, 0, 2, 110}, "池", int64(7)}}, {"a-pool", "s2", []any{"t", "s2", "a-pool", "a-subnet", int64(4), "DHCP", []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 192, 0, 2, 100}, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 192, 0, 2, 110}, "池", int64(7)}}, {"v6pool", "s", []any{"t", "s", "v6pool", "v6", int64(6), "IPAM", []byte{0x20, 1, 0xd, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 10}, []byte{0x20, 1, 0xd, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 20}, "六", int64(8)}}}
	for _, tc := range cases {
		got := make([]any, len(tc.expected))
		ptr := make([]any, len(got))
		for i := range got {
			ptr[i] = &got[i]
		}
		err = s.conn.QueryRowContext(context.Background(), `SELECT tenant_id,space_id,id,subnet_id,family,pool_kind,first_ip,last_ip,label,revision FROM main.ipam_pools WHERE tenant_id='t' AND space_id=? AND id=?`, tc.space, tc.id).Scan(ptr...)
		if err != nil {
			t.Fatal(err)
		}
		for i, w := range tc.expected {
			if b, ok := w.([]byte); ok {
				actual, ok := got[i].([]byte)
				if !ok || !bytes.Equal(actual, b) {
					t.Fatalf("%s blob field %d", tc.id, i)
				}
			} else if got[i] != w {
				t.Fatalf("%s field %d got %v want %v", tc.id, i, got[i], w)
			}
		}
	}
}
func TestRejectPoolAndLedgerFailures(t *testing.T) {
	for _, name := range []string{"overlap", "outside", "static conflict", "missing pool ledger", "unknown pool kind", "pool ignore", "pool metadata drift"} {
		t.Run(name, func(t *testing.T) {
			lines := addPoolFixture(sampleLines(), pool4("a-pool", "s", 100, 110, "DHCP"), ledger("POOL", "t", "s", "a-pool"))
			var h fixtureHooks
			var path string
			h.seePath = func(p string) { path = p }
			fired := false
			var saved *sql.Conn
			switch name {
			case "overlap":
				lines = addPoolFixture(lines, pool4("b-pool", "s", 110, 120, "IPAM"), ledger("POOL", "t", "s", "b-pool"))
			case "outside":
				for _, r := range lines {
					if r.Table == "ipam_pools" {
						r.Row["last_ip"] = ip(192, 0, 3, 1)
					}
				}
			case "static conflict":
				for _, r := range lines {
					if r.Table == "ipam_pools" {
						r.Row["first_ip"] = ip(192, 0, 2, 10)
					}
				}
			case "missing pool ledger":
				for i, r := range lines {
					if r.Table == "ipam_resource_ids" && r.Row["kind"] == "POOL" {
						lines = append(lines[:i], lines[i+1:]...)
						break
					}
				}
			case "unknown pool kind":
				for _, r := range lines {
					if r.Table == "ipam_pools" {
						r.Row["pool_kind"] = "INVALID"
					}
				}
			case "pool ignore":
				h.beforeInsert = func(c *sql.Conn) error {
					_, e := c.ExecContext(context.Background(), `CREATE TEMP TRIGGER skip_pool BEFORE INSERT ON main.ipam_pools BEGIN SELECT RAISE(IGNORE); END`)
					fired = e == nil
					return e
				}
				h.beforeCommit = func() error { return errors.New("unexpected commit phase") }
			case "pool metadata drift":
				h.beforeInsert = func(c *sql.Conn) error { saved = c; return nil }
				h.afterCommit = func() error {
					_, e := saved.ExecContext(context.Background(), `UPDATE main.ipam_pools SET label='wrong',revision=revision+1 WHERE id='a-pool'`)
					fired = e == nil
					return e
				}
			}
			m, tr, chunks := snapshot(t, lines)
			s, err := importFixture(t, context.Background(), m, tr, node(), chunks, h)
			if s != nil || err == nil {
				t.Fatal("invalid pool accepted")
			}
			want := map[string]string{"overlap": "IPAM_POOL_OVERLAP", "outside": "IPAM_POOL_OUTSIDE_SUBNET", "static conflict": "IPAM_ADDRESS_DHCP_MANAGED", "missing pool ledger": "active IDs missing from source ledger"}[name]
			if want != "" && !strings.Contains(err.Error(), want) {
				t.Fatalf("wrong failure phase: %v", err)
			}
			if (name == "pool ignore" || name == "pool metadata drift") && (!fired || !errors.Is(err, errFixture)) {
				t.Fatalf("injection not verified: %v", err)
			}
			assertRemoved(t, path)
		})
	}
}
func TestRetiredPoolIDCannotBeReused(t *testing.T) {
	lines := addPoolFixture(sampleLines(), ledger("POOL", "t", "s", "retired"))
	m, tr, chunks := snapshot(t, lines)
	var saved *sql.Conn
	tested := false
	h := fixtureHooks{beforeInsert: func(c *sql.Conn) error { saved = c; return nil }, afterCommit: func() error {
		_, e := saved.ExecContext(context.Background(), `INSERT INTO main.ipam_pools(tenant_id,space_id,id,subnet_id,family,pool_kind,first_ip,last_ip) VALUES('t','s','retired','a-subnet',4,'IPAM',x'000000000000000000000000C0000264',x'000000000000000000000000C000026E')`)
		tested = e != nil && strings.Contains(e.Error(), "IPAM_ID_ALREADY_USED")
		if !tested {
			return errors.New("retired pool ID reused")
		}
		return nil
	}}
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, h)
	if err != nil || s == nil || !tested {
		t.Fatalf("tested=%v err=%v", tested, err)
	}
}
