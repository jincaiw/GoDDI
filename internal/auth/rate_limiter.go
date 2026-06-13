package auth

import (
	"database/sql"
	"fmt"
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

	// Try database first. SQLite/MySQL provide their own concurrency
	// control, so we don't need the in-process mutex for the DB path; that
	// way DB latency doesn't block unrelated callers.
	if rl.db != nil {
		var count int
		err := rl.db.QueryRow(`SELECT attempt_count FROM login_rate_limits WHERE key = ?`, key).Scan(&count)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("querying rate limit: %w", err)
		}

		count++
		var execErr error
		if count >= rl.maxAttempts {
			lockedUntil := time.Now().Add(rl.lockDuration).Format(time.RFC3339)
			_, execErr = rl.db.Exec(`
				INSERT INTO login_rate_limits (key, attempt_count, locked_until, updated_at)
				VALUES (?, ?, ?, ?)
				ON CONFLICT(key) DO UPDATE SET attempt_count = ?, locked_until = ?, updated_at = ?`,
				key, count, lockedUntil, time.Now().Format(time.RFC3339),
				count, lockedUntil, time.Now().Format(time.RFC3339),
			)
		} else {
			_, execErr = rl.db.Exec(`
				INSERT INTO login_rate_limits (key, attempt_count, updated_at)
				VALUES (?, ?, ?)
				ON CONFLICT(key) DO UPDATE SET attempt_count = ?, updated_at = ?`,
				key, count, time.Now().Format(time.RFC3339),
				count, time.Now().Format(time.RFC3339),
			)
		}
		if execErr != nil {
			return fmt.Errorf("updating rate limit: %w", execErr)
		}
		return nil
	}

	// Fallback to in-memory
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rec, exists := rl.memoryAttempts[key]
	if !exists {
		rec = &attemptRecord{}
		rl.memoryAttempts[key] = rec
	}
	rec.count++
	if rec.count >= rl.maxAttempts {
		rec.lockedUntil = time.Now().Add(rl.lockDuration)
	}

	return nil
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

func (rl *RateLimiter) key(username, ip string) string {
	return fmt.Sprintf("%s:%s", username, ip)
}

// cleanupExpiredMemory removes expired entries from the in-memory fallback map.
// Must be called with rl.mu held.
func (rl *RateLimiter) cleanupExpiredMemory() {
	now := time.Now()
	for k, rec := range rl.memoryAttempts {
		if !rec.lockedUntil.IsZero() && now.After(rec.lockedUntil) {
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
