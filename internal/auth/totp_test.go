package auth

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	_ "modernc.org/sqlite"
)

func TestVerifyTOTP(t *testing.T) {
	// Generate a TOTP secret and validate a code against it
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "GoDDI-Test",
		AccountName: "testuser",
		SecretSize:  32,
	})
	if err != nil {
		t.Fatalf("failed to generate TOTP key: %v", err)
	}

	secret := key.Secret()

	// Generate a valid code using the simple GenerateCode function
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("failed to generate TOTP code: %v", err)
	}

	// Verify the valid code
	if !VerifyTOTP(secret, code) {
		t.Errorf("VerifyTOTP() should return true for valid code %q", code)
	}
}

func TestVerifyTOTP_InvalidCode(t *testing.T) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "GoDDI-Test",
		AccountName: "testuser",
		SecretSize:  32,
	})
	if err != nil {
		t.Fatalf("failed to generate TOTP key: %v", err)
	}

	secret := key.Secret()

	// An obviously wrong code should not verify
	if VerifyTOTP(secret, "000000") {
		t.Error("VerifyTOTP() should return false for invalid code")
	}
}

func TestVerifyTOTP_EmptySecret(t *testing.T) {
	// Empty secret should not verify any code
	if VerifyTOTP("", "123456") {
		t.Error("VerifyTOTP() should return false for empty secret")
	}
}

func TestVerifyTOTP_WrongSecret(t *testing.T) {
	key1, _ := totp.Generate(totp.GenerateOpts{
		Issuer:      "GoDDI-Test",
		AccountName: "user1",
		SecretSize:  32,
	})
	key2, _ := totp.Generate(totp.GenerateOpts{
		Issuer:      "GoDDI-Test",
		AccountName: "user2",
		SecretSize:  32,
	})

	// Generate code for key1
	code, err := totp.GenerateCode(key1.Secret(), time.Now())
	if err != nil {
		t.Fatalf("failed to generate TOTP code: %v", err)
	}

	// Code from key1 should not verify against key2's secret
	if VerifyTOTP(key2.Secret(), code) {
		t.Error("VerifyTOTP() should return false for code generated with different secret")
	}
}

func TestNewTOTPManager_DefaultIssuer(t *testing.T) {
	t.Parallel()

	tm := NewTOTPManager(nil, "")
	if tm.issuer != "GoDDI" {
		t.Errorf("default issuer = %q, want %q", tm.issuer, "GoDDI")
	}
}

func TestNewTOTPManager_CustomIssuer(t *testing.T) {
	t.Parallel()

	tm := NewTOTPManager(nil, "CustomIssuer")
	if tm.issuer != "CustomIssuer" {
		t.Errorf("issuer = %q, want %q", tm.issuer, "CustomIssuer")
	}
}

func TestGenerateRecoveryCode_Entropy(t *testing.T) {
	// Recovery codes must have at least 128 bits of entropy (16 random
	// bytes -> 32 hex chars). This is the threshold above which
	// offline brute force is computationally infeasible.
	code, err := generateRecoveryCode()
	if err != nil {
		t.Fatalf("generateRecoveryCode() error = %v", err)
	}
	if len(code) != 32 {
		t.Errorf("recovery code length = %d, want 32 (16 bytes hex-encoded)", len(code))
	}
}

func TestTOTPSecretEncryptionRoundTrip(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE user_totp_secrets (
			user_id TEXT PRIMARY KEY,
			secret_ciphertext TEXT NOT NULL,
			enabled BOOLEAN DEFAULT FALSE,
			confirmed_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		);
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	tm := NewTOTPManager(db, "GoDDI-Test", "unit-test-encryption-key")
	secret, _, err := tm.GenerateTOTPSecret("user-1", "alice")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret() error = %v", err)
	}

	var stored string
	if err := db.QueryRow(`SELECT secret_ciphertext FROM user_totp_secrets WHERE user_id = ?`, "user-1").Scan(&stored); err != nil {
		t.Fatalf("query stored secret: %v", err)
	}
	if stored == secret {
		t.Fatal("stored TOTP secret should be encrypted, but it matches plaintext")
	}
	if !strings.HasPrefix(stored, totpSecretCipherPrefix) {
		t.Fatalf("stored TOTP secret = %q, want prefix %q", stored, totpSecretCipherPrefix)
	}

	got, err := tm.GetTOTPSecret("user-1")
	if err != nil {
		t.Fatalf("GetTOTPSecret() error = %v", err)
	}
	if got != secret {
		t.Fatalf("GetTOTPSecret() = %q, want %q", got, secret)
	}

	if err := tm.EnableTOTP("user-1", secret); err != nil {
		t.Fatalf("EnableTOTP() error = %v", err)
	}
	var enabled bool
	if err := db.QueryRow(`SELECT enabled FROM user_totp_secrets WHERE user_id = ?`, "user-1").Scan(&enabled); err != nil {
		t.Fatalf("query enabled: %v", err)
	}
	if !enabled {
		t.Fatal("TOTP should be enabled after successful verification")
	}
}
