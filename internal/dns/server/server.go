package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jasonwa/goddi/internal/config"
	dnsquerylog "github.com/jasonwa/goddi/internal/dns"
	"github.com/jasonwa/goddi/internal/dns/cache"
	"github.com/jasonwa/goddi/internal/dns/dynamic_update"
	"github.com/jasonwa/goddi/internal/dns/filter"
	"github.com/jasonwa/goddi/internal/dns/forwarder"
	"github.com/jasonwa/goddi/internal/dns/transfer"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/jasonwa/goddi/internal/metrics"
	"github.com/miekg/dns"
)

// ZoneData represents an authoritative DNS zone in memory.
type ZoneData struct {
	Name    string
	Records map[string][]dns.RR
}

// Server is the core DNS server.
type Server struct {
	cfg         *config.Config
	cache       *cache.Cache
	filter      *filter.FilterEngine
	forwarder   *forwarder.ForwarderGroup
	conditional *forwarder.ConditionalForwarderManager
	queryLog    *dnsquerylog.QueryLogger
	zoneStore   *zone.Store

	// Optional modules wired by the bootstrap.
	rateLimiter   *RateLimiter
	axfrHandler   *transfer.AXFRHandler
	updateHandler *dynamic_update.UpdateHandler
	// notifyHandler is invoked for inbound NOTIFY (RFC 1996) messages.
	notifyHandler func(zoneName string)

	// notifyGuard bounds NOTIFY-triggered refreshes: at most
	// notifyMaxConcurrent refreshes run at once and the same zone is only
	// refreshed once per notifySuppress window.
	notifyMu        sync.Mutex
	notifyInflight  int
	notifyLastStart map[string]time.Time

	udpServer *dns.Server
	tcpServer *dns.Server
	dotServer *dns.Server
	dohServer *DoHServer
	doqServer *DoQServer
	handler   dns.Handler

	// listenerMu serializes encrypted-listener configuration updates and
	// restarts (SetListenerConfig / RestartListener).
	listenerMu sync.Mutex

	mu      sync.RWMutex
	running bool
	zones   map[string]*ZoneData // In-memory authoritative zones (legacy, kept for compatibility)

	// Recursion ACL.
	recursionMu   sync.RWMutex
	recursionNets []*net.IPNet

	// EDNS Client Subnet (RFC 7871) forwarding policy.
	ecsMu         sync.RWMutex
	ecsMode       forwarder.ECSMode
	ecsIPv4Prefix int
	ecsIPv6Prefix int

	// RFC 6303/6761 locally served zones switch (default true).
	specialMu    sync.RWMutex
	specialZones bool

	// TSIG keys (RFC 8945) applied to TCP/DoT listeners for signed
	// AXFR/IXFR/NOTIFY transactions.
	tsigMu      sync.Mutex
	tsigSecrets map[string]string

	// inflight coalesces equivalent cache-miss forwarding work. It is kept at
	// the server boundary rather than in the upstream package because this is
	// where the fully prepared request and selected routing policy meet.
	inflightMu    sync.Mutex
	inflight      map[string]*inflightQuery
	inflightLimit int
}

const defaultInflightLimit = 1024

// ErrInflightLimitReached is returned when a distinct cache-miss query cannot
// enter the bounded shared-forwarding table. Callers may safely fall back to a
// regular bounded forward attempt rather than allocating another goroutine.
var ErrInflightLimitReached = errors.New("DNS shared-forwarding capacity reached")

type inflightQuery struct {
	done      chan struct{}
	response  *dns.Msg
	forwarder *forwarder.Forwarder
	duration  time.Duration
	err       error
}

// New creates a new DNS server.
func New(
	cfg *config.Config,
	dnsCache *cache.Cache,
	filterEngine *filter.FilterEngine,
	fwdGroup *forwarder.ForwarderGroup,
	condManager *forwarder.ConditionalForwarderManager,
	queryLog *dnsquerylog.QueryLogger,
	zoneStore *zone.Store,
) *Server {
	s := &Server{
		cfg:           cfg,
		cache:         dnsCache,
		filter:        filterEngine,
		forwarder:     fwdGroup,
		conditional:   condManager,
		queryLog:      queryLog,
		zoneStore:     zoneStore,
		zones:         make(map[string]*ZoneData),
		inflight:      make(map[string]*inflightQuery),
		inflightLimit: defaultInflightLimit,

		// ECS defaults to the privacy-preserving strip policy.
		ecsMode:       forwarder.ECSStrip,
		ecsIPv4Prefix: forwarder.DefaultECSIPv4Prefix,
		ecsIPv6Prefix: forwarder.DefaultECSIPv6Prefix,

		// Locally served zones on by default.
		specialZones: true,
	}

	// Parse recursion ACL networks.
	s.parseRecursionACL()

	return s
}

// parseRecursionACL parses the recursion allow networks from config.
func (s *Server) parseRecursionACL() {
	nets := s.cfg.DNS.Recursion.AllowNets
	if len(nets) == 0 {
		// Default: only private networks.
		defaultNets := []string{
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"fc00::/7",
		}
		nets = defaultNets
	}

	parsed := make([]*net.IPNet, 0, len(nets))
	for _, cidr := range nets {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			slog.Error("dns_server: invalid recursion ACL CIDR", "cidr", cidr, "error", err)
			continue
		}
		parsed = append(parsed, ipNet)
	}

	s.recursionMu.Lock()
	s.recursionNets = parsed
	s.recursionMu.Unlock()
}

// SetRecursionEnabled hot-updates the recursion master switch.
// LogExternalQuery records a query executed by an API diagnostic client. Such
// queries bypass the DNS listener handler but belong in the same operator log.
func (s *Server) LogExternalQuery(name string, qtype uint16, rcode, upstream string, duration time.Duration, blocked bool) {
	if s.queryLog == nil {
		return
	}
	clientIP := "api"
	clientPort := 0
	protocol := "API"
	s.queryLog.Log(dnsquerylog.QueryLogEntry{
		ClientIP:       clientIP,
		ClientPort:     clientPort,
		Protocol:       protocol,
		QueryName:      dns.Fqdn(name),
		QueryType:      dns.TypeToString[qtype],
		ResponseCode:   rcode,
		ResponseTimeMs: float64(duration.Microseconds()) / 1000,
		Upstream:       upstream,
		Cached:         false,
		Blocked:        blocked,
	})
}

func (s *Server) SetRecursionEnabled(enabled bool) {
	s.cfg.DNS.Recursion.Enabled = enabled
	slog.Info("dns_server: recursion", "enabled", enabled)
}

// SetRateLimiter attaches a query rate limiter.
func (s *Server) SetRateLimiter(rl *RateLimiter) {
	s.rateLimiter = rl
}

// SetAXFRHandler attaches the zone transfer (AXFR/IXFR) handler.
func (s *Server) SetAXFRHandler(h *transfer.AXFRHandler) {
	s.axfrHandler = h
}

// SetUpdateHandler attaches the RFC 2136 dynamic update handler.
func (s *Server) SetUpdateHandler(h *dynamic_update.UpdateHandler) {
	s.updateHandler = h
}

// SetNotifyHandler attaches the inbound NOTIFY (RFC 1996) handler, which
// typically triggers a secondary zone refresh.
func (s *Server) SetNotifyHandler(fn func(zoneName string)) {
	s.notifyMu.Lock()
	s.notifyHandler = fn
	if s.notifyLastStart == nil {
		s.notifyLastStart = make(map[string]time.Time)
	}
	s.notifyMu.Unlock()
}

const (
	// notifyMaxConcurrent caps concurrent NOTIFY-triggered refreshes.
	notifyMaxConcurrent = 2
	// notifySuppress drops repeat NOTIFYs for the same zone in this window.
	notifySuppress = 10 * time.Second
)

// scheduleNotifyRefresh runs the NOTIFY refresh for zoneName asynchronously,
// subject to a concurrency cap and per-zone suppression.
func (s *Server) scheduleNotifyRefresh(zoneName string) {
	zone := dns.Fqdn(strings.ToLower(zoneName))

	s.notifyMu.Lock()
	if s.notifyLastStart == nil {
		s.notifyLastStart = make(map[string]time.Time)
	}
	now := time.Now()
	if last, ok := s.notifyLastStart[zone]; ok && now.Sub(last) < notifySuppress {
		s.notifyMu.Unlock()
		return
	}
	if s.notifyInflight >= notifyMaxConcurrent {
		s.notifyMu.Unlock()
		slog.Debug("dns_server: dropping NOTIFY refresh, too many in flight", "zone", zone)
		return
	}
	s.notifyInflight++
	s.notifyLastStart[zone] = now
	handler := s.notifyHandler
	s.notifyMu.Unlock()

	if handler == nil {
		s.notifyMu.Lock()
		s.notifyInflight--
		s.notifyMu.Unlock()
		return
	}

	go func() {
		defer func() {
			s.notifyMu.Lock()
			s.notifyInflight--
			s.notifyMu.Unlock()
		}()
		handler(zone)
	}()
}

// SetECSConfig hot-updates the EDNS Client Subnet (RFC 7871) forwarding
// policy. mode is one of "strip" (default), "passthrough", or "add"; the
// prefix lengths apply only to the "add" mode and are sanitized to their
// valid ranges.
func (s *Server) SetECSConfig(mode string, ipv4Prefix, ipv6Prefix int) {
	m := forwarder.ParseECSMode(mode)
	if ipv4Prefix < 0 || ipv4Prefix > 32 {
		ipv4Prefix = forwarder.DefaultECSIPv4Prefix
	}
	if ipv6Prefix < 0 || ipv6Prefix > 128 {
		ipv6Prefix = forwarder.DefaultECSIPv6Prefix
	}
	s.ecsMu.Lock()
	s.ecsMode = m
	s.ecsIPv4Prefix = ipv4Prefix
	s.ecsIPv6Prefix = ipv6Prefix
	s.ecsMu.Unlock()
	slog.Info("dns_server: ECS policy updated", "mode", m, "ipv4_prefix", ipv4Prefix, "ipv6_prefix", ipv6Prefix)
}

// ecsCacheBypass reports whether responses must bypass the answer cache.
// When ECS data is forwarded upstream (passthrough/add), the cached answer
// would be tied to one client topology but served to others, so caching is
// disabled for correctness. In strip mode the forwarded query carries no
// ECS and the shared cache is safe.
func (s *Server) ecsCacheBypass() bool {
	s.ecsMu.RLock()
	defer s.ecsMu.RUnlock()
	return s.ecsMode != forwarder.ECSStrip
}

// PrepareUpstreamMsg returns the message to send upstream, applying the
// configured ECS policy. It works on a copy so the client's original query
// (used for response correlation and logging) is never mutated.
func (s *Server) PrepareUpstreamMsg(req *dns.Msg, clientIP net.IP) *dns.Msg {
	s.ecsMu.RLock()
	mode, v4, v6 := s.ecsMode, s.ecsIPv4Prefix, s.ecsIPv6Prefix
	s.ecsMu.RUnlock()

	switch mode {
	case forwarder.ECSPassthrough:
		// Forward the client's ECS option unchanged; nothing to do.
		return req
	case forwarder.ECSAdd:
		up := req.Copy()
		forwarder.InjectECS(up, clientIP, v4, v6)
		return up
	default: // ECSStrip
		if !forwarder.HasECS(req) {
			return req
		}
		up := req.Copy()
		forwarder.StripECS(up)
		return up
	}
}

// isRecursionAllowed checks if a client IP is allowed to use recursive resolution.
func (s *Server) isRecursionAllowed(clientIP net.IP) bool {
	if !s.cfg.DNS.Recursion.Enabled {
		return false
	}

	s.recursionMu.RLock()
	defer s.recursionMu.RUnlock()
	for _, ipNet := range s.recursionNets {
		if ipNet.Contains(clientIP) {
			return true
		}
	}
	return false
}

// Prefetch refreshes one cache entry in the background. It is wired as
// the cache's prefetch callback: when a hot entry's remaining TTL drops
// below the prefetch threshold, the cache calls this to re-resolve the
// name upstream and refresh the entry without blocking the client.
func (s *Server) Prefetch(qname string, qtype uint16) error {
	if s.cache == nil {
		return nil
	}
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(qname), qtype)
	msg.RecursionDesired = true

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	resp, _, _, err := s.resolveForward(ctx, s.PrepareUpstreamMsg(msg, nil))
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("prefetch %s/%d: nil upstream response", qname, qtype)
	}

	// Prefetch has no client context. Use a public sentinel so rebinding
	// protection rejects private-address responses before they can enter the
	// shared cache; client-specific enforcement still runs on every hit.
	if responseBlockReason(s.filter, resp, "8.8.8.8") != "" {
		return fmt.Errorf("prefetch %s/%d: response rejected by policy", qname, qtype)
	}
	s.cache.Set(qname, qtype, resp)
	return nil
}

// AddEDE attaches an Extended DNS Error (RFC 8914) option to the message's
// OPT record, creating the OPT if needed. Best-effort: failures are logged
// at debug level and never abort the response.
func AddEDE(m *dns.Msg, infoCode uint16, extraText string) {
	opt := m.IsEdns0()
	if opt == nil {
		m.SetEdns0(1232, false)
		opt = m.IsEdns0()
		if opt == nil {
			return
		}
	}
	for _, o := range opt.Option {
		if e, ok := o.(*dns.EDNS0_EDE); ok && e.InfoCode == infoCode {
			return // already present; do not duplicate
		}
	}
	opt.Option = append(opt.Option, &dns.EDNS0_EDE{
		InfoCode:  infoCode,
		ExtraText: extraText,
	})
}

// EDE info codes used by GoDDI (RFC 8914 registry subset).
const (
	EDENetworkError         = 10 // Network Error
	EDENoReachableAuthority = 11 // No Reachable Authority
	EDEBlocked              = 15 // Blocked
	EDECensored             = 16 // Censored
	EDEFilteredPolicy       = 17 // Filtered Policy
)

// specialZonesEnabled reports whether RFC 6303/6761 locally served zones
// are active (default true, toggled via the dns_special_zones setting).
func (s *Server) specialZonesEnabled() bool {
	s.specialMu.RLock()
	defer s.specialMu.RUnlock()
	return s.specialZones
}

// SetSpecialZonesEnabled hot-updates the locally served zones switch.
func (s *Server) SetSpecialZonesEnabled(enabled bool) {
	s.specialMu.Lock()
	s.specialZones = enabled
	s.specialMu.Unlock()
	slog.Info("dns_server: locally served zones", "enabled", enabled)
}

// SetTSIGSecretMap installs the RFC 8945 key map used to verify inbound
// signed requests and sign responses on TCP-based listeners. Passing nil
// disables TSIG. Takes effect for listeners (re)started afterwards.
func (s *Server) SetTSIGSecretMap(secrets map[string]string) {
	s.tsigMu.Lock()
	s.tsigSecrets = secrets
	s.tsigMu.Unlock()
}

// tsigSnapshot returns a copy of the current TSIG secret map for listener
// construction. Must not be called concurrently with SetTSIGSecretMap from
// the same goroutine flow; Start serializes listener setup through
// listenerMu before calling startDoT/startDoH.
func (s *Server) tsigSnapshot() map[string]string {
	s.tsigMu.Lock()
	defer s.tsigMu.Unlock()
	if s.tsigSecrets == nil {
		return nil
	}
	out := make(map[string]string, len(s.tsigSecrets))
	for k, v := range s.tsigSecrets {
		out[k] = v
	}
	return out
}

// Start binds every enabled listener before serving requests. This makes a
// successful return a readiness guarantee: no configured listener can fail its
// initial bind asynchronously after the process has announced itself running.
func (s *Server) Start(ctx context.Context) (err error) {
	if err := s.validateTLSListenerConfigs(); err != nil {
		return err
	}

	handler := &DNSHandler{server: s}
	s.handler = handler

	s.tsigMu.Lock()
	tsigSecrets := s.tsigSecrets
	s.tsigMu.Unlock()

	var udpConn net.PacketConn
	var tcpListener, dotListener net.Listener
	var dohListener net.Listener
	var doqConn net.PacketConn
	cleanupBound := func() {
		for _, closer := range []io.Closer{udpConn, tcpListener, dotListener, dohListener, doqConn} {
			if closer != nil {
				_ = closer.Close()
			}
		}
	}
	defer func() {
		if err == nil {
			return
		}
		cleanupBound()
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if s.dotServer != nil {
			_ = s.dotServer.ShutdownContext(rollbackCtx)
			s.dotServer = nil
		}
		if s.dohServer != nil {
			_ = s.dohServer.Shutdown(rollbackCtx)
			s.dohServer = nil
		}
		if s.doqServer != nil {
			_ = s.doqServer.Shutdown()
			s.doqServer = nil
		}
		s.udpServer = nil
		s.tcpServer = nil
		s.handler = nil
	}()

	if s.cfg.DNS.Listeners.UDP.Enabled {
		udpConn, err = net.ListenPacket("udp", s.cfg.DNS.Listeners.UDP.Address)
		if err != nil {
			return fmt.Errorf("binding UDP listener %s: %w", s.cfg.DNS.Listeners.UDP.Address, err)
		}
		s.udpServer = &dns.Server{PacketConn: udpConn, Handler: handler}
	}
	if s.cfg.DNS.Listeners.TCP.Enabled {
		tcpListener, err = net.Listen("tcp", s.cfg.DNS.Listeners.TCP.Address)
		if err != nil {
			return fmt.Errorf("binding TCP listener %s: %w", s.cfg.DNS.Listeners.TCP.Address, err)
		}
		s.tcpServer = &dns.Server{Listener: tcpListener, Handler: handler, TsigSecret: tsigSecrets}
	}
	if s.cfg.DNS.Listeners.DOT.Enabled {
		dotListener, err = net.Listen("tcp", s.cfg.DNS.Listeners.DOT.Address)
		if err != nil {
			return fmt.Errorf("binding DoT listener %s: %w", s.cfg.DNS.Listeners.DOT.Address, err)
		}
		if err := s.startDoTWithListener(handler, dotListener); err != nil {
			return fmt.Errorf("starting DoT listener: %w", err)
		}
	}
	if s.cfg.DNS.Listeners.DOH.Enabled {
		dohListener, err = net.Listen("tcp", s.cfg.DNS.Listeners.DOH.Address)
		if err != nil {
			return fmt.Errorf("binding DoH listener %s: %w", s.cfg.DNS.Listeners.DOH.Address, err)
		}
		if err := s.startDoHWithListener(handler, dohListener); err != nil {
			return fmt.Errorf("starting DoH listener: %w", err)
		}
	}
	if s.cfg.DNS.Listeners.DOQ.Enabled {
		doqConn, err = net.ListenPacket("udp", s.cfg.DNS.Listeners.DOQ.Address)
		if err != nil {
			return fmt.Errorf("binding DoQ listener %s: %w", s.cfg.DNS.Listeners.DOQ.Address, err)
		}
		if err := s.startDoQWithConn(handler, doqConn); err != nil {
			return fmt.Errorf("starting DoQ listener: %w", err)
		}
	}

	if s.udpServer != nil {
		go s.serveDNSServer("UDP", s.udpServer)
	}
	if s.tcpServer != nil {
		go s.serveDNSServer("TCP", s.tcpServer)
	}

	s.mu.Lock()
	s.running = true
	s.mu.Unlock()
	slog.Info("dns_server: started successfully")
	return nil
}

func (s *Server) serveDNSServer(name string, server *dns.Server) {
	slog.Info("dns_server: starting listener", "listener", name, "addr", server.Addr)
	if err := server.ActivateAndServe(); err != nil {
		slog.Error("dns_server: listener error", "listener", name, "error", err)
	}
}

// validateTLSListenerConfigs verifies certificate material before any listener
// starts so Start cannot report success with an unusable encrypted endpoint.
func (s *Server) validateTLSListenerConfigs() error {
	listeners := []struct {
		name string
		cfg  config.DNSListenerTLSConfig
	}{
		{name: "DoT", cfg: s.cfg.DNS.Listeners.DOT},
		{name: "DoH", cfg: s.cfg.DNS.Listeners.DOH},
		{name: "DoQ", cfg: s.cfg.DNS.Listeners.DOQ},
	}
	for _, listener := range listeners {
		if !listener.cfg.Enabled {
			continue
		}
		if _, err := loadTLSConfig(listener.cfg.CertFile, listener.cfg.KeyFile); err != nil {
			return fmt.Errorf("invalid %s TLS configuration: %w", listener.name, err)
		}
	}
	return nil
}

// loadTLSConfig builds a tls.Config from the listener's certificate files.
func loadTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	if certFile == "" || keyFile == "" {
		return nil, fmt.Errorf("TLS listener requires cert_file and key_file")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("loading TLS key pair: %w", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
}

// startDoT starts the DNS-over-TLS listener (tcp-tls).
func (s *Server) startDoT(handler dns.Handler) error {
	cfg := s.cfg.DNS.Listeners.DOT
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return err
	}
	if err := s.startDoTWithListener(handler, listener); err != nil {
		_ = listener.Close()
		return err
	}
	return nil
}

func (s *Server) startDoTWithListener(handler dns.Handler, listener net.Listener) error {
	cfg := s.cfg.DNS.Listeners.DOT
	tlsCfg, err := loadTLSConfig(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return err
	}
	s.dotServer = &dns.Server{
		Listener:   listener,
		Net:        "tcp-tls",
		Handler:    handler,
		TLSConfig:  tlsCfg,
		TsigSecret: s.tsigSnapshot(),
	}
	go s.serveDNSServer("DoT", s.dotServer)
	return nil
}

// startDoQ starts the DNS-over-QUIC listener (RFC 9250).
func (s *Server) startDoQ(handler dns.Handler) error {
	cfg := s.cfg.DNS.Listeners.DOQ
	conn, err := net.ListenPacket("udp", cfg.Address)
	if err != nil {
		return err
	}
	if err := s.startDoQWithConn(handler, conn); err != nil {
		_ = conn.Close()
		return err
	}
	return nil
}

func (s *Server) startDoQWithConn(handler dns.Handler, conn net.PacketConn) error {
	cfg := s.cfg.DNS.Listeners.DOQ
	tlsCfg, err := loadTLSConfig(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return err
	}
	s.doqServer = NewDoQServer(cfg.Address, tlsCfg, handler)
	go func() {
		if err := s.doqServer.Serve(conn); err != nil {
			slog.Error("dns_server: DoQ listener error", "error", err)
		}
	}()
	return nil
}

// startDoH starts the DNS-over-HTTPS listener (RFC 8484).
func (s *Server) startDoH(handler dns.Handler) error {
	cfg := s.cfg.DNS.Listeners.DOH
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return err
	}
	if err := s.startDoHWithListener(handler, listener); err != nil {
		_ = listener.Close()
		return err
	}
	return nil
}

func (s *Server) startDoHWithListener(handler dns.Handler, listener net.Listener) error {
	cfg := s.cfg.DNS.Listeners.DOH
	tlsCfg, err := loadTLSConfig(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return err
	}
	s.dohServer = NewDoHServer(cfg.Address, tlsCfg, handler)
	go func() {
		slog.Info("dns_server: starting DoH listener", "addr", listener.Addr())
		if err := s.dohServer.ServeTLS(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("dns_server: DoH listener error", "error", err)
		}
	}()
	return nil
}

// SetListenerConfig validates an encrypted listener configuration before
// storing it. It does not restart the listener; callers should normally use
// ApplyListenerConfig for an atomic validate/start/swap update.
func (s *Server) SetListenerConfig(kind string, cfg config.DNSListenerTLSConfig) error {
	s.listenerMu.Lock()
	defer s.listenerMu.Unlock()
	if err := validateListenerKind(kind, cfg); err != nil {
		return err
	}
	s.setListenerConfig(kind, cfg)
	return nil
}

func validateListenerKind(kind string, cfg config.DNSListenerTLSConfig) error {
	switch kind {
	case "dot", "doh", "doq":
	default:
		return fmt.Errorf("unknown listener kind: %s", kind)
	}
	if !cfg.Enabled {
		return nil
	}
	if cfg.Address == "" {
		return fmt.Errorf("%s listener requires address", kind)
	}
	_, err := loadTLSConfig(cfg.CertFile, cfg.KeyFile)
	return err
}

func (s *Server) setListenerConfig(kind string, cfg config.DNSListenerTLSConfig) {
	switch kind {
	case "dot":
		s.cfg.DNS.Listeners.DOT = cfg
	case "doh":
		s.cfg.DNS.Listeners.DOH = cfg
	case "doq":
		s.cfg.DNS.Listeners.DOQ = cfg
	}
}

// ApplyListenerConfig validates the replacement, starts it on its new socket,
// then stops the prior listener. If validation or the new bind fails, the
// existing listener and its in-memory configuration remain intact.
func (s *Server) ApplyListenerConfig(kind string, cfg config.DNSListenerTLSConfig) error {
	s.listenerMu.Lock()
	defer s.listenerMu.Unlock()
	if err := validateListenerKind(kind, cfg); err != nil {
		return err
	}
	if s.handler == nil {
		s.setListenerConfig(kind, cfg)
		return nil
	}

	previous := s.listenerConfig(kind)
	// A persisted update is also delivered through the settings subscriber
	// after the API has already applied it. Treat the identical configuration
	// as a no-op so that confirmation persistence never causes a second
	// stop/start cycle or unnecessary availability gap.
	if previous == cfg {
		return nil
	}
	if !cfg.Enabled {
		if err := s.stopListenerLocked(kind); err != nil {
			return err
		}
		s.setListenerConfig(kind, cfg)
		return nil
	}

	// Binding the same address while the old listener is active is impossible.
	// Stop only in this special case, and restore the old endpoint if the new
	// listener then fails to launch.
	if previous.Enabled && previous.Address == cfg.Address {
		if err := s.stopListenerLocked(kind); err != nil {
			return err
		}
		s.setListenerConfig(kind, cfg)
		if err := s.startListenerLocked(kind); err != nil {
			s.setListenerConfig(kind, previous)
			if rollbackErr := s.startListenerLocked(kind); rollbackErr != nil {
				return fmt.Errorf("starting replacement: %w; restoring prior listener: %v", err, rollbackErr)
			}
			return fmt.Errorf("starting replacement: %w", err)
		}
		return nil
	}

	// Different-address replacement can bind first, avoiding downtime on an
	// invalid or occupied new address.
	oldServer := s.detachListener(kind)
	s.setListenerConfig(kind, cfg)
	if err := s.startListenerLocked(kind); err != nil {
		s.setListenerConfig(kind, previous)
		s.restoreListener(kind, oldServer)
		return fmt.Errorf("starting replacement: %w", err)
	}
	if err := shutdownDetachedListener(kind, oldServer); err != nil {
		slog.Warn("dns_server: prior listener shutdown warning", "kind", kind, "error", err)
	}
	return nil
}

func (s *Server) listenerConfig(kind string) config.DNSListenerTLSConfig {
	switch kind {
	case "dot":
		return s.cfg.DNS.Listeners.DOT
	case "doh":
		return s.cfg.DNS.Listeners.DOH
	default:
		return s.cfg.DNS.Listeners.DOQ
	}
}

func (s *Server) detachListener(kind string) any {
	switch kind {
	case "dot":
		old := s.dotServer
		s.dotServer = nil
		return old
	case "doh":
		old := s.dohServer
		s.dohServer = nil
		return old
	default:
		old := s.doqServer
		s.doqServer = nil
		return old
	}
}

func (s *Server) restoreListener(kind string, old any) {
	switch kind {
	case "dot":
		s.dotServer, _ = old.(*dns.Server)
	case "doh":
		s.dohServer, _ = old.(*DoHServer)
	case "doq":
		s.doqServer, _ = old.(*DoQServer)
	}
}

func shutdownDetachedListener(kind string, old any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	switch srv := old.(type) {
	case *dns.Server:
		if srv != nil {
			return srv.ShutdownContext(ctx)
		}
	case *DoHServer:
		if srv != nil {
			return srv.Shutdown(ctx)
		}
	case *DoQServer:
		if srv != nil {
			return srv.Shutdown()
		}
	}
	return nil
}

func (s *Server) stopListenerLocked(kind string) error {
	return shutdownDetachedListener(kind, s.detachListener(kind))
}

func (s *Server) startListenerLocked(kind string) error {
	switch kind {
	case "dot":
		return s.startDoT(s.handler)
	case "doh":
		return s.startDoH(s.handler)
	case "doq":
		return s.startDoQ(s.handler)
	default:
		return fmt.Errorf("unknown listener kind: %s", kind)
	}
}

// RestartListener stops and (if enabled) restarts the encrypted listener
// identified by kind ("dot" or "doh") using the current configuration.
func (s *Server) RestartListener(kind string) error {
	s.listenerMu.Lock()
	defer s.listenerMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch kind {
	case "dot":
		if s.dotServer != nil {
			if err := s.dotServer.ShutdownContext(ctx); err != nil {
				slog.Warn("dns_server: DoT listener shutdown warning", "error", err)
			}
			s.dotServer = nil
		}
		if !s.cfg.DNS.Listeners.DOT.Enabled {
			return nil
		}
		if s.handler == nil {
			// Server not started yet; config will apply on Start.
			return nil
		}
		return s.startDoT(s.handler)
	case "doh":
		if s.dohServer != nil {
			if err := s.dohServer.Shutdown(ctx); err != nil {
				slog.Warn("dns_server: DoH listener shutdown warning", "error", err)
			}
			s.dohServer = nil
		}
		if !s.cfg.DNS.Listeners.DOH.Enabled {
			return nil
		}
		if s.handler == nil {
			return nil
		}
		return s.startDoH(s.handler)
	case "doq":
		if s.doqServer != nil {
			if err := s.doqServer.Shutdown(); err != nil {
				slog.Warn("dns_server: DoQ listener shutdown warning", "error", err)
			}
			s.doqServer = nil
		}
		if !s.cfg.DNS.Listeners.DOQ.Enabled {
			return nil
		}
		if s.handler == nil {
			return nil
		}
		return s.startDoQ(s.handler)
	default:
		return fmt.Errorf("unknown listener kind: %s", kind)
	}
}

// Shutdown gracefully stops the DNS server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	var errs []error

	if s.udpServer != nil {
		slog.Info("dns_server: shutting down UDP listener...")
		if err := s.udpServer.ShutdownContext(ctx); err != nil {
			slog.Error("dns_server: UDP shutdown error", "error", err)
			errs = append(errs, err)
		}
	}

	if s.tcpServer != nil {
		slog.Info("dns_server: shutting down TCP listener...")
		if err := s.tcpServer.ShutdownContext(ctx); err != nil {
			slog.Error("dns_server: TCP shutdown error", "error", err)
			errs = append(errs, err)
		}
	}

	if s.dotServer != nil {
		slog.Info("dns_server: shutting down DoT listener...")
		if err := s.dotServer.ShutdownContext(ctx); err != nil {
			slog.Error("dns_server: DoT shutdown error", "error", err)
			errs = append(errs, err)
		}
	}

	if s.dohServer != nil {
		slog.Info("dns_server: shutting down DoH listener...")
		if err := s.dohServer.Shutdown(ctx); err != nil {
			slog.Error("dns_server: DoH shutdown error", "error", err)
			errs = append(errs, err)
		}
	}

	if s.doqServer != nil {
		slog.Info("dns_server: shutting down DoQ listener...")
		if err := s.doqServer.Shutdown(); err != nil {
			slog.Error("dns_server: DoQ shutdown error", "error", err)
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	slog.Info("dns_server: stopped gracefully")
	return nil
}

// IsRunning returns whether the server is currently running.
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// ReloadZones reloads zone data without restart.
func (s *Server) ReloadZones(zones map[string]*ZoneData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.zones = zones
	slog.Info("dns_server: zones reloaded", "count", len(zones))
}

// GetCache returns the DNS cache.
func (s *Server) GetCache() *cache.Cache {
	return s.cache
}

// GetFilter returns the filter engine.
func (s *Server) GetFilter() *filter.FilterEngine {
	return s.filter
}

// GetForwarderGroup returns the forwarder group.
func (s *Server) GetForwarderGroup() *forwarder.ForwarderGroup {
	return s.forwarder
}

// GetConditionalManager returns the conditional forwarder manager.
func (s *Server) GetConditionalManager() *forwarder.ConditionalForwarderManager {
	return s.conditional
}

// GetZoneStore returns the zone store.
func (s *Server) GetZoneStore() *zone.Store {
	return s.zoneStore
}

// zoneSOAFor returns the SOA of the most specific local zone for qname.
func (s *Server) zoneSOAFor(qname string) dns.RR {
	if s.zoneStore == nil {
		return nil
	}
	zoneName := s.zoneStore.MatchingZone(qname)
	if zoneName == "" {
		return nil
	}
	return s.zoneStore.ZoneSOA(zoneName)
}

// lookupAuthoritative checks if the query matches a local authoritative zone.
// Returns (response, denied, found); denied indicates a zone-level query ACL
// rejected the client and the caller must answer REFUSED.
func (s *Server) lookupAuthoritative(qname string, qtype uint16, clientIP string) (*dns.Msg, bool, bool) {
	// If Zone Store is available, use it directly.
	if s.zoneStore != nil {
		// Zone-level query ACL (most specific zone wins; empty ACL = allow).
		if !s.zoneStore.ACLAllows(zone.ACLQuery, qname, clientIP) {
			return nil, true, false
		}

		_, answers, found := s.zoneStore.Lookup(qname, qtype)
		if found && len(answers) > 0 {
			resp := new(dns.Msg)
			resp.Answer = answers
			resp.Authoritative = true
			return resp, false, true
		}
		// Forward and stub zones delegate record misses to their configured
		// upstreams. Other local zone types retain authoritative negative
		// answers to avoid leaking names to recursive resolution.
		if s.zoneStore.IsForwardingZone(qname) {
			return nil, false, false
		}
		if zoneName := s.zoneStore.MatchingZone(qname); zoneName != "" {
			resp := new(dns.Msg)
			resp.Authoritative = true
			if !s.zoneStore.NameExists(zoneName, qname) {
				resp.Rcode = dns.RcodeNameError
			}
			if soa := s.zoneStore.ZoneSOA(zoneName); soa != nil {
				resp.Ns = []dns.RR{soa}
			}
			return resp, false, true
		}
		// No match found in zone store.
		return nil, false, false
	}

	// Legacy path: use in-memory zones map.
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Try exact zone match and parent zone match.
	name := qname
	for {
		if zone, ok := s.zones[name]; ok {
			// Found a matching zone. Look for the record.
			resp := s.answerFromZone(zone, qname, qtype)
			if resp != nil {
				return resp, false, true
			}
		}

		// Move up one label.
		idx := strings.IndexByte(name, '.')
		if idx < 0 || idx >= len(name)-1 {
			break
		}
		name = name[idx+1:]

		// Stop at root.
		if name == "." {
			if zone, ok := s.zones["."]; ok {
				resp := s.answerFromZone(zone, qname, qtype)
				if resp != nil {
					return resp, false, true
				}
			}
			break
		}
	}

	return nil, false, false
}

// answerFromZone attempts to answer a query from a zone's records.
// It first tries the Zone Store (if available), then falls back to the legacy zones map.
func (s *Server) answerFromZone(zoneData *ZoneData, qname string, qtype uint16) *dns.Msg {
	// Try Zone Store first (preferred path).
	if s.zoneStore != nil {
		_, answers, found := s.zoneStore.Lookup(qname, qtype)
		if found && len(answers) > 0 {
			resp := new(dns.Msg)
			resp.Answer = answers
			resp.Authoritative = true
			// Add SOA to authority section for negative responses or NS queries.
			return resp
		}

		// Check if the qname belongs to a known zone but has no matching record.
		// Return NOERROR with empty answer (NODATA) for existing zone.
		zoneName, _, zoneFound := s.zoneStore.Lookup(qname, dns.TypeSOA)
		if zoneFound {
			// The qname is within a known zone but no record matched.
			// Return authoritative NOERROR with SOA in authority section.
			soa := s.zoneStore.GetZoneSOA(zoneName)
			resp := new(dns.Msg)
			resp.Authoritative = true
			if soa != nil {
				resp.Ns = append(resp.Ns, soa)
			}
			return resp
		}

		return nil
	}

	// Legacy path: use the in-memory zones map.
	if zoneData == nil || zoneData.Records == nil {
		return nil
	}

	// Look up records by name.
	qnameLower := strings.ToLower(qname)
	rrs, ok := zoneData.Records[qnameLower]
	if !ok {
		return nil
	}

	// Filter by type.
	var answers []dns.RR
	for _, rr := range rrs {
		if rr.Header().Rrtype == qtype {
			answers = append(answers, rr)
		}
	}

	// If no specific type match, check for CNAME (unless querying for CNAME).
	if len(answers) == 0 && qtype != dns.TypeCNAME {
		for _, rr := range rrs {
			if rr.Header().Rrtype == dns.TypeCNAME {
				answers = append(answers, rr)
			}
		}
	}

	if len(answers) == 0 {
		return nil
	}

	resp := new(dns.Msg)
	resp.Answer = answers
	resp.Authoritative = true
	return resp
}

// resolveSharedForward coalesces semantically identical forwarding work. The
// shared call owns an independent bounded context, so cancellation of the
// first waiting DNS client never aborts a query needed by other waiters.
func (s *Server) resolveSharedForward(ctx context.Context, msg *dns.Msg) (*dns.Msg, *forwarder.Forwarder, time.Duration, error) {
	key, ok := forwardingKey(msg)
	if !ok {
		return s.resolveForward(ctx, msg)
	}

	s.inflightMu.Lock()
	if current := s.inflight[key]; current != nil {
		s.inflightMu.Unlock()
		select {
		case <-ctx.Done():
			return nil, nil, 0, ctx.Err()
		case <-current.done:
			return copyInflightResult(current)
		}
	}
	if s.inflightLimit > 0 && len(s.inflight) >= s.inflightLimit {
		metrics.RecordDNSInflightRejected()
		s.inflightMu.Unlock()
		return nil, nil, 0, ErrInflightLimitReached
	}
	current := &inflightQuery{done: make(chan struct{})}
	s.inflight[key] = current
	metrics.SetDNSInflightQueries(len(s.inflight))
	s.inflightMu.Unlock()

	go func() {
		// This deliberately does not derive from any client context. The timeout
		// is aligned with the regular handler forwarding budget.
		sharedCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		resp, fwd, duration, err := s.resolveForward(sharedCtx, msg.Copy())
		current.response = resp
		current.forwarder = fwd
		current.duration = duration
		current.err = err
		s.inflightMu.Lock()
		delete(s.inflight, key)
		metrics.SetDNSInflightQueries(len(s.inflight))
		close(current.done)
		s.inflightMu.Unlock()
	}()

	select {
	case <-ctx.Done():
		return nil, nil, 0, ctx.Err()
	case <-current.done:
		return copyInflightResult(current)
	}
}

func copyInflightResult(current *inflightQuery) (*dns.Msg, *forwarder.Forwarder, time.Duration, error) {
	if current.response == nil {
		return nil, current.forwarder, current.duration, current.err
	}
	return current.response.Copy(), current.forwarder, current.duration, current.err
}

// forwardingKey packs a copied, fully prepared upstream query after clearing
// the client-specific DNS ID. Any flag, QCLASS or EDNS option that can affect
// an answer remains in the key. TSIG and multi-question requests bypass
// coalescing because their response semantics cannot safely be shared.
func forwardingKey(msg *dns.Msg) (string, bool) {
	if msg == nil || msg.Opcode != dns.OpcodeQuery || len(msg.Question) != 1 || msg.IsTsig() != nil {
		return "", false
	}
	keyMsg := msg.Copy()
	keyMsg.Id = 0
	packed, err := keyMsg.Pack()
	if err != nil {
		return "", false
	}
	return string(packed), true
}

// resolveForward forwards the query to upstream servers. Queries inside
// a forward/stub zone are sent to that zone's configured targets first.
func (s *Server) resolveForward(ctx context.Context, msg *dns.Msg) (*dns.Msg, *forwarder.Forwarder, time.Duration, error) {
	// Zone-level forwarding (forward/stub zone types).
	if s.zoneStore != nil && len(msg.Question) > 0 {
		if _, targets, ok := s.zoneStore.ForwardTargets(msg.Question[0].Name); ok {
			resp, duration, err := s.forwarder.ForwardToAddresses(ctx, msg, targets)
			return resp, nil, duration, err
		}
	}

	if s.conditional != nil {
		return s.conditional.Forward(ctx, msg)
	}
	return s.forwarder.Forward(ctx, msg)
}
