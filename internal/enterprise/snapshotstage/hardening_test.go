package snapshotstage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jincaiw/GoDDI/internal/enterprise/replicaschema"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotipam"
)

func assertRemoved(t *testing.T, path string) {
	t.Helper()
	if path == "" {
		t.Fatal("path hook not reached")
	}
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("candidate remained %s: %v", p, err)
		}
	}
}
func hooksForStop(kind string, stop func(), path *string) fixtureHooks {
	h := fixtureHooks{seePath: func(p string) {
		*path = p
		if kind == "seePath" {
			stop()
		}
	}}
	switch kind {
	case "afterCreate":
		h.afterCreate = func(*sql.DB) error { stop(); return nil }
	case "beforeInsert":
		h.beforeInsert = func(*sql.Conn) error { stop(); return nil }
	case "beforeCommit":
		h.beforeCommit = func() error { stop(); return nil }
	case "afterCommit":
		h.afterCommit = func() error { stop(); return nil }
	}
	return h
}
func TestPanicNilCleanupSubprocess(t *testing.T) {
	if kind := os.Getenv("SNAPSHOTSTAGE_PANIC_KIND"); kind != "" {
		m, tr, chunks := snapshot(t, sampleLines())
		var path string
		fired := false
		h := hooksForStop(kind, func() { fired = true; panic(nil) }, &path)
		s, err := importFixture(t, context.Background(), m, tr, node(), chunks, h)
		if !fired || s != nil || !errors.Is(err, errFixture) {
			t.Fatalf("kind=%s fired=%v stage=%v error=%v", kind, fired, s, err)
		}
		assertRemoved(t, path)
		return
	}
	for _, kind := range []string{"seePath", "afterCreate", "beforeInsert", "beforeCommit", "afterCommit"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPanicNilCleanupSubprocess$")
			cmd.Env = append(os.Environ(), "GODEBUG=panicnil=1", "SNAPSHOTSTAGE_PANIC_KIND="+kind)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("child %s: %v\n%s", kind, err, output)
			}
		})
	}
}
func TestGoexitCleansBeforeReturn(t *testing.T) {
	for _, kind := range []string{"seePath", "afterCreate", "beforeInsert", "beforeCommit", "afterCommit"} {
		t.Run(kind, func(t *testing.T) {
			m, tr, chunks := snapshot(t, sampleLines())
			var path string
			var fired, returned bool
			done := make(chan struct{})
			go func() {
				defer close(done)
				h := hooksForStop(kind, func() { fired = true; runtime.Goexit() }, &path)
				_, _ = importFixture(t, context.Background(), m, tr, node(), chunks, h)
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
			assertRemoved(t, path)
		})
	}
}
func TestCleanupErrorRemainsVisible(t *testing.T) {
	failure := errors.New("unlink failed")
	calls := 0
	s := &stageFixture{path: filepath.Join(t.TempDir(), "candidate.sqlite"), remove: func(string) error {
		calls++
		if calls == 1 {
			return failure
		}
		return nil
	}}
	first := s.close()
	second := s.close()
	if !errors.Is(first, failure) || !errors.Is(second, failure) || calls != 3 {
		t.Fatalf("error swallowed or repeated operations: %d", calls)
	}
}
func TestSinkMutationCannotChangeVerifiedRow(t *testing.T) {
	m, tr, chunks := snapshot(t, sampleLines())
	fired := false
	h := fixtureHooks{sink: func(r snapshotipam.Row) error {
		if r.Table == "ipam_tenants" {
			old := []byte("租户")
			replacement := []byte("伪造")
			i := bytes.Index(r.Payload, old)
			if i < 0 {
				return errors.New("missing expected Unicode")
			}
			copy(r.Payload[i:], replacement)
			fired = true
		}
		return nil
	}}
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, h)
	if err != nil || !fired {
		t.Fatalf("fired=%v error=%v", fired, err)
	}
	var name string
	if e := s.conn.QueryRowContext(context.Background(), `SELECT display_name FROM ipam_tenants WHERE id='t'`).Scan(&name); e != nil || name != "租户" {
		t.Fatalf("changed payload imported %q %v", name, e)
	}
}
func TestSchemaDriftAfterCommitRejected(t *testing.T) {
	for name, q := range map[string]string{"drop": `DROP TRIGGER ipam_tenants_rowid_insert_guard`, "extra": `CREATE TRIGGER unexpected BEFORE INSERT ON ipam_tenants BEGIN SELECT 1; END`, "temp": `CREATE TEMP TRIGGER unexpected BEFORE INSERT ON main.ipam_tenants BEGIN SELECT 1; END`, "shadow": `CREATE TEMP TABLE ipam_resource_ids(kind,tenant_id,space_id,id)`} {
		t.Run(name, func(t *testing.T) {
			m, tr, chunks := snapshot(t, sampleLines())
			var path string
			fired, committed := false, false
			h := fixtureHooks{seePath: func(p string) { path = p }, beforeInsert: func(c *sql.Conn) error {
				_, err := c.ExecContext(context.Background(), q)
				fired = err == nil
				return err
			}, afterCommit: func() error { committed = true; return nil }}
			s, err := importFixture(t, context.Background(), m, tr, node(), chunks, h)
			if !fired || !committed || s != nil || !errors.Is(err, replicaschema.ErrInvalid) {
				t.Fatalf("fired=%v committed=%v err=%v", fired, committed, err)
			}
			assertRemoved(t, path)
		})
	}
}
func TestIgnoreTriggeredBeforeCommit(t *testing.T) {
	m, tr, chunks := snapshot(t, sampleLines())
	var path string
	installed, committed := false, false
	h := fixtureHooks{seePath: func(p string) { path = p }, beforeInsert: func(c *sql.Conn) error {
		_, e := c.ExecContext(context.Background(), `CREATE TEMP TRIGGER suppress BEFORE INSERT ON main.ipam_tenants BEGIN SELECT RAISE(IGNORE); END`)
		installed = e == nil
		return e
	}, beforeCommit: func() error { committed = true; return nil }}
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, h)
	if !installed || committed || s != nil || !errors.Is(err, errFixture) {
		t.Fatalf("installed=%v committed=%v err=%v", installed, committed, err)
	}
	assertRemoved(t, path)
}
func TestIndependentTypedColumnAssertions(t *testing.T) {
	lines := sampleLines()
	v6 := make([]byte, 16)
	v6[0] = 0x20
	v6[1] = 0x01
	v6[2] = 0x0d
	v6[3] = 0xb8
	v6[15] = 1
	encoded := base64.RawURLEncoding.EncodeToString(v6)
	extra := line{"ipam_prefixes", map[string]any{"tenant_id": "t", "space_id": "s", "id": "b-v6", "parent_id": nil, "family": "6", "prefix_len": "128", "first_ip": encoded, "last_ip": encoded, "kind": "SUBNET", "usage": "HOST", "site_ref": nil, "label": "六", "revision": "8"}}
	lines = append(lines[:3], append([]line{extra}, lines[3:]...)...)
	for i, l := range lines {
		if l.Table == "ipam_resource_ids" && l.Row["id"] == "z-parent" {
			lines = append(lines[:i], append([]line{ledger("PREFIX", "t", "s", "b-v6")}, lines[i:]...)...)
			break
		}
	}
	m, tr, chunks := snapshot(t, lines)
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		q    string
		want []any
	}{
		{`SELECT id,slug,display_name,revision FROM ipam_tenants WHERE id='t'`, []any{"t", "t", "租户", int64(2)}},
		{`SELECT tenant_id,id,slug,display_name,revision FROM ipam_spaces WHERE id='s'`, []any{"t", "s", "s", "Space", int64(3)}},
		{`SELECT tenant_id,space_id,id,parent_id,family,prefix_len,first_ip,last_ip,kind,usage,site_ref,label,revision FROM ipam_prefixes WHERE id='a-subnet'`, []any{"t", "s", "a-subnet", "z-parent", int64(4), int64(24), []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 192, 0, 2, 0}, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 192, 0, 2, 255}, "SUBNET", "STANDARD", nil, "subnet", int64(4)}},
		{`SELECT tenant_id,space_id,id,parent_id,family,prefix_len,first_ip,last_ip,kind,usage,site_ref,label,revision FROM ipam_prefixes WHERE id='b-v6'`, []any{"t", "s", "b-v6", nil, int64(6), int64(128), []byte{0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, []byte{0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, "SUBNET", "HOST", nil, "六", int64(8)}},
		{`SELECT tenant_id,space_id,id,parent_id,family,prefix_len,first_ip,last_ip,kind,usage,site_ref,label,revision FROM ipam_prefixes WHERE id='z-parent'`, []any{"t", "s", "z-parent", nil, int64(4), int64(16), []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 192, 0, 0, 0}, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 192, 0, 255, 255}, "CONTAINER", "AGGREGATE", "site", "parent", int64(5)}},
		{`SELECT tenant_id,space_id,id,subnet_id,family,address,kind,owner_ref,expires_at_ms,created_at_ms,revision FROM ipam_occupancies WHERE id='o'`, []any{"t", "s", "o", "a-subnet", int64(4), []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 192, 0, 2, 10}, "STATIC", "host", nil, int64(1), int64(6)}},
	}
	for _, tc := range cases {
		got := make([]any, len(tc.want))
		ptr := make([]any, len(got))
		for i := range got {
			ptr[i] = &got[i]
		}
		if err := s.conn.QueryRowContext(context.Background(), tc.q).Scan(ptr...); err != nil {
			t.Fatal(err)
		}
		for i, w := range tc.want {
			if wb, ok := w.([]byte); ok {
				gb, ok := got[i].([]byte)
				if !ok || !bytes.Equal(gb, wb) {
					t.Fatalf("blob col %d", i)
				}
			} else if got[i] != w {
				t.Fatalf("column %d got %v (%T) want %v (%T)", i, got[i], got[i], w, w)
			}
		}
	}
}
