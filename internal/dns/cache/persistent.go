package cache

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// PersistentCache wraps Cache with SQLite persistence.
type PersistentCache struct {
	*Cache
	db        *sql.DB
	quit      chan struct{}
	done      chan struct{}
	flushMu   sync.Mutex
	closeOnce sync.Once
}

// NewPersistentCache creates a cache that persists to SQLite.
func NewPersistentCache(cache *Cache, db *sql.DB) *PersistentCache {
	pc := &PersistentCache{
		Cache: cache,
		db:    db,
		quit:  make(chan struct{}),
		done:  make(chan struct{}),
	}

	// Load existing entries from database.
	pc.load()

	// Start periodic flush.
	go pc.flushLoop()

	return pc
}

// load reads cached entries from the database.
func (pc *PersistentCache) load() {
	if pc.db == nil {
		return
	}

	rows, err := pc.db.Query(`
		SELECT query_name, query_type, response_data, ttl, expires_at
		FROM dns_cache
		WHERE expires_at > datetime('now')
	`)
	if err != nil {
		slog.Error("persistent_cache: failed to load entries", "error", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var qname, qtypeStr, responseData string
		var ttl int
		var expiresAtStr string

		if err := rows.Scan(&qname, &qtypeStr, &responseData, &ttl, &expiresAtStr); err != nil {
			slog.Error("persistent_cache: failed to scan entry", "error", err)
			continue
		}

		qtype, ok := dns.StringToType[qtypeStr]
		if !ok {
			continue
		}

		msg := new(dns.Msg)
		if err := msg.Unpack([]byte(responseData)); err != nil {
			continue
		}

		expiresAt, err := time.ParseInLocation("2006-01-02 15:04:05", expiresAtStr, time.UTC)
		if err != nil || !expiresAt.After(time.Now()) {
			continue
		}
		t := pc.Cache.currentTunables()
		pc.Cache.setEntry(&entry{
			Key:         cacheKey(qname, qtype),
			QName:       qname,
			QType:       qtype,
			Msg:         []byte(responseData),
			ExpiresAt:   expiresAt,
			StaleUntil:  expiresAt.Add(time.Duration(t.staleTTL) * time.Second),
			OriginalTTL: int64(time.Duration(ttl) * time.Second),
			LastAccess:  time.Now(),
		})
		count++
	}
	if err := rows.Err(); err != nil {
		slog.Warn("persistent_cache: failed to iterate entries", "error", err)
	}

	slog.Info("persistent_cache: loaded entries", "count", count)
}

// flushLoop periodically writes cache entries to the database.
func (pc *PersistentCache) flushLoop() {
	defer close(pc.done)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := pc.flush(); err != nil {
				slog.Error("persistent_cache: periodic flush failed", "error", err)
			}
		case <-pc.quit:
			// Final flush before shutdown.
			if err := pc.flush(); err != nil {
				slog.Error("persistent_cache: final flush failed", "error", err)
			}
			return
		}
	}
}

// flush writes all cache entries to the database.
func (pc *PersistentCache) flush() error {
	if pc.db == nil {
		return nil
	}

	pc.flushMu.Lock()
	defer pc.flushMu.Unlock()

	entries := pc.Cache.Entries()

	tx, err := pc.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Persistence is an exact snapshot. Removing all rows first prevents
	// entries deleted from memory (for example by Flush) from reappearing at
	// the next process start.
	if _, err := tx.Exec(`DELETE FROM dns_cache`); err != nil {
		return fmt.Errorf("clear entries: %w", err)
	}

	if len(entries) == 0 {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit empty snapshot: %w", err)
		}
		return nil
	}

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO dns_cache (id, query_name, query_type, response_data, ttl, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	now := time.Now()
	for _, e := range entries {
		// Only persist entries that haven't expired yet.
		if now.After(e.ExpiresAt) {
			continue
		}

		ttl := int(e.ExpiresAt.Sub(now).Seconds())
		if ttl <= 0 {
			continue
		}

		qtypeStr := dns.TypeToString[e.QType]
		id := e.Key // Use cache key as ID for simplicity

		if _, err := stmt.Exec(id, e.QName, qtypeStr, string(e.Msg), ttl, e.ExpiresAt.UTC().Format("2006-01-02 15:04:05")); err != nil {
			return fmt.Errorf("upsert entry %q: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	slog.Debug("persistent_cache: flushed entries", "count", len(entries))
	return nil
}

// Flush removes all in-memory entries and persists the empty snapshot so
// cleared entries cannot be loaded again after a restart.
func (pc *PersistentCache) Flush() error {
	pc.Cache.Flush()
	return pc.flush()
}

// Remove deletes a cache entry from memory and SQLite without taking a full
// persistence snapshot. It is intended for explicit management operations;
// ordinary internal evictions continue to affect memory only until the next
// periodic snapshot.
func (pc *PersistentCache) Remove(qname string, qtype uint16) error {
	pc.flushMu.Lock()
	defer pc.flushMu.Unlock()

	pc.Cache.Remove(qname, qtype)
	if pc.db == nil {
		return nil
	}
	if _, err := pc.db.Exec(`DELETE FROM dns_cache WHERE id = ?`, cacheKey(qname, qtype)); err != nil {
		return fmt.Errorf("delete entry: %w", err)
	}
	return nil
}

// Close stops the persistent cache and performs a final flush.
func (pc *PersistentCache) Close() {
	pc.closeOnce.Do(func() {
		close(pc.quit)
		<-pc.done // Wait for flushLoop goroutine to finish.
	})
}
