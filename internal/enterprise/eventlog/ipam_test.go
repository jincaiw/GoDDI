package eventlog

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jincaiw/GoDDI/internal/enterprise/idempotency"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamrequest"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamresponse"
)

func staticSetup(t *testing.T) (*sql.DB, *Log, idempotency.Scope, idempotency.Receipt, StaticOccupancy) {
	t.Helper()
	db, l := logFixture(t)
	raw, err := os.ReadFile("../ipamstore/migrations/001_ipam.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(raw)); err != nil {
		t.Fatal(err)
	}
	tenant := "11111111-1111-4111-8111-111111111111"
	space := "22222222-2222-4222-8222-222222222222"
	subnet := "33333333-3333-4333-8333-333333333333"
	id := "44444444-4444-4444-8444-444444444444"
	for _, q := range []struct {
		s string
		a []any
	}{
		{`INSERT INTO ipam_tenants(id,slug,display_name) VALUES (?,'t','tenant')`, []any{tenant}},
		{`INSERT INTO ipam_spaces(tenant_id,id,slug,display_name) VALUES (?,?,'s','space')`, []any{tenant, space}},
		{`INSERT INTO ipam_prefixes(tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES (?,?,?,4,24,x'000000000000000000000000C0000200',x'000000000000000000000000C00002FF','SUBNET','STANDARD')`, []any{tenant, space, subnet}},
		{`INSERT INTO replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES (?,1,0,?,1,1)`, []any{l.cluster[:], make([]byte, 32)}},
	} {
		if _, err = db.Exec(q.s, q.a...); err != nil {
			t.Fatal(err)
		}
	}
	tid := uuid.MustParse(tenant)
	scope := idempotency.Scope{Tenant: [16]byte(tid), Principal: "admin", Method: "POST", Path: "/api/v1/ddi/tenants/" + tenant + "/ipam/spaces/" + space + "/allocations", Key: "55555555-5555-4555-8555-555555555555"}
	response, err := ipamresponse.Static(id, subnet, "192.0.2.10", "asset:one")
	if err != nil {
		t.Fatal(err)
	}
	r := idempotency.Receipt{Response: response, CreatedAtMS: 1, ExpiresAtMS: 1 + StaticReceiptMinRetentionMS}
	o := StaticOccupancy{TenantID: tenant, SpaceID: space, SubnetID: subnet, ID: id, Family: 4, Address: [16]byte{12: 192, 13: 0, 14: 2, 15: 10}, OwnerRef: "asset:one", CreatedAtMS: 1}
	return db, l, scope, r, o
}
func staticDigest(t *testing.T, o StaticOccupancy) [32]byte {
	t.Helper()
	address, err := o.addressString()
	if err != nil {
		t.Fatal(err)
	}
	_, digest, err := ipamrequest.Normalize(ipamrequest.Request{TenantID: o.TenantID, SpaceID: o.SpaceID, SubnetID: o.SubnetID, Address: address, OwnerRef: o.OwnerRef})
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
func lookupMiss(t *testing.T, tx *WriteTx, s idempotency.Scope, o StaticOccupancy) {
	t.Helper()
	if _, found, err := tx.LookupReceipt(context.Background(), s, staticDigest(t, o)); err != nil || found {
		t.Fatalf("lookup: %v %v", found, err)
	}
}
func assertStaticCounts(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	for _, table := range []string{"ipam_occupancies", "replica_event", "idempotency_receipt", "static_occupancy_audit"} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil || n != want {
			t.Fatalf("%s: %d %v", table, n, err)
		}
	}
}
func staticAudit(s idempotency.Scope, o StaticOccupancy) StaticAudit {
	return StaticAudit{TenantID: o.TenantID, SpaceID: o.SpaceID, ResourceID: o.ID, Principal: s.Principal, Action: "ipam.occupancy.create", RequestID: "99999999-9999-4999-8999-999999999999", PriorRevision: 0, NewRevision: 1, Result: "SUCCESS", OccurredAtMS: o.CreatedAtMS}
}
func TestStaticInsertReceiptAndCorrespondingEvent(t *testing.T) {
	db, l, s, r, o := staticSetup(t)
	ctx := context.Background()
	tx := fixtureWriteTx(t, db, l)
	lookupMiss(t, tx, s, o)
	if err := tx.InsertStatic(ctx, o); err != nil {
		t.Fatal(err)
	}
	if err := tx.InsertReceipt(ctx, s, staticDigest(t, o), r); err != nil {
		t.Fatal(err)
	}
	if err := tx.InsertStaticAudit(ctx, staticAudit(s, o)); err != nil {
		t.Fatal(err)
	}
	changes, err := tx.StaticChanges()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Append(ctx, o.CreatedAtMS, changes...); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	assertStaticCounts(t, db, 1)
}
func TestStaticIgnoredInsertFailsAndRollsBack(t *testing.T) {
	db, l, scope, _, occupancy := staticSetup(t)
	if _, err := db.Exec(`CREATE TRIGGER suppress_static BEFORE INSERT ON ipam_occupancies BEGIN SELECT RAISE(IGNORE); END`); err != nil {
		t.Fatal(err)
	}
	tx := fixtureWriteTx(t, db, l)
	lookupMiss(t, tx, scope, occupancy)
	if err := tx.InsertStatic(context.Background(), occupancy); err == nil {
		t.Fatal("ignored insertion reported success")
	}
	if err := tx.Commit(context.Background()); err == nil {
		t.Fatal("transaction committed after ignored insertion")
	}
	assertStaticCounts(t, db, 0)
}

func TestStaticRejectsScopeDuplicateAndInvalidInputs(t *testing.T) {
	tests := map[string]func(*StaticOccupancy){
		"cross tenant":   func(o *StaticOccupancy) { o.TenantID = "66666666-6666-4666-8666-666666666666" },
		"cross space":    func(o *StaticOccupancy) { o.SpaceID = "66666666-6666-4666-8666-666666666666" },
		"outside subnet": func(o *StaticOccupancy) { o.Address[14] = 3 },
		"ipv4 padding":   func(o *StaticOccupancy) { o.Address[0] = 1 },
		"mapped ipv6":    func(o *StaticOccupancy) { o.Family = 6; o.Address[10] = 255; o.Address[11] = 255 },
		"negative time":  func(o *StaticOccupancy) { o.CreatedAtMS = -1 },
		"invalid uuid":   func(o *StaticOccupancy) { o.ID = "bad" },
		"empty owner":    func(o *StaticOccupancy) { o.OwnerRef = "" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			db, l, s, _, o := staticSetup(t)
			tx := fixtureWriteTx(t, db, l)
			lookupMiss(t, tx, s, o)
			mutate(&o)
			if err := tx.InsertStatic(context.Background(), o); err == nil {
				t.Fatal("invalid insert succeeded")
			}
			if err := tx.Commit(context.Background()); err == nil {
				t.Fatal("dirty commit succeeded")
			}
			assertStaticCounts(t, db, 0)
		})
	}
	t.Run("duplicate address", func(t *testing.T) {
		db, l, s, r, o := staticSetup(t)
		ctx := context.Background()
		tx := fixtureWriteTx(t, db, l)
		lookupMiss(t, tx, s, o)
		if err := tx.InsertStatic(ctx, o); err != nil {
			t.Fatal(err)
		}
		if err := tx.InsertReceipt(ctx, s, staticDigest(t, o), r); err != nil {
			t.Fatal(err)
		}
		if err := tx.InsertStaticAudit(ctx, staticAudit(s, o)); err != nil {
			t.Fatal(err)
		}
		changes, err := tx.StaticChanges()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Append(ctx, o.CreatedAtMS, changes...); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		s.Key = "77777777-7777-4777-8777-777777777777"
		o.ID = "88888888-8888-4888-8888-888888888888"
		tx = fixtureWriteTx(t, db, l)
		lookupMiss(t, tx, s, o)
		if err := tx.InsertStatic(ctx, o); err == nil {
			t.Fatal("duplicate address accepted")
		}
		if err := tx.Commit(ctx); err == nil {
			t.Fatal("dirty commit accepted")
		}
		assertStaticCounts(t, db, 1)
	})
}
func TestStaticDigestAndConditionalRequestAreBound(t *testing.T) {
	for _, kind := range []string{"owner", "address", "if-match"} {
		t.Run(kind, func(t *testing.T) {
			db, l, s, _, o := staticSetup(t)
			ctx := context.Background()
			tx := fixtureWriteTx(t, db, l)
			digest := staticDigest(t, o)
			if kind == "if-match" {
				address, _ := o.addressString()
				_, d, err := ipamrequest.Normalize(ipamrequest.Request{TenantID: o.TenantID, SpaceID: o.SpaceID, SubnetID: o.SubnetID, Address: address, OwnerRef: o.OwnerRef, IfMatch: []string{`"v1"`}})
				if err != nil {
					t.Fatal(err)
				}
				digest = d
			}
			if _, found, err := tx.LookupReceipt(ctx, s, digest); err != nil || found {
				t.Fatalf("lookup %v %v", found, err)
			}
			if kind == "owner" {
				o.OwnerRef = "asset:other"
			}
			if kind == "address" {
				o.Address[15]++
			}
			if err := tx.InsertStatic(ctx, o); err == nil {
				t.Fatal("mismatched request accepted")
			}
			if err := tx.Commit(ctx); err == nil {
				t.Fatal("dirty transaction committed")
			}
			assertStaticCounts(t, db, 0)
		})
	}
}

func TestStaticRequiresLookupAndMatchingEvent(t *testing.T) {
	t.Run("no lookup", func(t *testing.T) {
		db, l, _, _, o := staticSetup(t)
		tx := fixtureWriteTx(t, db, l)
		if err := tx.InsertStatic(context.Background(), o); !errors.Is(err, ErrStaticOccupancy) {
			t.Fatalf("%v", err)
		}
		if err := tx.Commit(context.Background()); err == nil {
			t.Fatal("commit succeeded")
		}
		assertStaticCounts(t, db, 0)
	})
	t.Run("wrong event", func(t *testing.T) {
		db, l, s, _, o := staticSetup(t)
		ctx := context.Background()
		tx := fixtureWriteTx(t, db, l)
		lookupMiss(t, tx, s, o)
		if err := tx.InsertStatic(ctx, o); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Append(ctx, 2, sampleChange()); err == nil {
			t.Fatal("wrong event accepted")
		}
		if err := tx.Commit(ctx); err == nil {
			t.Fatal("commit succeeded")
		}
		assertStaticCounts(t, db, 0)
	})
	t.Run("missing receipt", func(t *testing.T) {
		db, l, s, _, o := staticSetup(t)
		ctx := context.Background()
		tx := fixtureWriteTx(t, db, l)
		lookupMiss(t, tx, s, o)
		if err := tx.InsertStatic(ctx, o); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.StaticChanges(); err == nil {
			t.Fatal("missing receipt accepted")
		}
		if err := tx.Commit(ctx); err == nil {
			t.Fatal("commit without receipt succeeded")
		}
		assertStaticCounts(t, db, 0)
	})
	t.Run("wrong receipt", func(t *testing.T) {
		db, l, s, r, o := staticSetup(t)
		ctx := context.Background()
		tx := fixtureWriteTx(t, db, l)
		lookupMiss(t, tx, s, o)
		if err := tx.InsertStatic(ctx, o); err != nil {
			t.Fatal(err)
		}
		s.Principal = "other"
		if err := tx.InsertReceipt(ctx, s, [32]byte{1}, r); err == nil {
			t.Fatal("unbound receipt accepted")
		}
		if err := tx.Commit(ctx); err == nil {
			t.Fatal("commit succeeded")
		}
		assertStaticCounts(t, db, 0)
	})
	t.Run("two changes", func(t *testing.T) {
		db, l, s, _, o := staticSetup(t)
		ctx := context.Background()
		tx := fixtureWriteTx(t, db, l)
		lookupMiss(t, tx, s, o)
		if err := tx.InsertStatic(ctx, o); err != nil {
			t.Fatal(err)
		}
		if err := tx.InsertStatic(ctx, o); err == nil {
			t.Fatal("second insertion accepted")
		}
		if err := tx.Commit(ctx); err == nil {
			t.Fatal("commit succeeded")
		}
		assertStaticCounts(t, db, 0)
	})
}
