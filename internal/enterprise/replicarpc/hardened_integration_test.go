package replicarpc

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/peeridentity"
	replica "github.com/jasonwa/goddi/internal/gen/replica/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"modernc.org/sqlite"
)

// blockingReadConnector pauses the eventlog head query at the driver boundary.
// It simulates an in-flight read; it does not reproduce a physical SQLite fault.
type blockingReadConnector struct {
	path    string
	entered chan struct{}
	once    sync.Once
}

func (c *blockingReadConnector) Driver() driver.Driver { return &sqlite.Driver{} }
func (c *blockingReadConnector) Connect(ctx context.Context) (driver.Conn, error) {
	base, err := (&sqlite.Driver{}).Open(c.path)
	if err != nil {
		return nil, err
	}
	x := base.(driver.ExecerContext)
	for _, q := range []string{`PRAGMA foreign_keys=ON`, `PRAGMA synchronous=FULL`, `PRAGMA busy_timeout=5000`} {
		if _, err = x.ExecContext(ctx, q, nil); err != nil {
			base.Close()
			return nil, err
		}
	}
	return &blockingReadConn{Conn: base, owner: c}, nil
}

type blockingReadConn struct {
	driver.Conn
	owner *blockingReadConnector
}

func (c *blockingReadConn) BeginTx(ctx context.Context, o driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, o)
}
func (c *blockingReadConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, q, args)
}
func (c *blockingReadConn) CheckNamedValue(v *driver.NamedValue) error {
	if n, ok := c.Conn.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(v)
	}
	return driver.ErrSkip
}
func (c *blockingReadConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(q, "FROM main.replica_head WHERE cluster_id = ?") {
		c.owner.once.Do(func() { close(c.owner.entered) })
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
}
func TestCancellationDuringSnapshotRead(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	var n int
	var name, filename string
	if err := sv.service.db.QueryRow(`PRAGMA database_list`).Scan(&n, &name, &filename); err != nil {
		t.Fatal(err)
	}
	blocked := &blockingReadConnector{path: filename, entered: make(chan struct{})}
	db := sql.OpenDB(blocked)
	defer db.Close()
	sv.service.db = db
	finished := make(chan struct{})
	sv.service.rollback = func(tx *sql.Tx) error { defer close(finished); return tx.Rollback() }
	conn := connect(t, sv, f, &f.client, f.ca)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		out, err := replica.NewReplicationClient(conn).Pull(ctx, request())
		if out != nil {
			done <- errors.New("partial output")
			return
		}
		done <- err
	}()
	select {
	case <-blocked.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("head read not entered")
	}
	cancel()
	select {
	case err := <-done:
		if status.Code(err) != codes.Canceled {
			t.Fatalf("cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("call stuck")
	}
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup stuck")
	}
}

func TestSameCAAndSANDifferentKeyFailsPinsBeforeDB(t *testing.T) {
	for _, pinType := range []peeridentity.PinType{peeridentity.CertificateSHA256, peeridentity.PublicKeySHA256} {
		t.Run(string(rune('0'+pinType)), func(t *testing.T) {
			sv, f := testServer(t, nil, time.Time{})
			leaf, err := x509.ParseCertificate(f.clientDER)
			if err != nil {
				t.Fatal(err)
			}
			if pinType == peeridentity.PublicKeySHA256 {
				sv.service.policy.PinType = pinType
				sv.service.policy.Pin = sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
			}
			rogue, _ := f.issue(t, "standby.example.test", x509.ExtKeyUsageClientAuth, f.now.Add(-time.Minute), f.now.Add(time.Minute))
			conn := connect(t, sv, f, &rogue, f.ca)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			ctx = metadata.AppendToOutgoingContext(ctx, "node-id", sv.service.standby.String(), "cluster-id", sv.service.cluster.String())
			_, err = replica.NewReplicationClient(conn).Pull(ctx, request())
			if status.Code(err) != codes.Unauthenticated {
				t.Fatalf("wrong pin: %v", err)
			}
			if sv.service.reads.Load() != 0 {
				t.Fatal("untrusted certificate reached database")
			}
		})
	}
}
func TestClientPinsServerBeforePull(t *testing.T) {
	for _, pinType := range []peeridentity.PinType{peeridentity.CertificateSHA256, peeridentity.PublicKeySHA256} {
		t.Run(string(rune('0'+pinType)), func(t *testing.T) {
			sv, f := testServer(t, nil, time.Time{})
			good := connectPinned(t, sv, f, &f.client, f.ca, pinType)
			if out, err := pull(good, request()); err != nil || len(out.Events) != 1 {
				t.Fatalf("pinned server %v %v", out, err)
			}
			rogue, _ := f.issue(t, "server.example.test", x509.ExtKeyUsageServerAuth, f.now.Add(-time.Minute), f.now.Add(time.Minute))
			roots := x509.NewCertPool()
			roots.AddCert(f.ca)
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			bad, err := startFixture(listener, rogue, roots, sv.service.policy, f.now, sv.service.log, sv.service.db, sv.service.cluster, sv.service.leader, sv.service.standby)
			if err != nil {
				listener.Close()
				t.Fatal(err)
			}
			defer bad.stop()
			// The same rogue certificate passes the ordinary CA/DNS client
			// handshake; the rejection below must come from the pin callback.
			baseline, err := tls.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, "tcp", listener.Addr().String(), &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "server.example.test", Certificates: []tls.Certificate{f.client}, Time: func() time.Time { return f.now }})
			if err != nil {
				t.Fatalf("baseline CA/DNS handshake: %v", err)
			}
			baseline.Close()
			observed := make(chan error, 1)
			conn := connectPinnedObserved(t, bad, f, &f.client, f.ca, pinType, func(err error) {
				select {
				case observed <- err:
				default:
				}
			})
			conn.Connect()
			select {
			case err := <-observed:
				if !errors.Is(err, peeridentity.ErrInvalid) {
					t.Fatalf("pin callback: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("pin callback not reached")
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			for ctx.Err() == nil {
				state := conn.GetState()
				if state == connectivity.Ready {
					t.Fatal("wrong server key accepted")
				}
				if !conn.WaitForStateChange(ctx, state) {
					break
				}
			}
			if bad.service.reads.Load() != 0 {
				t.Fatal("wrong server reached DB before Pull")
			}
		})
	}
}

func TestHardenedSchemaFailureBeforeResponse(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	if _, err := sv.service.db.Exec(`DROP TRIGGER replica_event_rowid_positive_guard`); err != nil {
		t.Fatal(err)
	}
	conn := connect(t, sv, f, &f.client, f.ca)
	if out, err := pull(conn, request()); status.Code(err) != codes.FailedPrecondition || out != nil {
		t.Fatalf("schema drift %v %v", out, err)
	}
}
func TestRollbackFailureCannotReturnEvents(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	sv.service.rollback = func(tx *sql.Tx) error {
		if err := tx.Rollback(); err != nil {
			return err
		}
		return errors.New("simulated rollback report failure")
	}
	conn := connect(t, sv, f, &f.client, f.ca)
	out, err := pull(conn, request())
	if status.Code(err) != codes.FailedPrecondition || out != nil {
		t.Fatalf("rollback failure %v %v", out, err)
	}
}
func TestPartialPrefixDoesNotProveUnseenGap(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	conn := connect(t, sv, f, &f.client, f.ca)
	first, err := pull(conn, request())
	if err != nil {
		t.Fatal(err)
	}
	old := first.Events[0]
	var previous [32]byte
	copy(previous[:], old.Position.Hash)
	e := eventlog.Event{ClusterID: sv.service.cluster, LeaderID: sv.service.leader, Position: eventlog.Position{Epoch: 1, Seq: 3}, Schema: 1, CommitMs: 102, PrevHash: previous, Payload: old.Payload}
	e.Position.Hash, err = e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sv.service.db.Exec(`INSERT INTO main.replica_event(cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,1,3,?,1,102,?,?,?)`, e.ClusterID[:], e.LeaderID[:], e.PrevHash[:], e.Position.Hash[:], e.Payload); err != nil {
		t.Fatal(err)
	}
	if _, err = sv.service.db.Exec(`UPDATE main.replica_head SET seq=3,hash=?,advanced_at_unix_ms=102 WHERE cluster_id=?`, e.Position.Hash[:], e.ClusterID[:]); err != nil {
		t.Fatal(err)
	}
	r := request()
	r.MaxEvents = 1
	out, err := pull(conn, r)
	if err != nil || len(out.Events) != 1 || out.ObservedHead.Seq != 3 {
		t.Fatalf("prefix %v %v", out, err)
	}
	r.MaxEvents = 3
	if out, err = pull(conn, r); err == nil || out != nil {
		t.Fatalf("gap accepted %v %v", out, err)
	}
}

func TestDifferentStoredLeaderRejected(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	conn := connect(t, sv, f, &f.client, f.ca)
	first, err := pull(conn, request())
	if err != nil {
		t.Fatal(err)
	}
	old := first.Events[0]
	var previous [32]byte
	copy(previous[:], old.Position.Hash)
	foreign := sv.service.leader
	foreign[0] = 7
	e := eventlog.Event{ClusterID: sv.service.cluster, LeaderID: foreign, Position: eventlog.Position{Epoch: 1, Seq: 2}, Schema: 1, CommitMs: 102, PrevHash: previous, Payload: old.Payload}
	e.Position.Hash, err = e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sv.service.db.Exec(`INSERT INTO main.replica_event(cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,1,2,?,1,102,?,?,?)`, e.ClusterID[:], e.LeaderID[:], e.PrevHash[:], e.Position.Hash[:], e.Payload); err != nil {
		t.Fatal(err)
	}
	if _, err = sv.service.db.Exec(`UPDATE main.replica_head SET seq=2,hash=?,advanced_at_unix_ms=102 WHERE cluster_id=?`, e.Position.Hash[:], e.ClusterID[:]); err != nil {
		t.Fatal(err)
	}
	if out, err := pull(conn, request()); status.Code(err) != codes.FailedPrecondition || out != nil {
		t.Fatalf("mixed leader %v %v", out, err)
	}
}

func TestDeadlineAtInspection(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	sv.service.hook = func(stage string, ctx context.Context) {
		if stage == "after-inspection" {
			<-ctx.Done()
		}
	}
	conn := connect(t, sv, f, &f.client, f.ca)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	out, err := replica.NewReplicationClient(conn).Pull(ctx, request())
	if out != nil || status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("deadline %v %v", out, err)
	}
}

func TestCancellationAtBoundaries(t *testing.T) {
	for _, phase := range []string{"before-connection", "after-inspection", "after-read", "after-build"} {
		t.Run(phase, func(t *testing.T) {
			sv, f := testServer(t, nil, time.Time{})
			entered := make(chan struct{})
			finished := make(chan struct{})
			sv.service.hook = func(stage string, ctx context.Context) {
				if stage == phase {
					close(entered)
					<-ctx.Done()
				}
			}
			sv.service.rollback = func(tx *sql.Tx) error { defer close(finished); return tx.Rollback() }
			conn := connect(t, sv, f, &f.client, f.ca)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				out, err := replica.NewReplicationClient(conn).Pull(ctx, request())
				if out != nil {
					done <- errors.New("canceled call returned response")
					return
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("hook not reached")
			}
			cancel()
			select {
			case err := <-done:
				if status.Code(err) != codes.Canceled {
					t.Fatalf("cancel: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("call stuck")
			}
			if phase == "before-connection" {
				if sv.service.reads.Load() != 0 {
					t.Fatal("canceled call acquired connection")
				}
			} else {
				select {
				case <-finished:
				case <-time.After(5 * time.Second):
					t.Fatal("rollback not completed")
				}
			}
		})
	}
}
