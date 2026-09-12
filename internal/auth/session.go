package auth

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Session represents an active user session.
//
// The two account fields are filled in only by GetSessionByID, which is the
// lookup the request path uses. Their zero values are the restrictive ones —
// a Session assembled anywhere else reads as "account disabled, password must
// change" — so a caller that forgets to look them up fails closed rather than
// open.
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TokenHash string    `json:"-"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`

	// MustChangePassword mirrors users.must_change_password. While it is set,
	// the session may only reach the account-security endpoints.
	MustChangePassword bool `json:"must_change_password"`

	// UserEnabled mirrors users.enabled. A session belonging to a disabled
	// account is refused immediately rather than living out its expiry.
	UserEnabled bool `json:"user_enabled"`
}

// SessionManager manages user sessions in the database.
type SessionManager struct {
	db *sql.DB
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager(db *sql.DB) *SessionManager {
	return &SessionManager{db: db}
}

// CreateSession creates a new session for a user and returns both the
// session record (with the stored token hash) and the raw session token
// (which the caller should hand to the client exactly once).
func (sm *SessionManager) CreateSession(userID, ipAddress, userAgent string) (rawToken string, session *Session, err error) {
	sessionID := uuid.New().String()
	token := uuid.New().String() // This is the raw session token
	tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))

	now := time.Now()
	expiresAt := now.Add(RefreshTokenDuration)

	_, dbErr := sm.db.Exec(`
		INSERT INTO sessions (id, user_id, token_hash, ip_address, user_agent, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sessionID, userID, tokenHash, ipAddress, userAgent, expiresAt.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if dbErr != nil {
		return "", nil, fmt.Errorf("inserting session: %w", dbErr)
	}

	session = &Session{
		ID:        sessionID,
		UserID:    userID,
		TokenHash: tokenHash,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}

	// Fill in the account flags the request path will enforce. Without this, a
	// freshly created session would describe itself more restrictively than the
	// session the next request reads back, and the caller would have no way to
	// tell whether the account is allowed to proceed.
	if err := sm.db.QueryRow(
		`SELECT COALESCE(must_change_password, 0), COALESCE(enabled, 0) FROM users WHERE id = ?`, userID,
	).Scan(&session.MustChangePassword, &session.UserEnabled); err != nil {
		return "", nil, fmt.Errorf("reading account flags for new session: %w", err)
	}

	return token, session, nil
}

// ValidateSession validates a session by its token hash.
func (sm *SessionManager) ValidateSession(token string) (*Session, error) {
	tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))

	var s Session
	var expiresAt, createdAt string

	err := sm.db.QueryRow(`
		SELECT id, user_id, token_hash, ip_address, user_agent, expires_at, created_at
		FROM sessions WHERE token_hash = ?`,
		tokenHash,
	).Scan(&s.ID, &s.UserID, &s.TokenHash, &s.IPAddress, &s.UserAgent, &expiresAt, &createdAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying session: %w", err)
	}

	s.ExpiresAt, err = time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		slog.Warn("parsing expires_at for session", "error", err)
	}
	s.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		slog.Warn("parsing created_at for session", "error", err)
	}

	if time.Now().After(s.ExpiresAt) {
		// Clean up expired session
		if _, err := sm.db.Exec("DELETE FROM sessions WHERE id = ?", s.ID); err != nil {
			slog.Error("deleting expired session", "error", err)
		}
		return nil, fmt.Errorf("session expired")
	}

	return &s, nil
}

// DeleteSession deletes a session by its ID.
func (sm *SessionManager) DeleteSession(sessionID string) error {
	_, err := sm.db.Exec("DELETE FROM sessions WHERE id = ?", sessionID)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

// GetSessionByID retrieves a session by its ID.
//
// The account row is joined in rather than queried separately: the request
// path already pays for this lookup, and a second round trip on a
// single-connection database is a cost every request would carry. The join is
// inner, so a session whose user no longer exists is not a session.
func (sm *SessionManager) GetSessionByID(sessionID string) (*Session, error) {
	var s Session
	var expiresAt, createdAt string

	err := sm.db.QueryRow(`
		SELECT s.id, s.user_id, s.token_hash, s.ip_address, s.user_agent, s.expires_at, s.created_at,
		       COALESCE(u.must_change_password, 0), COALESCE(u.enabled, 0)
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.id = ?`,
		sessionID,
	).Scan(&s.ID, &s.UserID, &s.TokenHash, &s.IPAddress, &s.UserAgent, &expiresAt, &createdAt,
		&s.MustChangePassword, &s.UserEnabled)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying session: %w", err)
	}

	s.ExpiresAt, err = time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		slog.Warn("parsing expires_at for session", "error", err)
	}
	s.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		slog.Warn("parsing created_at for session", "error", err)
	}

	if time.Now().After(s.ExpiresAt) {
		// Clean up expired session
		if _, err := sm.db.Exec("DELETE FROM sessions WHERE id = ?", s.ID); err != nil {
			slog.Error("deleting expired session", "error", err)
		}
		return nil, fmt.Errorf("session expired")
	}

	return &s, nil
}

// DeleteSessionByToken deletes a session by the raw token value.
func (sm *SessionManager) DeleteSessionByToken(token string) error {
	tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
	_, err := sm.db.Exec("DELETE FROM sessions WHERE token_hash = ?", tokenHash)
	if err != nil {
		return fmt.Errorf("deleting session by token: %w", err)
	}
	return nil
}

// ListUserSessions returns all active sessions for a user.
// expires_at is stored as RFC3339 ("T" separator); julianday() is required for
// a correct comparison against the current time (plain string comparison
// against datetime('now') output yields wrong results).
func (sm *SessionManager) ListUserSessions(userID string) ([]Session, error) {
	rows, err := sm.db.Query(`
		SELECT id, user_id, token_hash, ip_address, user_agent, expires_at, created_at
		FROM sessions WHERE user_id = ? AND julianday(expires_at) > julianday('now') ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		var expiresAt, createdAt string
		if err := rows.Scan(&s.ID, &s.UserID, &s.TokenHash, &s.IPAddress, &s.UserAgent, &expiresAt, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		s.ExpiresAt, err = time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			slog.Warn("parsing expires_at for session", "error", err)
		}
		s.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			slog.Warn("parsing created_at for session", "error", err)
		}
		s.TokenHash = "" // Never expose token hash
		sessions = append(sessions, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating session rows: %w", err)
	}

	return sessions, nil
}

// CleanupExpiredSessions removes all expired sessions from the database.
func (sm *SessionManager) CleanupExpiredSessions() error {
	now := time.Now().Format(time.RFC3339)
	_, err := sm.db.Exec("DELETE FROM sessions WHERE expires_at < ?", now)
	if err != nil {
		return fmt.Errorf("cleaning up expired sessions: %w", err)
	}
	return nil
}

// HashToken returns the SHA256 hash of a token string.
func HashToken(token string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
}
