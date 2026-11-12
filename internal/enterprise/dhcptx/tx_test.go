package dhcptx

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/sqlitepolicy"
)

const tenant = "11111111-1111-4111-8111-111111111111"
const space = "22222222-2222-4222-8222-222222222222"
const leaseID = "33333333-3333-4333-8333-333333333333"
const leaseID2 = "44444444-4444-4444-8444-444444444444"
const cluster = "77777777-7777-4777-8777-777777777777"
const node = "88888888-8888-4888-8888-888888888888"
const testSchema = `
CREATE TABLE dhcp_lease(tenant TEXT NOT NULL,space TEXT NOT NULL,lease_id TEXT NOT NULL,client_key BLOB NOT NULL,address TEXT NOT NULL,kind TEXT NOT NULL,offer_xid INTEGER NOT NULL,issued_ms INTEGER NOT NULL,expires_ms INTEGER NOT NULL,revision INTEGER NOT NULL,PRIMARY KEY(tenant,space,address),UNIQUE(tenant,space,client_key),UNIQUE(tenant,space,lease_id));
CREATE TABLE dhcp_outbox(cluster_id BLOB NOT NULL,epoch INTEGER NOT NULL,seq INTEGER NOT NULL,hash BLOB NOT NULL,payload BLOB NOT NULL,PRIMARY KEY(cluster_id,epoch,seq));
CREATE TABLE dhcp_receipt(tenant TEXT NOT NULL,space TEXT NOT NULL,client_key BLOB NOT NULL,xid INTEGER NOT NULL,kind TEXT NOT NULL,digest BLOB NOT NULL,reply_kind TEXT NOT NULL,address TEXT NOT NULL,expires_ms INTEGER NOT NULL,revision INTEGER NOT NULL,seq INTEGER NOT NULL,PRIMARY KEY(tenant,space,client_key,xid,kind));`

func setup(t *testing.T) (*sql.DB, *fixture, command, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lease.db")
	db := sqlitepolicy.OpenWriter(path)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if err := eventlog.Init(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(testSchema); err != nil {
		t.Fatal(err)
	}
	cl, _ := eventlog.ParseID(cluster)
	n, _ := eventlog.ParseID(node)
	anchor, _ := eventlog.NewAnchor(1)
	if _, err := db.Exec(`INSERT INTO replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,1,0,?,1,1)`, cl[:], anchor.Hash[:]); err != nil {
		t.Fatal(err)
	}
	c := command{Tenant: tenant, Space: space, LeaseID: leaseID, ClientKey: []byte{1, 2, 3}, XID: 99, Kind: "OFFER", Address: "192.0.2.10", IssuedMS: 100, ExpiresMS: 200}
	return db, &fixture{db: db, cluster: cl, node: n, tenant: tenant, space: space, serverID: "192.0.2.1", head: anchor}, c, path
}
func counts(t *testing.T, db *sql.DB, leases, events int) {
	t.Helper()
	for table, want := range map[string]int{"dhcp_lease": leases, "replica_event": events, "dhcp_outbox": events, "dhcp_receipt": events} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil || n != want {
			t.Fatalf("%s=%d want=%d error=%v", table, n, want, err)
		}
	}
	var seq int
	if err := db.QueryRow(`SELECT seq FROM replica_head`).Scan(&seq); err != nil || seq != events {
		t.Fatalf("seq=%d err=%v", seq, err)
	}
}
func TestPublicClosed(t *testing.T) {
	if !errors.Is(Begin(context.Background()), ErrNoAuthority) || !errors.Is(Commit(context.Background()), ErrNoAuthority) {
		t.Fatal("public write enabled")
	}
}
func TestOfferAckReplayAndRestart(t *testing.T) {
	db, f, c, path := setup(t)
	ctx := context.Background()
	a, err := f.apply(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	counts(t, db, 1, 1)
	later := c
	later.IssuedMS = 105
	later.ExpiresMS = 500
	b, err := f.apply(ctx, later)
	if err != nil || a != b {
		t.Fatalf("replay %+v %+v %v", a, b, err)
	}
	counts(t, db, 1, 1)
	ack := c
	ack.Kind = "ACK"
	ack.Stage, ack.ServerID, ack.RequestedIP = "SELECTING", "192.0.2.1", ack.Address
	ack.IssuedMS = 110
	ack.ExpiresMS = 300
	result, err := f.apply(ctx, ack)
	if err != nil || result.Revision != 2 || result.ExpiresMS != 300 {
		t.Fatalf("ack %+v %v", result, err)
	}
	counts(t, db, 1, 2)
	head := f.head
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := sqlitepolicy.OpenWriter(path)
	defer reopened.Close()
	reopened.SetMaxOpenConns(1)
	fresh := &fixture{db: reopened, cluster: f.cluster, node: f.node, tenant: f.tenant, space: f.space, serverID: f.serverID, head: head}
	repeat, err := fresh.apply(ctx, ack)
	if err != nil || repeat != result {
		t.Fatalf("restart replay %+v %v", repeat, err)
	}
	counts(t, reopened, 1, 2)
	var payload []byte
	if err = reopened.QueryRow(`SELECT payload FROM replica_event WHERE seq=2`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	changes, err := eventlog.DecodePayload(payload)
	if err != nil || len(changes) != 1 || changes[0].Kind != "dhcp.lease.committed" {
		t.Fatalf("candidate event %v %+v", err, changes)
	}
}
func TestReplayMismatchStops(t *testing.T) {
	for _, alter := range []func(*command){func(c *command) { c.Address = "192.0.2.11" }, func(c *command) { c.LeaseID = leaseID2 }, func(c *command) { c.Tenant = leaseID2 }, func(c *command) { c.Space = leaseID2 }} {
		db, f, c, _ := setup(t)
		if _, err := f.apply(context.Background(), c); err != nil {
			t.Fatal(err)
		}
		bad := c
		alter(&bad)
		if r, err := f.apply(context.Background(), bad); err == nil || r != (reply{}) {
			t.Fatalf("mismatch %+v %v", r, err)
		}
		counts(t, db, 1, 1)
		if _, err := f.apply(context.Background(), c); !errors.Is(err, ErrStopped) {
			t.Fatalf("not stopped %v", err)
		}
	}
}
func TestClientKeyBoundariesAndXID(t *testing.T) {
	for _, tc := range []struct {
		n   int
		xid uint32
		ok  bool
	}{{256, 1, true}, {257, 1, false}, {1, 0, false}} {
		db, f, c, _ := setup(t)
		c.ClientKey = make([]byte, tc.n)
		c.XID = tc.xid
		_, err := f.apply(context.Background(), c)
		if (err == nil) != tc.ok {
			t.Fatalf("len=%d xid=%d err=%v", tc.n, tc.xid, err)
		}
		if tc.ok {
			counts(t, db, 1, 1)
		} else {
			counts(t, db, 0, 0)
		}
	}
}
func TestOfferXIDBindingAndRenewal(t *testing.T) {
	ctx := context.Background()
	t.Run("mismatched selecting ack rolls back", func(t *testing.T) {
		db, f, c, _ := setup(t)
		if _, err := f.apply(ctx, c); err != nil {
			t.Fatal(err)
		}
		ack := c
		ack.Kind = "ACK"
		ack.Stage, ack.ServerID, ack.RequestedIP = "SELECTING", "192.0.2.1", ack.Address
		ack.XID++
		ack.IssuedMS = 110
		ack.ExpiresMS = 300
		if got, err := f.apply(ctx, ack); err == nil || got != (reply{}) {
			t.Fatalf("mismatched ACK: %+v %v", got, err)
		}
		counts(t, db, 1, 1)
	})
	t.Run("matching selecting and different renewal", func(t *testing.T) {
		db, f, c, _ := setup(t)
		if _, err := f.apply(ctx, c); err != nil {
			t.Fatal(err)
		}
		ack := c
		ack.Kind = "ACK"
		ack.Stage, ack.ServerID, ack.RequestedIP = "SELECTING", "192.0.2.1", ack.Address
		ack.IssuedMS = 110
		ack.ExpiresMS = 300
		if _, err := f.apply(ctx, ack); err != nil {
			t.Fatal(err)
		}
		renew := ack
		renew.XID++
		renew.Stage, renew.ServerID, renew.RequestedIP, renew.CIAddr = "RENEW_REBIND", "", "", renew.Address
		renew.IssuedMS = 120
		renew.ExpiresMS = 400
		result, err := f.apply(ctx, renew)
		if err != nil || result.Revision != 3 {
			t.Fatalf("renew %+v %v", result, err)
		}
		if replay, err := f.apply(ctx, renew); err != nil || replay != result {
			t.Fatalf("replay %+v %v", replay, err)
		}
		counts(t, db, 1, 3)
	})
	t.Run("live offer cannot be replaced", func(t *testing.T) {
		db, f, c, _ := setup(t)
		if _, err := f.apply(ctx, c); err != nil {
			t.Fatal(err)
		}
		next := c
		next.XID++
		next.IssuedMS = 110
		next.ExpiresMS = 300
		if _, err := f.apply(ctx, next); err == nil {
			t.Fatal("replaced live offer")
		}
		counts(t, db, 1, 1)
	})
	t.Run("expired offer can be replaced", func(t *testing.T) {
		db, f, c, _ := setup(t)
		if _, err := f.apply(ctx, c); err != nil {
			t.Fatal(err)
		}
		next := c
		next.XID++
		next.IssuedMS = 200
		next.ExpiresMS = 300
		if _, err := f.apply(ctx, next); err != nil {
			t.Fatal(err)
		}
		ack := next
		ack.Kind = "ACK"
		ack.Stage, ack.ServerID, ack.RequestedIP = "SELECTING", "192.0.2.1", ack.Address
		ack.IssuedMS = 210
		ack.ExpiresMS = 400
		if _, err := f.apply(ctx, ack); err != nil {
			t.Fatal(err)
		}
		counts(t, db, 1, 3)
	})
	t.Run("tampered offer xid cannot select or replay", func(t *testing.T) {
		for _, selecting := range []bool{false, true} {
			db, f, c, _ := setup(t)
			if _, err := f.apply(ctx, c); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE dhcp_lease SET offer_xid=100`); err != nil {
				t.Fatal(err)
			}
			if selecting {
				c.Kind = "ACK"
				c.Stage, c.ServerID, c.RequestedIP = "SELECTING", "192.0.2.1", c.Address
				c.XID = 100
				c.IssuedMS = 110
				c.ExpiresMS = 300
			}
			if _, err := f.apply(ctx, c); err == nil {
				t.Fatal("accepted modified offer xid")
			}
			counts(t, db, 1, 1)
		}
	})
}

func TestACKStagesAndPriorStateIntegrity(t *testing.T) {
	t.Run("offer cannot accept renew", func(t *testing.T) {
		db, f, c, _ := setup(t)
		if _, err := f.apply(context.Background(), c); err != nil {
			t.Fatal(err)
		}
		c.Kind = "ACK"
		c.Stage = "RENEW_REBIND"
		c.CIAddr = c.Address
		c.IssuedMS = 110
		c.ExpiresMS = 300
		if _, err := f.apply(context.Background(), c); err == nil {
			t.Fatal("renew accepted against offer")
		}
		counts(t, db, 1, 1)
	})
	for _, tc := range []struct {
		name, stage, server, requested, ci string
		valid                              bool
	}{
		{"renew", "RENEW_REBIND", "", "", "192.0.2.10", true},
		{"init reboot", "INIT_REBOOT", "", "192.0.2.10", "", true},
		{"new selecting after ACK", "SELECTING", "192.0.2.1", "192.0.2.10", "", false},
		{"wrong requested address", "INIT_REBOOT", "", "192.0.2.11", "", false},
		{"wrong ciaddr", "RENEW_REBIND", "", "", "192.0.2.11", false},
		{"renew with server", "RENEW_REBIND", "192.0.2.1", "", "192.0.2.10", false},
		{"missing stage", "", "", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, f, c, _ := setup(t)
			if _, err := f.apply(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			ack := c
			ack.Kind = "ACK"
			ack.Stage = "SELECTING"
			ack.ServerID = "192.0.2.1"
			ack.RequestedIP = ack.Address
			ack.IssuedMS = 110
			ack.ExpiresMS = 300
			if _, err := f.apply(context.Background(), ack); err != nil {
				t.Fatal(err)
			}
			ack.XID++
			ack.IssuedMS = 120
			ack.ExpiresMS = 400
			ack.Stage, ack.ServerID, ack.RequestedIP, ack.CIAddr = tc.stage, tc.server, tc.requested, tc.ci
			_, err := f.apply(context.Background(), ack)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if tc.valid {
				counts(t, db, 1, 3)
			} else {
				counts(t, db, 1, 2)
			}
		})
	}
	for _, column := range []string{"offer_xid=100", "issued_ms=109", "expires_ms=301", "revision=3"} {
		t.Run("tamper "+column, func(t *testing.T) {
			db, f, c, _ := setup(t)
			if _, err := f.apply(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			c.Kind = "ACK"
			c.Stage = "SELECTING"
			c.ServerID = "192.0.2.1"
			c.RequestedIP = c.Address
			c.IssuedMS = 110
			c.ExpiresMS = 300
			if _, err := f.apply(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE dhcp_lease SET ` + column); err != nil {
				t.Fatal(err)
			}
			c.XID++
			c.Stage = "RENEW_REBIND"
			c.ServerID = ""
			c.RequestedIP = ""
			c.CIAddr = c.Address
			c.IssuedMS = 120
			c.ExpiresMS = 400
			if _, err := f.apply(context.Background(), c); err == nil {
				t.Fatal("accepted changed prior lease")
			}
			counts(t, db, 1, 2)
		})
	}
}

func TestNewOfferVerifiesEveryPriorLease(t *testing.T) {
	for _, oldKind := range []string{"OFFER", "ACK"} {
		for _, mutation := range []string{"expires_ms=120", "issued_ms=99", "offer_xid=100", "revision=9", ""} {
			t.Run(oldKind+"/"+mutation, func(t *testing.T) {
				db, f, c, _ := setup(t)
				if _, err := f.apply(context.Background(), c); err != nil {
					t.Fatal(err)
				}
				events := 1
				if oldKind == "ACK" {
					ack := c
					ack.Kind = "ACK"
					ack.Stage = "SELECTING"
					ack.ServerID = "192.0.2.1"
					ack.RequestedIP = ack.Address
					ack.IssuedMS = 110
					ack.ExpiresMS = 300
					if _, err := f.apply(context.Background(), ack); err != nil {
						t.Fatal(err)
					}
					events++
				}
				original := f.head
				if mutation != "" {
					if _, err := db.Exec(`UPDATE dhcp_lease SET ` + mutation); err != nil {
						t.Fatal(err)
					}
				}
				next := c
				next.XID++
				next.IssuedMS = 150
				if mutation == "" {
					if oldKind == "ACK" {
						next.IssuedMS = 300
					} else {
						next.IssuedMS = 200
					}
				}
				next.ExpiresMS = 400
				_, err := f.apply(context.Background(), next)
				if mutation == "" {
					if err != nil {
						t.Fatal(err)
					}
					counts(t, db, 1, events+1)
					return
				}
				if err == nil {
					t.Fatal("accepted modified prior lease")
				}
				counts(t, db, 1, events)
				var seq int64
				var hash []byte
				if err := db.QueryRow(`SELECT seq,hash FROM replica_head`).Scan(&seq, &hash); err != nil || uint64(seq) != original.Seq || !bytes.Equal(hash, original.Hash[:]) || f.head != original {
					t.Fatalf("head changed: %d %x %v", seq, hash, err)
				}
			})
		}
	}
}

func TestNewOfferRejectsPriorLeaseOutsideCurrentHead(t *testing.T) {
	db, f, c, _ := setup(t)
	if _, err := f.apply(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	other := c
	other.ClientKey = []byte{8}
	other.LeaseID = leaseID2
	other.Address = "192.0.2.11"
	other.XID++
	other.IssuedMS = 110
	if _, err := f.apply(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	original := f.head
	c.XID += 2
	c.IssuedMS = 200
	c.ExpiresMS = 300
	if _, err := f.apply(context.Background(), c); err == nil {
		t.Fatal("accepted lease outside current head")
	}
	counts(t, db, 2, 2)
	if f.head != original {
		t.Fatal("head changed")
	}
}

func TestConcurrentMatchingSelectingACK(t *testing.T) {
	db, f, c, _ := setup(t)
	if _, err := f.apply(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	c.Kind = "ACK"
	c.Stage, c.ServerID, c.RequestedIP = "SELECTING", "192.0.2.1", c.Address
	c.IssuedMS = 110
	c.ExpiresMS = 300
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.apply(context.Background(), c); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	counts(t, db, 1, 2)
}

func TestAckExpiryAndNoActiveDowngrade(t *testing.T) {
	for _, when := range []int64{199, 200, 201} {
		t.Run(fmt.Sprint(when), func(t *testing.T) {
			db, f, c, _ := setup(t)
			if _, err := f.apply(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			ack := c
			ack.Kind = "ACK"
			ack.Stage, ack.ServerID, ack.RequestedIP = "SELECTING", "192.0.2.1", ack.Address
			ack.IssuedMS = when
			ack.ExpiresMS = 300
			_, err := f.apply(context.Background(), ack)
			if (err == nil) != (when < 200) {
				t.Fatalf("time=%d err=%v", when, err)
			}
			if when < 200 {
				counts(t, db, 1, 2)
			} else {
				counts(t, db, 1, 1)
			}
		})
	}
	db, f, c, _ := setup(t)
	if _, err := f.apply(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	ack := c
	ack.Kind = "ACK"
	ack.Stage, ack.ServerID, ack.RequestedIP = "SELECTING", "192.0.2.1", ack.Address
	ack.IssuedMS = 110
	ack.ExpiresMS = 300
	if _, err := f.apply(context.Background(), ack); err != nil {
		t.Fatal(err)
	}
	c.XID++
	c.IssuedMS = 120
	c.ExpiresMS = 250
	if _, err := f.apply(context.Background(), c); err == nil {
		t.Fatal("active ack downgraded")
	}
	counts(t, db, 1, 2)
}
func TestReplayRequiresLiveMatchingState(t *testing.T) {
	for _, kind := range []string{"expired", "receipt", "lease", "issued", "outbox"} {
		t.Run(kind, func(t *testing.T) {
			db, f, c, _ := setup(t)
			if _, err := f.apply(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "expired":
				c.IssuedMS = 200
				c.ExpiresMS = 300
			case "receipt":
				if _, err := db.Exec(`UPDATE dhcp_receipt SET expires_ms=201`); err != nil {
					t.Fatal(err)
				}
			case "lease":
				if _, err := db.Exec(`UPDATE dhcp_lease SET revision=2`); err != nil {
					t.Fatal(err)
				}
			case "issued":
				if _, err := db.Exec(`UPDATE dhcp_lease SET issued_ms=99`); err != nil {
					t.Fatal(err)
				}
			case "outbox":
				if _, err := db.Exec(`UPDATE dhcp_outbox SET payload=x'00'`); err != nil {
					t.Fatal(err)
				}
			}
			if r, err := f.apply(context.Background(), c); err == nil || r != (reply{}) {
				t.Fatalf("bad replay %+v %v", r, err)
			}
			counts(t, db, 1, 1)
		})
	}
}

func TestTimeRollbackAndShorteningFail(t *testing.T) {
	for _, kind := range []string{"head time", "replay time", "shorter ack"} {
		t.Run(kind, func(t *testing.T) {
			db, f, c, _ := setup(t)
			if _, err := f.apply(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			ack := c
			ack.Kind = "ACK"
			ack.Stage, ack.ServerID, ack.RequestedIP = "SELECTING", "192.0.2.1", ack.Address
			ack.IssuedMS = 110
			ack.ExpiresMS = 300
			if kind == "head time" {
				ack.IssuedMS = 99
			} else if kind == "replay time" {
				later := c
				later.IssuedMS = 150
				later.ExpiresMS = 400
				if _, err := f.apply(context.Background(), later); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := f.apply(context.Background(), ack); err != nil {
					t.Fatal(err)
				}
				ack.XID++
				ack.IssuedMS = 120
				ack.ExpiresMS = 299
			}
			if r, err := f.apply(context.Background(), ack); err == nil || r != (reply{}) {
				t.Fatalf("accepted %+v %v", r, err)
			}
			if kind == "shorter ack" {
				counts(t, db, 1, 2)
			} else {
				counts(t, db, 1, 1)
			}
		})
	}
}
func TestSuccessfulHooksCannotSkipCommit(t *testing.T) {
	db, f, c, path := setup(t)
	f.beforeCommit = func() error { return nil }
	f.afterCommit = func() error { return nil }
	if _, err := f.apply(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	fresh := sqlitepolicy.OpenWriter(path)
	defer fresh.Close()
	counts(t, fresh, 1, 1)
}

func TestConcurrentSingleAddress(t *testing.T) {
	db, f, c, _ := setup(t)
	second := c
	second.ClientKey = []byte{4, 5}
	second.LeaseID = leaseID2
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, cmd := range []command{c, second} {
		wg.Add(1)
		go func(v command) { defer wg.Done(); _, err := f.apply(context.Background(), v); results <- err }(cmd)
	}
	wg.Wait()
	close(results)
	ok := 0
	for err := range results {
		if err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("successes %d", ok)
	}
	counts(t, db, 1, 1)
}
func TestFailuresRollbackEveryTableAndHead(t *testing.T) {
	for _, table := range []string{"dhcp_lease", "replica_event", "dhcp_outbox", "dhcp_receipt", "replica_head"} {
		for _, mode := range []string{"ABORT", "IGNORE"} {
			t.Run(table+mode, func(t *testing.T) {
				db, f, c, _ := setup(t)
				operation := "INSERT"
				if table == "replica_head" {
					operation = "UPDATE"
				}
				action := "RAISE(IGNORE)"
				if mode == "ABORT" {
					action = "RAISE(ABORT,'injected')"
				}
				if _, err := db.Exec(`CREATE TRIGGER injected BEFORE ` + operation + ` ON ` + table + ` BEGIN SELECT ` + action + `; END`); err != nil {
					t.Fatal(err)
				}
				if r, err := f.apply(context.Background(), c); err == nil || r != (reply{}) {
					t.Fatalf("accepted %+v %v", r, err)
				}
				counts(t, db, 0, 0)
				if _, err := f.apply(context.Background(), c); !errors.Is(err, ErrStopped) {
					t.Fatalf("not stopped %v", err)
				}
			})
		}
	}
}
func TestWrongHeadAndIdentityNoProgress(t *testing.T) {
	for _, kind := range []string{"head", "epoch", "cluster", "no ack without offer"} {
		t.Run(kind, func(t *testing.T) {
			db, f, c, _ := setup(t)
			switch kind {
			case "head":
				f.head.Hash[0] = 1
			case "epoch":
				f.head.Epoch = 2
			case "cluster":
				f.cluster[0] ^= 1
			case "no ack without offer":
				c.Kind = "ACK"
				c.Stage, c.ServerID, c.RequestedIP = "SELECTING", "192.0.2.1", c.Address
			}
			if _, err := f.apply(context.Background(), c); err == nil {
				t.Fatal("accepted")
			}
			counts(t, db, 0, 0)
		})
	}
}
func TestSimulatedCommitUncertaintyDoesNotReturnReply(t *testing.T) {
	for _, kind := range []string{"before", "after"} {
		t.Run(kind, func(t *testing.T) {
			db, f, c, _ := setup(t)
			inject := func() error { return errors.New("injected ambiguous commit") }
			if kind == "before" {
				f.beforeCommit = inject
			} else {
				f.afterCommit = inject
			}
			if r, err := f.apply(context.Background(), c); err == nil || r != (reply{}) {
				t.Fatalf("reply on unknown %+v %v", r, err)
			}
			if kind == "before" {
				counts(t, db, 0, 0)
			} else {
				counts(t, db, 1, 1)
			}
			if _, err := f.apply(context.Background(), c); !errors.Is(err, ErrStopped) {
				t.Fatalf("not stopped %v", err)
			}
		})
	}
}
