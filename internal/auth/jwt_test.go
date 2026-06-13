package auth

import (
	"testing"
	"time"
)

// withTestMode is a no-op marker for readability. TestMain in
// test_helpers_test.go sets GODDI_TEST_MODE=true for the entire test
// binary so that tests building a JWTManager with a short secret
// (e.g. "test-secret-key") do not trigger the production
// secret-length check. Tests that need to exercise the strict check
// (e.g. TestNewJWTManager_RejectsShortSecret) override the env via
// t.Setenv, which is scoped to that single test.
func withTestMode(t *testing.T) {
	t.Helper()
}

func mustJWTManager(t *testing.T, secret string) *JWTManager {
	t.Helper()
	m, err := NewJWTManager(secret)
	if err != nil {
		t.Fatalf("NewJWTManager(%q) error = %v", secret, err)
	}
	return m
}

func TestNewJWTManager(t *testing.T) {
	withTestMode(t)
	manager := mustJWTManager(t, "test-secret-key")
	if manager == nil {
		t.Fatal("NewJWTManager() returned nil")
	}
}

func TestNewJWTManager_RejectsShortSecret(t *testing.T) {
	// Ensure neither opt-in is set so we exercise the new strict check.
	// Use t.Setenv (not manual cleanup) so the previous value is restored
	// when the test ends without racing against parallel tests.
	t.Setenv("GODDI_TEST_MODE", "")
	t.Setenv("GODDI_ALLOW_WEAK_JWT_SECRET", "")

	if _, err := NewJWTManager("short"); err == nil {
		t.Fatal("NewJWTManager should reject short secret without opt-in")
	}

	// With opt-in it should be allowed. Re-enable the env var explicitly
	// here because TestMain's value was overridden by the t.Setenv above.
	t.Setenv("GODDI_TEST_MODE", "true")
	if _, err := NewJWTManager("short"); err != nil {
		t.Fatalf("NewJWTManager should accept short secret with opt-in: %v", err)
	}
}

func TestGenerateAndParseToken(t *testing.T) {
	t.Parallel()
	withTestMode(t)

	manager := mustJWTManager(t, "test-secret-key")

	userID := "user-123"
	username := "testuser"
	roleIDs := []string{"role-admin", "role-editor"}
	sessionID := "session-abc"

	tokenString, err := manager.GenerateToken(userID, username, roleIDs, sessionID)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	if tokenString == "" {
		t.Fatal("GenerateToken() returned empty token")
	}

	claims, err := manager.ParseToken(tokenString)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("Claims.UserID = %q, want %q", claims.UserID, userID)
	}
	if claims.Username != username {
		t.Errorf("Claims.Username = %q, want %q", claims.Username, username)
	}
	if claims.SessionID != sessionID {
		t.Errorf("Claims.SessionID = %q, want %q", claims.SessionID, sessionID)
	}
	if len(claims.RoleIDs) != len(roleIDs) {
		t.Errorf("Claims.RoleIDs length = %d, want %d", len(claims.RoleIDs), len(roleIDs))
	}
	for i, rid := range roleIDs {
		if claims.RoleIDs[i] != rid {
			t.Errorf("Claims.RoleIDs[%d] = %q, want %q", i, claims.RoleIDs[i], rid)
		}
	}
	if claims.Issuer != "goddi" {
		t.Errorf("Claims.Issuer = %q, want %q", claims.Issuer, "goddi")
	}
	if claims.Subject != userID {
		t.Errorf("Claims.Subject = %q, want %q", claims.Subject, userID)
	}
}

func TestParseToken_WrongSecret(t *testing.T) {
	t.Parallel()
	withTestMode(t)

	manager1 := mustJWTManager(t, "secret-1-secret-aaa")
	manager2 := mustJWTManager(t, "secret-2-secret-bbb")

	tokenString, err := manager1.GenerateToken("user-1", "test", nil, "sess-1")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	_, err = manager2.ParseToken(tokenString)
	if err == nil {
		t.Error("ParseToken() should fail with wrong secret key")
	}
}

func TestParseToken_InvalidToken(t *testing.T) {
	t.Parallel()
	withTestMode(t)

	manager := mustJWTManager(t, "test-secret-key")

	tests := []struct {
		name  string
		token string
	}{
		{"empty string", ""},
		{"random string", "not.a.valid.token"},
		{"malformed JWT", "header.payload"},
		{"completely invalid", "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := manager.ParseToken(tt.token)
			if err == nil {
				t.Errorf("ParseToken(%q) should return error for invalid token", tt.token)
			}
		})
	}
}

func TestParseToken_ExpiredToken(t *testing.T) {
	withTestMode(t)
	// We cannot easily test expired tokens without modifying the code,
	// but we can verify that the token has the correct expiration time.
	manager := mustJWTManager(t, "test-secret-key")

	tokenString, err := manager.GenerateToken("user-1", "test", nil, "sess-1")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	claims, err := manager.ParseToken(tokenString)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}

	// Verify expiration is approximately 2 hours from now
	expiresAt := claims.ExpiresAt.Time
	expectedExpiry := time.Now().Add(AccessTokenDuration)
	diff := expiresAt.Sub(expectedExpiry)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Minute {
		t.Errorf("token expiration differs from expected by %v", diff)
	}
}

func TestGenerateToken_EmptyRoleIDs(t *testing.T) {
	t.Parallel()
	withTestMode(t)

	manager := mustJWTManager(t, "test-secret-key")

	tokenString, err := manager.GenerateToken("user-1", "test", nil, "sess-1")
	if err != nil {
		t.Fatalf("GenerateToken() with nil roleIDs error = %v", err)
	}

	claims, err := manager.ParseToken(tokenString)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}

	if claims.RoleIDs != nil {
		t.Errorf("Claims.RoleIDs should be nil, got %v", claims.RoleIDs)
	}
}

func TestGenerateToken_EmptySessionID(t *testing.T) {
	withTestMode(t)

	manager := mustJWTManager(t, "test-secret-key")

	tokenString, err := manager.GenerateToken("user-1", "test", []string{"role-1"}, "")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	claims, err := manager.ParseToken(tokenString)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}

	if claims.SessionID != "" {
		t.Errorf("Claims.SessionID should be empty, got %q", claims.SessionID)
	}
}

func TestGenerateToken_MultipleTokens(t *testing.T) {
	t.Parallel()
	withTestMode(t)

	manager := mustJWTManager(t, "test-secret-key")

	token1, _ := manager.GenerateToken("user-1", "test1", nil, "sess-1")
	token2, _ := manager.GenerateToken("user-2", "test2", nil, "sess-2")

	if token1 == token2 {
		t.Error("different users should get different tokens")
	}
}
