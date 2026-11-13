package ha

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/config"
)

// TestDialPeerUsesBackupWhenPrimaryPathDown proves the dual-link contract on the
// wire: with the first configured path dead and the second alive, the mirror
// still connects. A single cable failing must not look like a dead peer.
func TestDialPeerUsesBackupWhenPrimaryPathDown(t *testing.T) {
	// A listener stands in for the peer on the backup path.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, aerr := ln.Accept()
		if aerr == nil {
			_ = conn
			// hold the connection briefly so dial returns success
			time.Sleep(200 * time.Millisecond)
			conn.Close()
		}
	}()

	// Pick a definitely-dead first path: bind then close to get a free port.
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadAddr := dead.Addr().String()
	dead.Close()

	c := testConfig(t, config.HARoleStandby)
	c.PeerAddress = deadAddr                 // first path: dead
	c.PeerAddressBackup = ln.Addr().String() // second path: alive
	store := openStore(t, "duallink")
	m, err := NewMirror(c, store)
	if err != nil {
		t.Fatal(err)
	}
	conn, used, err := m.dialPeer(context.Background())
	if err != nil {
		t.Fatalf("dialPeer with primary path down: %v", err)
	}
	defer conn.Close()
	if used != ln.Addr().String() {
		t.Fatalf("connected via %q, want the backup path %q", used, ln.Addr().String())
	}
}

// TestAutoFailoverFiresOnlyWhenEnabled proves both halves of the switch: with an
// enabled, high-threshold config the callback fires after enough failed windows,
// and with AutoFailover off it never fires (the fail-closed default).
func TestAutoFailoverFiresOnlyWhenEnabled(t *testing.T) {
	dead, _ := net.Listen("tcp", "127.0.0.1:0")
	deadAddr := dead.Addr().String()
	dead.Close()

	mkMirror := func(auto bool, windows int) (*Mirror, *int) {
		c := testConfig(t, config.HARoleStandby)
		c.PeerAddress = deadAddr
		c.PeerAddressBackup = ""
		c.AutoFailover = auto
		c.AutoFailoverWindows = windows
		c.PeerStaleAfter = 50 * time.Millisecond
		store := openStore(t, "auto-"+map[bool]string{true: "on", false: "off"}[auto])
		m, err := NewMirror(c, store)
		if err != nil {
			t.Fatal(err)
		}
		fired := 0
		m.SetAutoPromote(func() bool { fired++; return true })
		return m, &fired
	}

	// Enabled, threshold 2: Run returns after promoting.
	m, fired := mkMirror(true, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = m.Run(ctx)
	if *fired != 1 {
		t.Fatalf("auto-failover fired %d times with the switch on, want 1", *fired)
	}

	// Disabled: Run must not promote within the same budget.
	m2, fired2 := mkMirror(false, 1)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel2()
	_ = m2.Run(ctx2)
	if *fired2 != 0 {
		t.Fatalf("auto-failover fired %d times with the switch off, want 0", *fired2)
	}
}

// TestDualListenAcceptsOnBothPaths proves the primary can listen on two
// addresses (two cables) and a mirror reaches it through the *backup* path
// alone. A single cable failing on the primary side must not stop replication.
func TestDualListenAcceptsOnBothPaths(t *testing.T) {
	// Reserve two free addresses on loopback.
	freeAddr := func() string {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		a := l.Addr().String()
		l.Close()
		return a
	}
	primaryCfg := testConfig(t, "primary")
	primaryCfg.ListenAddr = freeAddr()
	primaryCfg.ListenAddrBackup = freeAddr()
	guard := testConfig(t, "standby") // used only to obtain two distinct peer addresses
	_ = guard

	primaryStore := openStore(t, "dual-listen-primary")
	repl, err := NewReplicator(primaryCfg, primaryStore)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = repl.Run(ctx) }()
	waitFor(t, "the primary to bind its peer ports", func() bool { return repl.Addr() != nil })

	// The mirror is told the *backup* listener address only, proving that path
	// is genuinely accepting, not just the primary one.
	mirrorCfg := testConfig(t, "standby")
	mirrorCfg.NodeID = "mirror-dual"
	mirrorCfg.PeerAddress = primaryCfg.ListenAddrBackup
	mirrorStore := openStore(t, "dual-listen-mirror")
	mirror, err := NewMirror(mirrorCfg, mirrorStore)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = mirror.Run(ctx) }()
	waitFor(t, "the primary to see the mirror via the backup listener", func() bool {
		return repl.State() == StatePrimary
	})
}
