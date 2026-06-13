package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
)

// APIToken represents an API token in the system.
type APIToken struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	Name           string     `json:"name"`
	TokenPrefix    string     `json:"token_prefix"`
	Description    string     `json:"description,omitempty"`
	Scope          string     `json:"scope,omitempty"`
	IsSingleUse    bool       `json:"is_single_use"`
	IsReadonly     bool       `json:"is_readonly"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	LastUsedIP     string     `json:"last_used_ip,omitempty"`
	Enabled        bool       `json:"enabled"`
	IPRestrictions []string   `json:"ip_restrictions,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TokenOptions holds options for creating an API token.
type TokenOptions struct {
	ExpiresAt      *time.Time
	IsSingleUse    bool
	IsReadonly     bool
	IPRestrictions []string
}

// TokenManager manages API tokens.
type TokenManager struct {
	db *sql.DB
}

// NewTokenManager creates a new TokenManager.
func NewTokenManager(db *sql.DB) *TokenManager {
	return &TokenManager{db: db}
}

// CreateToken creates a new API token.
// The full token is only returned once at creation time.
func (tm *TokenManager) CreateToken(userID, name, scope string, opts TokenOptions) (*APIToken, string, error) {
	// Validate every IP restriction up front so we never persist a malformed
	// CIDR/IP. Persisting bad entries and only failing at use time would let
	// an attacker push the server into the fail-closed branch of
	// isIPAllowed (see also the warning there).
	for _, cidr := range opts.IPRestrictions {
		if err := validateIPRestriction(cidr); err != nil {
			return nil, "", fmt.Errorf("invalid IP restriction %q: %w", cidr, err)
		}
	}

	// Validate ExpiresAt up front: it must be in the future. Without this
	// check the token would be created, returned to the caller, and then
	// immediately be unusable.
	if opts.ExpiresAt != nil && !opts.ExpiresAt.After(time.Now()) {
		return nil, "", fmt.Errorf("expires_at must be in the future")
	}

	// Generate a secure random token: goddi_<32 random bytes hex>
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, "", fmt.Errorf("generating token: %w", err)
	}
	fullToken := "goddi_" + hex.EncodeToString(tokenBytes)
	tokenHash := HashToken(fullToken)
	tokenPrefix := fullToken[:12] // goddi_ + first 6 hex chars

	id := uuid.New().String()
	now := time.Now()

	var expiresAtVal interface{}
	if opts.ExpiresAt != nil {
		expiresAtVal = opts.ExpiresAt.Format(time.RFC3339)
	}

	tx, err := tm.db.Begin()
	if err != nil {
		return nil, "", fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO api_tokens (id, user_id, name, token_hash, token_prefix, description, scope,
			is_single_use, is_readonly, expires_at, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, userID, name, tokenHash, tokenPrefix, "", scope,
		opts.IsSingleUse, opts.IsReadonly, expiresAtVal, true,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, "", fmt.Errorf("inserting API token: %w", err)
	}

	// Store IP restrictions
	for _, cidr := range opts.IPRestrictions {
		iprID := uuid.New().String()
		_, err := tx.Exec(`
			INSERT INTO token_ip_restrictions (id, token_id, cidr, created_at)
			VALUES (?, ?, ?, ?)`,
			iprID, id, cidr, now.Format(time.RFC3339),
		)
		if err != nil {
			return nil, "", fmt.Errorf("inserting IP restriction: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("committing transaction: %w", err)
	}

	token := &APIToken{
		ID:             id,
		UserID:         userID,
		Name:           name,
		TokenPrefix:    tokenPrefix,
		Scope:          scope,
		IsSingleUse:    opts.IsSingleUse,
		IsReadonly:     opts.IsReadonly,
		ExpiresAt:      opts.ExpiresAt,
		Enabled:        true,
		IPRestrictions: opts.IPRestrictions,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	return token, fullToken, nil
}

// validateIPRestriction parses a single entry of the IP-restriction list.
// A valid entry is either a bare IP address (v4 or v6) or a CIDR range.
// Anything else is rejected so we never persist garbage that would later
// flip isIPAllowed into its fail-closed branch.
func validateIPRestriction(entry string) error {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return fmt.Errorf("empty")
	}
	if strings.Contains(entry, "/") {
		if _, _, err := net.ParseCIDR(entry); err != nil {
			return err
		}
		return nil
	}
	if net.ParseIP(entry) == nil {
		return fmt.Errorf("not a valid IP address or CIDR range")
	}
	return nil
}

// ValidateAPIToken validates an API token string and returns the token metadata.
func (tm *TokenManager) ValidateAPIToken(tokenString, sourceIP string) (*APIToken, error) {
	tokenHash := HashToken(tokenString)

	var t APIToken
	var expiresAt, lastUsedAt, createdAt, updatedAt, description, scope, lastUsedIP sql.NullString

	err := tm.db.QueryRow(`
		SELECT id, user_id, name, token_prefix, description, scope,
			is_single_use, is_readonly, expires_at, last_used_at, last_used_ip,
			enabled, created_at, updated_at
		FROM api_tokens WHERE token_hash = ?`,
		tokenHash,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.TokenPrefix, &description, &scope,
		&t.IsSingleUse, &t.IsReadonly, &expiresAt, &lastUsedAt, &lastUsedIP,
		&t.Enabled, &createdAt, &updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("token not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying token: %w", err)
	}

	if description.Valid {
		t.Description = description.String
	}
	if scope.Valid {
		t.Scope = scope.String
	}
	if lastUsedIP.Valid {
		t.LastUsedIP = lastUsedIP.String
	}

	if !t.Enabled {
		return nil, fmt.Errorf("token is disabled")
	}

	if expiresAt.Valid {
		ea, err := time.Parse(time.RFC3339, expiresAt.String)
		if err != nil {
			return nil, fmt.Errorf("parsing expires_at for token: %w", err)
		}
		t.ExpiresAt = &ea
		if time.Now().After(ea) {
			return nil, fmt.Errorf("token has expired")
		}
	}

	if lastUsedAt.Valid {
		la, err := time.Parse(time.RFC3339, lastUsedAt.String)
		if err != nil {
			slog.Warn("parsing last_used_at for token", "error", err)
		}
		t.LastUsedAt = &la
	}
	if createdAt.Valid {
		ca, err := time.Parse(time.RFC3339, createdAt.String)
		if err != nil {
			slog.Warn("parsing created_at for token", "error", err)
		}
		t.CreatedAt = ca
	}
	if updatedAt.Valid {
		ua, err := time.Parse(time.RFC3339, updatedAt.String)
		if err != nil {
			slog.Warn("parsing updated_at for token", "error", err)
		}
		t.UpdatedAt = ua
	}

	// Check IP restrictions
	ipRestrictions, err := tm.getIPRestrictions(t.ID)
	if err != nil {
		return nil, fmt.Errorf("checking IP restrictions: %w", err)
	}
	if len(ipRestrictions) > 0 {
		if sourceIP == "" {
			return nil, fmt.Errorf("IP address required for this token but none provided")
		}
		if !isIPAllowed(sourceIP, ipRestrictions) {
			return nil, fmt.Errorf("IP address not allowed for this token")
		}
	}

	// Use a transaction for atomic update of last_used_at and single-use disable.
	now := time.Now()
	if t.IsSingleUse {
		// Atomically disable single-use token and update last_used_at.
		result, err := tm.db.Exec(`
			UPDATE api_tokens SET enabled = 0, last_used_at = ?, last_used_ip = ? WHERE id = ? AND enabled = 1`,
			now.Format(time.RFC3339), sourceIP, t.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("disabling single-use token: %w", err)
		}
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			return nil, fmt.Errorf("token has already been used")
		}
	} else {
		// Update last used info
		if _, err := tm.db.Exec(`
			UPDATE api_tokens SET last_used_at = ?, last_used_ip = ? WHERE id = ?`,
			now.Format(time.RFC3339), sourceIP, t.ID,
		); err != nil {
			slog.Error("updating token last_used info", "error", err)
		}
	}

	return &t, nil
}

// RevokeToken revokes (disables) an API token.
func (tm *TokenManager) RevokeToken(tokenID string) error {
	_, err := tm.db.Exec(`UPDATE api_tokens SET enabled = 0, updated_at = ? WHERE id = ?`,
		time.Now().Format(time.RFC3339), tokenID,
	)
	if err != nil {
		return fmt.Errorf("revoking token: %w", err)
	}
	return nil
}

// ListTokens returns all API tokens for a user.
func (tm *TokenManager) ListTokens(userID string) ([]APIToken, error) {
	rows, err := tm.db.Query(`
		SELECT id, user_id, name, token_prefix, description, scope,
			is_single_use, is_readonly, expires_at, last_used_at, last_used_ip,
			enabled, created_at, updated_at
		FROM api_tokens WHERE user_id = ? AND enabled = 1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying tokens: %w", err)
	}
	defer rows.Close()

	var tokens []APIToken
	for rows.Next() {
		var t APIToken
		var expiresAt, lastUsedAt, createdAt, updatedAt, description, scope, lastUsedIP sql.NullString
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenPrefix, &description, &scope,
			&t.IsSingleUse, &t.IsReadonly, &expiresAt, &lastUsedAt, &lastUsedIP,
			&t.Enabled, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning token: %w", err)
		}

		if description.Valid {
			t.Description = description.String
		}
		if scope.Valid {
			t.Scope = scope.String
		}
		if lastUsedIP.Valid {
			t.LastUsedIP = lastUsedIP.String
		}

		if expiresAt.Valid {
			ea, err := time.Parse(time.RFC3339, expiresAt.String)
			if err != nil {
				slog.Warn("parsing expires_at for token", "error", err)
			}
			t.ExpiresAt = &ea
		}
		if lastUsedAt.Valid {
			la, err := time.Parse(time.RFC3339, lastUsedAt.String)
			if err != nil {
				slog.Warn("parsing last_used_at for token", "error", err)
			}
			t.LastUsedAt = &la
		}
		if createdAt.Valid {
			ca, err := time.Parse(time.RFC3339, createdAt.String)
			if err != nil {
				slog.Warn("parsing created_at for token", "error", err)
			}
			t.CreatedAt = ca
		}
		if updatedAt.Valid {
			ua, err := time.Parse(time.RFC3339, updatedAt.String)
			if err != nil {
				slog.Warn("parsing updated_at for token", "error", err)
			}
			t.UpdatedAt = ua
		}

		tokens = append(tokens, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating token rows: %w", err)
	}

	return tokens, nil
}

// GetTokenByID retrieves a token by its ID.
func (tm *TokenManager) GetTokenByID(tokenID string) (*APIToken, error) {
	var t APIToken
	var expiresAt, lastUsedAt, createdAt, updatedAt, description, scope, lastUsedIP sql.NullString

	err := tm.db.QueryRow(`
		SELECT id, user_id, name, token_prefix, description, scope,
			is_single_use, is_readonly, expires_at, last_used_at, last_used_ip,
			enabled, created_at, updated_at
		FROM api_tokens WHERE id = ?`,
		tokenID,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.TokenPrefix, &description, &scope,
		&t.IsSingleUse, &t.IsReadonly, &expiresAt, &lastUsedAt, &lastUsedIP,
		&t.Enabled, &createdAt, &updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("token not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying token: %w", err)
	}

	if description.Valid {
		t.Description = description.String
	}
	if scope.Valid {
		t.Scope = scope.String
	}
	if lastUsedIP.Valid {
		t.LastUsedIP = lastUsedIP.String
	}

	if expiresAt.Valid {
		ea, err := time.Parse(time.RFC3339, expiresAt.String)
		if err != nil {
			slog.Warn("parsing expires_at for token", "error", err)
		}
		t.ExpiresAt = &ea
	}
	if lastUsedAt.Valid {
		la, err := time.Parse(time.RFC3339, lastUsedAt.String)
		if err != nil {
			slog.Warn("parsing last_used_at for token", "error", err)
		}
		t.LastUsedAt = &la
	}
	if createdAt.Valid {
		ca, err := time.Parse(time.RFC3339, createdAt.String)
		if err != nil {
			slog.Warn("parsing created_at for token", "error", err)
		}
		t.CreatedAt = ca
	}
	if updatedAt.Valid {
		ua, err := time.Parse(time.RFC3339, updatedAt.String)
		if err != nil {
			slog.Warn("parsing updated_at for token", "error", err)
		}
		t.UpdatedAt = ua
	}

	return &t, nil
}

func (tm *TokenManager) getIPRestrictions(tokenID string) ([]string, error) {
	rows, err := tm.db.Query(`
		SELECT cidr FROM token_ip_restrictions WHERE token_id = ?`, tokenID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var restrictions []string
	for rows.Next() {
		var cidr string
		if err := rows.Scan(&cidr); err != nil {
			return nil, err
		}
		restrictions = append(restrictions, cidr)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return restrictions, nil
}

// isIPAllowed checks if an IP address is within any of the allowed CIDR
// ranges. If ANY of the configured CIDR entries fails to parse, the whole
// IP-allowlist is treated as untrusted and the function returns false:
// fail-closed, since silently skipping a malformed CIDR would let attackers
// bypass the restriction by injecting a bad entry.
func isIPAllowed(ipStr string, allowed []string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	for _, cidr := range allowed {
		if strings.Contains(cidr, "/") {
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				slog.Warn("invalid CIDR in IP restriction; rejecting all matches",
					"cidr", cidr, "error", err)
				return false
			}
			if network.Contains(ip) {
				return true
			}
		} else {
			parsed := net.ParseIP(cidr)
			if parsed == nil {
				slog.Warn("invalid IP in IP restriction; rejecting all matches",
					"ip", cidr)
				return false
			}
			if ip.Equal(parsed) {
				return true
			}
		}
	}

	return false
}
