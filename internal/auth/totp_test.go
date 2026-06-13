package auth

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
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
