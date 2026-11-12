package replicarpc

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/ipamevent"
	"github.com/jasonwa/goddi/internal/enterprise/ipamstore"
	"github.com/jasonwa/goddi/internal/enterprise/peeridentity"
	replica "github.com/jasonwa/goddi/internal/gen/replica/v1"
	"github.com/jasonwa/goddi/internal/sqlitepolicy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type certs struct {
	ca             *x509.Certificate
	key            ed25519.PrivateKey
	server, client tls.Certificate
	clientDER      []byte
	now            time.Time
}

func caFixture(t *testing.T) *certs {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test root"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	f := &certs{ca: ca, key: key, now: now}
	f.server, _ = f.issue(t, "server.example.test", x509.ExtKeyUsageServerAuth, now.Add(-time.Minute), now.Add(time.Minute))
	f.client, f.clientDER = f.issue(t, "standby.example.test", x509.ExtKeyUsageClientAuth, now.Add(-time.Minute), now.Add(time.Minute))
	return f
}
func (f *certs) issue(t *testing.T, name string, usage x509.ExtKeyUsage, from, to time.Time) (tls.Certificate, []byte) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: serial, DNSNames: []string{name}, NotBefore: from, NotAfter: to, ExtKeyUsage: []x509.ExtKeyUsage{usage}, KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, f.ca, pub, f.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, der
}
func ids() (eventlog.ID, eventlog.ID, eventlog.ID) {
	var c, l, s eventlog.ID
	c[15] = 1
	l[15] = 2
	s[15] = 3
	return c, l, s
}
func testServer(t *testing.T, mutate func(*peeridentity.Policy), clock time.Time) (*fixture, *certs) {
	t.Helper()
	f := caFixture(t)
	if clock.IsZero() {
		clock = f.now
	}
	roots := x509.NewCertPool()
	roots.AddCert(f.ca)
	cluster, leader, standby := ids()
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "log.db"))
	db.SetMaxOpenConns(fixtureConcurrency)
	t.Cleanup(func() { _ = db.Close() })
	if err := ipamstore.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := eventlog.Init(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(fixtureGuardSQL))) != fixtureGuardDigest || fmt.Sprintf("%x", sha256.Sum256([]byte(eventlog.Schema))) != fixtureEventDigest {
		t.Fatal("fixture schema pin mismatch")
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(fixtureGuardSQL); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(fixtureGuardMarker); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO main.eventlog_reference_guard_migrations VALUES(1,?,?)`, fixtureEventDigest, fixtureGuardDigest); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	o := ipamevent.Occupancy{TenantID: "11111111-1111-4111-8111-111111111111", SpaceID: "22222222-2222-4222-8222-222222222222", ID: "33333333-3333-4333-8333-333333333333", SubnetID: "44444444-4444-4444-8444-444444444444", Family: 4, Address: "192.0.2.10", Kind: "STATIC", OwnerRef: strings.Repeat("f", 900), CreatedAtMS: "100", Revision: "1"}
	change, err := ipamevent.EncodeChange(ipamevent.Set, o)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := eventlog.MarshalChanges(change)
	if err != nil {
		t.Fatal(err)
	}
	e := eventlog.Event{ClusterID: cluster, LeaderID: leader, Position: eventlog.Position{Epoch: 1, Seq: 1}, Schema: 1, CommitMs: 100, PrevHash: eventlog.GenesisHash(), Payload: payload}
	e.Position.Hash, err = e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO replica_event(cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,?,?,?,?,?,?,?,?)`, cluster[:], 1, 1, leader[:], 1, 100, e.PrevHash[:], e.Position.Hash[:], payload); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,1,1,?,1,100)`, cluster[:], e.Position.Hash[:]); err != nil {
		t.Fatal(err)
	}
	p := peeridentity.Policy{ClusterID: cluster.String(), NodeID: standby.String(), DNSName: "standby.example.test", PinType: peeridentity.CertificateSHA256, Pin: sha256.Sum256(f.clientDER), RootDER: [][]byte{f.ca.Raw}, EKU: x509.ExtKeyUsageClientAuth}
	if mutate != nil {
		mutate(&p)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	sv, err := startFixture(listener, f.server, roots, p, clock, eventlog.NewLog(db, cluster, leader), db, cluster, leader, standby)
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sv.stop(); err != nil {
			t.Error(err)
		}
	})
	return sv, f
}
func connect(t *testing.T, sv *fixture, f *certs, client *tls.Certificate, ca *x509.Certificate) *grpc.ClientConn {
	return connectPinned(t, sv, f, client, ca, peeridentity.CertificateSHA256)
}
func connectPinned(t *testing.T, sv *fixture, f *certs, client *tls.Certificate, ca *x509.Certificate, pinType peeridentity.PinType) *grpc.ClientConn {
	return connectPinnedObserved(t, sv, f, client, ca, pinType, nil)
}
func connectPinnedObserved(t *testing.T, sv *fixture, f *certs, client *tls.Certificate, ca *x509.Certificate, pinType peeridentity.PinType, observe func(error)) *grpc.ClientConn {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	cluster, leader, _ := ids()
	policy := peeridentity.Policy{ClusterID: cluster.String(), NodeID: leader.String(), DNSName: "server.example.test", PinType: pinType, Pin: sha256.Sum256(f.server.Certificate[0]), RootDER: [][]byte{ca.Raw}, EKU: x509.ExtKeyUsageServerAuth}
	if pinType == peeridentity.PublicKeySHA256 {
		leaf, err := x509.ParseCertificate(f.server.Certificate[0])
		if err != nil {
			t.Fatal(err)
		}
		policy.Pin = sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, RootCAs: pool, ServerName: "server.example.test", Time: func() time.Time { return f.now }, VerifyConnection: func(state tls.ConnectionState) error {
		id, err := peeridentity.VerifyHandshake(state, policy, f.now)
		if observe != nil {
			observe(err)
		}
		if err != nil {
			return err
		}
		if id.ClusterID != cluster.String() || id.NodeID != leader.String() {
			return peeridentity.ErrInvalid
		}
		return nil
	}}
	if client != nil {
		cfg.Certificates = []tls.Certificate{*client}
	}
	c, err := grpc.NewClient("passthrough:///"+sv.listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(cfg)), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(fixtureBytes+1), grpc.MaxCallSendMsgSize(fixtureBytes+1024)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}
func request() *replica.PullRequest {
	cluster, _, _ := ids()
	return &replica.PullRequest{ClusterId: cluster.Bytes(), After: &replica.Position{Epoch: 1, Hash: make([]byte, 32)}, ReadableSchemaVersions: []uint32{1}}
}
func pull(c *grpc.ClientConn, r *replica.PullRequest, opts ...grpc.CallOption) (*replica.PullResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return replica.NewReplicationClient(c).Pull(ctx, r, opts...)
}
func TestPublicStartCannotAuthorize(t *testing.T) {
	if !errors.Is(Start(), ErrNoAuthority) {
		t.Fatal("start authorized")
	}
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if _, err = startFixture(listener, tls.Certificate{}, nil, peeridentity.Policy{}, time.Time{}, nil, nil, eventlog.ID{}, eventlog.ID{}, eventlog.ID{}); !errors.Is(err, ErrNoAuthority) {
		t.Fatal("unsafe fixture accepted")
	}
}
func TestFixtureRejectsWildcardEvenWithValidConfiguration(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	pool := x509.NewCertPool()
	pool.AddCert(f.ca)
	if _, err = startFixture(listener, f.server, pool, sv.service.policy, f.now, sv.service.log, sv.service.db, sv.service.cluster, sv.service.leader, sv.service.standby); !errors.Is(err, ErrNoAuthority) {
		t.Fatalf("wildcard accepted: %v", err)
	}
}

func TestRealMutualTLSAndRequestValidation(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	c := connect(t, sv, f, &f.client, f.ca)
	out, err := pull(c, request())
	if err != nil || out.GetDisposition() != replica.PullDisposition_BATCH || out.ObservedHead.GetSeq() != 1 || len(out.Events) != 1 {
		t.Fatalf("pull %v %v", out, err)
	}
	small := request()
	small.MaxBytes = 1024
	tooSmall, err := pull(c, small)
	if err != nil || tooSmall.Disposition != replica.PullDisposition_REQUEST_BUDGET_TOO_SMALL || tooSmall.RequiredBytes <= 1024 || len(tooSmall.Events) != 0 {
		t.Fatalf("budget response: %v %v", tooSmall, err)
	}
	if proto.Size(tooSmall) > int(small.MaxBytes) {
		t.Fatal("error response exceeded budget")
	}
	small.MaxBytes = tooSmall.RequiredBytes - 1
	almost, err := pull(c, small)
	if err != nil || almost.Disposition != replica.PullDisposition_REQUEST_BUDGET_TOO_SMALL || almost.RequiredBytes != tooSmall.RequiredBytes || proto.Size(almost) > int(small.MaxBytes) {
		t.Fatalf("required-1: %v %v", almost, err)
	}
	small.MaxBytes = tooSmall.RequiredBytes
	exact, err := pull(c, small)
	if err != nil || exact.Disposition != replica.PullDisposition_BATCH || len(exact.Events) != 1 || proto.Size(exact) > int(small.MaxBytes) || !bytes.Equal(exact.Events[0].Payload, out.Events[0].Payload) || !bytes.Equal(exact.Events[0].Position.Hash, out.Events[0].Position.Hash) {
		t.Fatalf("required: %v %v", exact, err)
	}
	before := sv.service.reads.Load()
	cases := []func(*replica.PullRequest){func(r *replica.PullRequest) { r.ClusterId[0] ^= 1 }, func(r *replica.PullRequest) { r.After.Hash = []byte{1} }, func(r *replica.PullRequest) { r.After.Epoch = 0 }, func(r *replica.PullRequest) { r.MaxBytes = 1023 }, func(r *replica.PullRequest) { r.ReadableSchemaVersions = []uint32{1, 1} }, func(r *replica.PullRequest) { r.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 1}) }}
	for _, change := range cases {
		r := request()
		change(r)
		if _, err = pull(c, r); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	if sv.service.reads.Load() != before {
		t.Fatal("invalid request reached storage")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = replica.NewReplicationClient(c).ReportApplied(ctx, &replica.ReportAppliedRequest{})
	if status.Code(err) != codes.Unimplemented {
		t.Fatal(err)
	}
}
func TestPeerPolicyClockRejectsAfterSuccessfulTLS(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	// TLS still uses its original fixture time; only independent peer policy
	// verification sees the later simulated time. Neither is a trusted clock.
	sv.service.now = f.now.Add(2 * time.Minute)
	c := connect(t, sv, f, &f.client, f.ca)
	out, err := pull(c, request())
	if status.Code(err) != codes.Unauthenticated || out != nil || sv.service.reads.Load() != 0 {
		t.Fatalf("expired policy: %v %v", out, err)
	}
}

func TestRealTLSRejections(t *testing.T) {
	for _, kind := range []string{"no certificate", "wrong client ca", "wrong client eku", "wrong client san", "wrong peer pin", "expired", "wrong server ca"} {
		t.Run(kind, func(t *testing.T) {
			var mutate func(*peeridentity.Policy)
			if kind == "wrong peer pin" {
				mutate = func(p *peeridentity.Policy) { p.Pin[0] ^= 1 }
			}
			sv, f := testServer(t, mutate, time.Time{})
			client := &f.client
			root := f.ca
			switch kind {
			case "no certificate":
				client = nil
			case "wrong client ca":
				other := caFixture(t)
				client = &other.client
			case "wrong client eku":
				v, _ := f.issue(t, "standby.example.test", x509.ExtKeyUsageServerAuth, f.now.Add(-time.Minute), f.now.Add(time.Minute))
				client = &v
			case "wrong client san":
				v, der := f.issue(t, "wrong.example.test", x509.ExtKeyUsageClientAuth, f.now.Add(-time.Minute), f.now.Add(time.Minute))
				// Match the pin so this case specifically exercises SAN rejection.
				sv.service.policy.Pin = sha256.Sum256(der)
				client = &v
			case "expired":
				v, _ := f.issue(t, "standby.example.test", x509.ExtKeyUsageClientAuth, f.now.Add(-time.Hour), f.now.Add(-time.Minute))
				client = &v
			case "wrong server ca":
				root = caFixture(t).ca
			}
			conn := connect(t, sv, f, client, root)
			out, err := pull(conn, request())
			if err == nil || out != nil || sv.service.reads.Load() != 0 {
				t.Fatalf("accepted %s: %v %v", kind, out, err)
			}
		})
	}
}
func TestCompressedAndOversizedRequestsFail(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	conn := connect(t, sv, f, &f.client, f.ca)
	if _, err := pull(conn, request(), grpc.UseCompressor("gzip")); err == nil || sv.service.reads.Load() != 0 {
		t.Fatalf("compressed request accepted: %v", err)
	}
	r := request()
	r.ProtoReflect().SetUnknown(append([]byte{0xa2, 0x06, 0x80, 0x80, 0x80, 0x04}, make([]byte, fixtureBytes)...))
	if _, err := pull(conn, r); err == nil || sv.service.reads.Load() != 0 {
		t.Fatalf("oversized accepted: %v", err)
	}
}
func TestConcurrentRequestsAndCancellation(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	conn := connect(t, sv, f, &f.client, f.ca)
	var wg sync.WaitGroup
	// The fixture bounds concurrency (fixtureConcurrency slots) and wraps each
	// request in its own fixtureTimeout context. Under a loaded machine -- in
	// particular a full `go test -race ./...` -- both ResourceExhausted (slot
	// busy) and DeadlineExceeded (the fixture's bounded deadline expiring while
	// earlier requests still hold slots) are legitimate backpressure outcomes.
	// This test is about concurrency safety and cancellation, not timing, so
	// tolerate both. Require at least one real success so a completely hung
	// server cannot pass silently.
	var succeeded atomic.Int32
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := pull(conn, request())
			if err == nil && out != nil {
				succeeded.Add(1)
				return
			}
			switch status.Code(err) {
			case codes.ResourceExhausted, codes.DeadlineExceeded:
				// expected bounded-fixture backpressure
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if succeeded.Load() == 0 {
		t.Fatal("no concurrent pull succeeded; the server appears hung")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := replica.NewReplicationClient(conn).Pull(ctx, request()); status.Code(err) != codes.Canceled {
		t.Fatalf("cancel: %v", err)
	}
}
