package auth

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// RateLimiter manages login attempt tracking and rate limiting.
type RateLimiter struct {
	db           *sql.DB
	maxAttempts  int
	lockDuration time.Duration
	mu           sync.Mutex
	// In-memory fallback for when database is not available
	memoryAttempts map[string]*attemptRecord
}

type attemptRecord struct {
	count       int
	lockedUntil time.Time
	// lastSeen tracks the most recent activity for this key so entries for
	// keys that never reach the lockout threshold (lockedUntil zero) can
	// still be garbage-collected; otherwise an attacker can grow the map
	// without bound by sending failures for arbitrary username/IP pairs.
	lastSeen time.Time
}

// NewRateLimiter creates a new RateLimiter.
func NewRateLimiter(db *sql.DB, maxAttempts int, lockDuration time.Duration) *RateLimiter {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if lockDuration <= 0 {
		lockDuration = 15 * time.Minute
	}
	return &RateLimiter{
		db:             db,
		maxAttempts:    maxAttempts,
		lockDuration:   lockDuration,
		memoryAttempts: make(map[string]*attemptRecord),
	}
}

// CheckLoginRate checks if a login attempt is allowed for the given username and IP.
// Returns true if the attempt is allowed, false if rate limited.
func (rl *RateLimiter) CheckLoginRate(username, ip string) (bool, error) {
	key := rl.key(username, ip)

	// Try database first (without holding the in-process mutex so DB latency
	// does not serialize unrelated callers).
	if rl.db != nil {
		allowed, locked, err := rl.queryRateLimit(key)
		if err != nil {
			// Database query failed; fall back to in-memory rate limiting
			// instead of blocking login entirely.
			rl.mu.Lock()
			defer rl.mu.Unlock()
			rl.cleanupExpiredMemory()
			rec, exists := rl.memoryAttempts[key]
			if !exists {
				return true, nil
			}
			if !rec.lockedUntil.IsZero() && time.Now().Before(rec.lockedUntil) {
				return false, nil
			}
			if rec.count >= rl.maxAttempts {
				delete(rl.memoryAttempts, key)
			}
			return true, nil
		}
		if locked {
			return false, nil
		}
		if !allowed {
			return false, nil
		}
		// Allowed by DB. Now check the in-memory fallback in case a previous
		// request populated it when the DB was unavailable.
		rl.mu.Lock()
		defer rl.mu.Unlock()
		if rec, ok := rl.memoryAttempts[key]; ok {
			if !rec.lockedUntil.IsZero() && time.Now().Before(rec.lockedUntil) {
				return false, nil
			}
			if rec.count >= rl.maxAttempts {
				delete(rl.memoryAttempts, key)
				return false, nil
			}
		}
		return true, nil
	}

	// No DB configured; use in-memory state.
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.cleanupExpiredMemory()
	rec, exists := rl.memoryAttempts[key]
	if !exists {
		return true, nil
	}

	if !rec.lockedUntil.IsZero() && time.Now().Before(rec.lockedUntil) {
		return false, nil
	}

	if rec.count >= rl.maxAttempts {
		// Lock expired, allow retry
		delete(rl.memoryAttempts, key)
	}

	return true, nil
}

// queryRateLimit reads the current rate-limit state for a key from the DB.
// Returns (allowed, locked, err). sql.ErrNoRows is treated as a brand-new key
// (allowed, not locked, no error) and is NOT silently swallowed as a DB
// failure. A real DB error is returned to the caller.
func (rl *RateLimiter) queryRateLimit(key string) (allowed bool, locked bool, err error) {
	var count int
	var lockedUntil sql.NullString
	qerr := rl.db.QueryRow(`
		SELECT attempt_count, locked_until FROM login_rate_limits WHERE key = ?`,
		key,
	).Scan(&count, &lockedUntil)

	if qerr == sql.ErrNoRows {
		// Brand-new key: not currently rate-limited and allowed.
		return true, false, nil
	}
	if qerr != nil {
		return false, false, fmt.Errorf("querying rate limit: %w", qerr)
	}

	if lockedUntil.Valid {
		lt, parseErr := time.Parse(time.RFC3339, lockedUntil.String)
		if parseErr == nil && time.Now().Before(lt) {
			return false, true, nil
		}
		// Lock has expired; reset the counter so the user can try again.
		if _, execErr := rl.db.Exec(`UPDATE login_rate_limits SET attempt_count = 0, locked_until = NULL WHERE key = ?`, key); execErr != nil {
			return false, false, fmt.Errorf("resetting expired rate limit: %w", execErr)
		}
		count = 0
	}

	if count >= rl.maxAttempts {
		return false, false, nil
	}
	return true, false, nil
}

// RecordFailedLogin records a failed login attempt.
func (rl *RateLimiter) RecordFailedLogin(username, ip string) error {
	key := rl.key(username, ip)

	// Database path: use a single atomic UPSERT so concurrent failed logins
	// cannot overwrite each other's counters (a read-modify-write across two
	// statements allowed N parallel requests to all record count=N/2 and
	// bypass lockout). The lock is engaged inside the same statement the
	// moment attempt_count reaches maxAttempts.
	if rl.db != nil {
		now := time.Now().Format(time.RFC3339)
		lockedUntil := time.Now().Add(rl.lockDuration).Format(time.RFC3339)
		_, execErr := rl.db.Exec(`
			INSERT INTO login_rate_limits (key, attempt_count, locked_until, updated_at)
			VALUES (?, 1, CASE WHEN 1 >= ? THEN ? ELSE NULL END, ?)
			ON CONFLICT(key) DO UPDATE SET
				attempt_count = attempt_count + 1,
				locked_until = CASE
					WHEN locked_until IS NOT NULL AND julianday(locked_until) > julianday('now') THEN locked_until
					WHEN attempt_count + 1 >= ? THEN ?
					ELSE locked_until
				END,
				updated_at = ?`,
			key, rl.maxAttempts, lockedUntil, now, rl.maxAttempts, lockedUntil, now,
		)
		if execErr != nil {
			return fmt.Errorf("updating rate limit: %w", execErr)
		}
		rl.warnIfLocked(key, username, ip)
		return nil
	}

	// Fallback to in-memory
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.cleanupExpiredMemory()
	rec, exists := rl.memoryAttempts[key]
	if !exists {
		rec = &attemptRecord{}
		rl.memoryAttempts[key] = rec
	}
	rec.count++
	rec.lastSeen = time.Now()
	if rec.count >= rl.maxAttempts {
		rec.lockedUntil = time.Now().Add(rl.lockDuration)
		slog.Warn("login lockout engaged", "username", username, "ip", ip,
			"attempts", rec.count, "locked_until", rec.lockedUntil.Format(time.RFC3339))
	}

	return nil
}

// warnIfLocked logs a warning the moment a DB-tracked lockout engages, so
// operators can tell "user mistyped the password" from "someone is hammering
// this account" in the request logs.
func (rl *RateLimiter) warnIfLocked(key, username, ip string) {
	var attempts int
	var lockedUntil sql.NullString
	if err := rl.db.QueryRow(`SELECT attempt_count, locked_until FROM login_rate_limits WHERE key = ?`, key).
		Scan(&attempts, &lockedUntil); err != nil {
		return
	}
	if lockedUntil.Valid && attempts >= rl.maxAttempts {
		if until := parseLockTime(lockedUntil); !until.IsZero() && time.Now().Before(until) {
			slog.Warn("login lockout engaged", "username", username, "ip", ip,
				"attempts", attempts, "locked_until", until.Format(time.RFC3339))
		}
	}
}

// ResetLoginAttempts resets the login attempt counter for a username and IP.
func (rl *RateLimiter) ResetLoginAttempts(username, ip string) error {
	key := rl.key(username, ip)

	if rl.db != nil {
		if _, err := rl.db.Exec(`DELETE FROM login_rate_limits WHERE key = ?`, key); err != nil {
			return fmt.Errorf("resetting rate limit: %w", err)
		}
	}

	rl.mu.Lock()
	delete(rl.memoryAttempts, key)
	rl.mu.Unlock()
	return nil
}

// ResetUserAttempts clears every rate-limit entry recorded for username,
// across all source IPs. It backs the admin unlock API and the
// `goddi unlock` CLI command; without it, an operator locked out by an
// attacker hammering their username had no recovery path short of editing
// the database directly.
// Returns the number of entries removed.
func (rl *RateLimiter) ResetUserAttempts(username string) (int64, error) {
	var removed int64

	if rl.db != nil {
		res, err := rl.db.Exec(`DELETE FROM login_rate_limits WHERE key = ? OR key LIKE ?`,
			username+":", username+":%")
		if err != nil {
			return 0, fmt.Errorf("resetting rate limit for user: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil {
			removed = n
		}
	}

	rl.mu.Lock()
	prefix := username + ":"
	for k := range rl.memoryAttempts {
		if strings.HasPrefix(k, prefix) {
			delete(rl.memoryAttempts, k)
			removed++
		}
	}
	rl.mu.Unlock()

	slog.Warn("login lockout cleared by operator", "username", username, "entries", removed)
	return removed, nil
}

// ResetAllLockouts clears every rate-limit entry for all users. Intended for
// the `goddi unlock --all` CLI escape hatch.
func (rl *RateLimiter) ResetAllLockouts() (int64, error) {
	var removed int64

	if rl.db != nil {
		res, err := rl.db.Exec(`DELETE FROM login_rate_limits`)
		if err != nil {
			return 0, fmt.Errorf("resetting all lockouts: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil {
			removed = n
		}
	}

	rl.mu.Lock()
	removed += int64(len(rl.memoryAttempts))
	rl.memoryAttempts = make(map[string]*attemptRecord)
	rl.mu.Unlock()

	slog.Warn("all login lockouts cleared by operator", "entries", removed)
	return removed, nil
}

// LockedEntry describes a currently locked username/IP pair.
type LockedEntry struct {
	Username    string    `json:"username"`
	IP          string    `json:"ip"`
	Attempts    int       `json:"attempts"`
	LockedUntil time.Time `json:"locked_until"`
}

// ListLockedEntries returns entries that are currently locked out, for the
// admin unlock API and the `goddi unlock` CLI listing.
func (rl *RateLimiter) ListLockedEntries() ([]LockedEntry, error) {
	now := time.Now()
	out := make([]LockedEntry, 0)

	if rl.db != nil {
		rows, err := rl.db.Query(`SELECT key, attempt_count, locked_until FROM login_rate_limits WHERE locked_until IS NOT NULL`)
		if err != nil {
			return nil, fmt.Errorf("listing lockouts: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var key string
			var attempts int
			var lockedUntil sql.NullString
			if err := rows.Scan(&key, &attempts, &lockedUntil); err != nil {
				continue
			}
			until := parseLockTime(lockedUntil)
			if until.IsZero() || !now.Before(until) {
				continue
			}
			username, ip := splitLockKey(key)
			out = append(out, LockedEntry{Username: username, IP: ip, Attempts: attempts, LockedUntil: until})
		}
	}

	rl.mu.Lock()
	for k, rec := range rl.memoryAttempts {
		if !rec.lockedUntil.IsZero() && now.Before(rec.lockedUntil) {
			username, ip := splitLockKey(k)
			out = append(out, LockedEntry{Username: username, IP: ip, Attempts: rec.count, LockedUntil: rec.lockedUntil})
		}
	}
	rl.mu.Unlock()

	return out, nil
}

// splitLockKey splits "username:ip" on the FIRST colon. Usernames are
// identifiers and never contain colons, while IPv6 addresses do — splitting
// on the first colon keeps "v6user:fe80::1" intact as ("v6user", "fe80::1").
func splitLockKey(key string) (username, ip string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return key, ""
}

// parseLockTime parses the RFC3339 timestamp stored in locked_until.
func parseLockTime(s sql.NullString) time.Time {
	if !s.Valid || s.String == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s.String)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (rl *RateLimiter) key(username, ip string) string {
	return fmt.Sprintf("%s:%s", username, ip)
}

// cleanupExpiredMemory removes expired entries from the in-memory fallback map:
// records whose lockout has passed, and records idle long enough that they can
// never contribute to a fresh lockout. Must be called with rl.mu held.
func (rl *RateLimiter) cleanupExpiredMemory() {
	now := time.Now()
	for k, rec := range rl.memoryAttempts {
		if !rec.lockedUntil.IsZero() && now.After(rec.lockedUntil) {
			delete(rl.memoryAttempts, k)
			continue
		}
		// Idle entries: no activity for 2x the lock window means the
		// attempt series has ended; drop it to bound memory usage.
		if now.Sub(rec.lastSeen) > 2*rl.lockDuration {
			delete(rl.memoryAttempts, k)
		}
	}
}

// EnsureRateLimitTable creates the login_rate_limits table if it does not exist.
func EnsureRateLimitTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS login_rate_limits (
			key TEXT PRIMARY KEY,
			attempt_count INTEGER NOT NULL DEFAULT 0,
			locked_until DATETIME,
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`)
	if err != nil {
		return fmt.Errorf("creating login_rate_limits table: %w", err)
	}
	return nil
}
