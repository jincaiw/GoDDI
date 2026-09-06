package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
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

	udpServer *dns.Server
	tcpServer *dns.Server
	dotServer *dns.Server
	dohServer *DoHServer
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
	ecsMu sync.RWMutex
	ecsMode forwarder.ECSMode
	ecsIPv4Prefix int
	ecsIPv6Prefix int
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
		cfg:         cfg,
		cache:       dnsCache,
		filter:      filterEngine,
		forwarder:   fwdGroup,
		conditional: condManager,
		queryLog:    queryLog,
		zoneStore:   zoneStore,
		zones:       make(map[string]*ZoneData),

		// ECS defaults to the privacy-preserving strip policy.
		ecsMode:       forwarder.ECSStrip,
		ecsIPv4Prefix: forwarder.DefaultECSIPv4Prefix,
		ecsIPv6Prefix: forwarder.DefaultECSIPv6Prefix,
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
	s.notifyHandler = fn
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

// Start starts the DNS server (UDP and TCP listeners).
func (s *Server) Start(ctx context.Context) error {
	handler := &DNSHandler{server: s}
	s.handler = handler

	// Start UDP listener.
	if s.cfg.DNS.Listeners.UDP.Enabled {
		s.udpServer = &dns.Server{
			Addr:    s.cfg.DNS.Listeners.UDP.Address,
			Net:     "udp",
			Handler: handler,
		}
		go func() {
			slog.Info("dns_server: starting UDP listener", "addr", s.cfg.DNS.Listeners.UDP.Address)
			if err := s.udpServer.ListenAndServe(); err != nil {
				slog.Error("dns_server: UDP listener error", "error", err)
			}
		}()
	}

	// Start TCP listener.
	if s.cfg.DNS.Listeners.TCP.Enabled {
		s.tcpServer = &dns.Server{
			Addr:    s.cfg.DNS.Listeners.TCP.Address,
			Net:     "tcp",
			Handler: handler,
		}
		go func() {
			slog.Info("dns_server: starting TCP listener", "addr", s.cfg.DNS.Listeners.TCP.Address)
			if err := s.tcpServer.ListenAndServe(); err != nil {
				slog.Error("dns_server: TCP listener error", "error", err)
			}
		}()
	}

	// Start DoT (DNS-over-TLS, RFC 7858) listener.
	if s.cfg.DNS.Listeners.DOT.Enabled {
		if err := s.startDoT(handler); err != nil {
			slog.Error("dns_server: DoT listener failed to start", "error", err)
		}
	}

	// Start DoH (DNS-over-HTTPS, RFC 8484) listener.
	if s.cfg.DNS.Listeners.DOH.Enabled {
		if err := s.startDoH(handler); err != nil {
			slog.Error("dns_server: DoH listener failed to start", "error", err)
		}
	}

	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	slog.Info("dns_server: started successfully")
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
	tlsCfg, err := loadTLSConfig(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return err
	}
	s.dotServer = &dns.Server{
		Addr:      cfg.Address,
		Net:       "tcp-tls",
		Handler:   handler,
		TLSConfig: tlsCfg,
	}
	go func() {
		slog.Info("dns_server: starting DoT listener", "addr", cfg.Address)
		if err := s.dotServer.ListenAndServe(); err != nil {
			slog.Error("dns_server: DoT listener error", "error", err)
		}
	}()
	return nil
}

// startDoH starts the DNS-over-HTTPS listener (RFC 8484).
func (s *Server) startDoH(handler dns.Handler) error {
	cfg := s.cfg.DNS.Listeners.DOH
	tlsCfg, err := loadTLSConfig(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return err
	}
	s.dohServer = NewDoHServer(cfg.Address, tlsCfg, handler)
	go func() {
		slog.Info("dns_server: starting DoH listener", "addr", cfg.Address)
		if err := s.dohServer.ListenAndServeTLS(); err != nil && err != http.ErrServerClosed {
			slog.Error("dns_server: DoH listener error", "error", err)
		}
	}()
	return nil
}

// SetListenerConfig hot-updates the DoT ("dot") or DoH ("doh") listener
// configuration. The change takes effect immediately when followed by
// RestartListener, or on next startup otherwise.
func (s *Server) SetListenerConfig(kind string, cfg config.DNSListenerTLSConfig) error {
	s.listenerMu.Lock()
	defer s.listenerMu.Unlock()
	switch kind {
	case "dot":
		s.cfg.DNS.Listeners.DOT = cfg
	case "doh":
		s.cfg.DNS.Listeners.DOH = cfg
	default:
		return fmt.Errorf("unknown listener kind: %s", kind)
	}
	return nil
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
		// Zone match but no record: answer authoritatively (NXDOMAIN or
		// NODATA with SOA). Returning false here would leak queries for
		// names inside local zones to upstream resolvers.
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

// resolveForward forwards the query to upstream DNS servers. Queries inside
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
