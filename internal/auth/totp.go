package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// TOTPManager handles TOTP two-factor authentication.
type TOTPManager struct {
	db     *sql.DB
	issuer string
}

// NewTOTPManager creates a new TOTPManager.
func NewTOTPManager(db *sql.DB, issuer string) *TOTPManager {
	if issuer == "" {
		issuer = "GoDDI"
	}
	return &TOTPManager{db: db, issuer: issuer}
}

// GenerateTOTPSecret generates a new TOTP secret for a user.
// Returns the secret and a URL that can be used to generate a QR code.
func (tm *TOTPManager) GenerateTOTPSecret(userID, username string) (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      tm.issuer,
		AccountName: username,
		SecretSize:  32,
	})
	if err != nil {
		return "", "", fmt.Errorf("generating TOTP key: %w", err)
	}

	// Store the secret (temporarily unconfirmed) in the database.
	// It will be activated only after successful verification.
	now := time.Now().Format(time.RFC3339)
	_, err = tm.db.Exec(`
		INSERT INTO user_totp_secrets (user_id, secret_ciphertext, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET secret_ciphertext = ?, enabled = 0, confirmed_at = NULL, updated_at = ?`,
		userID, key.Secret(), now, now,
		key.Secret(), now,
	)
	if err != nil {
		return "", "", fmt.Errorf("storing TOTP secret: %w", err)
	}

	return key.Secret(), key.URL(), nil
}

// VerifyTOTP verifies a TOTP code against a secret.
// Uses Skew=1 to allow 1 time step drift per RFC 6238 recommendation.
func VerifyTOTP(secret, code string) bool {
	ok, _ := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return ok
}

// EnableTOTP enables TOTP for a user after successful verification.
func (tm *TOTPManager) EnableTOTP(userID string, secret string) error {
	now := time.Now().Format(time.RFC3339)
	result, err := tm.db.Exec(`
		UPDATE user_totp_secrets SET enabled = 1, confirmed_at = ?, updated_at = ?
		WHERE user_id = ? AND secret_ciphertext = ?`,
		now, now, userID, secret,
	)
	if err != nil {
		return fmt.Errorf("enabling TOTP: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("TOTP secret not found for user")
	}

	return nil
}

// DisableTOTP disables TOTP for a user.
func (tm *TOTPManager) DisableTOTP(userID string) error {
	_, err := tm.db.Exec(`DELETE FROM user_totp_secrets WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("disabling TOTP: %w", err)
	}
	return nil
}

// IsTOTPEnabled checks if TOTP is enabled for a user.
func (tm *TOTPManager) IsTOTPEnabled(userID string) (bool, error) {
	var enabled bool
	err := tm.db.QueryRow(`
		SELECT enabled FROM user_totp_secrets WHERE user_id = ?`, userID,
	).Scan(&enabled)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking TOTP status: %w", err)
	}
	return enabled, nil
}

// GetTOTPSecret retrieves the TOTP secret for a user.
func (tm *TOTPManager) GetTOTPSecret(userID string) (string, error) {
	var secret string
	err := tm.db.QueryRow(`
		SELECT secret_ciphertext FROM user_totp_secrets WHERE user_id = ?`, userID,
	).Scan(&secret)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("getting TOTP secret: %w", err)
	}
	return secret, nil
}

// GenerateRecoveryCodes generates a set of recovery codes for a user.
// Each code is a random 8-character hex string.
func (tm *TOTPManager) GenerateRecoveryCodes(userID string) ([]string, error) {
	// Delete existing recovery codes
	_, err := tm.db.Exec(`DELETE FROM user_recovery_codes WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("deleting old recovery codes: %w", err)
	}

	codes := make([]string, 10) // Generate 10 recovery codes
	for i := 0; i < 10; i++ {
		code, err := generateRecoveryCode()
		if err != nil {
			return nil, fmt.Errorf("generating recovery code: %w", err)
		}
		codes[i] = code

		codeHash := HashToken(code)
		_, err = tm.db.Exec(`
			INSERT INTO user_recovery_codes (id, user_id, code_hash, created_at)
			VALUES (?, ?, ?, ?)`,
			uuid.New().String(), userID, codeHash, time.Now().Format(time.RFC3339),
		)
		if err != nil {
			return nil, fmt.Errorf("storing recovery code: %w", err)
		}
	}

	return codes, nil
}

// VerifyRecoveryCode verifies a recovery code for a user.
// If valid, the code is consumed (marked as used) and cannot be reused.
// Uses atomic UPDATE to prevent TOCTOU race condition.
func (tm *TOTPManager) VerifyRecoveryCode(userID, code string) (bool, error) {
	codeHash := HashToken(code)

	now := time.Now().Format(time.RFC3339)
	result, err := tm.db.Exec(`
		UPDATE user_recovery_codes SET used_at = ?
		WHERE user_id = ? AND code_hash = ? AND used_at IS NULL`,
		now, userID, codeHash,
	)
	if err != nil {
		return false, fmt.Errorf("consuming recovery code: %w", err)
	}

	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// generateRecoveryCode generates a random recovery code (32 hex chars,
// 128-bit entropy). 128 bits is well beyond the brute-force threshold
// even if the stored SHA-256 hash is somehow disclosed.
func generateRecoveryCode() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
