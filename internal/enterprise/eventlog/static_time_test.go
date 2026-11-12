package eventlog

import (
	"context"
	"math"
	"testing"
)

func TestStaticReceiptTimeFailuresRollBack(t *testing.T) {
	for _, name := range []string{"short", "wrong-created", "overflow"} {
		t.Run(name, func(t *testing.T) {
			db, l, s, r, o := staticSetup(t)
			ctx := context.Background()
			if name == "short" {
				r.ExpiresAtMS--
			}
			if name == "wrong-created" {
				r.CreatedAtMS++
			}
			if name == "overflow" {
				o.CreatedAtMS = math.MaxInt64 - StaticReceiptMinRetentionMS + 1
				r.CreatedAtMS = o.CreatedAtMS
				r.ExpiresAtMS = math.MaxInt64
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
				t.Fatal("accepted invalid receipt time")
			}
			if err := tx.Commit(ctx); err == nil {
				t.Fatal("committed dirty transaction")
			}
			assertStaticCounts(t, db, 0)
		})
	}
}
func TestStaticEventTimeMismatchRollsBack(t *testing.T) {
	db, l, s, r, o := staticSetup(t)
	ctx := context.Background()
	tx := fixtureWriteTx(t, db, l)
	changes := preparedStatic(t, tx, s, r, o)
	if _, err := tx.Append(ctx, o.CreatedAtMS+1, changes...); err == nil {
		t.Fatal("accepted mismatched event time")
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("committed dirty transaction")
	}
	assertStaticCounts(t, db, 0)
}
func TestStaticSevenDayBoundaryAndOverflowBoundary(t *testing.T) {
	for _, created := range []int64{1, math.MaxInt64 - StaticReceiptMinRetentionMS} {
		t.Run(timeName(created), func(t *testing.T) {
			db, l, s, r, o := staticSetup(t)
			ctx := context.Background()
			o.CreatedAtMS = created
			r.CreatedAtMS = created
			r.ExpiresAtMS = created + StaticReceiptMinRetentionMS
			tx := fixtureWriteTx(t, db, l)
			changes := preparedStatic(t, tx, s, r, o)
			if _, err := tx.Append(ctx, created, changes...); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			assertStaticCounts(t, db, 1)
		})
	}
}
func timeName(v int64) string {
	if v == 1 {
		return "minimum"
	}
	return "max-int64-expiry"
}
