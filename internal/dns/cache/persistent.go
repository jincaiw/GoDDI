package cache

import (
	"database/sql"
	"log/slog"
	"time"

	"github.com/miekg/dns"
)

// PersistentCache wraps Cache with SQLite persistence.
type PersistentCache struct {
	*Cache
	db   *sql.DB
	path string
	quit chan struct{}
	done chan struct{}
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

		pc.Cache.Set(qname, qtype, msg)
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
			pc.flush()
		case <-pc.quit:
			// Final flush before shutdown.
			pc.flush()
			return
		}
	}
}

// flush writes all cache entries to the database.
func (pc *PersistentCache) flush() {
	if pc.db == nil {
		return
	}

	entries := pc.Cache.Entries()
	if len(entries) == 0 {
		return
	}

	tx, err := pc.db.Begin()
	if err != nil {
		slog.Error("persistent_cache: failed to begin transaction", "error", err)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO dns_cache (id, query_name, query_type, response_data, ttl, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		slog.Error("persistent_cache: failed to prepare insert", "error", err)
		return
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
			slog.Error("persistent_cache: failed to upsert entry", "error", err)
			continue
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("persistent_cache: failed to commit transaction", "error", err)
		return
	}

	slog.Debug("persistent_cache: flushed entries", "count", len(entries))
}

// Close stops the persistent cache and performs a final flush.
func (pc *PersistentCache) Close() {
	close(pc.quit)
	<-pc.done // Wait for flushLoop goroutine to finish.
}
