package snapshotipam

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotpipeline"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotrows"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotverify"
)

func pool() map[string]any {
	return map[string]any{"tenant_id": "t", "space_id": "s", "id": "pool", "subnet_id": "sub", "family": "4", "pool_kind": "IPAM", "first_ip": ip4(192, 0, 2, 1), "last_ip": ip4(192, 0, 2, 10), "label": "", "revision": "1"}
}
func v6(s string) string {
	v := netip.MustParseAddr(s).As16()
	return base64.RawURLEncoding.EncodeToString(v[:])
}
func TestPoolValidAndCopy(t *testing.T) {
	for _, family := range []string{"4", "6"} {
		p := pool()
		p["family"] = family
		if family == "6" {
			p["pool_kind"] = "DHCP"
			p["first_ip"] = v6("2001:db8::1")
			p["last_ip"] = v6("2001:db8::ffff")
		}
		raw := row(t, p)
		want := string(raw)
		result, err := New().Validate("ipam_pools", raw)
		if err != nil {
			t.Fatal(err)
		}
		raw[0] = 'x'
		if string(result.Payload) != want {
			t.Fatal("source alias")
		}
	}
}
func TestPoolRejectInvalid(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"reverse4", func(p map[string]any) { p["first_ip"], p["last_ip"] = p["last_ip"], p["first_ip"] }},
		{"reverse6", func(p map[string]any) {
			p["family"] = "6"
			p["first_ip"] = v6("2001:db8::2")
			p["last_ip"] = v6("2001:db8::1")
		}},
		{"mapped crossing", func(p map[string]any) {
			p["family"] = "6"
			p["first_ip"] = v6("::fffe:ffff:ffff")
			p["last_ip"] = v6("::1:0:0:0")
		}},
		{"mapped endpoint", func(p map[string]any) {
			p["family"] = "6"
			p["first_ip"] = v6("::ffff:192.0.2.1")
			p["last_ip"] = v6("::ffff:192.0.2.2")
		}},
		{"bad v4 padding", func(p map[string]any) { p["first_ip"] = v6("2001:db8::1") }},
		{"length", func(p map[string]any) { p["first_ip"] = b64(1, 2) }}, {"padded", func(p map[string]any) { p["first_ip"] = p["first_ip"].(string) + "=" }},
		{"family numeric", func(p map[string]any) { p["family"] = 4 }}, {"family leading zero", func(p map[string]any) { p["family"] = "04" }},
		{"revision overflow", func(p map[string]any) { p["revision"] = "9223372036854775808" }}, {"revision 0", func(p map[string]any) { p["revision"] = "0" }},
		{"kind", func(p map[string]any) { p["pool_kind"] = "other" }}, {"null label", func(p map[string]any) { p["label"] = nil }}, {"nul", func(p map[string]any) { p["subnet_id"] = "x\x00y" }},
		{"long", func(p map[string]any) { p["id"] = strings.Repeat("a", 4097) }}, {"unknown", func(p map[string]any) { p["unknown"] = "x" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := pool()
			tc.change(p)
			if _, err := New().Validate("ipam_pools", row(t, p)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	raw := strings.Replace(string(row(t, pool())), `"pool_kind":"IPAM"`, `"pool_kind":"IPAM","pool_kind":"DHCP"`, 1)
	if _, err := New().Validate("ipam_pools", []byte(raw)); err == nil {
		t.Fatal("duplicate field")
	}
}
func TestPoolOrderingAndDirty(t *testing.T) {
	v := New()
	p := pool()
	p["id"] = "p2"
	if _, err := v.Validate("ipam_pools", row(t, p)); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Validate("ipam_pools", row(t, p)); err == nil {
		t.Fatal("duplicate")
	}
	if _, err := v.Validate("ipam_pools", row(t, p)); !errors.Is(err, ErrDirty) {
		t.Fatal(err)
	}
	v = New()
	if _, err := v.Validate("ipam_pools", row(t, p)); err != nil {
		t.Fatal(err)
	}
	p["id"] = "p1"
	if _, err := v.Validate("ipam_pools", row(t, p)); err == nil {
		t.Fatal("decreasing")
	}
}
func TestPoolAcrossChunks(t *testing.T) {
	p := pool()
	p["label"] = "历史"
	payload := []byte(fmt.Sprintf("{\"table\":\"ipam_pools\",\"row\":%s}\n", row(t, p)))
	split := bytes.Index(payload, []byte("历史")) + 1
	chunks := [][]byte{payload[:split], payload[split:]}
	m := snapshotverify.Manifest{Cluster: [16]byte{1}, Leader: [16]byte{2}, Snapshot: [16]byte{3}, Schema: 1, Anchor: snapshotverify.Position{Epoch: 1}, Encoding: snapshotverify.EncodingNDJSONV1, RetentionUntilMS: 200, TotalBytes: uint64(len(payload))}
	for i, c := range chunks {
		m.Chunks = append(m.Chunks, snapshotverify.ChunkSpec{Index: uint32(i), Size: uint64(len(c)), SHA256: sha256.Sum256(c)})
	}
	for _, n := range order {
		nrows := uint64(0)
		if n == "ipam_pools" {
			nrows = 1
		}
		m.Tables = append(m.Tables, snapshotverify.TableCount{Name: n, Rows: nrows})
	}
	trust := snapshotverify.Trust{Authenticated: true, Cluster: m.Cluster, Leader: m.Leader, Snapshot: m.Snapshot, Anchor: m.Anchor}
	var got []Row
	pipe, err := snapshotpipeline.New(m, trust, snapshotverify.Limits{Schema: 1, MaxBytes: 1 << 20, MaxChunks: 4, MaxRows: 10}, snapshotrows.Limits{LineBytes: 1 << 20, TotalBytes: 1 << 20, Rows: 10, Tables: 8}, New().Tables(func(r Row) error { got = append(got, r); return nil }), 100)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range chunks {
		if err = pipe.Feed(uint32(i), c, 101); err != nil {
			t.Fatal(err)
		}
	}
	result, err := pipe.Finish(102)
	if err != nil || len(got) != 1 || got[0].Table != "ipam_pools" || result.BusinessComplete {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestPoolNilPanicSubprocess(t *testing.T) {
	if os.Getenv("POOL_NIL_CHILD") == "1" {
		v := New()
		effects := 0
		callbacks := v.Tables(func(Row) error { effects++; panic(nil) })
		raw := row(t, pool())
		returned := false
		func() {
			defer func() {
				if recover() != nil {
					t.Error("nil panic changed")
				}
				if err := callbacks[3].ValidateRow(raw); !errors.Is(err, ErrDirty) {
					t.Errorf("not dirty: %v", err)
				}
			}()
			_ = callbacks[3].ValidateRow(raw)
			returned = true
		}()
		if returned || effects != 1 {
			t.Fatalf("returned=%v effects=%d", returned, effects)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPoolNilPanicSubprocess$")
	cmd.Env = append(os.Environ(), "GODEBUG=panicnil=1", "POOL_NIL_CHILD=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
}

func TestPoolSinkGoexitAndConcurrentRetry(t *testing.T) {
	v := New()
	effects := 0
	callbacks := v.Tables(func(Row) error { effects++; runtime.Goexit(); return nil })
	raw := row(t, pool())
	done := make(chan struct{})
	returned := false
	go func() { defer close(done); _ = callbacks[3].ValidateRow(raw); returned = true }()
	<-done
	if returned || effects != 1 {
		t.Fatal("Goexit did not exit")
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := callbacks[3].ValidateRow(raw); !errors.Is(err, ErrDirty) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if effects != 1 {
		t.Fatal("sink repeated")
	}
}
