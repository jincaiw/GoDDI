package server

import (
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// RateLimiter enforces per-client query rate limits and a response rate
// limit (RRL) that throttles floods of identical responses across all
// clients — the classic DNS amplification / reflection mitigation.
type RateLimiter struct {
	// Client QPS limit (token bucket per client IP).
	clientQPS   int64 // sustained queries per second, 0 = unlimited
	clientBurst int64 // bucket capacity

	// RRL: identical (qname, qtype, rcode) responses across all clients
	// within a one-second window above rrlThreshold are refused for
	// every second request (slip = 2).
	rrlThreshold int64

	mu        sync.Mutex
	buckets   map[string]*clientBucket
	rrlCounts map[string]*rrlWindow
	lastSweep time.Time

	exceededCount int64
}

// clientBucket is a token bucket for one client IP.
type clientBucket struct {
	tokens float64
	last   time.Time
}

// rrlWindow counts identical responses in the current one-second window.
type rrlWindow struct {
	window int64 // unix second
	count  int64
	slip   int64 // counter used to decide which exceeding queries get refused
}

// NewRateLimiter creates a rate limiter. qps <= 0 disables client limiting,
// rrlThreshold <= 0 disables response rate limiting.
func NewRateLimiter(clientQPS, clientBurst, rrlThreshold int64) *RateLimiter {
	if clientBurst <= 0 {
		clientBurst = clientQPS // default burst equals one second of traffic
	}
	return &RateLimiter{
		clientQPS:    clientQPS,
		clientBurst:  clientBurst,
		rrlThreshold: rrlThreshold,
		buckets:      make(map[string]*clientBucket),
		rrlCounts:    make(map[string]*rrlWindow),
		lastSweep:    time.Now(),
	}
}

// Configure hot-updates the limits (used by the settings notifier).
// A value of 0 or less disables the corresponding limiter.
func (rl *RateLimiter) Configure(clientQPS, clientBurst, rrlThreshold int64) {
	if clientBurst <= 0 {
		clientBurst = clientQPS
	}
	rl.mu.Lock()
	rl.clientQPS = clientQPS
	rl.clientBurst = clientBurst
	rl.rrlThreshold = rrlThreshold
	rl.mu.Unlock()
	slog.Info("ratelimit: reconfigured", "client_qps", clientQPS, "client_burst", clientBurst, "rrl_threshold", rrlThreshold)
}

// AllowQuery reports whether a query from clientIP may proceed. It applies
// the per-client token bucket. Requests are logged (rate limited) at most
// once per second per client to avoid log flooding.
func (rl *RateLimiter) AllowQuery(clientIP string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.clientQPS <= 0 {
		return true
	}

	now := time.Now()
	rl.sweepLocked(now)

	b, ok := rl.buckets[clientIP]
	if !ok {
		b = &clientBucket{tokens: float64(rl.clientBurst), last: now}
		rl.buckets[clientIP] = b
	}

	// Refill tokens.
	elapsed := now.Sub(b.last).Seconds()
	b.last = now
	b.tokens += elapsed * float64(rl.clientQPS)
	if max := float64(rl.clientBurst); b.tokens > max {
		b.tokens = max
	}

	if b.tokens >= 1 {
		b.tokens--
		return true
	}

	rl.exceededCount++
	return false
}

// AllowResponse applies response rate limiting. key must uniquely identify
// the response content (qname + qtype + rcode + answer summary). It returns
// false when the response should be withheld (truncated/refused) because
// the identical response is being emitted above the configured threshold.
// With slip = 2, every second response above the threshold is withheld so
// that legitimate clients retry over TCP while floods are blunted.
func (rl *RateLimiter) AllowResponse(key string) bool {
	if rl.rrlThreshold <= 0 {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now().Unix()
	w, ok := rl.rrlCounts[key]
	if !ok || w.window != now {
		w = &rrlWindow{window: now}
		rl.rrlCounts[key] = w
	}

	w.count++
	if w.count <= rl.rrlThreshold {
		return true
	}

	// Slip: withhold every second response above the threshold.
	w.slip++
	return w.slip%2 == 0
}

// ExceededCount returns how many queries have been rate limited so far.
func (rl *RateLimiter) ExceededCount() int64 {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.exceededCount
}

// sweepLocked periodically drops idle buckets/windows to bound memory.
// Must be called with rl.mu held.
func (rl *RateLimiter) sweepLocked(now time.Time) {
	if now.Sub(rl.lastSweep) < time.Minute {
		return
	}
	rl.lastSweep = now

	for ip, b := range rl.buckets {
		if now.Sub(b.last) > 10*time.Minute {
			delete(rl.buckets, ip)
		}
	}
	cutoff := now.Unix() - 5
	for key, w := range rl.rrlCounts {
		if w.window < cutoff {
			delete(rl.rrlCounts, key)
		}
	}
}

// ResponseKey builds an RRL key from a response message. Only meaningful
// for UDP traffic; TCP clients are exempt from RRL.
func ResponseKey(qname string, qtype uint16, resp *dns.Msg) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(qname))
	b.WriteByte('/')
	b.WriteString(dns.TypeToString[qtype])
	b.WriteByte('/')
	b.WriteString(dns.RcodeToString[resp.Rcode])
	// Include a light answer fingerprint so different answers do not
	// share a bucket: number of answers + first answer type.
	b.WriteByte('/')
	b.WriteByte(byte(len(resp.Answer)))
	for _, rr := range resp.Answer {
		b.WriteByte(byte(rr.Header().Rrtype))
		break
	}
	return b.String()
}

// normalizeClientIP strips IPv6 zone info so the limiter keys cleanly.
func normalizeClientIP(ipStr string) string {
	if ip := net.ParseIP(ipStr); ip != nil {
		return ip.String()
	}
	return ipStr
}
