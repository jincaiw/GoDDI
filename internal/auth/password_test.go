package auth

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	t.Parallel()

	password := "TestPass123!@#"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	if hash == "" {
		t.Fatal("HashPassword() returned empty hash")
	}

	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("HashPassword() hash should start with $argon2id$, got %q", hash[:20])
	}

	if !strings.Contains(hash, "m=65536") {
		t.Errorf("HashPassword() hash should contain m=65536 (64MB), got %q", hash)
	}

	if !strings.Contains(hash, "t=2") {
		t.Errorf("HashPassword() hash should contain t=2, got %q", hash)
	}

	if !strings.Contains(hash, "p=4") {
		t.Errorf("HashPassword() hash should contain p=4, got %q", hash)
	}
}

func TestVerifyPassword(t *testing.T) {
	t.Parallel()

	password := "TestPass123!@#"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	match, err := VerifyPassword(hash, password)
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if !match {
		t.Error("VerifyPassword() should return true for correct password")
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	t.Parallel()

	password := "TestPass123!@#"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	match, err := VerifyPassword(hash, "WrongPass456!@#")
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if match {
		t.Error("VerifyPassword() should return false for wrong password")
	}
}

func TestVerifyPassword_InvalidHash(t *testing.T) {
	t.Parallel()

	_, err := VerifyPassword("invalid-hash", "password")
	if err == nil {
		t.Error("VerifyPassword() should return error for invalid hash format")
	}
}

func TestVerifyPassword_DifferentHashesForSamePassword(t *testing.T) {
	t.Parallel()

	password := "TestPass123!@#"
	hash1, _ := HashPassword(password)
	hash2, _ := HashPassword(password)

	if hash1 == hash2 {
		t.Error("Two hashes of the same password should differ (different salts)")
	}

	match1, _ := VerifyPassword(hash1, password)
	match2, _ := VerifyPassword(hash2, password)
	if !match1 || !match2 {
		t.Error("Both hashes should verify correctly")
	}
}

func TestValidatePasswordComplexity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid complex password", "TestPass123!", false},
		{"valid with more special chars", "MyP@ssw0rd#", false},
		{"too short", "Ab1!", true},
		{"no uppercase", "testpass123!", true},
		{"no lowercase", "TESTPASS123!", true},
		{"no digit", "TestPassword!!", true},
		{"no special char", "TestPassword123", true},
		{"only lowercase", "abcdefgh", true},
		{"only digits", "12345678", true},
		{"empty string", "", true},
		{"exactly 8 chars valid", "Aa1!Bb2@", false},
		{"7 chars valid complexity", "Aa1!Bb@", true},
		{"unicode special char", "TestPass123\u2764", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePasswordComplexity(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePasswordComplexity(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestHashPassword_DifferentPasswords(t *testing.T) {
	t.Parallel()

	passwords := []string{"Password1!", "Another2@", "Third3#"}
	hashes := make(map[string]bool)

	for _, pw := range passwords {
		hash, err := HashPassword(pw)
		if err != nil {
			t.Fatalf("HashPassword(%q) error = %v", pw, err)
		}
		if hashes[hash] {
			t.Errorf("duplicate hash for password %q", pw)
		}
		hashes[hash] = true
	}
}

func TestHashPassword_EmptyPassword(t *testing.T) {
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("HashPassword(empty) error = %v", err)
	}
	if hash == "" {
		t.Error("HashPassword(empty) should still return a hash")
	}

	// Empty password should verify correctly
	match, err := VerifyPassword(hash, "")
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if !match {
		t.Error("VerifyPassword() should return true for empty password")
	}
}
