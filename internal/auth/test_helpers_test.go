package auth

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMain sets GODDI_TEST_MODE=true for the entire test binary so that
// tests which build a JWTManager with a short, non-production secret
// (e.g. "test-secret-key") do not trigger the production secret-length
// check. This is safe because:
//  1. The flag is a no-op in non-test code paths.
//  2. The constant is only consulted inside NewJWTManager.
//  3. Tests that need to exercise the strict check (e.g.
//     TestNewJWTManager_RejectsShortSecret) override the env via
//     t.Setenv, which is scoped to that single test.
func TestMain(m *testing.M) {
	_ = os.Setenv("GODDI_TEST_MODE", "true")
	os.Exit(m.Run())
}

// setupTestDB creates an in-memory SQLite database for testing.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	return db
}
