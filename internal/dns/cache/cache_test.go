package cache

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func newTestCache() *Cache {
	return New(Config{
		Enabled:     true,
		MaxEntries:  100,
		MinTTL:      5,
		MaxTTL:      3600,
		NegativeTTL: 60,
		ServeStale:  true,
		StaleTTL:    300,
		Prefetch:    false,
	})
}

func newDNSMsg(name string, qtype uint16, ttl uint32, answers []string) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(name, qtype)
	m.SetReply(m)
	for _, ans := range answers {
		switch qtype {
		case dns.TypeA:
			rr := &dns.A{
				Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl},
				A:   net.ParseIP(ans),
			}
			m.Answer = append(m.Answer, rr)
		case dns.TypeAAAA:
			rr := &dns.AAAA{
				Hdr:  dns.RR_Header{Name: name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: ttl},
				AAAA: net.ParseIP(ans),
			}
			m.Answer = append(m.Answer, rr)
		}
	}
	return m
}

func TestNew_DefaultConfig(t *testing.T) {
	t.Parallel()

	c := New(Config{})
	if c.maxEntries != 1000000 {
		t.Errorf("default maxEntries = %d, want 1000000", c.maxEntries)
	}
	if c.minTTL != 60 {
		t.Errorf("default minTTL = %d, want 60", c.minTTL)
	}
	if c.maxTTL != 86400 {
		t.Errorf("default maxTTL = %d, want 86400", c.maxTTL)
	}
	if c.negTTL != 300 {
		t.Errorf("default negTTL = %d, want 300", c.negTTL)
	}
	if c.staleTTL != 3600 {
		t.Errorf("default staleTTL = %d, want 3600", c.staleTTL)
	}
}

func TestCache_SetAndGet(t *testing.T) {
	c := newTestCache()

	msg := newDNSMsg("example.com.", dns.TypeA, 300, []string{"1.2.3.4"})
	c.Set("example.com.", dns.TypeA, msg)

	got, hit, stale := c.Get("example.com.", dns.TypeA)
	if !hit {
		t.Error("Get() should return hit=true for cached entry")
	}
	if stale {
		t.Error("Get() should return stale=false for fresh entry")
	}
	if got == nil {
		t.Fatal("Get() should return non-nil message")
	}
	if len(got.Answer) != 1 {
		t.Errorf("Get() returned %d answers, want 1", len(got.Answer))
	}
}

func TestCache_GetMiss(t *testing.T) {
	c := newTestCache()

	got, hit, stale := c.Get("notcached.example.com.", dns.TypeA)
	if hit {
		t.Error("Get() should return hit=false for missing entry")
	}
	if stale {
		t.Error("Get() should return stale=false for missing entry")
	}
	if got != nil {
		t.Error("Get() should return nil for missing entry")
	}
}

func TestCache_SetNil(t *testing.T) {
	c := newTestCache()

	// Setting nil should not panic and should not add an entry
	c.Set("example.com.", dns.TypeA, nil)

	got, hit, _ := c.Get("example.com.", dns.TypeA)
	if hit {
		t.Error("Get() should return hit=false after Set(nil)")
	}
	if got != nil {
		t.Error("Get() should return nil after Set(nil)")
	}
}

func TestCache_Remove(t *testing.T) {
	c := newTestCache()

	msg := newDNSMsg("example.com.", dns.TypeA, 300, []string{"1.2.3.4"})
	c.Set("example.com.", dns.TypeA, msg)

	c.Remove("example.com.", dns.TypeA)

	got, hit, _ := c.Get("example.com.", dns.TypeA)
	if hit {
		t.Error("Get() should return hit=false after Remove()")
	}
	if got != nil {
		t.Error("Get() should return nil after Remove()")
	}
}

func TestCache_Flush(t *testing.T) {
	c := newTestCache()

	msg1 := newDNSMsg("a.example.com.", dns.TypeA, 300, []string{"1.2.3.4"})
	msg2 := newDNSMsg("b.example.com.", dns.TypeA, 300, []string{"5.6.7.8"})
	c.Set("a.example.com.", dns.TypeA, msg1)
	c.Set("b.example.com.", dns.TypeA, msg2)

	c.Flush()

	stats := c.Stats()
	if stats.Entries != 0 {
		t.Errorf("after Flush(), entries = %d, want 0", stats.Entries)
	}
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Errorf("after Flush(), hits=%d misses=%d, want 0,0", stats.Hits, stats.Misses)
	}
}

func TestCache_Stats(t *testing.T) {
	c := newTestCache()

	// Miss
	c.Get("missing.example.com.", dns.TypeA)

	// Set and hit
	msg := newDNSMsg("example.com.", dns.TypeA, 300, []string{"1.2.3.4"})
	c.Set("example.com.", dns.TypeA, msg)
	c.Get("example.com.", dns.TypeA)

	stats := c.Stats()
	if stats.Entries != 1 {
		t.Errorf("Entries = %d, want 1", stats.Entries)
	}
	if stats.Hits != 1 {
		t.Errorf("Hits = %d, want 1", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Misses = %d, want 1", stats.Misses)
	}
	if stats.HitRate != 50.0 {
		t.Errorf("HitRate = %f, want 50.0", stats.HitRate)
	}
}

func TestCache_CaseInsensitive(t *testing.T) {
	c := newTestCache()

	msg := newDNSMsg("Example.COM.", dns.TypeA, 300, []string{"1.2.3.4"})
	c.Set("Example.COM.", dns.TypeA, msg)

	// Lookup with different case should still hit
	got, hit, _ := c.Get("example.com.", dns.TypeA)
	if !hit {
		t.Error("Get() should be case-insensitive")
	}
	if got == nil {
		t.Fatal("Get() should return non-nil for case-insensitive match")
	}
}

func TestCache_DifferentTypes(t *testing.T) {
	c := newTestCache()

	msgA := newDNSMsg("example.com.", dns.TypeA, 300, []string{"1.2.3.4"})
	msgAAAA := newDNSMsg("example.com.", dns.TypeAAAA, 300, []string{"::1"})
	c.Set("example.com.", dns.TypeA, msgA)
	c.Set("example.com.", dns.TypeAAAA, msgAAAA)

	gotA, hitA, _ := c.Get("example.com.", dns.TypeA)
	gotAAAA, hitAAAA, _ := c.Get("example.com.", dns.TypeAAAA)

	if !hitA {
		t.Error("A record should be cached separately")
	}
	if !hitAAAA {
		t.Error("AAAA record should be cached separately")
	}
	if gotA == nil || gotAAAA == nil {
		t.Error("both records should be retrievable")
	}
}

func TestCache_EffectiveTTL_MinTTL(t *testing.T) {
	c := New(Config{
		MinTTL: 60,
		MaxTTL: 3600,
	})

	// Response with TTL below minTTL should be clamped to minTTL
	msg := newDNSMsg("example.com.", dns.TypeA, 5, []string{"1.2.3.4"})
	c.Set("example.com.", dns.TypeA, msg)

	stats := c.Stats()
	if stats.Entries != 1 {
		t.Error("entry should be stored")
	}

	// The entry should have TTL of at least minTTL (60s)
	// We can verify by checking it's still available immediately
	got, hit, _ := c.Get("example.com.", dns.TypeA)
	if !hit || got == nil {
		t.Error("entry with clamped TTL should be available")
	}
}

func TestCache_EffectiveTTL_NegativeResponse(t *testing.T) {
	c := New(Config{
		NegativeTTL: 30,
		MinTTL:      5,
		MaxTTL:      3600,
	})

	// Negative response (no answers)
	msg := new(dns.Msg)
	msg.SetQuestion("nxdomain.example.com.", dns.TypeA)
	msg.SetRcode(msg, dns.RcodeNameError)

	c.Set("nxdomain.example.com.", dns.TypeA, msg)

	got, hit, _ := c.Get("nxdomain.example.com.", dns.TypeA)
	if !hit {
		t.Error("negative response should be cached")
	}
	if got == nil {
		t.Fatal("Get() should return non-nil for cached negative response")
	}
}

func TestCache_ServeStale(t *testing.T) {
	c := New(Config{
		MinTTL:      1,
		MaxTTL:      1,
		NegativeTTL: 1,
		ServeStale:  true,
		StaleTTL:    300,
	})

	msg := newDNSMsg("stale.example.com.", dns.TypeA, 1, []string{"1.2.3.4"})
	c.Set("stale.example.com.", dns.TypeA, msg)

	// Wait for entry to expire
	time.Sleep(1100 * time.Millisecond)

	// Should serve stale since ServeStale is enabled
	got, hit, stale := c.Get("stale.example.com.", dns.TypeA)
	if !hit {
		t.Error("should serve stale entry when ServeStale is enabled")
	}
	if !stale {
		t.Error("stale flag should be true for expired entry served from stale cache")
	}
	if got == nil {
		t.Fatal("Get() should return non-nil for stale entry")
	}
}

func TestCache_NoServeStale(t *testing.T) {
	c := New(Config{
		MinTTL:      1,
		MaxTTL:      1,
		NegativeTTL: 1,
		ServeStale:  false,
		StaleTTL:    300,
	})

	msg := newDNSMsg("nostale.example.com.", dns.TypeA, 1, []string{"1.2.3.4"})
	c.Set("nostale.example.com.", dns.TypeA, msg)

	// Wait for entry to expire
	time.Sleep(1100 * time.Millisecond)

	// Should NOT serve stale since ServeStale is disabled
	got, hit, stale := c.Get("nostale.example.com.", dns.TypeA)
	if hit {
		t.Error("should NOT serve stale entry when ServeStale is disabled")
	}
	if stale {
		t.Error("stale flag should be false when ServeStale is disabled")
	}
	if got != nil {
		t.Error("Get() should return nil for expired entry without ServeStale")
	}
}

func TestCache_CleanExpired(t *testing.T) {
	c := New(Config{
		MinTTL:      1,
		MaxTTL:      1,
		NegativeTTL: 1,
		StaleTTL:    1,
	})

	msg := newDNSMsg("expired.example.com.", dns.TypeA, 1, []string{"1.2.3.4"})
	c.Set("expired.example.com.", dns.TypeA, msg)

	// Wait for entry to fully expire (including stale window)
	time.Sleep(2500 * time.Millisecond)

	removed := c.CleanExpired()
	if removed != 1 {
		t.Errorf("CleanExpired() removed %d entries, want 1", removed)
	}

	stats := c.Stats()
	if stats.Entries != 0 {
		t.Errorf("after CleanExpired(), entries = %d, want 0", stats.Entries)
	}
}

func TestCache_Entries(t *testing.T) {
	c := newTestCache()

	msg1 := newDNSMsg("a.example.com.", dns.TypeA, 300, []string{"1.2.3.4"})
	msg2 := newDNSMsg("b.example.com.", dns.TypeA, 300, []string{"5.6.7.8"})
	c.Set("a.example.com.", dns.TypeA, msg1)
	c.Set("b.example.com.", dns.TypeA, msg2)

	entries := c.Entries()
	if len(entries) != 2 {
		t.Errorf("Entries() returned %d entries, want 2", len(entries))
	}
}

func TestCache_PopularEntries(t *testing.T) {
	c := newTestCache()

	msg1 := newDNSMsg("popular.example.com.", dns.TypeA, 300, []string{"1.2.3.4"})
	msg2 := newDNSMsg("unpopular.example.com.", dns.TypeA, 300, []string{"5.6.7.8"})
	c.Set("popular.example.com.", dns.TypeA, msg1)
	c.Set("unpopular.example.com.", dns.TypeA, msg2)

	// Access popular entry multiple times
	for i := 0; i < 10; i++ {
		c.Get("popular.example.com.", dns.TypeA)
	}
	c.Get("unpopular.example.com.", dns.TypeA)

	popular := c.PopularEntries(1)
	if len(popular) != 1 {
		t.Fatalf("PopularEntries(1) returned %d entries, want 1", len(popular))
	}
	if popular[0].QName != "popular.example.com." {
		t.Errorf("most popular entry = %q, want %q", popular[0].QName, "popular.example.com.")
	}
}

func TestSerializeAndDeserializeEntry(t *testing.T) {
	now := time.Now()
	e := &entry{
		Key:        "example.com./A",
		QName:      "example.com.",
		QType:      dns.TypeA,
		Msg:        []byte{0x01, 0x02, 0x03},
		ExpiresAt:  now,
		StaleUntil: now.Add(5 * time.Minute),
		HitCount:   42,
		LastAccess: now,
	}

	data, err := SerializeEntry(e)
	if err != nil {
		t.Fatalf("SerializeEntry() error = %v", err)
	}

	decoded, err := DeserializeEntry(data)
	if err != nil {
		t.Fatalf("DeserializeEntry() error = %v", err)
	}

	if decoded.Key != e.Key {
		t.Errorf("Key = %q, want %q", decoded.Key, e.Key)
	}
	if decoded.QType != e.QType {
		t.Errorf("QType = %d, want %d", decoded.QType, e.QType)
	}
	if decoded.HitCount != e.HitCount {
		t.Errorf("HitCount = %d, want %d", decoded.HitCount, e.HitCount)
	}
}

func TestDeserializeEntry_InvalidJSON(t *testing.T) {
	_, err := DeserializeEntry([]byte("invalid json"))
	if err == nil {
		t.Error("DeserializeEntry() should return error for invalid JSON")
	}
}

func TestCache_Eviction(t *testing.T) {
	// Create a cache with very small max entries
	c := New(Config{
		MaxEntries: 5,
		MinTTL:     300,
		MaxTTL:     3600,
	})

	// Add more entries than maxEntries
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("host%d.example.com.", i)
		msg := newDNSMsg(name, dns.TypeA, 300, []string{fmt.Sprintf("1.2.3.%d", i)})
		c.Set(name, dns.TypeA, msg)
	}

	stats := c.Stats()
	// After eviction, entries should be at or below maxEntries
	if stats.Entries > 5 {
		t.Errorf("entries = %d, should be at or below maxEntries=5 after eviction", stats.Entries)
	}
}
