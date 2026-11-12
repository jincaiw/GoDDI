package replicarpc

// All code in this file is compiled only by go test. This loopback mTLS
// experiment demonstrates transport identity, not production authorization,
// a trustworthy clock, role, ownership, fencing, or quarantine.
import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"database/sql/driver"
	"errors"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/eventlogguard"
	"github.com/jasonwa/goddi/internal/enterprise/peeridentity"
	"github.com/jasonwa/goddi/internal/enterprise/pullbudget"
	"github.com/jasonwa/goddi/internal/enterprise/pullresponse"
	replica "github.com/jasonwa/goddi/internal/gen/replica/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const fixtureBytes = 8 << 20
const fixtureConcurrency = 8
const fixtureTimeout = 15 * time.Second
const fixtureMaxRequests = 4096

// The test listener bounds concurrent connections, including handshakes.
type boundedListener struct {
	net.Listener
	slots chan struct{}
}
type boundedConn struct {
	net.Conn
	once  sync.Once
	slots chan struct{}
}

func (c *boundedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { <-c.slots })
	return err
}
func (l *boundedListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case l.slots <- struct{}{}:
			return &boundedConn{Conn: c, slots: l.slots}, nil
		default:
			_ = c.Close()
		}
	}
}

type rpcState struct{ compressed atomic.Bool }
type rpcStateKey struct{}
type noCompression struct{}

func (noCompression) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	return context.WithValue(ctx, rpcStateKey{}, &rpcState{})
}
func (noCompression) HandleRPC(ctx context.Context, s stats.RPCStats) {
	if h, ok := s.(*stats.InHeader); ok && h.Compression != "" && h.Compression != "identity" {
		if st, ok := ctx.Value(rpcStateKey{}).(*rpcState); ok {
			st.compressed.Store(true)
		}
	}
}
func (noCompression) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context { return ctx }
func (noCompression) HandleConn(context.Context, stats.ConnStats)                       {}

type fixtureService struct {
	replica.UnimplementedReplicationServer
	log                      *eventlog.Log
	db                       *sql.DB
	hook                     func(string, context.Context)
	rollback                 func(*sql.Tx) error
	cluster, leader, standby eventlog.ID
	policy                   peeridentity.Policy
	now                      time.Time
	slots                    chan struct{}
	reads                    atomic.Int64
	requests                 atomic.Uint64
}

func (s *fixtureService) Pull(ctx context.Context, r *replica.PullRequest) (*replica.PullResponse, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	info, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || info.State.Version != tls.VersionTLS13 {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	id, err := peeridentity.Verify(info.State, s.policy, s.now)
	if err != nil || id.ClusterID != s.cluster.String() || id.NodeID != s.standby.String() {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if s.requests.Add(1) > fixtureMaxRequests {
		return nil, status.Error(codes.ResourceExhausted, "busy")
	}
	if st, ok := ctx.Value(rpcStateKey{}).(*rpcState); !ok || st.compressed.Load() {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	if r == nil || len(r.ProtoReflect().GetUnknown()) != 0 || !bytes.Equal(r.ClusterId, s.cluster[:]) || r.After == nil || len(r.After.ProtoReflect().GetUnknown()) != 0 || len(r.After.Hash) != 32 || (r.After.Epoch == 0 || r.After.Epoch > math.MaxInt64) || r.After.Seq > math.MaxInt64 || len(r.ReadableSchemaVersions) == 0 || len(r.ReadableSchemaVersions) > 256 {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	count, limit := r.MaxEvents, r.MaxBytes
	if count == 0 {
		count = pullbudget.MaxEvents
	}
	if limit == 0 {
		limit = pullbudget.MaxBytes
	}
	if count > pullbudget.MaxEvents || limit < pullbudget.MinBytes || limit > pullbudget.MaxBytes {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	seen := map[uint32]bool{}
	for _, v := range r.ReadableSchemaVersions {
		if v == 0 || seen[v] {
			return nil, status.Error(codes.InvalidArgument, "invalid request")
		}
		seen[v] = true
	}
	var after eventlog.Position
	after.Epoch = r.After.Epoch
	after.Seq = r.After.Seq
	copy(after.Hash[:], r.After.Hash)
	if after.Epoch == 1 && after.Seq == 0 && after.Hash != eventlog.GenesisHash() {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return nil, status.Error(codes.ResourceExhausted, "busy")
	}
	ctx, cancel := context.WithTimeout(ctx, fixtureTimeout)
	defer cancel()
	if s.hook != nil {
		s.hook("before-connection", ctx)
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	wire, err := s.readCandidate(ctx, after, count, limit, r.ReadableSchemaVersions)
	if err != nil {
		if ctx.Err() != nil {
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		return nil, status.Error(codes.FailedPrecondition, "unavailable")
	}
	var out replica.PullResponse
	if err = proto.Unmarshal(wire, &out); err != nil || proto.Size(&out) != len(wire) || len(wire) > int(limit) || len(out.ProtoReflect().GetUnknown()) != 0 {
		return nil, status.Error(codes.Internal, "unavailable")
	}
	if err = ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	return &out, nil
}

// controlResponse encodes only a local fixture observation, not authority.
func (s *fixtureService) controlResponse(ctx context.Context, disposition replica.PullDisposition, input pullresponse.ControlInput) ([]byte, error) {
	wire, err := pullresponse.BuildControl(input, disposition)
	if s.hook != nil {
		s.hook("after-build", ctx)
	}
	if ctx.Err() != nil {
		return nil, errors.Join(err, ctx.Err())
	}
	if err != nil {
		return nil, err
	}
	return wire, nil
}

// readCandidate is a test-only composition, never a source, role or authority proof.
// A dedicated connection is retired after use so query_only cannot leak to a pool.
func (s *fixtureService) readCandidate(ctx context.Context, after eventlog.Position, count, limit uint32, schemas []uint32) (wire []byte, err error) {
	s.reads.Add(1)
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		retire := conn.Raw(func(any) error { return driver.ErrBadConn })
		if retire != nil && !errors.Is(retire, driver.ErrBadConn) {
			err = errors.Join(err, retire)
			wire = nil
		}
		if e := conn.Close(); e != nil && !errors.Is(e, sql.ErrConnDone) {
			err = errors.Join(err, e)
			wire = nil
		}
		if e := ctx.Err(); e != nil {
			err = errors.Join(err, e)
			wire = nil
		}
	}()
	if _, err = conn.ExecContext(ctx, `PRAGMA query_only=ON`); err != nil {
		return nil, err
	}
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() {
		var e error
		if s.rollback != nil {
			e = s.rollback(tx)
		} else {
			e = tx.Rollback()
		}
		if e != nil {
			err = errors.Join(err, e)
			wire = nil
		}
	}()
	version, err := eventlogguard.InspectTx(ctx, tx)
	if err != nil || version != eventlogguard.HardenedReference {
		return nil, errors.Join(err, errors.New("fixture: hardened schema required"))
	}
	if s.hook != nil {
		s.hook("after-inspection", ctx)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	// This head is a local test-fixture observation, not a role or authority.
	// Check its SQLite types before using it as an anchor in this same view.
	var epoch, seq, schema, when int64
	var hash []byte
	err = tx.QueryRowContext(ctx, `SELECT CASE WHEN typeof(epoch)='integer' THEN epoch END, CASE WHEN typeof(seq)='integer' THEN seq END, CASE WHEN typeof(schema_version)='integer' THEN schema_version END, CASE WHEN typeof(advanced_at_unix_ms)='integer' THEN advanced_at_unix_ms END, CASE WHEN typeof(hash)='blob' AND length(hash)=32 THEN hash END FROM main.replica_head WHERE cluster_id=?`, s.cluster[:]).Scan(&epoch, &seq, &schema, &when, &hash)
	if err != nil || epoch <= 0 || seq < 0 || schema != int64(eventlog.SchemaVersionV1) || when < 0 || len(hash) != 32 {
		return nil, errors.Join(err, errors.New("fixture: invalid observed head"))
	}
	observed := eventlog.Position{Epoch: uint64(epoch), Seq: uint64(seq)}
	copy(observed.Hash[:], hash)
	// Genesis has no event leader; it remains an unauthenticated local
	// observation. Every nonempty head must match the configured test leader.
	if observed.Seq > 0 {
		var leader []byte
		e := tx.QueryRowContext(ctx, `SELECT CASE WHEN typeof(leader_id)='blob' AND length(leader_id)=16 THEN leader_id END FROM main.replica_event WHERE cluster_id=? AND epoch=? AND seq=?`, s.cluster[:], epoch, seq).Scan(&leader)
		if e != nil || !bytes.Equal(leader, s.leader[:]) {
			return nil, errors.Join(e, errors.New("fixture: head leader mismatch"))
		}
	}
	if observed.Epoch != after.Epoch {
		// ReadSnapshotTx verifies the head event and this anchor; it does not
		// prove a handover, role, source generation or any unread history.
		events, head, e := s.log.ReadSnapshotTx(ctx, tx, observed, 1)
		if e != nil || len(events) != 0 || head != observed {
			return nil, errors.Join(e, errors.New("fixture: unverified observed head"))
		}
		return s.controlResponse(ctx, replica.PullDisposition_EPOCH_REBASE_REQUIRED, pullresponse.ControlInput{Cluster: s.cluster, Leader: s.leader, After: after, Head: observed, MaxEvents: count, MaxBytes: limit, ReadableSchemas: schemas})
	}
	events, head, err := s.log.ReadSnapshotTx(ctx, tx, after, int(count))
	if err != nil {
		return nil, err
	}
	if s.hook != nil {
		s.hook("after-read", ctx)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if len(events) > 0 && events[0].LeaderID != s.leader {
		return nil, errors.New("fixture: event leader mismatch")
	}
	readable := false
	if len(events) > 0 {
		for _, v := range schemas {
			if v == events[0].Schema {
				readable = true
				break
			}
		}
	}
	if len(events) > 0 && !readable {
		return s.controlResponse(ctx, replica.PullDisposition_SCHEMA_INCOMPATIBLE, pullresponse.ControlInput{Cluster: s.cluster, Leader: s.leader, After: after, Head: head, MaxEvents: count, MaxBytes: limit, ReadableSchemas: schemas, Next: &events[0]})
	}
	if len(events) == 0 {
		found := false
		for _, v := range schemas {
			if v == eventlog.SchemaVersionV1 {
				found = true
			}
		}
		if !found {
			return nil, errors.New("fixture: at head with no readable candidate schema")
		}
	}
	wire, err = pullresponse.Build(pullresponse.Input{Cluster: s.cluster, Leader: s.leader, After: after, Head: head, Events: events, MaxEvents: count, MaxBytes: limit, ReadableSchemas: schemas})
	if s.hook != nil {
		s.hook("after-build", ctx)
	}
	if ctx.Err() != nil {
		return nil, errors.Join(err, ctx.Err())
	}
	if err != nil {
		return nil, err
	}
	return wire, nil
}

type fixture struct {
	server   *grpc.Server
	listener net.Listener
	service  *fixtureService
	done     chan struct{}
	serveErr error
	stopOnce sync.Once
}

func startFixture(l net.Listener, cert tls.Certificate, clientRoots *x509.CertPool, p peeridentity.Policy, clock time.Time, log *eventlog.Log, db *sql.DB, cluster, leader, standby eventlog.ID) (*fixture, error) {
	if l == nil || clientRoots == nil || log == nil || db == nil || clock.IsZero() || len(cert.Certificate) == 0 || cert.PrivateKey == nil || cluster == (eventlog.ID{}) || leader == (eventlog.ID{}) || standby == (eventlog.ID{}) || cluster == leader || leader == standby || cluster == standby || p.ClusterID != cluster.String() || p.NodeID != standby.String() || p.EKU != x509.ExtKeyUsageClientAuth || log.Cluster() != cluster || log.Node() != leader {
		return nil, ErrNoAuthority
	}
	a, ok := l.Addr().(*net.TCPAddr)
	if !ok || a.IP == nil || !a.IP.IsLoopback() {
		return nil, ErrNoAuthority
	}
	if _, ok = l.(*net.TCPListener); !ok {
		return nil, ErrNoAuthority
	}
	// Copy policy roots; the test controls clock and network and must not treat
	// these values as evidence of a real deployment's authorization.
	p.RootDER = append([][]byte(nil), p.RootDER...)
	for i := range p.RootDER {
		p.RootDER[i] = bytes.Clone(p.RootDER[i])
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientRoots, Time: func() time.Time { return clock }}
	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)), grpc.MaxRecvMsgSize(fixtureBytes), grpc.MaxSendMsgSize(fixtureBytes), grpc.MaxConcurrentStreams(fixtureConcurrency), grpc.ConnectionTimeout(fixtureTimeout), grpc.StatsHandler(noCompression{}))
	service := &fixtureService{log: log, db: db, cluster: cluster, leader: leader, standby: standby, policy: p, now: clock, slots: make(chan struct{}, fixtureConcurrency)}
	replica.RegisterReplicationServer(srv, service)
	f := &fixture{server: srv, listener: l, service: service, done: make(chan struct{})}
	bounded := &boundedListener{Listener: l, slots: make(chan struct{}, fixtureConcurrency)}
	go func() { f.serveErr = srv.Serve(bounded); close(f.done) }()
	return f, nil
}
func (f *fixture) stop() error {
	f.stopOnce.Do(f.server.Stop)
	<-f.done
	if f.serveErr != nil && !errors.Is(f.serveErr, grpc.ErrServerStopped) {
		return f.serveErr
	}
	return nil
}
