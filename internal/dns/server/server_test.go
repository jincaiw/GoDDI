package server

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
	"github.com/jasonwa/goddi/internal/dns/cache"
	"github.com/jasonwa/goddi/internal/dns/filter"
	"github.com/jasonwa/goddi/internal/dns/forwarder"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/miekg/dns"
)

func newTestZoneStore(t *testing.T, zoneType string, acl *zone.ZoneACL) *zone.Store {
	t.Helper()

	dir := t.TempDir()
	dbh, err := database.New(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(dir, "dns.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbh.Close() })
	if err := dbh.RunMigrations(filepath.Join("..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	var aclJSON interface{}
	if acl != nil {
		aclJSON, err = json.Marshal(acl)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := dbh.Exec(`
		INSERT INTO dns_zones (
			id, name, type, enabled, default_ttl, soa_mname, soa_rname,
			serial, refresh, retry, expire, minimum, acl
		) VALUES (
			'z1', 'example.test.', ?, 1, 300, 'ns1.example.test.',
			'admin.example.test.', 1, 3600, 600, 86400, 300, ?
		)
	`, zoneType, aclJSON); err != nil {
		t.Fatal(err)
	}
	return zone.NewStore(dbh.DB)
}

func TestLookupAuthoritativeDefersForwardAndStubZoneMisses(t *testing.T) {
	for _, zoneType := range []string{string(zone.ZoneTypeForward), string(zone.ZoneTypeStub)} {
		t.Run(zoneType, func(t *testing.T) {
			srv := New(&config.Config{}, nil, nil, nil, nil, nil, newTestZoneStore(t, zoneType, nil))

			resp, denied, found := srv.lookupAuthoritative("missing.example.test.", dns.TypeA, "192.0.2.1")
			if denied {
				t.Fatal("forward/stub zone miss was denied")
			}
			if found || resp != nil {
				t.Fatalf("forward/stub zone miss = (%v, found=%v), want no authoritative response", resp, found)
			}
		})
	}
}

func TestLookupAuthoritativePreservesQueryACLForForwardZone(t *testing.T) {
	srv := New(
		&config.Config{},
		nil,
		nil,
		nil,
		nil,
		nil,
		newTestZoneStore(t, string(zone.ZoneTypeForward), &zone.ZoneACL{QueryAccess: zone.QueryAccessDeny}),
	)

	resp, denied, found := srv.lookupAuthoritative("missing.example.test.", dns.TypeA, "192.0.2.1")
	if !denied || found || resp != nil {
		t.Fatalf("forward zone with denied query ACL = (%v, denied=%v, found=%v), want denied", resp, denied, found)
	}
}

func TestLookupAuthoritativeKeepsPrimaryNegativeAnswer(t *testing.T) {
	srv := New(&config.Config{}, nil, nil, nil, nil, nil, newTestZoneStore(t, string(zone.ZoneTypePrimary), nil))

	resp, denied, found := srv.lookupAuthoritative("missing.example.test.", dns.TypeA, "192.0.2.1")
	if denied || !found {
		t.Fatalf("primary zone miss = (denied=%v, found=%v), want authoritative negative answer", denied, found)
	}
	if resp == nil || resp.Rcode != dns.RcodeNameError || !resp.Authoritative {
		t.Fatalf("primary zone negative response = %#v, want authoritative NXDOMAIN", resp)
	}
}

type memoryResponseWriter struct {
	remote net.Addr
	msg    *dns.Msg
}

func (w *memoryResponseWriter) LocalAddr() net.Addr         { return &net.UDPAddr{} }
func (w *memoryResponseWriter) RemoteAddr() net.Addr        { return w.remote }
func (w *memoryResponseWriter) WriteMsg(msg *dns.Msg) error { w.msg = msg.Copy(); return nil }
func (w *memoryResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *memoryResponseWriter) Close() error                { return nil }
func (w *memoryResponseWriter) TsigStatus() error           { return nil }
func (w *memoryResponseWriter) TsigTimersOnly(bool)         {}
func (w *memoryResponseWriter) Hijack()                     {}

func newCacheSecurityServer(t *testing.T, rebinding bool) (*Server, *cache.Cache) {
	t.Helper()
	cfg := &config.Config{}
	cfg.DNS.Recursion.Enabled = true
	cfg.DNS.Recursion.AllowNets = []string{"0.0.0.0/0", "::/0"}
	dnsCache := cache.New(cache.Config{MaxEntries: 16, MinTTL: 60, MaxTTL: 300})
	fwdGroup := forwarder.NewForwarderGroup(forwarder.StrategySequential, time.Second)
	return New(cfg, dnsCache, filter.NewFilterEngine(rebinding), fwdGroup, nil, nil, nil), dnsCache
}

func serveCachedQuery(srv *Server, qname string) *dns.Msg {
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn(qname), dns.TypeA)
	writer := &memoryResponseWriter{remote: &net.UDPAddr{IP: net.ParseIP("8.8.8.8"), Port: 53000}}
	(&DNSHandler{server: srv}).ServeDNS(writer, req)
	return writer.msg
}

func TestWriteResponseTruncatesUDPWirePayload(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("large.example.", dns.TypeTXT)
	resp := new(dns.Msg)
	resp.SetReply(req)
	for i := 0; i < 8; i++ {
		resp.Answer = append(resp.Answer, &dns.TXT{
			Hdr: dns.RR_Header{Name: "large.example.", Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 60},
			Txt: []string{string(make([]byte, 240))},
		})
	}
	writer := &memoryResponseWriter{remote: &net.UDPAddr{IP: net.ParseIP("192.0.2.1"), Port: 53000}}
	h := &DNSHandler{}
	if err := h.writeResponse(writer, req, resp); err != nil {
		t.Fatalf("writeResponse: %v", err)
	}
	if writer.msg == nil || !writer.msg.Truncated {
		t.Fatal("UDP response was not marked truncated")
	}
	wire, err := writer.msg.Pack()
	if err != nil {
		t.Fatalf("packing truncated response: %v", err)
	}
	if len(wire) > dns.MinMsgSize {
		t.Fatalf("UDP wire length = %d, want <= %d", len(wire), dns.MinMsgSize)
	}
}

func TestCachedResponseAppliesRebindingProtection(t *testing.T) {
	srv, dnsCache := newCacheSecurityServer(t, true)
	cached := new(dns.Msg)
	cached.SetReply(&dns.Msg{Question: []dns.Question{{Name: "rebind.example.", Qtype: dns.TypeA, Qclass: dns.ClassINET}}})
	cached.Answer = []dns.RR{&dns.A{
		Hdr: dns.RR_Header{Name: "rebind.example.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP("192.168.1.1"),
	}}
	dnsCache.Set("rebind.example.", dns.TypeA, cached)

	resp := serveCachedQuery(srv, "rebind.example.")
	if resp == nil || resp.Rcode != dns.RcodeNameError {
		t.Fatalf("cached rebinding response = %#v, want NXDOMAIN", resp)
	}
}

func TestCachedResponseAppliesCNAMECloakingProtection(t *testing.T) {
	srv, dnsCache := newCacheSecurityServer(t, false)
	srv.filter.BlockListMgr.AddList(&filter.BlockList{ID: "trackers", Enabled: true})
	srv.filter.BlockListMgr.AddRule("trackers", filter.MatchRule{
		ID: "tracker-rule", Pattern: "tracker.example", MatchType: "exact", ResponseType: "NXDOMAIN", Enabled: true,
	})
	cached := new(dns.Msg)
	cached.SetReply(&dns.Msg{Question: []dns.Question{{Name: "alias.example.", Qtype: dns.TypeA, Qclass: dns.ClassINET}}})
	cached.Answer = []dns.RR{&dns.CNAME{
		Hdr:    dns.RR_Header{Name: "alias.example.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 60},
		Target: "tracker.example.",
	}}
	dnsCache.Set("alias.example.", dns.TypeA, cached)

	resp := serveCachedQuery(srv, "alias.example.")
	if resp == nil || resp.Rcode != dns.RcodeNameError {
		t.Fatalf("cached cloaking response = %#v, want NXDOMAIN", resp)
	}
}

func TestPrefetchRejectsUnsafeResponse(t *testing.T) {
	srv, dnsCache := newCacheSecurityServer(t, true)
	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening upstream: %v", err)
	}
	upstream := &dns.Server{PacketConn: packetConn, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(req)
		resp.Answer = []dns.RR{&dns.A{
			Hdr: dns.RR_Header{Name: req.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
			A:   net.ParseIP("192.168.1.1"),
		}}
		_ = w.WriteMsg(resp)
	})}
	go func() { _ = upstream.ActivateAndServe() }()
	t.Cleanup(func() { _ = upstream.Shutdown() })

	srv.forwarder.SetForwarders([]*forwarder.Forwarder{{
		ID: "test", Protocol: "udp", Address: packetConn.LocalAddr().String(), Enabled: true,
	}})
	_ = srv.Prefetch("rebind.example.", dns.TypeA)
	if _, hit, _ := dnsCache.Get("rebind.example.", dns.TypeA); hit {
		t.Fatal("prefetch cached a rebinding response")
	}
}

func TestPrefetchRejectsCNAMECloakingResponse(t *testing.T) {
	srv, dnsCache := newCacheSecurityServer(t, false)
	srv.filter.BlockListMgr.AddList(&filter.BlockList{ID: "trackers", Enabled: true})
	srv.filter.BlockListMgr.AddRule("trackers", filter.MatchRule{
		ID: "tracker-rule", Pattern: "tracker.example", MatchType: "exact", ResponseType: "NXDOMAIN", Enabled: true,
	})
	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening upstream: %v", err)
	}
	upstream := &dns.Server{PacketConn: packetConn, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(req)
		resp.Answer = []dns.RR{&dns.CNAME{
			Hdr:    dns.RR_Header{Name: req.Question[0].Name, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 60},
			Target: "tracker.example.",
		}}
		_ = w.WriteMsg(resp)
	})}
	go func() { _ = upstream.ActivateAndServe() }()
	t.Cleanup(func() { _ = upstream.Shutdown() })

	srv.forwarder.SetForwarders([]*forwarder.Forwarder{{
		ID: "test", Protocol: "udp", Address: packetConn.LocalAddr().String(), Enabled: true,
	}})
	_ = srv.Prefetch("alias.example.", dns.TypeA)
	if _, hit, _ := dnsCache.Get("alias.example.", dns.TypeA); hit {
		t.Fatal("prefetch cached a CNAME cloaking response")
	}
}

func TestStartReturnsBoundListenerErrors(t *testing.T) {
	for _, listener := range []struct {
		name    string
		network string
		apply   func(*config.Config, string)
	}{
		{name: "udp", network: "udp", apply: func(cfg *config.Config, addr string) {
			cfg.DNS.Listeners.UDP.Enabled, cfg.DNS.Listeners.UDP.Address = true, addr
		}},
		{name: "tcp", network: "tcp", apply: func(cfg *config.Config, addr string) {
			cfg.DNS.Listeners.TCP.Enabled, cfg.DNS.Listeners.TCP.Address = true, addr
		}},
	} {
		t.Run(listener.name, func(t *testing.T) {
			var addr string
			if listener.network == "udp" {
				conn, err := net.ListenPacket("udp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				addr = conn.LocalAddr().String()
			} else {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer ln.Close()
				addr = ln.Addr().String()
			}

			cfg := &config.Config{}
			listener.apply(cfg, addr)
			srv := New(cfg, nil, filter.NewFilterEngine(false), forwarder.NewForwarderGroup(forwarder.StrategySequential, time.Second), nil, nil, nil)
			if err := srv.Start(context.Background()); err == nil {
				t.Fatal("Start() error = nil, want listener bind error")
			}
			if srv.IsRunning() {
				t.Fatal("server marked running after listener bind error")
			}
		})
	}
}

func TestResolveSharedForwardCoalescesEquivalentQueries(t *testing.T) {
	var calls atomic.Int32
	upstream := &forwarder.Forwarder{ID: "test", Name: "test", Protocol: "udp", Address: "127.0.0.1:0", Enabled: true}
	upstream.SetHealthy(true)
	srv := New(&config.Config{}, nil, nil, forwarder.NewForwarderGroup(forwarder.StrategySequential, time.Second), nil, nil, nil)

	// Install a local forwarding function by using a completed inflight entry
	// is not sufficient to exercise the leader. Instead, use a replacement
	// forwarder group with an unexported transport-free path through a valid
	// single response served by a local UDP DNS listener.
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	mux := dns.NewServeMux()
	mux.HandleFunc(".", func(w dns.ResponseWriter, req *dns.Msg) {
		calls.Add(1)
		time.Sleep(80 * time.Millisecond)
		resp := new(dns.Msg)
		resp.SetReply(req)
		_ = w.WriteMsg(resp)
	})
	dnsSrv := &dns.Server{PacketConn: conn, Handler: mux}
	go func() { _ = dnsSrv.ActivateAndServe() }()
	defer dnsSrv.Shutdown()

	upstream.Address = conn.LocalAddr().String()
	group := forwarder.NewForwarderGroup(forwarder.StrategySequential, time.Second)
	group.SetForwarders([]*forwarder.Forwarder{upstream})
	srv.forwarder = group

	base := new(dns.Msg)
	base.SetQuestion("coalesce.example.", dns.TypeA)
	base.RecursionDesired = true
	first := base.Copy()
	first.Id = 100
	second := base.Copy()
	second.Id = 200

	results := make(chan *dns.Msg, 2)
	errs := make(chan error, 2)
	for _, msg := range []*dns.Msg{first, second} {
		go func(query *dns.Msg) {
			resp, _, _, err := srv.resolveSharedForward(context.Background(), query)
			if err == nil {
				results <- resp
			}
			errs <- err
		}(msg)
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("resolveSharedForward: %v", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
	for range 2 {
		resp := <-results
		if resp == nil || len(resp.Question) != 1 || resp.Question[0].Name != "coalesce.example." {
			t.Fatalf("invalid shared response: %#v", resp)
		}
	}
}

func TestResolveSharedForwardCallerCancellationDoesNotAbortLeader(t *testing.T) {
	started := make(chan struct{})
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	mux := dns.NewServeMux()
	mux.HandleFunc(".", func(w dns.ResponseWriter, req *dns.Msg) {
		close(started)
		time.Sleep(100 * time.Millisecond)
		resp := new(dns.Msg)
		resp.SetReply(req)
		_ = w.WriteMsg(resp)
	})
	dnsSrv := &dns.Server{PacketConn: conn, Handler: mux}
	go func() { _ = dnsSrv.ActivateAndServe() }()
	defer dnsSrv.Shutdown()

	fwd := &forwarder.Forwarder{ID: "test", Name: "test", Protocol: "udp", Address: conn.LocalAddr().String(), Enabled: true}
	fwd.SetHealthy(true)
	group := forwarder.NewForwarderGroup(forwarder.StrategySequential, time.Second)
	group.SetForwarders([]*forwarder.Forwarder{fwd})
	srv := New(&config.Config{}, nil, nil, group, nil, nil, nil)
	query := new(dns.Msg)
	query.SetQuestion("cancel.example.", dns.TypeA)

	cancelCtx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, _, _, err := srv.resolveSharedForward(cancelCtx, query)
		firstDone <- err
	}()
	<-started
	cancel()
	if err := <-firstDone; err == nil {
		t.Fatal("cancelled waiter error = nil")
	}

	resp, _, _, err := srv.resolveSharedForward(context.Background(), query)
	if err != nil {
		t.Fatalf("remaining waiter lost shared query: %v", err)
	}
	if resp == nil {
		t.Fatal("remaining waiter response = nil")
	}
}

func TestStartReturnsTLSListenerConfigurationErrors(t *testing.T) {
	for _, listener := range []string{"dot", "doh", "doq"} {
		t.Run(listener, func(t *testing.T) {
			cfg := &config.Config{}
			switch listener {
			case "dot":
				cfg.DNS.Listeners.DOT.Enabled = true
			case "doh":
				cfg.DNS.Listeners.DOH.Enabled = true
			case "doq":
				cfg.DNS.Listeners.DOQ.Enabled = true
			}
			srv := New(cfg, nil, filter.NewFilterEngine(false), forwarder.NewForwarderGroup(forwarder.StrategySequential, time.Second), nil, nil, nil)

			if err := srv.Start(context.Background()); err == nil {
				t.Fatal("Start() error = nil, want TLS configuration error")
			}
			if srv.IsRunning() {
				t.Fatal("server marked running after TLS configuration error")
			}
		})
	}
}
