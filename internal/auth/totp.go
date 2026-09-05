package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const totpSecretCipherPrefix = "enc:v1:"

// TOTPManager handles TOTP two-factor authentication.
type TOTPManager struct {
	db            *sql.DB
	issuer        string
	encryptionKey []byte
}

// NewTOTPManager creates a new TOTPManager.
//
// The optional encryptionKey parameter enables encrypted storage for TOTP
// secrets. When omitted, TOTP secrets are stored as-is for backward
// compatibility.
func NewTOTPManager(db *sql.DB, issuer string, encryptionKey ...string) *TOTPManager {
	if issuer == "" {
		issuer = "GoDDI"
	}
	var keyMaterial string
	if len(encryptionKey) > 0 {
		keyMaterial = encryptionKey[0]
	}
	return &TOTPManager{
		db:            db,
		issuer:        issuer,
		encryptionKey: deriveTOTPKey(keyMaterial),
	}
}

func deriveTOTPKey(keyMaterial string) []byte {
	if keyMaterial == "" {
		return nil
	}
	sum := sha256.Sum256([]byte("goddi-totp-v1:" + keyMaterial))
	key := make([]byte, len(sum))
	copy(key, sum[:])
	return key
}

func (tm *TOTPManager) encryptSecret(secret string) (string, error) {
	if len(tm.encryptionKey) == 0 {
		return secret, nil
	}

	block, err := aes.NewCipher(tm.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("creating TOTP cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating TOTP AEAD: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generating TOTP nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(secret), nil)
	payload := append(nonce, ciphertext...)
	return totpSecretCipherPrefix + base64.RawStdEncoding.EncodeToString(payload), nil
}

func (tm *TOTPManager) decryptSecret(value string) (string, error) {
	if value == "" || !strings.HasPrefix(value, totpSecretCipherPrefix) {
		return value, nil
	}
	if len(tm.encryptionKey) == 0 {
		return "", fmt.Errorf("encrypted TOTP secret cannot be decrypted without an encryption key")
	}

	data, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, totpSecretCipherPrefix))
	if err != nil {
		return "", fmt.Errorf("decoding TOTP secret: %w", err)
	}

	block, err := aes.NewCipher(tm.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("creating TOTP cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating TOTP AEAD: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted TOTP secret is too short")
	}

	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypting TOTP secret: %w", err)
	}
	return string(plaintext), nil
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

	storedSecret, err := tm.encryptSecret(key.Secret())
	if err != nil {
		return "", "", fmt.Errorf("encrypting TOTP secret: %w", err)
	}

	// Store the secret (temporarily unconfirmed) in the database.
	// It will be activated only after successful verification.
	now := time.Now().Format(time.RFC3339)
	_, err = tm.db.Exec(`
		INSERT INTO user_totp_secrets (user_id, secret_ciphertext, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET secret_ciphertext = ?, enabled = 0, confirmed_at = NULL, updated_at = ?`,
		userID, storedSecret, false, now, now,
		storedSecret, now,
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
	storedSecret, err := tm.GetTOTPSecret(userID)
	if err != nil {
		return err
	}
	if storedSecret != secret {
		return fmt.Errorf("TOTP secret not found for user")
	}

	now := time.Now().Format(time.RFC3339)
	result, err := tm.db.Exec(`
		UPDATE user_totp_secrets SET enabled = 1, confirmed_at = ?, updated_at = ?
		WHERE user_id = ? AND secret_ciphertext IS NOT NULL`,
		now, now, userID,
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

	plain, err := tm.decryptSecret(secret)
	if err != nil {
		return "", fmt.Errorf("decrypting TOTP secret: %w", err)
	}
	return plain, nil
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
