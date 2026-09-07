package auth

import (
	"testing"
	"time"
)

func TestNewRateLimiter_Defaults(t *testing.T) {
	t.Parallel()

	// Test with zero maxAttempts and zero lockDuration - should use defaults
	rl := NewRateLimiter(nil, 0, 0)
	if rl.maxAttempts != 5 {
		t.Errorf("default maxAttempts = %d, want 5", rl.maxAttempts)
	}
	if rl.lockDuration != 15*time.Minute {
		t.Errorf("default lockDuration = %v, want 15m", rl.lockDuration)
	}
}

func TestNewRateLimiter_CustomValues(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(nil, 3, 5*time.Minute)
	if rl.maxAttempts != 3 {
		t.Errorf("maxAttempts = %d, want 3", rl.maxAttempts)
	}
	if rl.lockDuration != 5*time.Minute {
		t.Errorf("lockDuration = %v, want 5m", rl.lockDuration)
	}
}

func TestNewRateLimiter_NegativeValues(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(nil, -1, -1*time.Second)
	if rl.maxAttempts != 5 {
		t.Errorf("negative maxAttempts should default to 5, got %d", rl.maxAttempts)
	}
	if rl.lockDuration != 15*time.Minute {
		t.Errorf("negative lockDuration should default to 15m, got %v", rl.lockDuration)
	}
}

func TestCheckLoginRate_NoRecord(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(nil, 5, 15*time.Minute)

	allowed, err := rl.CheckLoginRate("testuser", "192.168.1.1")
	if err != nil {
		t.Fatalf("CheckLoginRate() error = %v", err)
	}
	if !allowed {
		t.Error("CheckLoginRate() should allow when no record exists")
	}
}

func TestRecordFailedLogin_AndCheck(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(nil, 3, 15*time.Minute)

	// First two attempts should be allowed
	for i := 0; i < 2; i++ {
		allowed, _ := rl.CheckLoginRate("testuser", "192.168.1.1")
		if !allowed {
			t.Errorf("attempt %d should be allowed", i+1)
		}
		rl.RecordFailedLogin("testuser", "192.168.1.1")
	}

	// Third attempt: count is 2, max is 3, should still be allowed
	allowed, _ := rl.CheckLoginRate("testuser", "192.168.1.1")
	if !allowed {
		t.Error("attempt 3 should still be allowed (count=2, max=3)")
	}

	// Record the third failed login
	rl.RecordFailedLogin("testuser", "192.168.1.1")

	// Fourth attempt should be blocked (count=3 >= max=3)
	allowed, _ = rl.CheckLoginRate("testuser", "192.168.1.1")
	if allowed {
		t.Error("attempt 4 should be blocked after 3 failed attempts")
	}
}

func TestCheckLoginRate_DifferentUsers(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(nil, 2, 15*time.Minute)

	// User1 fails 2 times
	rl.RecordFailedLogin("user1", "10.0.0.1")
	rl.RecordFailedLogin("user1", "10.0.0.1")

	// User2 should still be allowed
	allowed, _ := rl.CheckLoginRate("user2", "10.0.0.2")
	if !allowed {
		t.Error("user2 should be allowed (independent rate limiting)")
	}
}

func TestCheckLoginRate_SameUserDifferentIP(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(nil, 2, 15*time.Minute)

	// User from IP1 fails 2 times
	rl.RecordFailedLogin("user1", "10.0.0.1")
	rl.RecordFailedLogin("user1", "10.0.0.1")

	// Same user from IP2 should still be allowed
	allowed, _ := rl.CheckLoginRate("user1", "10.0.0.2")
	if !allowed {
		t.Error("same user from different IP should be allowed")
	}
}

func TestResetLoginAttempts(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(nil, 2, 15*time.Minute)

	// Fail enough times to get locked out
	rl.RecordFailedLogin("testuser", "192.168.1.1")
	rl.RecordFailedLogin("testuser", "192.168.1.1")

	// Should be blocked now
	allowed, _ := rl.CheckLoginRate("testuser", "192.168.1.1")
	if allowed {
		t.Error("should be blocked after 2 failed attempts")
	}

	// Reset
	err := rl.ResetLoginAttempts("testuser", "192.168.1.1")
	if err != nil {
		t.Fatalf("ResetLoginAttempts() error = %v", err)
	}

	// Should be allowed again
	allowed, _ = rl.CheckLoginRate("testuser", "192.168.1.1")
	if !allowed {
		t.Error("should be allowed after reset")
	}
}

func TestRateLimiter_Key(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(nil, 5, 15*time.Minute)
	key := rl.key("user", "1.2.3.4")
	expected := "user:1.2.3.4"
	if key != expected {
		t.Errorf("key() = %q, want %q", key, expected)
	}
}

func TestRateLimiter_WithDatabase(t *testing.T) {
	// Test with in-memory SQLite database
	db := setupTestDB(t)
	defer db.Close()

	// Create rate limit table
	if err := EnsureRateLimitTable(db); err != nil {
		t.Fatalf("EnsureRateLimitTable() error = %v", err)
	}

	rl := NewRateLimiter(db, 3, 15*time.Minute)

	// Record failed logins
	for i := 0; i < 3; i++ {
		rl.RecordFailedLogin("dbuser", "10.0.0.1")
	}

	// Should be blocked
	allowed, err := rl.CheckLoginRate("dbuser", "10.0.0.1")
	if err != nil {
		t.Fatalf("CheckLoginRate() error = %v", err)
	}
	if allowed {
		t.Error("should be blocked after 3 failed attempts with database")
	}

	// Reset and verify
	rl.ResetLoginAttempts("dbuser", "10.0.0.1")
	allowed, _ = rl.CheckLoginRate("dbuser", "10.0.0.1")
	if !allowed {
		t.Error("should be allowed after reset with database")
	}
}

func TestEnsureRateLimitTable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	err := EnsureRateLimitTable(db)
	if err != nil {
		t.Fatalf("EnsureRateLimitTable() error = %v", err)
	}

	// Verify table exists by inserting a row
	_, err = db.Exec(`INSERT INTO login_rate_limits (key, attempt_count) VALUES (?, ?)`, "test:1.2.3.4", 1)
	if err != nil {
		t.Fatalf("failed to insert into login_rate_limits table: %v", err)
	}

	// Calling EnsureRateLimitTable again should not error (CREATE IF NOT EXISTS)
	err = EnsureRateLimitTable(db)
	if err != nil {
		t.Fatalf("EnsureRateLimitTable() on existing table error = %v", err)
	}
}

func TestResetUserAttempts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	if err := EnsureRateLimitTable(db); err != nil {
		t.Fatalf("EnsureRateLimitTable() error = %v", err)
	}

	rl := NewRateLimiter(db, 2, 15*time.Minute)

	// Lock the user from two different IPs.
	for i := 0; i < 2; i++ {
		_ = rl.RecordFailedLogin("victim", "10.0.0.1")
		_ = rl.RecordFailedLogin("victim", "10.0.0.2")
	}
	// Another user must not be affected later.
	for i := 0; i < 2; i++ {
		_ = rl.RecordFailedLogin("other", "10.0.0.3")
	}

	locked, err := rl.ListLockedEntries()
	if err != nil {
		t.Fatalf("ListLockedEntries() error = %v", err)
	}
	if len(locked) != 3 {
		t.Fatalf("ListLockedEntries() = %d entries, want 3", len(locked))
	}

	removed, err := rl.ResetUserAttempts("victim")
	if err != nil {
		t.Fatalf("ResetUserAttempts() error = %v", err)
	}
	if removed < 2 {
		t.Errorf("ResetUserAttempts() removed %d entries, want >= 2", removed)
	}

	// The victim can log in again from both IPs.
	for _, ip := range []string{"10.0.0.1", "10.0.0.2"} {
		allowed, err := rl.CheckLoginRate("victim", ip)
		if err != nil {
			t.Fatalf("CheckLoginRate() error = %v", err)
		}
		if !allowed {
			t.Errorf("user should be unlocked for ip %s", ip)
		}
	}

	// Other users stay locked.
	remaining, err := rl.ListLockedEntries()
	if err != nil {
		t.Fatalf("ListLockedEntries() error = %v", err)
	}
	if len(remaining) != 1 || remaining[0].Username != "other" {
		t.Errorf("unexpected remaining lockouts: %+v", remaining)
	}
}

func TestResetUserAttempts_IPv6Key(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	if err := EnsureRateLimitTable(db); err != nil {
		t.Fatalf("EnsureRateLimitTable() error = %v", err)
	}

	rl := NewRateLimiter(db, 1, 15*time.Minute)
	_ = rl.RecordFailedLogin("v6user", "fe80::1")

	locked, err := rl.ListLockedEntries()
	if err != nil {
		t.Fatalf("ListLockedEntries() error = %v", err)
	}
	if len(locked) != 1 {
		t.Fatalf("ListLockedEntries() = %d entries, want 1", len(locked))
	}
	if locked[0].Username != "v6user" || locked[0].IP != "fe80::1" {
		t.Errorf("splitLockKey round-trip failed: %+v", locked[0])
	}

	if _, err := rl.ResetUserAttempts("v6user"); err != nil {
		t.Fatalf("ResetUserAttempts() error = %v", err)
	}
	allowed, _ := rl.CheckLoginRate("v6user", "fe80::1")
	if !allowed {
		t.Error("IPv6-keyed lockout should be cleared by username reset")
	}
}

func TestResetAllLockouts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	if err := EnsureRateLimitTable(db); err != nil {
		t.Fatalf("EnsureRateLimitTable() error = %v", err)
	}

	rl := NewRateLimiter(db, 1, 15*time.Minute)
	_ = rl.RecordFailedLogin("a", "10.0.0.1")
	_ = rl.RecordFailedLogin("b", "10.0.0.2")

	removed, err := rl.ResetAllLockouts()
	if err != nil {
		t.Fatalf("ResetAllLockouts() error = %v", err)
	}
	if removed < 2 {
		t.Errorf("ResetAllLockouts() removed %d, want >= 2", removed)
	}

	locked, _ := rl.ListLockedEntries()
	if len(locked) != 0 {
		t.Errorf("lockouts remain after ResetAllLockouts: %+v", locked)
	}
}
