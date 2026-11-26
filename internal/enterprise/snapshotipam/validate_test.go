package snapshotipam

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotpipeline"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotrows"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotverify"
)

func b64(b ...byte) string { return base64.RawURLEncoding.EncodeToString(b) }
func ip4(a, b, c, d byte) string {
	x := make([]byte, 16)
	copy(x[12:], []byte{a, b, c, d})
	return b64(x...)
}
func row(t *testing.T, m map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func tenant(id string) map[string]any {
	return map[string]any{"id": id, "slug": "s", "display_name": "Tenant", "revision": "1"}
}
func space(tenant, id string) map[string]any {
	return map[string]any{"tenant_id": tenant, "id": id, "slug": "s", "display_name": "Space", "revision": "1"}
}
func prefix() map[string]any {
	return map[string]any{"tenant_id": "t1", "space_id": "s1", "id": "p1", "parent_id": nil, "family": "4", "prefix_len": "24", "first_ip": ip4(192, 0, 2, 0), "last_ip": ip4(192, 0, 2, 255), "kind": "SUBNET", "usage": "STANDARD", "site_ref": nil, "label": "", "revision": "1"}
}
func occupancy() map[string]any {
	return map[string]any{"tenant_id": "t1", "space_id": "s1", "id": "a1", "subnet_id": "p1", "family": "4", "address": ip4(192, 0, 2, 10), "kind": "STATIC", "owner_ref": "asset", "expires_at_ms": nil, "created_at_ms": "1", "revision": "1"}
}
func TestFourTablesAndIndependentPayload(t *testing.T) {
	v := New()
	for _, tc := range []struct {
		name string
		obj  map[string]any
	}{{"ipam_tenants", tenant("t1")}, {"ipam_spaces", space("t1", "s1")}, {"ipam_prefixes", prefix()}, {"ipam_occupancies", occupancy()}} {
		b := row(t, tc.obj)
		r, err := v.Validate(tc.name, b)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		before := r.Payload[0]
		b[0] ^= 1
		if r.Payload[0] != before {
			t.Fatal("payload aliases caller")
		}
	}
}
func TestPrimaryKeyStrictSQLiteBinaryOrder(t *testing.T) {
	v := New()
	for _, x := range [][2]string{{"a", "z"}, {"b", "a"}, {"b", "b"}} {
		if _, err := v.Validate("ipam_spaces", row(t, space(x[0], x[1]))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := v.Validate("ipam_spaces", row(t, space("b", "b"))); err == nil {
		t.Fatal("duplicate accepted")
	}
	if _, err := v.Validate("ipam_spaces", row(t, space("c", "a"))); !errors.Is(err, ErrDirty) {
		t.Fatalf("not dirty %v", err)
	}
	v = New()
	if _, err := v.Validate("ipam_tenants", row(t, tenant("z"))); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Validate("ipam_tenants", row(t, tenant("a"))); err == nil {
		t.Fatal("descending key accepted")
	}
}
func TestRejectInvalidColumnsAndEncodings(t *testing.T) {
	base := `{"id":"t1","slug":"s","display_name":"x","revision":"1"}`
	cases := []string{
		strings.Replace(base, `"revision":"1"`, `"revision":1`, 1),
		strings.Replace(base, `"revision":"1"`, `"revision":1.5`, 1),
		strings.Replace(base, `"revision":"1"`, `"revision":"01"`, 1),
		strings.Replace(base, `"revision":"1"`, `"revision":"9223372036854775808"`, 1),
		strings.Replace(base, `"revision":"1"`, `"revision":null`, 1),
		strings.Replace(base, `"id":"t1"`, `"id":"t1","id":"t2"`, 1),
		strings.Replace(base, `"id":"t1"`, `"id":"\ud800"`, 1),
		strings.Replace(base, `"id":"t1"`, `"id":"t1","unexpected":1`, 1),
		strings.Replace(base, `,"slug":"s"`, "", 1),
	}
	for i, input := range cases {
		if _, err := New().Validate("ipam_tenants", []byte(input)); err == nil {
			t.Errorf("case %d accepted %s", i, input)
		}
	}
	if _, err := New().Validate("ipam_pools", []byte(`{}`)); err == nil {
		t.Fatal("unsupported pool accepted")
	}
	for _, x := range []string{ip4(192, 0, 2, 10) + "=", "____", ""} {
		o := occupancy()
		o["address"] = x
		if _, err := New().Validate("ipam_occupancies", row(t, o)); err == nil {
			t.Fatalf("blob %q accepted", x)
		}
	}
}
func TestPrefixAndOccupancyConstraints(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"unaligned first": func(m map[string]any) { m["first_ip"] = ip4(192, 0, 2, 1) }, "wrong last": func(m map[string]any) { m["last_ip"] = ip4(192, 0, 2, 254) },
		"p2p usage": func(m map[string]any) { m["usage"] = "P2P" }, "container usage": func(m map[string]any) { m["kind"] = "CONTAINER" },
		"self parent": func(m map[string]any) { m["parent_id"] = "p1" }, "generated": func(m map[string]any) { m["binary_prefix_bits"] = "120" },
	} {
		t.Run(name, func(t *testing.T) {
			m := prefix()
			mutate(m)
			if _, err := New().Validate("ipam_prefixes", row(t, m)); err == nil {
				t.Fatal("invalid prefix accepted")
			}
		})
	}
	for name, mutate := range map[string]func(map[string]any){
		"hold without expiry": func(m map[string]any) { m["kind"] = "HOLD" }, "hold expired": func(m map[string]any) { m["kind"] = "HOLD"; m["expires_at_ms"] = "1" },
		"static expiry": func(m map[string]any) { m["expires_at_ms"] = "2" }, "bad kind": func(m map[string]any) { m["kind"] = "FREE" },
		"mapped v6": func(m map[string]any) {
			m["family"] = "6"
			m["address"] = b64(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 192, 0, 2, 10)
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := occupancy()
			mutate(m)
			if _, err := New().Validate("ipam_occupancies", row(t, m)); err == nil {
				t.Fatal("invalid occupancy accepted")
			}
		})
	}
	hold := occupancy()
	hold["kind"] = "HOLD"
	hold["expires_at_ms"] = "2"
	if _, err := New().Validate("ipam_occupancies", row(t, hold)); err != nil {
		t.Fatal(err)
	}
	p := prefix()
	p["prefix_len"] = "31"
	p["last_ip"] = ip4(192, 0, 2, 1)
	p["usage"] = "P2P"
	if _, err := New().Validate("ipam_prefixes", row(t, p)); err != nil {
		t.Fatal(err)
	}
}
func TestIPv6AndMaximumIntegerBoundaries(t *testing.T) {
	p := prefix()
	p["family"] = "6"
	p["prefix_len"] = "128"
	p["usage"] = "HOST"
	p["first_ip"] = b64(0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1)
	p["last_ip"] = p["first_ip"]
	p["revision"] = "9223372036854775807"
	if _, err := New().Validate("ipam_prefixes", row(t, p)); err != nil {
		t.Fatal(err)
	}
	p["last_ip"] = b64(0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2)
	if _, err := New().Validate("ipam_prefixes", row(t, p)); err == nil {
		t.Fatal("unaligned IPv6 host accepted")
	}
	o := occupancy()
	o["family"] = "6"
	o["address"] = b64(0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1)
	o["created_at_ms"] = "9223372036854775807"
	if _, err := New().Validate("ipam_occupancies", row(t, o)); err != nil {
		t.Fatal(err)
	}
}
func TestConcurrentDuplicateKeysFailClosed(t *testing.T) {
	v := New()
	raw := row(t, tenant("t1"))
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := v.Validate("ipam_tenants", raw); results <- err }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successes: %d", success)
	}
	if _, err := v.Validate("ipam_tenants", row(t, tenant("t2"))); !errors.Is(err, ErrDirty) {
		t.Fatalf("not dirty: %v", err)
	}
}

func TestSinkPanicPoisonsValidatorAndRepanics(t *testing.T) {
	v := New()
	effects := 0
	callbacks := v.Tables(func(Row) error { effects++; panic("sink panic") })
	func() {
		defer func() {
			if x := recover(); x != "sink panic" {
				t.Fatalf("panic lost: %v", x)
			}
		}()
		_ = callbacks[0].ValidateRow(row(t, tenant("t1")))
	}()
	if _, err := v.Validate("ipam_tenants", row(t, tenant("t2"))); !errors.Is(err, ErrDirty) {
		t.Fatalf("validate not dirty: %v", err)
	}
	if err := callbacks[1].ValidateRow(row(t, space("t1", "s1"))); !errors.Is(err, ErrDirty) {
		t.Fatalf("callback not dirty: %v", err)
	}
	if effects != 1 {
		t.Fatalf("repeated side effects: %d", effects)
	}
}
func TestConcurrentCallbacksSerializeValidationAndSink(t *testing.T) {
	v := New()
	entered := make(chan int, 2)
	release := make(chan struct{})
	var n int
	callbacks := v.Tables(func(Row) error {
		n++
		entered <- n
		if n == 1 {
			<-release
		}
		return nil
	})
	first := row(t, tenant("t1"))
	second := row(t, tenant("t2"))
	results := make(chan error, 2)
	go func() { results <- callbacks[0].ValidateRow(first) }()
	if n := <-entered; n != 1 {
		t.Fatal(n)
	}
	started := make(chan struct{})
	go func() { close(started); results <- callbacks[0].ValidateRow(second) }()
	<-started
	select {
	case <-entered:
		close(release)
		t.Fatal("second sink ran while first blocked")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if n := <-entered; n != 2 {
		t.Fatal(n)
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestPipelineEndToEnd(t *testing.T) {
	objs := []map[string]any{tenant("t1"), space("t1", "s1"), prefix(), occupancy()}
	names := []string{"ipam_tenants", "ipam_spaces", "ipam_prefixes", "ipam_occupancies"}
	var stream []byte
	for i, obj := range objs {
		stream = append(stream, []byte(fmt.Sprintf("{\"table\":%q,\"row\":%s}\n", names[i], row(t, obj)))...)
	}
	chunks := [][]byte{stream[:len(stream)/2], stream[len(stream)/2:]}
	m := snapshotverify.Manifest{Cluster: [16]byte{1}, Leader: [16]byte{2}, Snapshot: [16]byte{3}, Schema: 1, Anchor: snapshotverify.Position{Epoch: 1}, Encoding: snapshotverify.EncodingNDJSONV1, RetentionUntilMS: 200, TotalBytes: uint64(len(stream))}
	for i, c := range chunks {
		m.Chunks = append(m.Chunks, snapshotverify.ChunkSpec{Index: uint32(i), Size: uint64(len(c)), SHA256: sha256.Sum256(c)})
	}
	for _, name := range names {
		if name == "ipam_occupancies" {
			m.Tables = append(m.Tables, snapshotverify.TableCount{Name: "ipam_pools", Rows: 0})
		}
		m.Tables = append(m.Tables, snapshotverify.TableCount{Name: name, Rows: 1})
	}
	m.Tables = append(m.Tables, snapshotverify.TableCount{Name: "ipam_resource_ids", Rows: 0}, snapshotverify.TableCount{Name: "static_occupancy_audit", Rows: 0}, snapshotverify.TableCount{Name: "idempotency_receipt", Rows: 0})
	trust := snapshotverify.Trust{Authenticated: true, Cluster: m.Cluster, Leader: m.Leader, Snapshot: m.Snapshot, Anchor: m.Anchor}
	limits := snapshotverify.Limits{Schema: 1, MaxBytes: 1 << 20, MaxChunks: 4, MaxRows: 10}
	rows := snapshotrows.Limits{LineBytes: 1 << 20, TotalBytes: 1 << 20, Rows: 10, Tables: 8}
	var output []Row
	p, err := snapshotpipeline.New(m, trust, limits, rows, New().Tables(func(r Row) error { output = append(output, r); return nil }), 100)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range chunks {
		if err = p.Feed(uint32(i), c, 101); err != nil {
			t.Fatal(err)
		}
	}
	result, err := p.Finish(102)
	if err != nil {
		t.Fatal(err)
	}
	if len(output) != 4 || !result.Rows.AllRowCallbacksPassed || result.BusinessComplete {
		t.Fatalf("output=%d result=%+v", len(output), result)
	}
}
