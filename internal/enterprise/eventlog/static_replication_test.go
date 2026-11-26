package eventlog

import (
	"context"
	"errors"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/auditwire"
	"github.com/jincaiw/GoDDI/internal/enterprise/idempotency"
	"github.com/jincaiw/GoDDI/internal/enterprise/receiptwire"
)

func preparedStatic(t *testing.T, tx *WriteTx, s idempotency.Scope, r idempotency.Receipt, o StaticOccupancy) []Change {
	t.Helper()
	ctx := context.Background()
	lookupMiss(t, tx, s, o)
	if err := tx.InsertStatic(ctx, o); err != nil {
		t.Fatal(err)
	}
	if err := tx.InsertStaticAudit(ctx, staticAudit(s, o)); err != nil {
		t.Fatal(err)
	}
	if err := tx.InsertReceipt(ctx, s, staticDigest(t, o), r); err != nil {
		t.Fatal(err)
	}
	c, err := tx.StaticChanges()
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestStaticCandidateChangesDecodeAndReplay(t *testing.T) {
	db, l, s, r, o := staticSetup(t)
	ctx := context.Background()
	tx := fixtureWriteTx(t, db, l)
	changes := preparedStatic(t, tx, s, r, o)
	if len(changes) != 3 || changes[0].Kind != "ipam.occupancy.set" || changes[1].Kind != "ipam.occupancy.audit" || changes[2].Kind != "idempotency.receipt.set" {
		t.Fatalf("unexpected changes: %+v", changes)
	}
	a, err := auditwire.Decode(changes[1].Body)
	if err != nil {
		t.Fatal(err)
	}
	rv, err := receiptwire.Decode(changes[2].Body)
	if err != nil {
		t.Fatal(err)
	}
	if a.RequestID != changes[1].ObjectID || a.ResourceID != o.ID || a.Principal != s.Principal || rv.Scope != s || rv.RequestDigest != staticDigest(t, o) || changes[2].ObjectID != s.Key || string(rv.Receipt.Response.Body) != string(r.Response.Body) {
		t.Fatal("candidate bodies do not match transaction")
	}
	// Caller mutation after insertion cannot change stored snapshot or event.
	r.Response.Body[0] ^= 1
	if _, err = tx.Append(ctx, o.CreatedAtMS, changes...); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	assertStaticCounts(t, db, 1)
	tx = fixtureWriteTx(t, db, l)
	replay, found, err := tx.LookupReceipt(ctx, s, staticDigest(t, o))
	if err != nil || !found || string(replay.Response.Body) != string(rv.Receipt.Response.Body) {
		t.Fatalf("replay: %v %v", found, err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	assertStaticCounts(t, db, 1)
}

func TestStaticResponseAndPathMismatchRollBack(t *testing.T) {
	for _, kind := range []string{"status", "etag", "body", "path"} {
		t.Run(kind, func(t *testing.T) {
			db, l, s, r, o := staticSetup(t)
			ctx := context.Background()
			if kind == "path" {
				s.Path = "/api/other"
			}
			tx := fixtureWriteTx(t, db, l)
			lookupMiss(t, tx, s, o)
			if err := tx.InsertStatic(ctx, o); err != nil {
				if kind != "path" {
					t.Fatal(err)
				}
			} else {
				if kind == "status" {
					r.Response.Status = 200
				}
				if kind == "etag" {
					r.Response.ETag = `"v2"`
				}
				if kind == "body" {
					r.Response.Body = []byte("other")
				}
				if err := tx.InsertReceipt(ctx, s, staticDigest(t, o), r); err == nil {
					t.Fatal("mismatched response accepted")
				}
			}
			if err := tx.Commit(ctx); err == nil {
				t.Fatal("invalid transaction committed")
			}
			assertStaticCounts(t, db, 0)
		})
	}
}
func TestStaticChangeSetMustBeExact(t *testing.T) {
	for _, kind := range []string{"missing", "extra", "tamper", "reordered"} {
		t.Run(kind, func(t *testing.T) {
			db, l, s, r, o := staticSetup(t)
			ctx := context.Background()
			tx := fixtureWriteTx(t, db, l)
			changes := preparedStatic(t, tx, s, r, o)
			switch kind {
			case "missing":
				changes = changes[:2]
			case "extra":
				changes = append(changes, changes[0])
			case "tamper":
				changes[1].Body = []byte(`{}`)
			case "reordered":
				changes[0], changes[1] = changes[1], changes[0]
			}
			if _, err := tx.Append(ctx, o.CreatedAtMS, changes...); !errors.Is(err, ErrStaticOccupancy) {
				t.Fatalf("accepted wrong changes: %v", err)
			}
			if err := tx.Commit(ctx); err == nil {
				t.Fatal("dirty transaction committed")
			}
			assertStaticCounts(t, db, 0)
		})
	}
}
func TestStaticReceiptFailureRollsBackAll(t *testing.T) {
	db, l, s, r, o := staticSetup(t)
	ctx := context.Background()
	if _, err := db.Exec(`CREATE TRIGGER fail_static_receipt BEFORE INSERT ON idempotency_receipt BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	tx := fixtureWriteTx(t, db, l)
	lookupMiss(t, tx, s, o)
	if err := tx.InsertStatic(ctx, o); err != nil {
		t.Fatal(err)
	}
	if err := tx.InsertStaticAudit(ctx, staticAudit(s, o)); err != nil {
		t.Fatal(err)
	}
	if err := tx.InsertReceipt(ctx, s, staticDigest(t, o), r); err == nil {
		t.Fatal("receipt insert succeeded")
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("dirty transaction committed")
	}
	assertStaticCounts(t, db, 0)
}

func TestStaticAppendFailureRollsBackAll(t *testing.T) {
	db, l, s, r, o := staticSetup(t)
	ctx := context.Background()
	if _, err := db.Exec(`CREATE TRIGGER fail_static_event BEFORE INSERT ON replica_event BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	tx := fixtureWriteTx(t, db, l)
	changes := preparedStatic(t, tx, s, r, o)
	if _, err := tx.Append(ctx, o.CreatedAtMS, changes...); err == nil {
		t.Fatal("append succeeded")
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("dirty transaction committed")
	}
	assertStaticCounts(t, db, 0)
}
