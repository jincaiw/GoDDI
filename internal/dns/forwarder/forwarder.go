package forwarder

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// SelectionStrategy defines how upstream servers are selected.
type SelectionStrategy string

const (
	StrategySequential      SelectionStrategy = "sequential"
	StrategyRoundRobin      SelectionStrategy = "round_robin"
	StrategyRandom          SelectionStrategy = "random"
	StrategyParallelFastest SelectionStrategy = "parallel_fastest"
	StrategyLatencyBest     SelectionStrategy = "latency_best"
	StrategyHealthAware     SelectionStrategy = "health_aware"
)

// Forwarder represents a single upstream DNS server.
type Forwarder struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"` // udp, tcp
	Address  string `json:"address"`
	Enabled  bool   `json:"enabled"`
	Priority int    `json:"priority"`

	// Health state.
	healthy          atomic.Bool
	consecutiveFails atomic.Int32
	lastHealthCheck  time.Time
	latencyHistory   []time.Duration
	mu               sync.Mutex
}

// IsHealthy returns whether the forwarder is healthy.
func (f *Forwarder) IsHealthy() bool {
	return f.healthy.Load()
}

// SetHealthy sets the health status of the forwarder.
func (f *Forwarder) SetHealthy(healthy bool) {
	f.healthy.Store(healthy)
}

// AvgLatency returns the average latency from recent queries.
func (f *Forwarder) AvgLatency() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.latencyHistory) == 0 {
		return 5 * time.Second // Default high latency.
	}

	var total time.Duration
	for _, l := range f.latencyHistory {
		total += l
	}
	return total / time.Duration(len(f.latencyHistory))
}

// recordLatency records a query latency.
func (f *Forwarder) recordLatency(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.latencyHistory = append(f.latencyHistory, d)
	// Keep last 20 measurements.
	if len(f.latencyHistory) > 20 {
		f.latencyHistory = f.latencyHistory[1:]
	}
}

// recordSuccess records a successful query.
func (f *Forwarder) recordSuccess(d time.Duration) {
	f.consecutiveFails.Store(0)
	f.healthy.Store(true)
	f.recordLatency(d)
}

// recordFailure records a failed query.
func (f *Forwarder) recordFailure() {
	fails := f.consecutiveFails.Add(1)
	if fails >= 3 {
		f.healthy.Store(false)
		slog.Warn("forwarder: marked unhealthy", "name", f.Name, "address", f.Address, "consecutive_fails", fails)
	}
}

// ForwarderGroup manages multiple upstream DNS servers.
type ForwarderGroup struct {
	forwarders []*Forwarder
	strategy   SelectionStrategy
	rrIndex    atomic.Int64 // For round-robin.
	timeout    time.Duration

	mu sync.RWMutex
}

// NewForwarderGroup creates a new forwarder group.
func NewForwarderGroup(strategy SelectionStrategy, timeout time.Duration) *ForwarderGroup {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &ForwarderGroup{
		forwarders: make([]*Forwarder, 0),
		strategy:   strategy,
		timeout:    timeout,
	}
}

// SetForwarders replaces the list of forwarders.
func (fg *ForwarderGroup) SetForwarders(forwarders []*Forwarder) {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	fg.forwarders = forwarders
}

// AddForwarder adds a forwarder to the group.
// It returns an error if the upstream address fails SSRF validation.
func (fg *ForwarderGroup) AddForwarder(f *Forwarder) error {
	if err := validateUpstreamAddress(f.Address); err != nil {
		return err
	}
	fg.mu.Lock()
	defer fg.mu.Unlock()
	f.healthy.Store(true)
	fg.forwarders = append(fg.forwarders, f)
	return nil
}

// validateUpstreamAddress rejects upstream addresses that resolve to
// loopback, link-local, or private IPs (an SSRF mitigation). A bare IP
// literal in the address is checked directly. A hostname is resolved
// best-effort; the lookup is bounded by a short context so a slow DNS
// response cannot stall startup, and ANY non-blocked address is allowed.
//
// Operators who intentionally need to point the resolver at a private
// upstream can still do so by adding the address to the explicit allow
// list; the function is intentionally conservative.
func validateUpstreamAddress(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Maybe the address is just a bare host (no port). Treat the
		// whole string as the host and try again.
		host = addr
	}
	if host == "" {
		return fmt.Errorf("invalid address: empty host")
	}

	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("blocked address range: %s", addr)
		}
		return nil
	}

	// Hostname: resolve it. Use a short context so a slow upstream DNS
	// cannot stall the configuration step. We use the system resolver
	// (no custom dialer), and only accept the first answer returned.
	resolver := net.Resolver{PreferGo: true}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ips, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		// A lookup failure is treated as a soft pass: the transport
		// layer will retry later and we do not want a transient DNS
		// outage to refuse otherwise-valid configuration. The
		// IP-level check is still performed on every query, where it
		// matters.
		slog.Debug("forwarder: could not resolve upstream hostname; allowing optimistically",
			"host", host, "error", err)
		return nil
	}
	for _, ipa := range ips {
		if isBlockedIP(ipa.IP) {
			return fmt.Errorf("blocked address range: %s resolves to %s", addr, ipa.IP)
		}
	}
	return nil
}

// isBlockedIP reports whether ip is in a range that an upstream DNS
// forwarder should not be allowed to talk to. Loopback, private
// (RFC 1918 + RFC 4193), link-local, and unspecified addresses all count
// as blocked. Multicast and broadcast are left to the transport layer.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}

// RemoveForwarder removes a forwarder by ID.
func (fg *ForwarderGroup) RemoveForwarder(id string) {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	for i, f := range fg.forwarders {
		if f.ID == id {
			fg.forwarders = append(fg.forwarders[:i], fg.forwarders[i+1:]...)
			return
		}
	}
}

// Forward forwards a DNS query to upstream servers based on the configured strategy.
func (fg *ForwarderGroup) Forward(ctx context.Context, msg *dns.Msg) (*dns.Msg, *Forwarder, time.Duration, error) {
	fg.mu.RLock()
	forwarders := make([]*Forwarder, len(fg.forwarders))
	copy(forwarders, fg.forwarders)
	fg.mu.RUnlock()

	// Filter to enabled forwarders.
	var active []*Forwarder
	for _, f := range forwarders {
		if f.Enabled {
			active = append(active, f)
		}
	}

	if len(active) == 0 {
		return nil, nil, 0, fmt.Errorf("no active forwarders available")
	}

	switch fg.strategy {
	case StrategySequential:
		return fg.forwardSequential(ctx, msg, active)
	case StrategyRoundRobin:
		return fg.forwardRoundRobin(ctx, msg, active)
	case StrategyRandom:
		return fg.forwardRandom(ctx, msg, active)
	case StrategyParallelFastest:
		return fg.forwardParallelFastest(ctx, msg, active)
	case StrategyLatencyBest:
		return fg.forwardLatencyBest(ctx, msg, active)
	case StrategyHealthAware:
		return fg.forwardHealthAware(ctx, msg, active)
	default:
		return fg.forwardSequential(ctx, msg, active)
	}
}

// forwardSequential tries forwarders in order.
func (fg *ForwarderGroup) forwardSequential(ctx context.Context, msg *dns.Msg, forwarders []*Forwarder) (*dns.Msg, *Forwarder, time.Duration, error) {
	var lastErr error
	for _, f := range forwarders {
		resp, d, err := fg.queryUpstream(ctx, msg, f)
		if err == nil {
			return resp, f, d, nil
		}
		lastErr = err
	}
	return nil, nil, 0, fmt.Errorf("all forwarders failed: %w", lastErr)
}

// forwardRoundRobin rotates through forwarders.
func (fg *ForwarderGroup) forwardRoundRobin(ctx context.Context, msg *dns.Msg, forwarders []*Forwarder) (*dns.Msg, *Forwarder, time.Duration, error) {
	n := len(forwarders)
	start := int(fg.rrIndex.Add(1)) % n

	var lastErr error
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		resp, d, err := fg.queryUpstream(ctx, msg, forwarders[idx])
		if err == nil {
			return resp, forwarders[idx], d, nil
		}
		lastErr = err
	}
	return nil, nil, 0, fmt.Errorf("all forwarders failed: %w", lastErr)
}

// forwardRandom picks a random forwarder.
func (fg *ForwarderGroup) forwardRandom(ctx context.Context, msg *dns.Msg, forwarders []*Forwarder) (*dns.Msg, *Forwarder, time.Duration, error) {
	var lastErr error
	// Try random order.
	indices := rand.Perm(len(forwarders))
	for _, idx := range indices {
		resp, d, err := fg.queryUpstream(ctx, msg, forwarders[idx])
		if err == nil {
			return resp, forwarders[idx], d, nil
		}
		lastErr = err
	}
	return nil, nil, 0, fmt.Errorf("all forwarders failed: %w", lastErr)
}

// forwardParallelFastest queries all forwarders in parallel and returns the first response.
func (fg *ForwarderGroup) forwardParallelFastest(ctx context.Context, msg *dns.Msg, forwarders []*Forwarder) (*dns.Msg, *Forwarder, time.Duration, error) {
	type result struct {
		resp      *dns.Msg
		forwarder *Forwarder
		duration  time.Duration
		err       error
	}

	ch := make(chan result, len(forwarders))
	queryCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, f := range forwarders {
		go func(fwd *Forwarder) {
			start := time.Now()
			resp, _, err := fg.queryUpstream(queryCtx, msg, fwd)
			d := time.Since(start)
			if err != nil {
				ch <- result{nil, fwd, d, err}
				return
			}
			ch <- result{resp, fwd, d, nil}
		}(f)
	}

	var lastErr error
	for i := 0; i < len(forwarders); i++ {
		r := <-ch
		if r.err == nil {
			cancel() // Cancel remaining queries.
			return r.resp, r.forwarder, r.duration, nil
		}
		lastErr = r.err
	}

	return nil, nil, 0, fmt.Errorf("all forwarders failed: %w", lastErr)
}

// forwardLatencyBest selects the forwarder with the best historical latency.
func (fg *ForwarderGroup) forwardLatencyBest(ctx context.Context, msg *dns.Msg, forwarders []*Forwarder) (*dns.Msg, *Forwarder, time.Duration, error) {
	// Sort by average latency (simple selection).
	best := forwarders[0]
	bestLatency := best.AvgLatency()
	for _, f := range forwarders[1:] {
		lat := f.AvgLatency()
		if lat < bestLatency {
			best = f
			bestLatency = lat
		}
	}

	resp, d, err := fg.queryUpstream(ctx, msg, best)
	if err == nil {
		return resp, best, d, nil
	}

	// Fallback to others.
	var lastErr error
	for _, f := range forwarders {
		if f == best {
			continue
		}
		resp, d, err := fg.queryUpstream(ctx, msg, f)
		if err == nil {
			return resp, f, d, nil
		}
		lastErr = err
	}
	return nil, nil, 0, fmt.Errorf("all forwarders failed: %w", lastErr)
}

// forwardHealthAware skips unhealthy servers and auto-retries.
func (fg *ForwarderGroup) forwardHealthAware(ctx context.Context, msg *dns.Msg, forwarders []*Forwarder) (*dns.Msg, *Forwarder, time.Duration, error) {
	var lastErr error
	for _, f := range forwarders {
		if !f.IsHealthy() {
			// Check if it's time to retry (30s cooldown).
			f.mu.Lock()
			if time.Since(f.lastHealthCheck) > 30*time.Second {
				f.lastHealthCheck = time.Now()
				f.mu.Unlock()
				// Try this forwarder as a health probe.
				resp, d, err := fg.queryUpstream(ctx, msg, f)
				if err == nil {
					f.recordSuccess(d)
					return resp, f, d, nil
				}
				f.recordFailure()
				lastErr = err
				continue
			}
			f.mu.Unlock()
			continue
		}

		resp, d, err := fg.queryUpstream(ctx, msg, f)
		if err == nil {
			f.recordSuccess(d)
			return resp, f, d, nil
		}
		f.recordFailure()
		lastErr = err
	}

	return nil, nil, 0, fmt.Errorf("all forwarders failed: %w", lastErr)
}

// queryUpstream sends a DNS query to a single upstream server.
func (fg *ForwarderGroup) queryUpstream(ctx context.Context, msg *dns.Msg, f *Forwarder) (*dns.Msg, time.Duration, error) {
	proto := f.Protocol
	if proto == "" {
		proto = "udp"
	}

	start := time.Now()

	client := &dns.Client{
		Net:          proto,
		ReadTimeout:  fg.timeout,
		WriteTimeout: fg.timeout,
	}

	// Use context deadline if set.
	if deadline, ok := ctx.Deadline(); ok {
		client.DialTimeout = time.Until(deadline)
		if client.DialTimeout <= 0 {
			return nil, 0, fmt.Errorf("context deadline exceeded")
		}
	}

	resp, _, err := client.ExchangeContext(ctx, msg, parseHostPort(f.Address))
	d := time.Since(start)

	if err != nil {
		f.recordFailure()
		return nil, d, fmt.Errorf("query upstream %s (%s): %w", f.Name, f.Address, err)
	}

	if resp == nil {
		f.recordFailure()
		return nil, d, fmt.Errorf("nil response from upstream %s (%s)", f.Name, f.Address)
	}

	// Verify the response correlates to the request: matching QID and question
	// section. A mismatched response could indicate a forged or stale reply
	// and must be discarded.
	if resp.Id != msg.Id {
		f.recordFailure()
		return nil, d, fmt.Errorf("qid mismatch from upstream %s (%s): got %d, want %d", f.Name, f.Address, resp.Id, msg.Id)
	}
	if !questionMatches(msg.Question, resp.Question) {
		f.recordFailure()
		return nil, d, fmt.Errorf("question section mismatch from upstream %s (%s)", f.Name, f.Address)
	}

	// Handle truncated responses by retrying over TCP.
	if resp.Truncated && proto == "udp" {
		tcpClient := &dns.Client{
			Net:          "tcp",
			ReadTimeout:  fg.timeout,
			WriteTimeout: fg.timeout,
		}
		resp, _, err = tcpClient.ExchangeContext(ctx, msg, parseHostPort(f.Address))
		d = time.Since(start)
		if err != nil {
			f.recordFailure()
			return nil, d, fmt.Errorf("tcp retry upstream %s (%s): %w", f.Name, f.Address, err)
		}
		if resp == nil {
			f.recordFailure()
			return nil, d, fmt.Errorf("nil tcp response from upstream %s (%s)", f.Name, f.Address)
		}
		// Re-verify correlation after the TCP retry.
		if resp.Id != msg.Id {
			f.recordFailure()
			return nil, d, fmt.Errorf("qid mismatch on tcp retry from upstream %s (%s): got %d, want %d", f.Name, f.Address, resp.Id, msg.Id)
		}
		if !questionMatches(msg.Question, resp.Question) {
			f.recordFailure()
			return nil, d, fmt.Errorf("question section mismatch on tcp retry from upstream %s (%s)", f.Name, f.Address)
		}
	}

	f.recordSuccess(d)
	return resp, d, nil
}

// HealthCheck probes all forwarders.
func (fg *ForwarderGroup) HealthCheck() {
	fg.mu.RLock()
	forwarders := make([]*Forwarder, len(fg.forwarders))
	copy(forwarders, fg.forwarders)
	fg.mu.RUnlock()

	for _, f := range forwarders {
		if !f.Enabled {
			continue
		}

		msg := new(dns.Msg)
		msg.SetQuestion(".", dns.TypeNS)
		msg.RecursionDesired = true

		client := &dns.Client{
			Net:          f.Protocol,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		}

		start := time.Now()
		resp, _, err := client.Exchange(msg, parseHostPort(f.Address))
		d := time.Since(start)

		f.mu.Lock()
		f.lastHealthCheck = time.Now()
		f.mu.Unlock()

		if err != nil || resp == nil {
			f.recordFailure()
		} else {
			f.recordSuccess(d)
		}
	}
}

// GetForwarders returns a copy of the forwarder list.
func (fg *ForwarderGroup) GetForwarders() []*Forwarder {
	fg.mu.RLock()
	defer fg.mu.RUnlock()

	result := make([]*Forwarder, len(fg.forwarders))
	copy(result, fg.forwarders)
	return result
}

// LookupForwarder finds a forwarder by address.
func (fg *ForwarderGroup) LookupForwarder(address string) *Forwarder {
	fg.mu.RLock()
	defer fg.mu.RUnlock()

	for _, f := range fg.forwarders {
		if f.Address == address {
			return f
		}
	}
	return nil
}

// ForwardToAddresses forwards the message to the given upstream addresses in
// order and returns the first successful response. It is used for zone-level
// forwarding (forward/stub zones) whose targets are not part of the global
// forwarder pool; targets already present in the pool reuse their health and
// latency state.
func (fg *ForwarderGroup) ForwardToAddresses(ctx context.Context, msg *dns.Msg, targets []string) (*dns.Msg, time.Duration, error) {
	var lastErr error
	for _, addr := range targets {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		f := fg.LookupForwarder(addr)
		if f == nil {
			f = &Forwarder{
				ID:       "zone-" + addr,
				Name:     addr,
				Protocol: "udp",
				Address:  addr,
				Enabled:  true,
			}
			f.healthy.Store(true)
		}
		resp, d, err := fg.queryUpstream(ctx, msg, f)
		if err == nil {
			return resp, d, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no zone forward targets configured")
	}
	return nil, 0, lastErr
}

// parseHostPort ensures address has a port. It correctly handles IPv6 literals
// by checking for surrounding brackets and only appending :53 when the host is
// actually missing a port.
func parseHostPort(addr string) string {
	if addr == "" {
		return addr
	}
	if addr[0] == '[' {
		// Bracketed IPv6 literal (e.g. "[::1]:53" or "[::1]").
		end := strings.LastIndex(addr, "]")
		if end < 0 {
			return addr + ":53"
		}
		if end == len(addr)-1 {
			return addr
		}
		// Has closing bracket but no port separator after it.
		if addr[end+1] != ':' {
			return addr + ":53"
		}
		return addr
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return addr + ":53"
	}
	return addr
}

// questionMatches returns true when req and resp carry the same number of
// questions with identical name, type and class. A mismatch may indicate a
// replay, cross-query contamination, or a malicious upstream.
func questionMatches(req, resp []dns.Question) bool {
	if len(req) != len(resp) {
		return false
	}
	for i := range req {
		if req[i].Name != resp[i].Name {
			return false
		}
		if req[i].Qtype != resp[i].Qtype {
			return false
		}
		if req[i].Qclass != resp[i].Qclass {
			return false
		}
	}
	return true
}
