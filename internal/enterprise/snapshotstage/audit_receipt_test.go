package snapshotstage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/idempotency"
)

func b64raw(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
func auditRow(cluster [16]byte) line {
	return line{"static_occupancy_audit", map[string]any{"cluster_id": b64raw(cluster[:]), "epoch": "1", "tenant_id": "11111111-1111-4111-8111-111111111111", "space_id": "22222222-2222-4222-8222-222222222222", "resource_id": "33333333-3333-4333-8333-333333333333", "principal": "operator", "action": "ipam.occupancy.create", "request_id": "44444444-4444-4444-8444-444444444444", "prior_revision": "0", "new_revision": "1", "result": "SUCCESS", "occurred_at_ms": "123", "request_digest": b64raw(bytes.Repeat([]byte{9}, 32))}}
}
func receiptRow(tenant [16]byte, body, etag []byte) line {
	blob := make([]byte, 8+len(etag)+len(body))
	copy(blob, "GIR1")
	binary.BigEndian.PutUint16(blob[4:6], 201)
	binary.BigEndian.PutUint16(blob[6:8], uint16(len(etag)))
	copy(blob[8:], etag)
	copy(blob[8+len(etag):], body)
	return line{"idempotency_receipt", map[string]any{"tenant_id": b64raw(tenant[:]), "principal": "operator", "method": "POST", "path": "/api/v1/a", "idempotency_key": "55555555-5555-4555-8555-555555555555", "request_digest": b64raw(bytes.Repeat([]byte{9}, 32)), "response": b64raw(blob), "created_at_unix_ms": "1", "expires_at_unix_ms": "2"}}
}
func TestAuditAndExpiredReceiptExactBytes(t *testing.T) {
	lines := sampleLines()
	cluster := [16]byte{1}
	tenant := [16]byte{7}
	r := receiptRow(tenant, []byte{0, 1, 0xff, '\n', 'x'}, []byte("tag"))
	lines = append(lines, auditRow(cluster), r)
	m, tr, chunks := snapshot(t, lines)
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{})
	if err != nil {
		t.Fatal(err)
	}
	var c, d []byte
	var epoch, prior, rev, at int64
	var tenantText, space, resource, principal, action, request, result string
	err = s.conn.QueryRowContext(context.Background(), `SELECT cluster_id,epoch,tenant_id,space_id,resource_id,principal,action,request_id,prior_revision,new_revision,result,occurred_at_ms,request_digest FROM main.static_occupancy_audit`).Scan(&c, &epoch, &tenantText, &space, &resource, &principal, &action, &request, &prior, &rev, &result, &at, &d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(c, cluster[:]) || epoch != 1 || tenantText != "11111111-1111-4111-8111-111111111111" || space != "22222222-2222-4222-8222-222222222222" || resource != "33333333-3333-4333-8333-333333333333" || principal != "operator" || action != "ipam.occupancy.create" || request != "44444444-4444-4444-8444-444444444444" || prior != 0 || rev != 1 || result != "SUCCESS" || at != 123 || !bytes.Equal(d, bytes.Repeat([]byte{9}, 32)) {
		t.Fatal("audit changed")
	}
	var tb, digest, blob []byte
	var method, path, key string
	var created, expires int64
	err = s.conn.QueryRowContext(context.Background(), `SELECT tenant_id,principal,method,path,idempotency_key,request_digest,response,created_at_unix_ms,expires_at_unix_ms FROM main.idempotency_receipt`).Scan(&tb, &principal, &method, &path, &key, &digest, &blob, &created, &expires)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := base64.RawURLEncoding.DecodeString(r.Row["response"].(string))
	if !bytes.Equal(tb, tenant[:]) || principal != "operator" || method != "POST" || path != "/api/v1/a" || key != "55555555-5555-4555-8555-555555555555" || !bytes.Equal(digest, bytes.Repeat([]byte{9}, 32)) || !bytes.Equal(blob, original) || created != 1 || expires != 2 {
		t.Fatal("receipt changed")
	}
}
func TestMaxReceiptSpansChunksUnchanged(t *testing.T) {
	tenant := [16]byte{7}
	body := bytes.Repeat([]byte{0x92}, idempotency.MaxBodyBytes)
	etag := bytes.Repeat([]byte{'e'}, idempotency.MaxETagBytes)
	r := receiptRow(tenant, body, etag)
	lines := append(sampleLines(), r)
	m, tr, chunks := snapshot(t, lines)
	if len(chunks) != 2 || len(chunks[0].data) > 1<<20 || len(chunks[1].data) > 1<<20 {
		t.Fatal("invalid chunk sizes")
	}
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{})
	if err != nil {
		t.Fatal(err)
	}
	var blob []byte
	if err = s.conn.QueryRowContext(context.Background(), `SELECT response FROM main.idempotency_receipt`).Scan(&blob); err != nil {
		t.Fatal(err)
	}
	want, _ := base64.RawURLEncoding.DecodeString(r.Row["response"].(string))
	if !bytes.Equal(blob, want) {
		t.Fatal("GIR1 bytes changed")
	}
}
func TestReceiptTenantKeyOrderUsesBlobBytes(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordered", true: "reverse"}[reverse], func(t *testing.T) {
			a := [16]byte{1}
			b := [16]byte{0xfb}
			first, second := receiptRow(a, []byte("a"), nil), receiptRow(b, []byte("b"), nil)
			if reverse {
				first, second = second, first
			}
			lines := append(sampleLines(), first, second)
			m, tr, chunks := snapshot(t, lines)
			s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{})
			if reverse {
				if err == nil || s != nil {
					t.Fatal("accepted reverse binary order")
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestRejectAuditReceiptTampering(t *testing.T) {
	for _, name := range []string{"audit wrong cluster", "duplicate audit", "duplicate receipt", "bad response", "audit ignore", "receipt ignore", "authority injection", "cancel after commit"} {
		t.Run(name, func(t *testing.T) {
			cluster := [16]byte{1}
			tenant := [16]byte{7}
			a, r := auditRow(cluster), receiptRow(tenant, []byte("ok"), []byte("tag"))
			lines := append(sampleLines(), a, r)
			var path string
			var h fixtureHooks
			h.seePath = func(p string) { path = p }
			ctx := context.Background()
			fired := false
			var c *sql.Conn
			switch name {
			case "audit wrong cluster":
				wrong := [16]byte{99}
				lines[len(lines)-2] = auditRow(wrong)
			case "duplicate audit":
				lines = append(lines[:len(lines)-1], a, r)
			case "duplicate receipt":
				lines = append(lines, r)
			case "bad response":
				r.Row["response"] = b64raw([]byte("not GIR1"))
			case "audit ignore", "receipt ignore":
				table := "static_occupancy_audit"
				if name == "receipt ignore" {
					table = "idempotency_receipt"
				}
				h.beforeInsert = func(c *sql.Conn) error {
					_, e := c.ExecContext(ctx, `CREATE TEMP TRIGGER skip_target BEFORE INSERT ON main.`+table+` BEGIN SELECT RAISE(IGNORE); END`)
					fired = e == nil
					return e
				}
				h.beforeCommit = func() error { return errors.New("unexpected commit") }
			case "authority injection":
				h.beforeInsert = func(x *sql.Conn) error { c = x; return nil }
				h.afterCommit = func() error {
					_, e := c.ExecContext(ctx, `INSERT INTO main.writer_authority VALUES(zeroblob(16),x'01000000000000000000000000000000',1,'x',zeroblob(32),1)`)
					fired = e == nil
					return e
				}
			case "cancel after commit":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
				h.afterCommit = func() error { fired = true; cancel(); return nil }
			}
			m, tr, chunks := snapshot(t, lines)
			s, err := importFixture(t, ctx, m, tr, node(), chunks, h)
			if err == nil || s != nil {
				t.Fatal("invalid accepted")
			}
			if strings.HasSuffix(name, "ignore") && (!fired || !errors.Is(err, errFixture)) {
				t.Fatalf("ignore not reached: %v", err)
			}
			if name == "authority injection" && (!fired || !errors.Is(err, errFixture)) {
				t.Fatalf("authority phase: %v", err)
			}
			if name == "cancel after commit" && (!fired || !errors.Is(err, context.Canceled)) {
				t.Fatalf("cancel phase: %v", err)
			}
			assertRemoved(t, path)
		})
	}
}
