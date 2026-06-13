package cache

import (
	"container/list"
	"encoding/json"
	"log/slog"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// entry represents a cached DNS response. The in-memory representation
// carries one extra field (prefetching) that is not part of the JSON
// wire format: the prefetch flag is purely a runtime latch and must
// not survive a process restart.
type entry struct {
	Key         string    `json:"key"`
	QName       string    `json:"qname"`
	QType       uint16    `json:"qtype"`
	Msg         []byte    `json:"msg"` // serialized dns.Msg
	ExpiresAt   time.Time `json:"expires_at"`
	StaleUntil  time.Time `json:"stale_until"`
	OriginalTTL int64     `json:"original_ttl_ns"` // initial TTL, used for the prefetch threshold
	HitCount    int64     `json:"hit_count"`
	LastAccess  time.Time `json:"last_access"`

	// prefetching is read and written from multiple goroutines (one
	// per active lookup of a hot key). Using a plain bool here would
	// race two callers into spawning duplicate prefetch goroutines, so
	// we use atomic.Bool and gate the "claim the prefetch" step with
	// CompareAndSwap.
	prefetching atomic.Bool `json:"-"`
}

// Stats holds cache statistics.
type Stats struct {
	Entries   int64   `json:"entries"`
	Hits      int64   `json:"hits"`
	Misses    int64   `json:"misses"`
	HitRate   float64 `json:"hit_rate"`
	SizeBytes int64   `json:"size_bytes"`
}

// lruItem is the value stored in the LRU list. Holding a pointer to
// the entry (rather than copying the struct) means Get/Set/eviction
// can all update the same instance in O(1).
type lruItem struct {
	key   string
	entry *entry
}

// Cache is a thread-safe DNS response cache with O(1) LRU eviction
// and an atomic prefetch latch.
type Cache struct {
	mu         sync.RWMutex
	entries    map[string]*list.Element // key -> list element holding *lruItem
	lru        *list.List               // front = most recent, back = least recent
	maxEntries int
	minTTL     int
	maxTTL     int
	negTTL     int
	serveStale bool
	staleTTL   int
	prefetch   bool

	hits   int64
	misses int64

	// Callback for prefetching; if set, called when an entry is about to expire.
	onPrefetch func(qname string, qtype uint16)
}

// Config holds cache configuration.
type Config struct {
	Enabled      bool
	MaxEntries   int
	MinTTL       int
	MaxTTL       int
	NegativeTTL  int
	ServeStale   bool
	StaleTTL     int
	Prefetch     bool
	AutoPrefetch bool
}

// New creates a new DNS cache with the given configuration.
func New(cfg Config) *Cache {
	maxEntries := cfg.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 1000000
	}
	minTTL := cfg.MinTTL
	if minTTL <= 0 {
		minTTL = 60
	}
	maxTTL := cfg.MaxTTL
	if maxTTL <= 0 {
		maxTTL = 86400
	}
	negTTL := cfg.NegativeTTL
	if negTTL <= 0 {
		negTTL = 300
	}
	staleTTL := cfg.StaleTTL
	if staleTTL <= 0 {
		staleTTL = 3600
	}

	return &Cache{
		entries:    make(map[string]*list.Element),
		lru:        list.New(),
		maxEntries: maxEntries,
		minTTL:     minTTL,
		maxTTL:     maxTTL,
		negTTL:     negTTL,
		serveStale: cfg.ServeStale,
		staleTTL:   staleTTL,
		prefetch:   cfg.Prefetch,
	}
}

// SetPrefetchCallback sets the callback for prefetching.
func (c *Cache) SetPrefetchCallback(fn func(qname string, qtype uint16)) {
	c.mu.Lock()
	c.onPrefetch = fn
	c.mu.Unlock()
}

// cacheKey generates a cache key from qname and qtype.
func cacheKey(qname string, qtype uint16) string {
	return strings.ToLower(qname) + "/" + dns.TypeToString[qtype]
}

// Get retrieves a cached DNS response.
// Returns the cached message, whether it was a hit, and whether it was served stale.
func (c *Cache) Get(qname string, qtype uint16) (*dns.Msg, bool, bool) {
	key := cacheKey(qname, qtype)

	c.mu.Lock()
	elem, ok := c.entries[key]
	if !ok {
		c.misses++
		c.mu.Unlock()
		return nil, false, false
	}

	item := elem.Value.(*lruItem)
	e := item.entry

	now := time.Now()

	// Check if entry is expired.
	if now.After(e.ExpiresAt) {
		// If serve-stale is enabled and within stale window, return stale entry.
		if c.serveStale && now.Before(e.StaleUntil) {
			c.hits++
			e.HitCount++
			e.LastAccess = now
			// Move to front (still touched).
			c.lru.MoveToFront(elem)

			msg := new(dns.Msg)
			if err := msg.Unpack(e.Msg); err != nil {
				c.mu.Unlock()
				return nil, false, false
			}
			c.mu.Unlock()
			return msg, true, true
		}

		// Entry is expired and not eligible for stale serving.
		c.misses++
		delete(c.entries, key)
		c.lru.Remove(elem)
		c.mu.Unlock()
		return nil, false, false
	}

	// Entry is fresh.
	c.hits++
	e.HitCount++
	e.LastAccess = now
	c.lru.MoveToFront(elem)

	// Snapshot the data we need before potentially dropping the lock
	// during prefetch callback.
	msg := new(dns.Msg)
	origTTL := time.Duration(e.OriginalTTL)
	remaining := e.ExpiresAt.Sub(now)

	// Determine whether to trigger a prefetch. The threshold is "10% of
	// the original TTL remaining". This is the standard interpretation
	// and matches what most resolvers do. Previously the code used
	// (staleWindow + remaining) as the denominator, which made the
	// threshold a function of the stale window rather than the TTL
	// itself, and in practice almost never fired.
	shouldPrefetch := c.prefetch && c.onPrefetch != nil &&
		origTTL > 0 && remaining < origTTL/10 &&
		!e.prefetching.Load()

	callback := c.onPrefetch
	c.mu.Unlock()

	if err := msg.Unpack(e.Msg); err != nil {
		return nil, false, false
	}

	if shouldPrefetch {
		// Claim the prefetch slot atomically. If two goroutines reach
		// this point at the same time, exactly one of them wins the
		// CAS and the other skips the prefetch.
		if e.prefetching.CompareAndSwap(false, true) {
			go func() {
				defer e.prefetching.Store(false)
				callback(qname, qtype)
			}()
		}
	}

	return msg, true, false
}

// Set stores a DNS response in the cache.
func (c *Cache) Set(qname string, qtype uint16, msg *dns.Msg) {
	if msg == nil {
		return
	}

	// Calculate effective TTL from the response.
	ttl := c.effectiveTTL(msg)

	key := cacheKey(qname, qtype)
	msgBytes, err := msg.Pack()
	if err != nil {
		slog.Error("cache: failed to pack DNS message", "error", err)
		return
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(ttl) * time.Second)
	staleUntil := expiresAt.Add(time.Duration(c.staleTTL) * time.Second)

	e := &entry{
		Key:         key,
		QName:       qname,
		QType:       qtype,
		Msg:         msgBytes,
		ExpiresAt:   expiresAt,
		StaleUntil:  staleUntil,
		OriginalTTL: int64(time.Duration(ttl) * time.Second),
		HitCount:    0,
		LastAccess:  now,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict entries if at capacity. The eviction step uses the LRU
	// list to drop entries from the back (oldest first) in O(1) per
	// removal.
	if len(c.entries) >= c.maxEntries {
		c.evict()
	}

	// If the key already exists, remove the old list element so we
	// don't end up with two entries for the same key.
	if old, ok := c.entries[key]; ok {
		c.lru.Remove(old)
	}

	elem := c.lru.PushFront(&lruItem{key: key, entry: e})
	c.entries[key] = elem
}

// effectiveTTL computes the effective TTL for a DNS response.
func (c *Cache) effectiveTTL(msg *dns.Msg) int {
	if len(msg.Answer) == 0 && len(msg.Ns) == 0 {
		// Negative response: only cache NXDOMAIN and NODATA (NOERROR with no answers).
		switch msg.Rcode {
		case dns.RcodeNameError, dns.RcodeSuccess:
			return c.negTTL
		default:
			// SERVFAIL, REFUSED, etc. — do not cache or use very short TTL.
			return 5
		}
	}

	minTTL := uint32(math.MaxUint32)
	for _, rr := range msg.Answer {
		if rr.Header().Ttl < minTTL {
			minTTL = rr.Header().Ttl
		}
	}
	for _, rr := range msg.Ns {
		if rr.Header().Ttl < minTTL {
			minTTL = rr.Header().Ttl
		}
	}

	ttl := int(minTTL)
	if ttl < c.minTTL {
		ttl = c.minTTL
	}
	if ttl > c.maxTTL {
		ttl = c.maxTTL
	}
	return ttl
}

// evict removes the oldest 10% of entries (LRU). The work is O(k)
// where k is the number of evictions, not O(n) over the entire
// cache, because we walk the LRU list from the back instead of doing
// a full sort over the map.
//
// Must be called with c.mu held.
func (c *Cache) evict() {
	toRemove := c.maxEntries / 10
	if toRemove < 1 {
		toRemove = 1
	}
	for i := 0; i < toRemove; i++ {
		elem := c.lru.Back()
		if elem == nil {
			return
		}
		item := elem.Value.(*lruItem)
		delete(c.entries, item.key)
		c.lru.Remove(elem)
	}
}

// Remove removes a specific entry from the cache.
func (c *Cache) Remove(qname string, qtype uint16) {
	key := cacheKey(qname, qtype)
	c.mu.Lock()
	if elem, ok := c.entries[key]; ok {
		delete(c.entries, key)
		c.lru.Remove(elem)
	}
	c.mu.Unlock()
}

// Flush removes all entries from the cache.
func (c *Cache) Flush() {
	c.mu.Lock()
	c.entries = make(map[string]*list.Element)
	c.lru.Init()
	c.hits = 0
	c.misses = 0
	c.mu.Unlock()
}

// Stats returns cache statistics.
func (c *Cache) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var sizeBytes int64
	for _, e := range c.entries {
		item := e.Value.(*lruItem)
		sizeBytes += int64(len(item.entry.Msg))
	}

	total := c.hits + c.misses
	var hitRate float64
	if total > 0 {
		hitRate = float64(c.hits) / float64(total) * 100
	}

	return Stats{
		Entries:   int64(len(c.entries)),
		Hits:      c.hits,
		Misses:    c.misses,
		HitRate:   math.Round(hitRate*100) / 100,
		SizeBytes: sizeBytes,
	}
}

// CleanExpired removes all expired entries.
func (c *Cache) CleanExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	count := 0
	// Walk the LRU list from the back (oldest) forward and remove any
	// entries whose stale window has elapsed. Because the list is
	// ordered by recency, the iteration is well-defined: we visit
	// every entry exactly once and stop as soon as we encounter a
	// still-fresh one.
	for elem := c.lru.Back(); elem != nil; {
		prev := elem.Prev()
		item := elem.Value.(*lruItem)
		if now.After(item.entry.StaleUntil) {
			delete(c.entries, item.key)
			c.lru.Remove(elem)
			count++
		}
		elem = prev
	}
	return count
}

// Entries returns a snapshot of all cache entries for persistence.
// Pointers are returned (rather than values) because the entry struct
// contains a sync/atomic field that must not be copied.
func (c *Cache) Entries() []*entry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*entry, 0, len(c.entries))
	for _, e := range c.entries {
		item := e.Value.(*lruItem)
		result = append(result, item.entry)
	}
	return result
}

// PopularEntries returns the most frequently accessed entries. The
// implementation uses a simple selection sort (the input is typically
// small, and popular-entry queries are infrequent) so we don't need
// an additional heap just for this. Pointers are returned to avoid
// copying the entry's atomic prefetch latch.
func (c *Cache) PopularEntries(n int) []*entry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	type kv struct {
		key   string
		entry *entry
		hits  int64
	}

	sorted := make([]kv, 0, len(c.entries))
	for _, e := range c.entries {
		item := e.Value.(*lruItem)
		sorted = append(sorted, kv{item.key, item.entry, item.entry.HitCount})
	}

	for i := 0; i < n && i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].hits > sorted[i].hits {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	result := make([]*entry, 0, n)
	for i := 0; i < n && i < len(sorted); i++ {
		result = append(result, sorted[i].entry)
	}
	return result
}

// SerializeEntry serializes a cache entry to JSON bytes.
func SerializeEntry(e *entry) ([]byte, error) {
	return json.Marshal(e)
}

// DeserializeEntry deserializes a cache entry from JSON bytes.
func DeserializeEntry(data []byte) (*entry, error) {
	var e entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	// prefetching must start cleared; the in-memory latch is not
	// carried over the wire.
	e.prefetching.Store(false)
	return &e, nil
}
