package replicaapply

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jincaiw/GoDDI/internal/enterprise/auditwire"
	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/idempotency"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamevent"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamrequest"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamresponse"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamstore"
	"github.com/jincaiw/GoDDI/internal/enterprise/receiptwire"
	"github.com/jincaiw/GoDDI/internal/sqlitepolicy"
)

const tenant = "11111111-1111-4111-8111-111111111111"
const space = "22222222-2222-4222-8222-222222222222"
const oid = "33333333-3333-4333-8333-333333333333"
const subnet = "44444444-4444-4444-8444-444444444444"
const requestID = "55555555-5555-4555-8555-555555555555"
const keyID = "66666666-6666-4666-8666-666666666666"
const clusterID = "77777777-7777-4777-8777-777777777777"
const leaderID = "88888888-8888-4888-8888-888888888888"

func setup(t *testing.T) (*sql.DB, *testApplier, eventlog.Event, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "standby.db")
	db := sqlitepolicy.OpenWriter(path)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if err := ipamstore.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := eventlog.Init(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO ipam_tenants(id,slug,display_name) VALUES(?,'t','tenant')`, []any{tenant}},
		{`INSERT INTO ipam_spaces(tenant_id,id,slug,display_name) VALUES(?,?,'s','space')`, []any{tenant, space}},
		{`INSERT INTO ipam_prefixes(tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES(?,?,?,4,24,x'000000000000000000000000C0000200',x'000000000000000000000000C00002FF','SUBNET','STANDARD')`, []any{tenant, space, subnet}},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	cluster, _ := eventlog.ParseID(clusterID)
	leader, _ := eventlog.ParseID(leaderID)
	// A separately supplied, explicit test genesis; never inferred from incoming data.
	if _, err := db.Exec(`INSERT INTO replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,1,0,?,1,1)`, cluster[:], make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	return db, &testApplier{db: db, cluster: cluster, leader: leader}, eventFixture(t, cluster, leader), path
}
func eventFixture(t *testing.T, cluster, leader eventlog.ID) eventlog.Event {
	return eventFixtureValues(t, cluster, leader, oid, requestID, keyID, "192.0.2.10")
}
func eventFixtureValues(t *testing.T, cluster, leader eventlog.ID, objectID, reqID, receiptKey, address string) eventlog.Event {
	t.Helper()
	o := ipamevent.Occupancy{TenantID: tenant, SpaceID: space, ID: objectID, SubnetID: subnet, Family: 4, Address: address, Kind: "STATIC", OwnerRef: "host-1", CreatedAtMS: "100", Revision: "1"}
	oc, err := ipamevent.EncodeChange(ipamevent.Set, o)
	if err != nil {
		t.Fatal(err)
	}
	_, digest, err := ipamrequest.Normalize(ipamrequest.Request{TenantID: tenant, SpaceID: space, SubnetID: subnet, Address: o.Address, OwnerRef: o.OwnerRef})
	if err != nil {
		t.Fatal(err)
	}
	audit, err := auditwire.Encode(auditwire.Audit{ClusterID: cluster.String(), TenantID: tenant, SpaceID: space, ResourceID: objectID, Principal: "operator", RequestID: reqID, Epoch: 1, OccurredAtMS: 100, RequestDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	response, err := ipamresponse.Static(objectID, subnet, o.Address, o.OwnerRef)
	if err != nil {
		t.Fatal(err)
	}
	tid := uuid.MustParse(tenant)
	r, err := receiptwire.Encode(receiptwire.Value{Scope: idempotency.Scope{Tenant: [16]byte(tid), Principal: "operator", Method: "POST", Path: "/api/v1/ddi/tenants/" + tenant + "/ipam/spaces/" + space + "/allocations", Key: receiptKey}, RequestDigest: digest, Receipt: idempotency.Receipt{Response: response, CreatedAtMS: 100, ExpiresAtMS: 604800100}})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := eventlog.MarshalChanges(oc, eventlog.Change{Kind: "ipam.occupancy.audit", ObjectID: reqID, Revision: "1", Body: audit}, eventlog.Change{Kind: "idempotency.receipt.set", ObjectID: receiptKey, Revision: "1", Body: r})
	if err != nil {
		t.Fatal(err)
	}
	e := eventlog.Event{ClusterID: cluster, LeaderID: leader, Position: eventlog.Position{Epoch: 1, Seq: 1}, Schema: 1, CommitMs: 100, Payload: payload}
	e.Position.Hash, err = e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func counts(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	for _, table := range []string{"ipam_occupancies", "static_occupancy_audit", "idempotency_receipt", "replica_event"} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil || n != want {
			t.Fatalf("%s=%d err=%v want=%d", table, n, err, want)
		}
	}
	var seq int
	if err := db.QueryRow(`SELECT seq FROM replica_head`).Scan(&seq); err != nil || seq != want {
		t.Fatalf("seq=%d err=%v", seq, err)
	}
}
func TestPublicApplyRefuses(t *testing.T) {
	if err := Apply(context.Background(), eventlog.Event{}); !errors.Is(err, ErrNoAuthority) {
		t.Fatalf("public entry: %v", err)
	}
}
func TestFixtureApplyReplayAndRestart(t *testing.T) {
	db, a, e, path := setup(t)
	ctx := context.Background()
	if err := a.applyFixture(ctx, e); err != nil {
		t.Fatal(err)
	}
	counts(t, db, 1)
	if err := a.applyFixture(ctx, e); err != nil {
		t.Fatal(err)
	}
	counts(t, db, 1)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	fresh := sqlitepolicy.OpenWriter(path)
	defer fresh.Close()
	fresh.SetMaxOpenConns(1)
	b := &testApplier{db: fresh, cluster: a.cluster, leader: a.leader}
	if err := b.applyFixture(ctx, e); err != nil {
		t.Fatal(err)
	}
	counts(t, fresh, 1)
	// Existing receipt must be consumable by the real receipt reader after replay.
	tx, err := fresh.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	tid := uuid.MustParse(tenant)
	scope := idempotency.Scope{Tenant: [16]byte(tid), Principal: "operator", Method: "POST", Path: "/api/v1/ddi/tenants/" + tenant + "/ipam/spaces/" + space + "/allocations", Key: keyID}
	_, digest, err := ipamrequest.Normalize(ipamrequest.Request{TenantID: tenant, SpaceID: space, SubnetID: subnet, Address: "192.0.2.10", OwnerRef: "host-1"})
	if err != nil {
		t.Fatal(err)
	}
	if r, found, err := idempotency.Lookup(ctx, tx, scope, digest); err != nil || !found || r.Response.Status != 201 {
		t.Fatalf("receipt: %+v %v %v", r, found, err)
	}
}
func TestEveryWriteFailureRollsBack(t *testing.T) {
	for _, table := range []string{"ipam_occupancies", "static_occupancy_audit", "idempotency_receipt", "replica_event", "replica_head"} {
		t.Run(table, func(t *testing.T) {
			db, a, e, _ := setup(t)
			op := "INSERT"
			if table == "replica_head" {
				op = "UPDATE"
			}
			if _, err := db.Exec(`CREATE TRIGGER injected BEFORE ` + op + ` ON ` + table + ` BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
				t.Fatal(err)
			}
			if err := a.applyFixture(context.Background(), e); err == nil {
				t.Fatal("failure accepted")
			}
			counts(t, db, 0)
		})
	}
}
func TestDivergenceAndGapStopUntilRebuilt(t *testing.T) {
	for _, kind := range []string{"divergence", "gap"} {
		t.Run(kind, func(t *testing.T) {
			db, a, e, _ := setup(t)
			ctx := context.Background()
			if kind == "divergence" {
				if err := a.applyFixture(ctx, e); err != nil {
					t.Fatal(err)
				}
				e.CommitMs++
				var err error
				e.Position.Hash, err = e.Hash()
				if err != nil {
					t.Fatal(err)
				}
			} else {
				e.Position.Seq = 2
				var err error
				e.Position.Hash, err = e.Hash()
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := a.applyFixture(ctx, e); err == nil {
				t.Fatal("conflict accepted")
			}
			valid := eventFixture(t, a.cluster, a.leader)
			if err := a.applyFixture(ctx, valid); !errors.Is(err, ErrStopped) {
				t.Fatalf("not stopped: %v", err)
			}
			if kind == "divergence" {
				counts(t, db, 1)
			} else {
				counts(t, db, 0)
			}
		})
	}
}

func TestReplayRevalidatesStoredEventBody(t *testing.T) {
	db, a, e, _ := setup(t)
	ctx := context.Background()
	if err := a.applyFixture(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TRIGGER replica_event_no_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE replica_event SET payload=?`, []byte(`{"changes":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := a.applyFixture(ctx, e); err == nil {
		t.Fatal("corrupt stored event accepted")
	}
	if err := a.applyFixture(ctx, e); !errors.Is(err, ErrStopped) {
		t.Fatalf("not stopped: %v", err)
	}
}

func TestApplyRejectsConnectionWithoutForeignKeys(t *testing.T) {
	db, a, e, path := setup(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	unsafeDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer unsafeDB.Close()
	unsafeDB.SetMaxOpenConns(1)
	a.db = unsafeDB
	if err := a.applyFixture(context.Background(), e); err == nil {
		t.Fatal("foreign keys disabled accepted")
	}
	if err := a.applyFixture(context.Background(), e); !errors.Is(err, ErrStopped) {
		t.Fatalf("unsafe channel not stopped: %v", err)
	}
	counts(t, unsafeDB, 0)
}

func TestRejectWrongTrustOrChain(t *testing.T) {
	for _, kind := range []string{"cluster", "leader", "epoch", "gap", "previous", "digest", "schema", "missing-head", "wrong-head"} {
		t.Run(kind, func(t *testing.T) {
			db, a, e, _ := setup(t)
			switch kind {
			case "cluster":
				e.ClusterID[0] ^= 1
			case "leader":
				e.LeaderID[0] ^= 1
			case "epoch":
				e.Position.Epoch = 2
			case "gap":
				e.Position.Seq = 2
			case "previous":
				e.PrevHash[0] = 1
			case "digest":
				e.Position.Hash[0] ^= 1
			case "schema":
				e.Schema = 2
			case "missing-head":
				if _, err := db.Exec(`DELETE FROM replica_head`); err != nil {
					t.Fatal(err)
				}
			case "wrong-head":
				if _, err := db.Exec(`UPDATE replica_head SET hash=?`, append([]byte{1}, make([]byte, 31)...)); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "gap" || kind == "previous" {
				var err error
				e.Position.Hash, err = e.Hash()
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := a.applyFixture(context.Background(), e); err == nil {
				t.Fatal("invalid event accepted")
			}
			if kind != "missing-head" {
				counts(t, db, 0)
			}
		})
	}
}

func TestIgnoredInsertsRollbackAndStop(t *testing.T) {
	for _, table := range []string{"ipam_occupancies", "static_occupancy_audit", "idempotency_receipt", "replica_event"} {
		t.Run(table, func(t *testing.T) {
			db, a, e, _ := setup(t)
			if _, err := db.Exec(`CREATE TRIGGER ignored_insert BEFORE INSERT ON ` + table + ` BEGIN SELECT RAISE(IGNORE); END`); err != nil {
				t.Fatal(err)
			}
			if err := a.applyFixture(context.Background(), e); err == nil {
				t.Fatal("ignored insert reported success")
			}
			counts(t, db, 0)
			var hash []byte
			if err := db.QueryRow(`SELECT hash FROM replica_head`).Scan(&hash); err != nil || len(hash) != 32 || string(hash) != string(make([]byte, 32)) {
				t.Fatalf("head changed: %x %v", hash, err)
			}
			if err := a.applyFixture(context.Background(), e); !errors.Is(err, ErrStopped) {
				t.Fatalf("channel not stopped: %v", err)
			}
		})
	}
}

type failingResult struct{}

func (failingResult) LastInsertId() (int64, error) { return 0, nil }
func (failingResult) RowsAffected() (int64, error) { return 0, errors.New("unavailable") }
func TestRequireOneRejectsUnknownCount(t *testing.T) {
	if err := requireOne(failingResult{}); err == nil {
		t.Fatal("accepted unknown row count")
	}
	if err := requireOne(nil); err == nil {
		t.Fatal("accepted missing result")
	}
}
