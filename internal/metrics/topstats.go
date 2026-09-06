package metrics

import (
	"sync"
	"time"
)

// TopEntry is one aggregated Top-N row.
type TopEntry struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// TopResult holds the three Top-N lists for one time range.
type TopResult struct {
	Range      string     `json:"range"`
	TopClients []TopEntry `json:"top_clients"`
	TopDomains []TopEntry `json:"top_domains"`
	TopBlocked []TopEntry `json:"top_blocked"`
}

const (
	topHourBuckets = 60  // per-minute buckets for the last hour
	topDayBuckets  = 24  // per-hour buckets for the last day
	topWeekBuckets = 7   // per-day buckets for the last week
	topMaxEntries  = 100 // per-bucket map cap guard
)

// topBucket aggregates counts for one time slice.
type topBucket struct {
	clients map[string]int64
	domains map[string]int64
	blocked map[string]int64
}

func newTopBucket() *topBucket {
	return &topBucket{
		clients: make(map[string]int64),
		domains: make(map[string]int64),
		blocked: make(map[string]int64),
	}
}

// TopStats is an in-memory sliding-window counter for dashboard Top-N
// statistics. It keeps three granularities: per-minute (last hour),
// per-hour (last day) and per-day (last week). Data is lost on restart;
// the DNS query log table remains the authoritative long-term record.
type TopStats struct {
	mu      sync.Mutex
	minutes []topSlot // ring buffer, 1-minute slots
	hours   []topSlot // ring buffer, 1-hour slots
	days    []topSlot // ring buffer, 1-day slots
}

type topSlot struct {
	key    string // bucket identifier (minute/hour/day start)
	bucket *topBucket
}

// TopStatsGlobal is the process-wide TopStats instance.
var TopStatsGlobal = NewTopStats()

// NewTopStats creates an initialized TopStats.
func NewTopStats() *TopStats {
	return &TopStats{
		minutes: make([]topSlot, topHourBuckets),
		hours:   make([]topSlot, topDayBuckets),
		days:    make([]topSlot, topWeekBuckets),
	}
}

// Record attributes one DNS query to the current minute/hour/day buckets.
// It is called from the DNS hot path and must stay cheap.
func (ts *TopStats) Record(clientIP, qname string, blocked bool) {
	now := time.Now()
	ts.mu.Lock()
	defer ts.mu.Unlock()

	mb := ts.slot(&ts.minutes, now.Format("2006-01-02T15:04"))
	hb := ts.slot(&ts.hours, now.Format("2006-01-02T15"))
	db := ts.slot(&ts.days, now.Format("2006-01-02"))

	for _, b := range []*topBucket{mb, hb, db} {
		if clientIP != "" {
			b.clients[clientIP]++
		}
		if qname != "" {
			b.domains[qname]++
		}
		if blocked {
			if clientIP != "" {
				b.blocked[clientIP]++
			}
			if qname != "" {
				b.blocked[qname]++
			}
		}
	}
}

// slot returns the bucket for key inside ring, reusing or rotating slots.
// The newest slot is always kept at the last index. Must be called with
// ts.mu held.
func (ts *TopStats) slot(ring *[]topSlot, key string) *topBucket {
	slots := *ring
	if slots[len(slots)-1].key == key {
		return slots[len(slots)-1].bucket
	}

	// Find an existing slot with this key (bucket reuse within the ring)
	// and move it to the end.
	for i := range slots {
		if slots[i].key == key {
			s := slots[i]
			copy(slots[i:], slots[i+1:])
			slots[len(slots)-1] = s
			return s.bucket
		}
	}

	// New key: drop the oldest slot (index 0), shift left, append new.
	newSlot := topSlot{key: key, bucket: newTopBucket()}
	copy(slots, slots[1:])
	slots[len(slots)-1] = newSlot
	return newSlot.bucket
}

// Top computes the Top-N lists for the requested range
// ("hour", "day" or "week").
func (ts *TopStats) Top(rangeName string, limit int) TopResult {
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()

	result := TopResult{Range: rangeName}

	var agg *topBucket
	switch rangeName {
	case "hour":
		agg = mergeSlots(ts.minutes)
	case "day":
		agg = mergeSlots(ts.hours)
	case "week":
		agg = mergeSlots(ts.days)
	default:
		agg = mergeSlots(ts.minutes)
	}

	result.TopClients = topN(agg.clients, limit)
	result.TopDomains = topN(agg.domains, limit)
	result.TopBlocked = topN(agg.blocked, limit)
	return result
}

// mergeSlots sums every non-empty slot in the ring.
func mergeSlots(slots []topSlot) *topBucket {
	agg := newTopBucket()
	for _, s := range slots {
		if s.bucket == nil {
			continue
		}
		for k, v := range s.bucket.clients {
			agg.clients[k] += v
		}
		for k, v := range s.bucket.domains {
			agg.domains[k] += v
		}
		for k, v := range s.bucket.blocked {
			agg.blocked[k] += v
		}
	}
	return agg
}

// topN returns the highest n entries of m.
func topN(m map[string]int64, n int) []TopEntry {
	if len(m) == 0 {
		return []TopEntry{}
	}
	entries := make([]TopEntry, 0, len(m))
	for k, v := range m {
		entries = append(entries, TopEntry{Name: k, Count: v})
	}
	// Partial selection sort; n is small (<=50) so this is fine.
	for i := 0; i < n && i < len(entries); i++ {
		max := i
		for j := i + 1; j < len(entries); j++ {
			if entries[j].Count > entries[max].Count {
				max = j
			}
		}
		entries[i], entries[max] = entries[max], entries[i]
	}
	if len(entries) > n {
		entries = entries[:n]
	}
	return entries
}
