package cache

import (
	"container/heap"
	"container/list"
	"encoding/json"
	"hash/fnv"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// numShards is the number of independent lock domains. Each shard owns
// its own mutex, map and LRU list so a hot lookup on one shard never
// contends with lookups on the other shards. 32 is a power of two (fast
// modulo) and keeps per-shard memory overhead negligible even for small
// caches.
const numShards = 32

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
// HitRate/MissRate are percentages (0-100) to match the console and the
// /metrics payload.
type Stats struct {
	Entries    int64   `json:"entries"`
	MaxEntries int64   `json:"max_entries"`
	Hits       int64   `json:"hits"`
	Misses     int64   `json:"misses"`
	HitRate    float64 `json:"hit_rate"`
	MissRate   float64 `json:"miss_rate"`
	SizeBytes  int64   `json:"size_bytes"`
}

// lruItem is the value stored in the LRU list. Holding a pointer to
// the entry (rather than copying the struct) means Get/Set/eviction
// can all update the same instance in O(1).
type lruItem struct {
	key   string
	entry *entry
}

// cacheShard is one independently locked slice of the cache. All map
// and LRU-list mutations for keys hashing to this shard happen under
// the shard's own mutex, so cross-shard lookups run fully in parallel.
type cacheShard struct {
	mu         sync.RWMutex
	entries    map[string]*list.Element // key -> list element holding *lruItem
	lru        *list.List               // front = most recent, back = least recent
	maxEntries int                      // per-shard capacity
}

// Cache is a thread-safe, sharded DNS response cache with O(1) LRU
// eviction and an atomic prefetch latch.
//
// Locking model:
//   - each shard's map/list is guarded by that shard's mutex;
//   - the hot-updatable tunables (serve-stale, TTL clamps, prefetch,
//     callback) are guarded by cfgMu and snapshotted by readers;
//   - hits/misses are atomic counters.
type Cache struct {
	shards [numShards]cacheShard

	// shardCount is the number of shards actually in use. Very small
	// caches (maxEntries < numShards) reduce the shard count so the
	// global capacity is still honored (one entry per shard minimum).
	shardCount int

	cfgMu      sync.RWMutex
	minTTL     int
	maxTTL     int
	negTTL     int
	serveStale bool
	staleTTL   int
	prefetch   bool

	// Callback for prefetching; if set, called when an entry is about to expire.
	onPrefetch func(qname string, qtype uint16)

	maxEntries int // total capacity across all shards

	hits   atomic.Int64
	misses atomic.Int64
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

	c := &Cache{
		shardCount: numShards,
		maxEntries: maxEntries,
		minTTL:     minTTL,
		maxTTL:     maxTTL,
		negTTL:     negTTL,
		serveStale: cfg.ServeStale,
		staleTTL:   staleTTL,
		prefetch:   cfg.Prefetch,
	}
	if maxEntries < c.shardCount {
		c.shardCount = maxEntries
	}

	perShard := maxEntries / c.shardCount
	if perShard < 1 {
		perShard = 1
	}
	for i := 0; i < c.shardCount; i++ {
		c.shards[i].entries = make(map[string]*list.Element)
		c.shards[i].lru = list.New()
		c.shards[i].maxEntries = perShard
	}
	return c
}

// shardFor returns the shard owning the given cache key. FNV-1a is
// cheap, well-distributed for short ASCII keys and needs no seed.
func (c *Cache) shardFor(key string) *cacheShard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return &c.shards[h.Sum32()%uint32(c.shardCount)]
}

// tunables is an immutable snapshot of the hot-updatable settings.
type tunables struct {
	serveStale bool
	staleTTL   int
	minTTL     int
	maxTTL     int
	negTTL     int
	prefetch   bool
	onPrefetch func(qname string, qtype uint16)
}

// currentTunables snapshots the tunables under cfgMu so readers never
// access the live fields unsynchronized.
func (c *Cache) currentTunables() tunables {
	c.cfgMu.RLock()
	defer c.cfgMu.RUnlock()
	return tunables{
		serveStale: c.serveStale,
		staleTTL:   c.staleTTL,
		minTTL:     c.minTTL,
		maxTTL:     c.maxTTL,
		negTTL:     c.negTTL,
		prefetch:   c.prefetch,
		onPrefetch: c.onPrefetch,
	}
}

// SetPrefetchCallback sets the callback for prefetching.
func (c *Cache) SetPrefetchCallback(fn func(qname string, qtype uint16)) {
	c.cfgMu.Lock()
	c.onPrefetch = fn
	c.cfgMu.Unlock()
}

// Configure hot-updates the tunable cache settings (Technitium v13/v14
// parity: serve-stale, prefetch and TTL clamps are runtime-adjustable).
// Non-positive values keep the current setting.
func (c *Cache) Configure(serveStale *bool, staleTTL, minTTL, maxTTL int, prefetch *bool) {
	c.cfgMu.Lock()
	defer c.cfgMu.Unlock()
	if serveStale != nil {
		c.serveStale = *serveStale
	}
	if staleTTL > 0 {
		c.staleTTL = staleTTL
	}
	if minTTL > 0 {
		c.minTTL = minTTL
	}
	if maxTTL > 0 {
		c.maxTTL = maxTTL
	}
	if prefetch != nil {
		c.prefetch = *prefetch
	}
}

// cacheKey generates a cache key from qname and qtype.
// Types unknown to dns.TypeToString map to "" which would collide distinct
// query types; fall back to the numeric type in that case.
func cacheKey(qname string, qtype uint16) string {
	typeName := dns.TypeToString[qtype]
	if typeName == "" {
		typeName = "TYPE" + strconv.Itoa(int(qtype))
	}
	return strings.ToLower(qname) + "/" + typeName
}

// Get retrieves a cached DNS response.
// Returns the cached message, whether it was a hit, and whether it was served stale.
func (c *Cache) Get(qname string, qtype uint16) (*dns.Msg, bool, bool) {
	key := cacheKey(qname, qtype)
	t := c.currentTunables()
	s := c.shardFor(key)

	s.mu.Lock()
	elem, ok := s.entries[key]
	if !ok {
		c.misses.Add(1)
		s.mu.Unlock()
		return nil, false, false
	}

	item := elem.Value.(*lruItem)
	e := item.entry

	now := time.Now()

	// Check if entry is expired.
	if now.After(e.ExpiresAt) {
		// If serve-stale is enabled and within stale window, return stale entry.
		if t.serveStale && now.Before(e.StaleUntil) {
			c.hits.Add(1)
			e.HitCount++
			e.LastAccess = now
			// Move to front (still touched).
			s.lru.MoveToFront(elem)

			msg := new(dns.Msg)
			if err := msg.Unpack(e.Msg); err != nil {
				s.mu.Unlock()
				return nil, false, false
			}
			callback := t.onPrefetch
			s.mu.Unlock()

			// RFC 8767 §5: stale answers should carry a short TTL so the
			// client retries quickly instead of pinning the stale data. Keep
			// a shorter original TTL short rather than extending it to 30.
			capTTL(msg, 30)
			// A stale response must prompt a refresh even when proactive
			// prefetching is disabled. The shared latch coalesces concurrent
			// stale hits with any in-flight prefetch for this entry.
			triggerRefresh(e, callback, qname, qtype)
			return msg, true, true
		}

		// Entry is expired and not eligible for stale serving.
		c.misses.Add(1)
		delete(s.entries, key)
		s.lru.Remove(elem)
		s.mu.Unlock()
		return nil, false, false
	}

	// Entry is fresh.
	c.hits.Add(1)
	e.HitCount++
	e.LastAccess = now
	s.lru.MoveToFront(elem)

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
	shouldPrefetch := t.prefetch && t.onPrefetch != nil &&
		origTTL > 0 && remaining < origTTL/10 &&
		!e.prefetching.Load()

	callback := t.onPrefetch
	s.mu.Unlock()

	if err := msg.Unpack(e.Msg); err != nil {
		return nil, false, false
	}

	// RFC 2181 §8: a cache must hand out each RR's remaining TTL, not
	// the TTL observed when the answer was stored. A cache-wide expiry is
	// still an upper bound, but must not extend shorter RRs in any section.
	// insertedAt is derived from ExpiresAt - OriginalTTL (see Set).
	if origTTL > 0 {
		insertedAt := e.ExpiresAt.Add(-origTTL)
		elapsed := now.Sub(insertedAt)
		if elapsed > 0 {
			adjustTTL(msg, elapsed, remaining)
		}
	}

	if shouldPrefetch {
		triggerRefresh(e, callback, qname, qtype)
	}

	return msg, true, false
}

// triggerRefresh starts one background refresh for an entry. Its latch is
// shared by proactive prefetching and stale serving so concurrent callers do
// not issue duplicate refreshes.
func triggerRefresh(e *entry, callback func(qname string, qtype uint16), qname string, qtype uint16) {
	if callback == nil || !e.prefetching.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer e.prefetching.Store(false)
		callback(qname, qtype)
	}()
}

func capTTL(msg *dns.Msg, maximum uint32) {
	for _, rr := range msg.Answer {
		if rr.Header().Ttl > maximum {
			rr.Header().Ttl = maximum
		}
	}
	for _, rr := range msg.Ns {
		if rr.Header().Ttl > maximum {
			rr.Header().Ttl = maximum
		}
	}
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

	t := c.currentTunables()
	now := time.Now()
	expiresAt := now.Add(time.Duration(ttl) * time.Second)
	staleUntil := expiresAt.Add(time.Duration(t.staleTTL) * time.Second)

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

	c.setEntry(e)
}

// setEntry inserts an entry without changing its absolute expiry. It is used
// by persistence loading, where recomputing an expiry would revive old data.
func (c *Cache) setEntry(e *entry) {
	s := c.shardFor(e.Key)
	s.mu.Lock()
	defer s.mu.Unlock()

	// Evict entries if at capacity. The eviction step uses the LRU
	// list to drop entries from the back (oldest first) in O(1) per
	// removal.
	if len(s.entries) >= s.maxEntries {
		s.evict()
	}

	// If the key already exists, remove the old list element so we
	// don't end up with two entries for the same key.
	if old, ok := s.entries[e.Key]; ok {
		s.lru.Remove(old)
	}

	elem := s.lru.PushFront(&lruItem{key: e.Key, entry: e})
	s.entries[e.Key] = elem
}

// adjustTTL reduces every cached RR by elapsed time, capped at the entry's
// remaining lifetime. This preserves each RR's original TTL and prevents a
// shorter RRset from being extended by a longer one. OPT pseudo-records are
// skipped because their TTL field carries EDNS flags, not a lifetime.
func adjustTTL(msg *dns.Msg, elapsed, entryRemaining time.Duration) {
	remaining := durationSeconds(entryRemaining)
	for _, rr := range msg.Answer {
		adjustRRTTL(rr, elapsed, remaining)
	}
	for _, rr := range msg.Ns {
		adjustRRTTL(rr, elapsed, remaining)
	}
	for _, rr := range msg.Extra {
		if rr.Header().Rrtype != dns.TypeOPT {
			adjustRRTTL(rr, elapsed, remaining)
		}
	}
}

func adjustRRTTL(rr dns.RR, elapsed time.Duration, entryRemaining uint32) {
	elapsedSeconds := durationSeconds(elapsed)
	ttl := rr.Header().Ttl
	if elapsedSeconds >= ttl {
		ttl = 0
	} else {
		ttl -= elapsedSeconds
	}
	if ttl > entryRemaining {
		ttl = entryRemaining
	}
	rr.Header().Ttl = ttl
}

func durationSeconds(d time.Duration) uint32 {
	if d <= 0 {
		return 0
	}
	if seconds := d / time.Second; seconds > time.Duration(math.MaxUint32) {
		return math.MaxUint32
	} else {
		return uint32(seconds)
	}
}

// effectiveTTL computes the effective TTL for a DNS response.
func (c *Cache) effectiveTTL(msg *dns.Msg) int {
	t := c.currentTunables()

	if len(msg.Answer) == 0 {
		// Negative response: RFC 2308 requires the authority SOA's TTL and
		// MINIMUM field to bound the cache lifetime. The configured negative
		// TTL is only a fallback when no SOA is present. A NOERROR referral
		// has NS records but no SOA and must retain normal positive handling.
		switch msg.Rcode {
		case dns.RcodeNameError:
			return negativeTTL(msg.Ns, t.negTTL)
		case dns.RcodeSuccess:
			for _, rr := range msg.Ns {
				if _, ok := rr.(*dns.SOA); ok {
					return negativeTTL(msg.Ns, t.negTTL)
				}
			}
			if len(msg.Ns) == 0 {
				return t.negTTL
			}
		default:
			if len(msg.Ns) == 0 {
				// SERVFAIL, REFUSED, etc. — do not cache or use very short TTL.
				return 5
			}
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
	if ttl < t.minTTL {
		ttl = t.minTTL
	}
	if ttl > t.maxTTL {
		ttl = t.maxTTL
	}
	return ttl
}

func negativeTTL(authority []dns.RR, fallback int) int {
	for _, rr := range authority {
		if soa, ok := rr.(*dns.SOA); ok {
			return int(min(soa.Hdr.Ttl, soa.Minttl))
		}
	}
	return fallback
}

// evict removes the oldest 10% of entries (LRU). The work is O(k)
// where k is the number of evictions, not O(n) over the entire
// cache, because we walk the LRU list from the back instead of doing
// a full sort over the map.
//
// Must be called with s.mu held.
func (s *cacheShard) evict() {
	toRemove := s.maxEntries / 10
	if toRemove < 1 {
		toRemove = 1
	}
	for i := 0; i < toRemove; i++ {
		elem := s.lru.Back()
		if elem == nil {
			return
		}
		item := elem.Value.(*lruItem)
		delete(s.entries, item.key)
		s.lru.Remove(elem)
	}
}

// Remove removes a specific entry from the cache.
func (c *Cache) Remove(qname string, qtype uint16) {
	key := cacheKey(qname, qtype)
	s := c.shardFor(key)
	s.mu.Lock()
	if elem, ok := s.entries[key]; ok {
		delete(s.entries, key)
		s.lru.Remove(elem)
	}
	s.mu.Unlock()
}

// Flush removes all entries from the cache.
func (c *Cache) Flush() {
	for i := 0; i < c.shardCount; i++ {
		s := &c.shards[i]
		s.mu.Lock()
		s.entries = make(map[string]*list.Element)
		s.lru.Init()
		s.mu.Unlock()
	}
	c.hits.Store(0)
	c.misses.Store(0)
}

// Stats returns cache statistics.
func (c *Cache) Stats() Stats {
	var sizeBytes int64
	var entries int64
	for i := 0; i < c.shardCount; i++ {
		s := &c.shards[i]
		s.mu.RLock()
		for _, e := range s.entries {
			item := e.Value.(*lruItem)
			sizeBytes += int64(len(item.entry.Msg))
		}
		entries += int64(len(s.entries))
		s.mu.RUnlock()
	}

	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses
	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}

	return Stats{
		Entries:    entries,
		MaxEntries: int64(c.maxEntries),
		Hits:       hits,
		Misses:     misses,
		HitRate:    math.Round(hitRate*100) / 100,
		MissRate:   math.Round((100-hitRate)*100) / 100,
		SizeBytes:  sizeBytes,
	}
}

// CleanExpired removes all expired entries.
func (c *Cache) CleanExpired() int {
	now := time.Now()
	count := 0
	for i := 0; i < c.shardCount; i++ {
		s := &c.shards[i]
		s.mu.Lock()
		// Walk the LRU list from the back (oldest) forward and remove any
		// entries whose stale window has elapsed. Because the list is
		// ordered by recency, the iteration is well-defined: we visit
		// every entry exactly once and stop as soon as we encounter a
		// still-fresh one.
		for elem := s.lru.Back(); elem != nil; {
			prev := elem.Prev()
			item := elem.Value.(*lruItem)
			if now.After(item.entry.StaleUntil) {
				delete(s.entries, item.key)
				s.lru.Remove(elem)
				count++
			}
			elem = prev
		}
		s.mu.Unlock()
	}
	return count
}

// Entries returns a snapshot of all cache entries for persistence.
// Pointers are returned (rather than values) because the entry struct
// contains a sync/atomic field that must not be copied.
func (c *Cache) Entries() []*entry {
	var result []*entry
	for i := 0; i < c.shardCount; i++ {
		s := &c.shards[i]
		s.mu.RLock()
		for _, e := range s.entries {
			item := e.Value.(*lruItem)
			result = append(result, item.entry)
		}
		s.mu.RUnlock()
	}
	return result
}

// EntryInfo is a JSON-friendly snapshot of one cache entry for the
// management API.
type EntryInfo struct {
	QName      string    `json:"qname"`
	QType      string    `json:"qtype"`
	TTLLeft    int       `json:"ttl_left"`
	ExpiresAt  time.Time `json:"expires_at"`
	StaleUntil time.Time `json:"stale_until"`
	HitCount   int64     `json:"hit_count"`
	LastAccess time.Time `json:"last_access"`
	SizeBytes  int       `json:"size_bytes"`
}

// ListEntries returns a filtered, paginated snapshot of cache entries.
// qnameFilter and qtypeFilter are substring/type-name filters ("" = all).
// Entries are sorted by hit count descending so the hottest records come
// first. The scan is bounded by maxScan to keep the O(n) walk cheap on
// very large caches.
func (c *Cache) ListEntries(qnameFilter, qtypeFilter string, limit, offset int) ([]EntryInfo, int) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	qnameFilter = strings.ToLower(qnameFilter)
	qtypeFilter = strings.ToUpper(qtypeFilter)

	const maxScan = 200000
	scanned := 0

	all := make([]EntryInfo, 0, 256)
	for i := 0; i < c.shardCount; i++ {
		s := &c.shards[i]
		s.mu.RLock()
		for _, e := range s.entries {
			item := e.Value.(*lruItem)
			if scanned++; scanned > maxScan {
				break
			}
			name := dns.TypeToString[item.entry.QType]
			if qtypeFilter != "" && name != qtypeFilter {
				continue
			}
			if qnameFilter != "" && !strings.Contains(strings.ToLower(item.entry.QName), qnameFilter) {
				continue
			}
			all = append(all, EntryInfo{
				QName:      strings.TrimSuffix(item.entry.QName, "."),
				QType:      name,
				TTLLeft:    int(time.Until(item.entry.ExpiresAt).Seconds()),
				ExpiresAt:  item.entry.ExpiresAt,
				StaleUntil: item.entry.StaleUntil,
				HitCount:   item.entry.HitCount,
				LastAccess: item.entry.LastAccess,
				SizeBytes:  len(item.entry.Msg),
			})
		}
		s.mu.RUnlock()
		if scanned > maxScan {
			break
		}
	}

	total := len(all)
	// O(n log n) sort of the (already filtered, typically small) slice.
	sort.Slice(all, func(i, j int) bool {
		if all[i].HitCount != all[j].HitCount {
			return all[i].HitCount > all[j].HitCount
		}
		return all[i].QName < all[j].QName
	})

	if offset >= total {
		return []EntryInfo{}, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total
}

// hitKV is one candidate for the PopularEntries top-n selection.
type hitKV struct {
	key   string
	entry *entry
	hits  int64
}

// topNHeap is a min-heap keyed on hit count: the root is the *least*
// popular of the current top-n candidates, so an incoming entry only
// needs one O(log n) comparison against the root. Selecting the top n
// out of m entries therefore costs O(m log n) instead of the previous
// selection sort's O(n·m) — for m=1M, n=10 that is ~20M vs ~10G ops.
type topNHeap []hitKV

func (h topNHeap) Len() int            { return len(h) }
func (h topNHeap) Less(i, j int) bool  { return h[i].hits < h[j].hits }
func (h topNHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *topNHeap) Push(x interface{}) { *h = append(*h, x.(hitKV)) }
func (h *topNHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// PopularEntries returns the n most frequently accessed entries. It
// maintains a bounded min-heap while walking every shard once, so the
// whole selection is O(m log n) over m total entries. Pointers are
// returned to avoid copying the entry's atomic prefetch latch.
func (c *Cache) PopularEntries(n int) []*entry {
	if n <= 0 {
		return nil
	}

	h := &topNHeap{}
	for i := 0; i < c.shardCount; i++ {
		s := &c.shards[i]
		s.mu.RLock()
		for _, e := range s.entries {
			item := e.Value.(*lruItem)
			cand := hitKV{item.key, item.entry, item.entry.HitCount}
			if h.Len() < n {
				heap.Push(h, cand)
			} else if cand.hits > (*h)[0].hits {
				heap.Pop(h)
				heap.Push(h, cand)
			}
		}
		s.mu.RUnlock()
	}

	result := make([]*entry, 0, h.Len())
	for h.Len() > 0 {
		result = append(result, heap.Pop(h).(hitKV).entry)
	}
	// heap.Pop yields ascending hits; reverse to most-popular-first.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
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
