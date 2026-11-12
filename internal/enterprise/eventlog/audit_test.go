package eventlog

import (
	"context"
	"errors"
	"testing"
)

type rowCountResult struct {
	count int64
	err   error
}

func (r rowCountResult) LastInsertId() (int64, error) { return 0, nil }
func (r rowCountResult) RowsAffected() (int64, error) { return r.count, r.err }

func TestRequireOneInserted(t *testing.T) {
	sentinel := errors.New("row count unavailable")
	for _, r := range []rowCountResult{{count: 0}, {count: 2}, {count: 1, err: sentinel}} {
		if err := requireOneInserted(r); err == nil {
			t.Fatalf("accepted %+v", r)
		} else if r.err != nil && !errors.Is(err, sentinel) {
			t.Fatalf("lost driver error: %v", err)
		}
	}
	if err := requireOneInserted(rowCountResult{count: 1}); err != nil {
		t.Fatal(err)
	}
	if err := requireOneInserted(nil); err == nil {
		t.Fatal("accepted nil result")
	}
}

// These tests bypass the deliberately closed Begin gate through a private
// fixture; they provide no proof of authorization or production HA readiness.
func TestStaticAuditRequiredAndAtomic(t *testing.T) {
	t.Run("missing audit", func(t *testing.T) {
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
		if _, err := tx.StaticChanges(); err == nil {
			t.Fatal("missing audit accepted")
		}
		if err := tx.Commit(ctx); err == nil {
			t.Fatal("committed without audit")
		}
		assertStaticCounts(t, db, 0)
	})
	for _, name := range []string{"principal", "resource", "tenant", "request id", "revision", "time", "action", "result"} {
		t.Run(name, func(t *testing.T) {
			db, l, s, _, o := staticSetup(t)
			ctx := context.Background()
			tx := fixtureWriteTx(t, db, l)
			lookupMiss(t, tx, s, o)
			if err := tx.InsertStatic(ctx, o); err != nil {
				t.Fatal(err)
			}
			a := staticAudit(s, o)
			switch name {
			case "principal":
				a.Principal = "other"
			case "resource":
				a.ResourceID = "77777777-7777-4777-8777-777777777777"
			case "tenant":
				a.TenantID = "77777777-7777-4777-8777-777777777777"
			case "request id":
				a.RequestID = "bad"
			case "revision":
				a.NewRevision = 2
			case "time":
				a.OccurredAtMS++
			case "action":
				a.Action = "delete"
			case "result":
				a.Result = "FAIL"
			}
			if err := tx.InsertStaticAudit(ctx, a); err == nil {
				t.Fatal("mismatched audit accepted")
			}
			if err := tx.Commit(ctx); err == nil {
				t.Fatal("dirty commit succeeded")
			}
			assertStaticCounts(t, db, 0)
		})
	}
	t.Run("audit failure after event and receipt", func(t *testing.T) {
		db, l, s, r, o := staticSetup(t)
		ctx := context.Background()
		if _, err := db.Exec(`CREATE TRIGGER audit_failure BEFORE INSERT ON static_occupancy_audit BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
			t.Fatal(err)
		}
		tx := fixtureWriteTx(t, db, l)
		lookupMiss(t, tx, s, o)
		if err := tx.InsertStatic(ctx, o); err != nil {
			t.Fatal(err)
		}
		if err := tx.InsertReceipt(ctx, s, staticDigest(t, o), r); err != nil {
			t.Fatal(err)
		}
		if err := tx.InsertStaticAudit(ctx, staticAudit(s, o)); err == nil {
			t.Fatal("failure not injected")
		}
		if err := tx.Commit(ctx); err == nil {
			t.Fatal("dirty commit succeeded")
		}
		assertStaticCounts(t, db, 0)
	})
	t.Run("duplicate audit", func(t *testing.T) {
		db, l, s, _, o := staticSetup(t)
		ctx := context.Background()
		tx := fixtureWriteTx(t, db, l)
		lookupMiss(t, tx, s, o)
		if err := tx.InsertStatic(ctx, o); err != nil {
			t.Fatal(err)
		}
		if err := tx.InsertStaticAudit(ctx, staticAudit(s, o)); err != nil {
			t.Fatal(err)
		}
		if err := tx.InsertStaticAudit(ctx, staticAudit(s, o)); err == nil {
			t.Fatal("duplicate accepted")
		}
		if err := tx.Commit(ctx); err == nil {
			t.Fatal("dirty commit succeeded")
		}
		assertStaticCounts(t, db, 0)
	})
}

func TestIgnoredInsertsAbortWholeStaticTransaction(t *testing.T) {
	for _, table := range []string{"static_occupancy_audit", "idempotency_receipt", "replica_event"} {
		t.Run(table, func(t *testing.T) {
			db, l, scope, receipt, occupancy := staticSetup(t)
			ctx := context.Background()
			if _, err := db.Exec(`CREATE TRIGGER suppress_insert BEFORE INSERT ON ` + table + ` BEGIN SELECT RAISE(IGNORE); END`); err != nil {
				t.Fatal(err)
			}
			tx := fixtureWriteTx(t, db, l)
			lookupMiss(t, tx, scope, occupancy)
			if err := tx.InsertStatic(ctx, occupancy); err != nil {
				t.Fatal(err)
			}
			err := tx.InsertStaticAudit(ctx, staticAudit(scope, occupancy))
			if table == "static_occupancy_audit" {
				if err == nil {
					t.Fatal("ignored audit was accepted")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				err = tx.InsertReceipt(ctx, scope, staticDigest(t, occupancy), receipt)
				if table == "idempotency_receipt" {
					if err == nil {
						t.Fatal("ignored receipt was accepted")
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					changes, e := tx.StaticChanges()
					if e != nil {
						t.Fatal(e)
					}
					if _, err = tx.Append(ctx, occupancy.CreatedAtMS, changes...); err == nil {
						t.Fatal("ignored replica event was accepted")
					}
				}
			}
			if err := tx.Commit(ctx); err == nil {
				t.Fatal("dirty transaction committed")
			}
			assertStaticCounts(t, db, 0)
			var seq int
			if err := db.QueryRow(`SELECT seq FROM replica_head`).Scan(&seq); err != nil || seq != 0 {
				t.Fatalf("head advanced: %d, %v", seq, err)
			}
		})
	}
}

func TestStaticAuditImmutableAndBound(t *testing.T) {
	db, l, s, r, o := staticSetup(t)
	ctx := context.Background()
	tx := fixtureWriteTx(t, db, l)
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
	var principal, tenant, space, resource, action, result, request string
	var before, after, at, epoch int64
	var digest []byte
	if err := db.QueryRow(`SELECT principal,tenant_id,space_id,resource_id,action,result,request_id,prior_revision,new_revision,occurred_at_ms,epoch,request_digest FROM static_occupancy_audit`).Scan(&principal, &tenant, &space, &resource, &action, &result, &request, &before, &after, &at, &epoch, &digest); err != nil {
		t.Fatal(err)
	}
	expected := staticDigest(t, o)
	if principal != s.Principal || tenant != o.TenantID || space != o.SpaceID || resource != o.ID || action != "ipam.occupancy.create" || result != "SUCCESS" || request != staticAudit(s, o).RequestID || before != 0 || after != 1 || at != o.CreatedAtMS || epoch != 1 || string(digest) != string(expected[:]) {
		t.Fatal("audit row differs from bound transaction")
	}
	for _, q := range []string{`UPDATE static_occupancy_audit SET principal='evil'`, `DELETE FROM static_occupancy_audit`, `INSERT OR REPLACE INTO static_occupancy_audit SELECT * FROM static_occupancy_audit`} {
		if _, err := db.Exec(q); err == nil {
			t.Fatalf("audit modification succeeded: %s", q)
		}
	}
}
