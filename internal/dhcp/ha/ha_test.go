package ha

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dataplane"
)

// These tests are written against the three properties the package claims:
// a binding is acknowledged only after the mirror has applied it, the mirror
// never holds anything the primary did not, and losing the mirror stops the
// node from promising rather than from answering.
//
// They are the load-bearing ones. A test that only asserted "the pair comes up"
// would pass for an implementation that confirms against its own store.

const testToken = "0123456789abcdef"

// The timings are short so that the tests are as fast as the mechanism is and
// no faster. The relationships between them are the ones the configuration
// validator enforces: a confirmation is given up on before the peer is
// declared stale, and a heartbeat arrives well inside the staleness bound.
const (
	testConfirmTimeout = 150 * time.Millisecond
	testHeartbeat      = 25 * time.Millisecond
	testPeerStaleAfter = 400 * time.Millisecond
)

func testConfig(t *testing.T, role string) Config {
	t.Helper()
	cfg := config.DefaultDHCPHAConfig()
	cfg.Enabled = true
	cfg.NodeID = "node-" + role
	cfg.Role = role
	cfg.ListenAddr = "127.0.0.1:0"
	cfg.PeerAddress = "127.0.0.1:0"
	cfg.PeerToken = testToken
	cfg.ConfirmTimeout = testConfirmTimeout.String()
	cfg.HeartbeatInterval = testHeartbeat.String()
	cfg.PeerStaleAfter = testPeerStaleAfter.String()

	resolved, err := FromConfig(cfg)
	if err != nil {
		t.Fatalf("resolving the test configuration: %v", err)
	}
	return resolved
}

func openStore(t *testing.T, name string) *dataplane.Store {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), name+".db")
	store, err := dataplane.Open(config.DataPlaneLease, dsn)
	if err != nil {
		t.Fatalf("opening %s: %v", name, err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func insertLease(t *testing.T, store *dataplane.Store, id, ip, mac, status string) {
	t.Helper()
	_, err := store.Exec(`
		INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen, generation)
		VALUES (?, 'scope-1', ?, ?, 'host', '', '2026-09-10 10:00:00', '2026-09-11 10:00:00', ?, '2026-09-10 10:00:00', 1)`,
		id, ip, mac, status)
	if err != nil {
		t.Fatalf("inserting lease %s: %v", id, err)
	}
}

func leaseCount(t *testing.T, store *dataplane.Store) int {
	t.Helper()
	var n int
	if err := store.QueryRow(`SELECT COUNT(*) FROM dhcp_leases`).Scan(&n); err != nil {
		t.Fatalf("counting leases: %v", err)
	}
	return n
}

func mirrorHas(t *testing.T, store *dataplane.Store, id, status string) bool {
	t.Helper()
	var got string
	if err := store.QueryRow(`SELECT status FROM dhcp_leases WHERE id = ?`, id).Scan(&got); err != nil {
		return false
	}
	return got == status
}

// waitFor polls a condition. A fixed sleep would either fail on a loaded
// machine or hide a regression behind a generous one.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// startPrimary brings up a primary with its peer port bound.
func startPrimary(t *testing.T, store *dataplane.Store) (*Replicator, context.CancelFunc) {
	t.Helper()
	repl, err := NewReplicator(testConfig(t, "primary"), store)
	if err != nil {
		t.Fatalf("building the primary: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = repl.Run(ctx) }()
	waitFor(t, "the primary to bind its peer port", func() bool { return repl.Addr() != nil })
	return repl, cancel
}

// startMirror brings up a standby pointed at a primary.
func startMirror(t *testing.T, primary *Replicator, name string) (*Mirror, *dataplane.Store, context.CancelFunc) {
	t.Helper()
	store := openStore(t, name)
	cfg := testConfig(t, "standby")
	cfg.NodeID = name
	cfg.PeerAddress = primary.Addr().String()
	mirror, err := NewMirror(cfg, store)
	if err != nil {
		t.Fatalf("building %s: %v", name, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = mirror.Run(ctx) }()
	waitFor(t, "the primary to see "+name, func() bool { return primary.State() == StatePrimary })
	return mirror, store, cancel
}

// startPair brings up a primary and a mirror, and returns them once the primary
// considers itself redundant.
func startPair(t *testing.T) (*Replicator, *dataplane.Store, *dataplane.Store) {
	t.Helper()
	primaryStore := openStore(t, "primary")
	primary, _ := startPrimary(t, primaryStore)
	_, mirrorStore, _ := startMirror(t, primary, "mirror")
	return primary, primaryStore, mirrorStore
}

// TestAPrimaryWithoutItsMirrorMakesNoPromise is the fail-closed rule.
//
// Before the mirror has answered, this node has no second copy. It must refuse
// the acknowledgement, and it must do so immediately rather than after the
// confirm timeout: there is nothing to wait for.
func TestAPrimaryWithoutItsMirrorMakesNoPromise(t *testing.T) {
	store := openStore(t, "primary")
	repl, err := NewReplicator(testConfig(t, "primary"), store)
	if err != nil {
		t.Fatalf("building the primary: %v", err)
	}

	if got := repl.State(); got != StatePaused {
		t.Fatalf("state = %s, want %s: a primary that has never seen its mirror is paused", got, StatePaused)
	}
	if repl.MayBind() {
		t.Fatal("a primary with no mirror reported that it may promise an address")
	}

	start := time.Now()
	err = repl.Confirm(context.Background(), LeaseRow{ID: "l1", ScopeID: "scope-1", IPAddress: "10.0.0.5", Status: "active"})
	elapsed := time.Since(start)
	if !errors.Is(err, ErrNoSecondCopy) {
		t.Fatalf("Confirm = %v, want ErrNoSecondCopy", err)
	}
	if elapsed > testConfirmTimeout/2 {
		t.Errorf("refusing took %s; there is no second copy to wait for, so it must be immediate", elapsed)
	}
}

// TestABindingIsAcknowledgedOnlyAfterTheMirrorHasIt is the property the whole
// package exists for.
//
// The assertion is not "the mirror eventually has the row" -- that would pass
// for any asynchronous copy. It is that the row is in the mirror's own store at
// the moment Confirm returns.
func TestABindingIsAcknowledgedOnlyAfterTheMirrorHasIt(t *testing.T) {
	primary, primaryStore, mirrorStore := startPair(t)

	insertLease(t, primaryStore, "l1", "10.0.0.5", "aa:bb:cc:dd:ee:01", "active")
	if err := primary.Confirm(context.Background(), LeaseRow{
		ID: "l1", ScopeID: "scope-1", IPAddress: "10.0.0.5", MACAddress: "aa:bb:cc:dd:ee:01",
		Hostname: "host", LeaseStart: "2026-09-10 10:00:00", LeaseEnd: "2026-09-11 10:00:00",
		Status: "active", LastSeen: "2026-09-10 10:00:00", Generation: 1,
	}); err != nil {
		t.Fatalf("Confirm on a live pair: %v", err)
	}

	if !mirrorHas(t, mirrorStore, "l1", "active") {
		t.Fatal("Confirm returned before the mirror held the binding")
	}
}

// TestAPromiseIsNotMadeWhenTheMirrorStopsAnswering covers the timeout path.
//
// The peer is connected and its handshake was accepted, so the link looks
// healthy and the node is still willing to promise. What it never sends is a
// confirmation. A client must not be told it holds an address on the strength
// of a healthy-looking socket.
func TestAPromiseIsNotMadeWhenTheMirrorStopsAnswering(t *testing.T) {
	store := openStore(t, "primary")
	primary, _ := startPrimary(t, store)

	// A peer that completes the handshake and then says nothing. It is the
	// mirror's failure mode that is hardest to notice: not a refused
	// connection, which is obvious, but a connected one that never confirms.
	conn, err := net.DialTimeout("tcp", primary.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dialling the primary: %v", err)
	}
	defer conn.Close()
	if err := writeFrame(conn, Frame{
		Type: FrameHello, Protocol: Protocol, Token: testToken,
		Watermarks: &Watermarks{NodeID: "mute-mirror", Role: "standby"},
	}); err != nil {
		t.Fatalf("handshaking: %v", err)
	}
	if _, err := readFrame(conn, maxFrameBytes); err != nil {
		t.Fatalf("reading the primary's handshake: %v", err)
	}
	if _, err := readFrame(conn, maxFrameBytes); err != nil {
		t.Fatalf("reading the snapshot: %v", err)
	}
	waitFor(t, "the primary to regard the link as live", func() bool { return primary.MayBind() })

	start := time.Now()
	err = primary.Confirm(context.Background(), LeaseRow{ID: "l1", ScopeID: "scope-1", IPAddress: "10.0.0.5", Status: "active"})
	elapsed := time.Since(start)
	if !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("Confirm = %v, want ErrNotConfirmed", err)
	}
	if elapsed < testConfirmTimeout {
		t.Errorf("Confirm gave up after %s, before the %s timeout", elapsed, testConfirmTimeout)
	}
	if primary.AckedSeq() >= primary.Seq() {
		t.Errorf("the acknowledged watermark (at %d) caught up with the sequence (%d) without a confirmation",
			primary.AckedSeq(), primary.Seq())
	}
}

// TestAResyncRebuildsTheMirrorFromASnapshot covers the recovery path.
//
// The mirror is brought up after the primary already has rows, which is what
// every reconnect after an outage looks like. Both the snapshot and the
// incremental path have to agree with the primary, and the mirror must not
// drift on its own while it is disconnected.
func TestAResyncRebuildsTheMirrorFromASnapshot(t *testing.T) {
	primaryStore := openStore(t, "primary")
	primary, _ := startPrimary(t, primaryStore)

	_, firstMirrorStore, stopFirstMirror := startMirror(t, primary, "mirror-1")
	insertLease(t, primaryStore, "seen-1", "10.0.0.5", "aa:bb:cc:dd:ee:01", "active")
	if err := primary.Confirm(context.Background(), LeaseRow{
		ID: "seen-1", ScopeID: "scope-1", IPAddress: "10.0.0.5", MACAddress: "aa:bb:cc:dd:ee:01",
		Status: "active", Generation: 1,
	}); err != nil {
		t.Fatalf("Confirm on the first pair: %v", err)
	}

	// The mirror goes away. The primary notices within the staleness bound and
	// stops promising, which is the behaviour the next assertion pins.
	stopFirstMirror()
	waitFor(t, "the primary to notice its mirror is gone", func() bool { return primary.State() == StatePaused })

	// Written while the mirror was down. Only a snapshot can carry these: the
	// primary has no connection over which to send them.
	insertLease(t, primaryStore, "offline-1", "10.0.0.6", "aa:bb:cc:dd:ee:02", "active")
	insertLease(t, primaryStore, "offline-2", "10.0.0.7", "aa:bb:cc:dd:ee:03", "expired")

	_, secondMirrorStore, _ := startMirror(t, primary, "mirror-2")
	waitFor(t, "the second mirror to be rebuilt", func() bool { return leaseCount(t, secondMirrorStore) == 3 })
	if !mirrorHas(t, secondMirrorStore, "seen-1", "active") {
		t.Error("the snapshot did not carry a binding that existed before the outage")
	}
	if !mirrorHas(t, secondMirrorStore, "offline-1", "active") {
		t.Error("the snapshot did not carry a binding written while the mirror was down")
	}
	if !mirrorHas(t, secondMirrorStore, "offline-2", "expired") {
		t.Error("the snapshot did not carry a dead row: the mirror is not a copy, it is a selection by status")
	}

	// A live change now travels the incremental path.
	insertLease(t, primaryStore, "new-1", "10.0.0.8", "aa:bb:cc:dd:ee:04", "active")
	if err := primary.Confirm(context.Background(), LeaseRow{
		ID: "new-1", ScopeID: "scope-1", IPAddress: "10.0.0.8", MACAddress: "aa:bb:cc:dd:ee:04",
		Status: "active", Generation: 1,
	}); err != nil {
		t.Fatalf("Confirm after the resync: %v", err)
	}
	if !mirrorHas(t, secondMirrorStore, "new-1", "active") {
		t.Error("a binding confirmed after the resync is not in the mirror")
	}

	// And nothing reaches the mirror that lost its primary.
	if n := leaseCount(t, firstMirrorStore); n != 1 {
		t.Errorf("the disconnected mirror has %d rows, want the 1 it had when the primary went away", n)
	}
}

// TestTheMirrorRefusesATokenItWasNotGiven covers authentication.
//
// The channel can rewrite lease state and it is reachable from the data
// network. A peer that cannot present the shared token must not be handed the
// lease table.
func TestTheMirrorRefusesATokenItWasNotGiven(t *testing.T) {
	store := openStore(t, "primary")
	primary, _ := startPrimary(t, store)

	conn, err := net.DialTimeout("tcp", primary.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dialling the primary: %v", err)
	}
	defer conn.Close()
	if err := writeFrame(conn, Frame{
		Type: FrameHello, Protocol: Protocol, Token: "not-the-shared-secret",
		Watermarks: &Watermarks{NodeID: "impostor", Role: "standby"},
	}); err != nil {
		t.Fatalf("handshaking: %v", err)
	}

	// The primary closes the connection without answering. A peer that was
	// answered here would be handed the whole lease table.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := readFrame(conn, maxFrameBytes); err == nil {
		t.Fatal("the primary answered a peer that presented the wrong token")
	}
	if primary.MayBind() {
		t.Error("the primary began promising addresses on the strength of an unauthenticated connection")
	}
}

// TestAPeerSpeakingAnotherVersionIsRefused covers the handshake version.
//
// Two builds that disagree about the wire format would otherwise exchange
// frames one of them reads as something else, and the failure would surface as
// a corrupted lease table rather than as a refusal.
func TestAPeerSpeakingAnotherVersionIsRefused(t *testing.T) {
	store := openStore(t, "primary")
	primary, _ := startPrimary(t, store)

	conn, err := net.DialTimeout("tcp", primary.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dialling the primary: %v", err)
	}
	defer conn.Close()
	if err := writeFrame(conn, Frame{
		Type: FrameHello, Protocol: Protocol + 1, Token: testToken,
		Watermarks: &Watermarks{NodeID: "future-node", Role: "standby"},
	}); err != nil {
		t.Fatalf("handshaking: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := readFrame(conn, maxFrameBytes); err == nil {
		t.Fatal("the primary answered a peer speaking another protocol version")
	}
}

// TestAPrimaryWillNotMirrorAnotherPrimary covers the role check from the
// primary's side.
//
// Two primaries exchanging lease facts is the split-brain shape: each would
// conclude the other is a mirror, and neither would stop serving clients.
func TestAPrimaryWillNotMirrorAnotherPrimary(t *testing.T) {
	store := openStore(t, "primary")
	primary, _ := startPrimary(t, store)

	conn, err := net.DialTimeout("tcp", primary.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dialling the primary: %v", err)
	}
	defer conn.Close()
	if err := writeFrame(conn, Frame{
		Type: FrameHello, Protocol: Protocol, Token: testToken,
		Watermarks: &Watermarks{NodeID: "another-primary", Role: "primary"},
	}); err != nil {
		t.Fatalf("handshaking: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := readFrame(conn, maxFrameBytes); err == nil {
		t.Fatal("the primary accepted another primary as its mirror")
	}
	if primary.MayBind() {
		t.Error("the primary began promising addresses to another primary")
	}
}

// TestAMirrorRefusesAPeerThatIsNotAPrimary covers the same rule from the
// mirror's side.
//
// An operator who pointed two standbys at each other would otherwise get a pair
// that both reports healthy and never serves.
func TestAMirrorRefusesAPeerThatIsNotAPrimary(t *testing.T) {
	cfg := testConfig(t, "standby")
	store := openStore(t, "mirror")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	defer ln.Close()

	// A fake peer that claims to be a standby too.
	peerDone := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			peerDone <- err
			return
		}
		defer conn.Close()
		if _, err := readFrame(conn, maxFrameBytes); err != nil {
			peerDone <- err
			return
		}
		peerDone <- writeFrame(conn, Frame{
			Type: FrameHello, Protocol: Protocol,
			Watermarks: &Watermarks{NodeID: "impostor", Role: "standby"},
		})
	}()

	cfg.PeerAddress = ln.Addr().String()
	mirror, err := NewMirror(cfg, store)
	if err != nil {
		t.Fatalf("building the mirror: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if err := mirror.Run(ctx); err != nil {
		t.Fatalf("the mirror's dial loop returned %v, want nil after cancellation", err)
	}
	if err := <-peerDone; err != nil {
		t.Fatalf("the fake peer failed: %v", err)
	}
	if got := mirror.State(); got != StateStandby {
		t.Errorf("state = %s, want %s: a refused handshake must not change what this node is", got, StateStandby)
	}
}

// TestAnOperatorCanApproveRunningAlone covers the degrade marker, and the fact
// that it clears itself.
//
// The approval is the only thing that lets a primary promise addresses without
// a mirror. Everything else follows from that: it is written by an explicit
// operator action, it survives a restart, and it stops applying the moment its
// reason does.
//
// The approval is written through Operator, which is a separate object on
// purpose. An earlier version had a MarkDegraded method on the Replicator
// itself, which put the one fact that must have a single author inside the
// process that serves -- the place an automatic promotion would come from.
func TestAnOperatorCanApproveRunningAlone(t *testing.T) {
	store := openStore(t, "primary")
	cfg := testConfig(t, "primary")
	repl, err := NewReplicator(cfg, store)
	if err != nil {
		t.Fatalf("building the primary: %v", err)
	}
	if repl.MayBind() {
		t.Fatal("a primary with no mirror promised an address before it was degraded")
	}

	op := NewOperator(cfg, store)

	// The gate lives in the mechanism, not in a flag parser: a caller that
	// forgets to ask the operator first is refused here.
	if err := op.Degrade(DegradeOptions{Reason: "drill: replacing the standby host"}); !errors.Is(err, ErrNoConfirmation) {
		t.Fatalf("Degrade without a confirmation = %v, want ErrNoConfirmation", err)
	}
	if repl.Degraded() {
		t.Fatal("an unconfirmed degrade changed the node's state")
	}

	if err := op.Degrade(DegradeOptions{Confirmed: true, Reason: "drill: replacing the standby host"}); err != nil {
		t.Fatalf("degrading: %v", err)
	}

	// The decision belonged to the deployment, not to the process, so a
	// restarted process reads it back.
	restarted, err := NewReplicator(cfg, store)
	if err != nil {
		t.Fatalf("rebuilding the primary: %v", err)
	}
	if got := restarted.State(); got != StatePrimaryDegraded {
		t.Fatalf("state after restart = %s, want %s", got, StatePrimaryDegraded)
	}
	if !restarted.MayBind() {
		t.Fatal("an explicitly degraded primary still refuses to promise an address")
	}
	if got := restarted.DegradedReason(); got != "drill: replacing the standby host" {
		t.Errorf("DegradedReason = %q, want the operator's own words", got)
	}

	// And it clears when the mirror catches up, because the reason for it is
	// gone. A marker that outlives its reason is how a deployment stays
	// degraded forever after one bad afternoon.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = restarted.Run(ctx) }()
	waitFor(t, "the primary to bind its peer port", func() bool { return restarted.Addr() != nil })

	mirrorStore := openStore(t, "mirror")
	mirrorCfg := testConfig(t, "standby")
	mirrorCfg.PeerAddress = restarted.Addr().String()
	mirror, err := NewMirror(mirrorCfg, mirrorStore)
	if err != nil {
		t.Fatalf("building the mirror: %v", err)
	}
	go func() { _ = mirror.Run(ctx) }()

	waitFor(t, "the degraded marker to clear itself", func() bool { return restarted.State() == StatePrimary })
	if restarted.Degraded() {
		t.Error("the degraded marker is still set after the mirror caught up")
	}
	marker, err := store.Meta(metaDegraded)
	if err != nil {
		t.Fatalf("reading the marker: %v", err)
	}
	if marker != "" {
		t.Errorf("the degraded marker is still in the store: %q", marker)
	}
}

// TestTheSequenceSurvivesARestart covers the arithmetic a takeover depends on.
//
// Two writers whose sequence spaces do not join up produce a mirror that treats
// the primary's rows as older than its own, or a primary that accepts a batch
// of rows that go backwards.
func TestTheSequenceSurvivesARestart(t *testing.T) {
	store := openStore(t, "primary")
	cfg := testConfig(t, "primary")

	first, err := NewReplicator(cfg, store)
	if err != nil {
		t.Fatalf("building the primary: %v", err)
	}
	first.Replicate(LeaseRow{ID: "l1", ScopeID: "scope-1", IPAddress: "10.0.0.5", Status: "offered"})
	first.Replicate(LeaseRow{ID: "l1", ScopeID: "scope-1", IPAddress: "10.0.0.5", Status: "active"})
	if first.Seq() != 2 {
		t.Fatalf("seq = %d, want 2", first.Seq())
	}

	second, err := NewReplicator(cfg, store)
	if err != nil {
		t.Fatalf("rebuilding the primary: %v", err)
	}
	if second.Seq() != 2 {
		t.Fatalf("seq after restart = %d, want 2: a counter that restarts from zero hands out numbers the mirror has already seen", second.Seq())
	}
	second.Replicate(LeaseRow{ID: "l2", ScopeID: "scope-1", IPAddress: "10.0.0.6", Status: "offered"})
	if second.Seq() != 3 {
		t.Errorf("seq = %d, want 3: the counter did not continue", second.Seq())
	}
}

// TestAWatermarkThatCannotBeReadIsNotZero covers the reading that would make a
// mirror look current.
//
// Zero means "this node has nothing", which is a legitimate value. A corrupt
// watermark that also reads as zero is indistinguishable from a fresh standby,
// and a takeover decided on that reading would proceed from nothing.
func TestAWatermarkThatCannotBeReadIsNotZero(t *testing.T) {
	store := openStore(t, "mirror")
	if _, err := store.Exec(`INSERT INTO dataplane_meta (key, value) VALUES (?, ?)`, metaAppliedSeq, "not-a-number"); err != nil {
		t.Fatalf("seeding the watermark: %v", err)
	}
	if _, err := NewMirror(testConfig(t, "standby"), store); err == nil {
		t.Fatal("a mirror started with an unreadable watermark")
	}
}

// TestAStandbyOnlyMirrorsAndNeverServes pins the decisions that keep a standby
// from becoming a second author.
func TestAStandbyOnlyMirrorsAndNeverServes(t *testing.T) {
	mirrorStore := openStore(t, "mirror")
	mirror, err := NewMirror(testConfig(t, "standby"), mirrorStore)
	if err != nil {
		t.Fatalf("building the mirror: %v", err)
	}
	if got := mirror.State(); got != StateStandby {
		t.Fatalf("state = %s, want %s", got, StateStandby)
	}
	if mirror.State().MayBind() {
		t.Error("a standby reported that it may promise an address")
	}
	if mirror.State().ServesClients() {
		t.Error("a standby reported that it serves clients")
	}

	// A fenced node answers nothing at all, which is the one state stricter
	// than paused. It is reached only by an operator action, never by the code.
	if err := mirror.Fence(); err != nil {
		t.Fatalf("fencing: %v", err)
	}
	if got := mirror.State(); got != StateFenced {
		t.Fatalf("state after fencing = %s, want %s", got, StateFenced)
	}
	if mirror.State().ServesClients() {
		t.Error("a fenced node reported that it serves clients")
	}
}

// TestPausedNodesStillAnswerWhatTheyCan is the other half of "paused": the node
// keeps running, and only the replies that create or extend a promise are
// withheld.
//
// Going dark entirely would turn a temporary loss of the second copy into every
// client losing sight of its server.
func TestPausedNodesStillAnswerWhatTheyCan(t *testing.T) {
	if !StatePaused.ServesClients() {
		t.Error("a paused node stopped listening; clients would lose sight of their server")
	}
	if StatePaused.MayBind() {
		t.Error("a paused node reported that it may promise an address")
	}
	if StatePaused.Redundant() {
		t.Error("a paused node reported that it has a second copy")
	}
	// Solo is not a degraded state: it is a deployment that never asked for a
	// pair, and it behaves exactly as it did before this contract existed.
	if !StateSolo.MayBind() || !StateSolo.ServesClients() {
		t.Error("a node with HA switched off changed its behaviour")
	}
	if StateFenced.ServesClients() || StateFenced.MayBind() {
		t.Error("a fenced node answers clients")
	}
	if StateStandby.ServesClients() {
		t.Error("a standby answers clients")
	}
	if !StatePrimary.Redundant() || !StatePrimaryDegraded.ServesClients() {
		t.Error("the primary states do not match their descriptions")
	}
}

// TestTheRowOnTheWireCarriesEveryColumn is a guard against the quietest
// possible regression in this package.
//
// A wire type that has drifted from the table it mirrors still compiles, still
// round-trips, and still replicates -- just not the field that was added. The
// check is against the table's own column list, so adding a column without
// adding it here fails rather than going unnoticed.
func TestTheRowOnTheWireCarriesEveryColumn(t *testing.T) {
	store := openStore(t, "primary")

	rows, err := store.Query(`SELECT * FROM dhcp_leases LIMIT 0`)
	if err != nil {
		t.Fatalf("reading the table shape: %v", err)
	}
	defer rows.Close()
	tableColumns, err := rows.Columns()
	if err != nil {
		t.Fatalf("reading the column names: %v", err)
	}

	onTheWire := map[string]bool{}
	for _, c := range leaseColumns {
		onTheWire[c] = true
	}
	for _, c := range tableColumns {
		if !onTheWire[c] {
			t.Errorf("dhcp_leases.%s is not carried by the peer protocol", c)
		}
	}
	if len(leaseColumns) != len(tableColumns) {
		t.Errorf("the protocol carries %d columns, the table has %d: %v",
			len(leaseColumns), len(tableColumns), tableColumns)
	}
}
