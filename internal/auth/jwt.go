package auth

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token expiration defaults.
const (
	AccessTokenDuration  = 2 * time.Hour
	RefreshTokenDuration = 7 * 24 * time.Hour
)

// MinJWTSecretLength is the minimum recommended length for a JWT secret.
// Secrets shorter than this are rejected unless the operator explicitly opts
// into insecure development mode via the GODDI_TEST_MODE environment variable
// or the GODDI_ALLOW_WEAK_JWT_SECRET override.
const MinJWTSecretLength = 16

// allowWeakJWTSecret reports whether the operator has explicitly opted into
// accepting short JWT secrets (used by tests and some development setups).
func allowWeakJWTSecret() bool {
	if os.Getenv("GODDI_TEST_MODE") == "true" {
		return true
	}
	if os.Getenv("GODDI_ALLOW_WEAK_JWT_SECRET") == "true" {
		return true
	}
	return false
}

// Claims represents the JWT claims for an access token.
type Claims struct {
	UserID    string   `json:"uid"`
	Username  string   `json:"uname"`
	RoleIDs   []string `json:"rids,omitempty"`
	SessionID string   `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

// JWTManager handles JWT token generation and validation.
type JWTManager struct {
	secretKey []byte
}

// NewJWTManager creates a new JWTManager with the given secret key.
// It returns an error if the secret is shorter than MinJWTSecretLength bytes
// unless the operator has explicitly allowed weak secrets (GODDI_TEST_MODE or
// GODDI_ALLOW_WEAK_JWT_SECRET).
func NewJWTManager(secretKey string) (*JWTManager, error) {
	if len(secretKey) < MinJWTSecretLength && !allowWeakJWTSecret() {
		return nil, fmt.Errorf("jwt secret must be at least %d characters long (current: %d); set GODDI_TEST_MODE=true or GODDI_ALLOW_WEAK_JWT_SECRET=true to allow weak secrets in development",
			MinJWTSecretLength, len(secretKey))
	}
	return &JWTManager{
		secretKey: []byte(secretKey),
	}, nil
}

// GenerateToken generates a new JWT access token for the given user.
func (m *JWTManager) GenerateToken(userID, username string, roleIDs []string, sessionID string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		Username:  username,
		RoleIDs:   roleIDs,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenDuration)),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "goddi",
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secretKey)
}

// CSRFKey returns a deterministic, cryptographically-separated CSRF key
// derived from the JWT secret via HKDF-SHA256. The same secret is used to
// sign JWTs and to derive the CSRF HMAC key, but the HKDF expansion
// ensures the two keys are independent.
func (m *JWTManager) CSRFKey() []byte {
	key, err := hkdf.Key(sha256.New, m.secretKey, []byte("goddi-csrf-v1"), "csrf-auth", 32)
	if err != nil {
		// hkdf.Key should not fail with our parameters; fall back to a
		// SHA-256 derived key to avoid panicking.
		sum := sha256.Sum256(append(m.secretKey, []byte("goddi-csrf-v1")...))
		return sum[:]
	}
	return key
}

// ParseToken parses and validates a JWT token string.
func (m *JWTManager) ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Pin the exact algorithm: accepting the whole HMAC family would let
		// HS384/HS512 tokens signed with the same key pass validation even
		// though they are never issued by this server.
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.secretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("parsing token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}
