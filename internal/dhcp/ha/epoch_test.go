package ha

import (
	"errors"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
)

// TestOwnerEpochTakeoverBumpsMonotonically proves a promotion bumps the
// persisted Owner Epoch and that it is monotonic across successive promotions.
func TestOwnerEpochTakeoverBumpsMonotonically(t *testing.T) {
	store := openStore(t, "epoch-bump")

	if got, err := OwnerEpoch(store); err != nil || got != 0 {
		t.Fatalf("fresh epoch = %d err=%v, want 0", got, err)
	}
	op := NewOperator(testConfig(t, config.HARoleStandby), store)
	out, err := op.Takeover(TakeoverOptions{Confirmed: true, OldPrimaryCannotWrite: true})
	if err != nil {
		t.Fatalf("first takeover: %v", err)
	}
	if out.Epoch != 1 {
		t.Fatalf("first takeover epoch = %d, want 1", out.Epoch)
	}
	if got, _ := OwnerEpoch(store); got != 1 {
		t.Fatalf("persisted epoch = %d, want 1", got)
	}

	// Demote back to standby (as a fresh standby lifecycle would) and promote
	// again; the epoch must increase, never repeat.
	if err := setMetaText(store.DB, metaRole, config.HARoleStandby); err != nil {
		t.Fatal(err)
	}
	out2, err := op.Takeover(TakeoverOptions{Confirmed: true, OldPrimaryCannotWrite: true})
	if err != nil {
		t.Fatalf("second takeover: %v", err)
	}
	if out2.Epoch != 2 {
		t.Fatalf("second takeover epoch = %d, want 2", out2.Epoch)
	}
}

// TestReplicatorSupersededByHigherStandbyEpoch proves a primary that sees a
// higher epoch from its peer stops binding (StateFenced / MayBind=false).
func TestReplicatorSupersededByHigherStandbyEpoch(t *testing.T) {
	store := openStore(t, "repl-supersede")
	r, err := NewReplicator(testConfig(t, config.HARolePrimary), store)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.observeStandbyEpoch(3); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("observeStandbyEpoch(higher) = %v, want ErrStaleEpoch", err)
	}
	if st := r.State(); st != StateFenced {
		t.Fatalf("state = %s, want fenced", st)
	}
	if r.MayBind() {
		t.Fatal("superseded primary still MayBind")
	}
}

// TestReplicatorIgnoresLowerPeerEpoch proves a stale peer epoch cannot fence a
// current primary.
func TestReplicatorIgnoresLowerPeerEpoch(t *testing.T) {
	store := openStore(t, "repl-ignore")
	if err := setMetaText(store.DB, metaOwnerEpoch, "5"); err != nil {
		t.Fatal(err)
	}
	r, err := NewReplicator(testConfig(t, config.HARolePrimary), store)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.observeStandbyEpoch(3); err != nil {
		t.Fatalf("observeStandbyEpoch(lower) = %v, want nil", err)
	}
	if r.State() == StateFenced {
		t.Fatal("current primary wrongly fenced by a lower peer epoch")
	}
}

// TestMirrorEpochRules proves the mirror refuses a stale primary and adopts a
// newer primary epoch as the takeover baseline.
func TestMirrorEpochRules(t *testing.T) {
	store := openStore(t, "mirror-epoch")
	if err := setMetaText(store.DB, metaOwnerEpoch, "5"); err != nil {
		t.Fatal(err)
	}
	m, err := NewMirror(testConfig(t, config.HARoleStandby), store)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.observePrimaryEpoch(3); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("stale primary epoch = %v, want ErrStaleEpoch", err)
	}
	if err := m.observePrimaryEpoch(8); err != nil {
		t.Fatalf("adopt newer epoch: %v", err)
	}
	if got, _ := OwnerEpoch(store); got != 8 {
		t.Fatalf("persisted epoch = %d, want 8", got)
	}
}

// TestStaleEpochFramePersistsFence proves the frame an old primary receives
// from a promoted peer leaves a durable mark: the recorded role becomes
// standby and the newer epoch is persisted, so a restart cannot come back as a
// primary. This is the only channel through which an old primary -- which
// cannot observe the promotion -- learns its write authority is gone.
func TestStaleEpochFramePersistsFence(t *testing.T) {
	store := openStore(t, "stale-frame")
	r, err := NewReplicator(testConfig(t, config.HARolePrimary), store)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.fenceForStaleEpoch(7); err != nil {
		t.Fatalf("fenceForStaleEpoch: %v", err)
	}
	if st := r.State(); st != StateFenced {
		t.Fatalf("state = %s, want fenced", st)
	}
	if r.MayBind() {
		t.Fatal("superseded primary still MayBind")
	}
	if got, err := OwnerEpoch(store); err != nil || got != 7 {
		t.Fatalf("persisted epoch = %d err=%v, want 7", got, err)
	}
	role, err := EffectiveRole(testConfig(t, config.HARolePrimary), store)
	if err != nil {
		t.Fatalf("EffectiveRole: %v", err)
	}
	if role != config.HARoleStandby {
		t.Fatalf("effective role after supersede = %q, want standby", role)
	}
}

// TestRecordSupersededIsDurable proves the startup probe's decision survives a
// restart: the newer epoch and the standby role are both written to the store,
// so a node that was superseded cannot come back as a primary.
func TestRecordSupersededIsDurable(t *testing.T) {
	store := openStore(t, "record-superseded")
	if err := RecordSuperseded(store, 5); err != nil {
		t.Fatalf("RecordSuperseded: %v", err)
	}
	if got, err := OwnerEpoch(store); err != nil || got != 5 {
		t.Fatalf("persisted epoch = %d err=%v, want 5", got, err)
	}
	role, err := EffectiveRole(testConfig(t, config.HARolePrimary), store)
	if err != nil {
		t.Fatalf("EffectiveRole: %v", err)
	}
	if role != config.HARoleStandby {
		t.Fatalf("effective role = %q, want standby (config says primary)", role)
	}
}

// TestPeerAddrsListsEveryConfiguredPath proves the dual-link contract: both
// configured paths to the peer are used, and an empty backup does not add a
// duplicate of the primary path.
func TestPeerAddrsListsEveryConfiguredPath(t *testing.T) {
	c := testConfig(t, config.HARoleStandby)
	if got := c.PeerAddrs(); len(got) != 1 || got[0] != c.PeerAddress {
		t.Fatalf("single-path PeerAddrs = %v, want [%s]", got, c.PeerAddress)
	}
	c.PeerAddressBackup = "10.0.0.9:9953"
	if got := c.PeerAddrs(); len(got) != 2 || got[1] != "10.0.0.9:9953" {
		t.Fatalf("dual-path PeerAddrs = %v, want two entries", got)
	}
	c.PeerAddressBackup = c.PeerAddress
	if got := c.PeerAddrs(); len(got) != 1 {
		t.Fatalf("duplicate backup should collapse to one path, got %v", got)
	}
}

// TestAutoFailoverDefaultWindows proves the threshold default is three and that
// a configured value overrides it.
func TestAutoFailoverDefaultWindows(t *testing.T) {
	c := testConfig(t, config.HARoleStandby)
	if got := c.autoFailoverWindows(); got != 3 {
		t.Fatalf("default windows = %d, want 3", got)
	}
	c.AutoFailoverWindows = 7
	if got := c.autoFailoverWindows(); got != 7 {
		t.Fatalf("configured windows = %d, want 7", got)
	}
}
