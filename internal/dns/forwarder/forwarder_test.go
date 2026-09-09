package forwarder

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestHealthCheckJitterIsBounded(t *testing.T) {
	interval := 10 * time.Second
	for range 100 {
		delay := healthCheckJitter(interval)
		if delay < interval || delay > 11*time.Second {
			t.Fatalf("health-check delay = %v, want within [%v, %v]", delay, interval, 11*time.Second)
		}
	}
	if got := healthCheckJitter(0); got != 0 {
		t.Fatalf("zero interval jitter = %v, want 0", got)
	}
}

func TestHealthCheckSchedulerHonorsCancellationBeforeProbe(t *testing.T) {
	fg := NewForwarderGroup(StrategySequential, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	fg.runHealthChecks(ctx, time.Hour, func(time.Duration) time.Duration { return time.Hour })
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cancelled health-check scheduler took %v", elapsed)
	}
}

func TestZoneForwarderReusesStateAndPrunesRemovedTargets(t *testing.T) {
	fg := NewForwarderGroup(StrategySequential, time.Second)
	address := "192.0.2.53:53"

	first := fg.zoneForwarder(address)
	first.recordFailure()
	second := fg.zoneForwarder(address)
	if first != second {
		t.Fatal("zone forwarder was recreated instead of reused")
	}
	if got := second.consecutiveFails.Load(); got != 1 {
		t.Fatalf("consecutive failures = %d, want 1", got)
	}

	first.lastUsedUnix.Store(0)
	fg.PruneZoneForwarders(time.Minute)
	third := fg.zoneForwarder(address)
	if third == first {
		t.Fatal("pruned zone forwarder remained cached")
	}
	if got := third.consecutiveFails.Load(); got != 0 {
		t.Fatalf("pruned replacement failures = %d, want 0", got)
	}
}

func TestForwardersOrderedByPriority(t *testing.T) {
	fg := NewForwarderGroup(StrategySequential, 0)
	fg.SetForwarders([]*Forwarder{
		{ID: "last", Priority: 100},
		{ID: "first", Priority: 10},
		{ID: "same-first", Priority: 20},
		{ID: "same-second", Priority: 20},
	})

	got := fg.GetForwarders()
	want := []string{"first", "same-first", "same-second", "last"}
	if len(got) != len(want) {
		t.Fatalf("forwarder count = %d, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("forwarder at %d = %q, want %q", i, got[i].ID, id)
		}
	}
}

func TestAddForwarderMaintainsPriorityOrder(t *testing.T) {
	fg := NewForwarderGroup(StrategySequential, 0)
	for _, fwd := range []*Forwarder{
		{ID: "high", Address: "8.8.8.8:53", Enabled: true, Priority: 100},
		{ID: "low", Address: "1.1.1.1:53", Enabled: true, Priority: 1},
	} {
		if err := fg.AddForwarder(fwd); err != nil {
			t.Fatalf("AddForwarder(%s): %v", fwd.ID, err)
		}
	}

	got := fg.GetForwarders()
	if got[0].ID != "low" || got[1].ID != "high" {
		t.Fatalf("priorities not ordered: %q, %q", got[0].ID, got[1].ID)
	}
}

func TestSequentialFallsBackAfterServfail(t *testing.T) {
	servfailAddr, closeServfail := startForwarderTestServer(t, dns.RcodeServerFailure)
	defer closeServfail()
	successAddr, closeSuccess := startForwarderTestServer(t, dns.RcodeSuccess)
	defer closeSuccess()

	fg := NewForwarderGroup(StrategySequential, time.Second)
	fg.SetForwarders([]*Forwarder{
		newTestForwarder("servfail", servfailAddr, 1),
		newTestForwarder("success", successAddr, 1),
	})
	query := new(dns.Msg)
	query.SetQuestion("example.org.", dns.TypeA)

	resp, selected, _, err := fg.Forward(context.Background(), query)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess || selected.ID != "success" {
		t.Fatalf("selected response = %s from %q, want success from fallback", dns.RcodeToString[resp.Rcode], selected.ID)
	}
	if got := fg.GetForwarders()[0].consecutiveFails.Load(); got != 1 {
		t.Fatalf("SERVFAIL failure count = %d, want 1", got)
	}
}

func TestSequentialKeepsNXDOMAINTerminal(t *testing.T) {
	nxdomainAddr, closeNXDOMAIN := startForwarderTestServer(t, dns.RcodeNameError)
	defer closeNXDOMAIN()
	successAddr, closeSuccess := startForwarderTestServer(t, dns.RcodeSuccess)
	defer closeSuccess()

	fg := NewForwarderGroup(StrategySequential, time.Second)
	fg.SetForwarders([]*Forwarder{
		newTestForwarder("nxdomain", nxdomainAddr, 1),
		newTestForwarder("success", successAddr, 1),
	})
	query := new(dns.Msg)
	query.SetQuestion("missing.example.", dns.TypeA)

	resp, selected, _, err := fg.Forward(context.Background(), query)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if resp.Rcode != dns.RcodeNameError || selected.ID != "nxdomain" {
		t.Fatalf("selected response = %s from %q, want terminal NXDOMAIN", dns.RcodeToString[resp.Rcode], selected.ID)
	}
	if got := fg.GetForwarders()[0].consecutiveFails.Load(); got != 0 {
		t.Fatalf("NXDOMAIN failure count = %d, want 0", got)
	}
}

func startForwarderTestServer(t *testing.T, rcode int) (string, func()) {
	t.Helper()
	mux := dns.NewServeMux()
	mux.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(r)
		resp.Rcode = rcode
		_ = w.WriteMsg(resp)
	})
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen UDP: %v", err)
	}
	srv := &dns.Server{PacketConn: conn, Handler: mux}
	go func() { _ = srv.ActivateAndServe() }()
	return conn.LocalAddr().String(), func() { _ = srv.Shutdown() }
}

func newTestForwarder(id, address string, priority int) *Forwarder {
	f := &Forwarder{ID: id, Name: id, Protocol: "udp", Address: address, Enabled: true, Priority: priority}
	f.healthy.Store(true)
	return f
}
