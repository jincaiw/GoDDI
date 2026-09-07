package auth

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/google/uuid"
)

// CheckIfInitialized checks if any admin user exists in the database.
func CheckIfInitialized(db *sql.DB) bool {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE enabled = 1`).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// InitializeAdmin creates the first admin user.
// This should only be called if no users exist yet.
//
// TOCTOU: callers (e.g. AutoInitializeAdmin) must still consult
// CheckIfInitialized first for the common "already initialized" case, but
// the actual creation is performed inside a SERIALIZABLE-like
// transaction that atomically re-checks user count and inserts the admin
// user, preventing two concurrent initializations from both succeeding.
func InitializeAdmin(db *sql.DB, username, password string) error {
	if username == "" || password == "" {
		return fmt.Errorf("username and password are required")
	}

	// Validate password complexity
	if err := ValidatePasswordComplexity(password); err != nil {
		return fmt.Errorf("password does not meet complexity requirements: %w", err)
	}

	// Hash the password
	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	// Begin an IMMEDIATE transaction so SQLite acquires the RESERVED lock
	// up front. This prevents two concurrent InitializeAdmin calls from
	// each seeing "no users" and both inserting. For non-SQLite drivers
	// the Begin semantics still give us a single atomic unit.
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Atomically re-check inside the transaction: only proceed if there
	// are still no users. INSERT OR IGNORE is not used here because we
	// also need the uniqueness check to apply to the entire initial
	// bootstrap. This guarded INSERT prevents duplicate admin creation.
	var existingCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&existingCount); err != nil {
		return fmt.Errorf("checking existing users: %w", err)
	}
	if existingCount > 0 {
		return fmt.Errorf("system already initialized")
	}

	// Get the admin role ID within the transaction
	adminRoleID := getAdminRoleIDTx(tx)
	if adminRoleID == "" {
		return fmt.Errorf("admin role not found")
	}

	// Create the admin user
	userID := uuid.New().String()
	_, err = tx.Exec(`
		INSERT INTO users (id, username, email, password_hash, display_name, enabled, must_change_password, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		userID, username, "", hash, "Administrator", true, false,
	)
	if err != nil {
		return fmt.Errorf("creating admin user: %w", err)
	}

	// Assign the admin role
	_, err = tx.Exec(`
		INSERT INTO user_roles (user_id, role_id, created_at)
		VALUES (?, ?, datetime('now'))`,
		userID, adminRoleID,
	)
	if err != nil {
		return fmt.Errorf("assigning admin role: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	committed = true
	return nil
}

// AutoInitializeAdmin checks environment variables and creates the initial admin user
// if no users exist and the GODDI_ADMIN_USERNAME/GODDI_ADMIN_PASSWORD env vars are set.
func AutoInitializeAdmin(db *sql.DB) error {
	if CheckIfInitialized(db) {
		return nil
	}

	username := os.Getenv("GODDI_ADMIN_USERNAME")
	password := os.Getenv("GODDI_ADMIN_PASSWORD")

	if username == "" || password == "" {
		return nil // Not configured, skip auto-initialization
	}

	return InitializeAdmin(db, username, password)
}

// getAdminRoleIDTx returns the ID of the admin role using a transaction.
func getAdminRoleIDTx(tx *sql.Tx) string {
	var id string
	err := tx.QueryRow(`SELECT id FROM roles WHERE name = 'admin' AND is_builtin = 1`).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}
