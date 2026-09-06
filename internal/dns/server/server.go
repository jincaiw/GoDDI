package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
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

	udpServer *dns.Server
	tcpServer *dns.Server

	mu      sync.RWMutex
	running bool
	zones   map[string]*ZoneData // In-memory authoritative zones (legacy, kept for compatibility)

	// Recursion ACL.
	recursionMu   sync.RWMutex
	recursionNets []*net.IPNet
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

	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	slog.Info("dns_server: started successfully")
	return nil
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

// lookupAuthoritative checks if the query matches a local authoritative zone.
func (s *Server) lookupAuthoritative(qname string, qtype uint16) (*dns.Msg, bool) {
	// If Zone Store is available, use it directly.
	if s.zoneStore != nil {
		_, answers, found := s.zoneStore.Lookup(qname, qtype)
		if found && len(answers) > 0 {
			resp := new(dns.Msg)
			resp.Answer = answers
			resp.Authoritative = true
			return resp, true
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
			return resp, true
		}
		// No match found in zone store.
		return nil, false
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
				return resp, true
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
					return resp, true
				}
			}
			break
		}
	}

	return nil, false
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

// resolveForward forwards the query to upstream DNS servers.
func (s *Server) resolveForward(ctx context.Context, msg *dns.Msg) (*dns.Msg, *forwarder.Forwarder, time.Duration, error) {
	if s.conditional != nil {
		return s.conditional.Forward(ctx, msg)
	}
	return s.forwarder.Forward(ctx, msg)
}
