package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"unicode"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters following OWASP recommendations.
const (
	argon2Time    = 2
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4
	argon2KeyLen  = 32
	argon2SaltLen = 16
)

// HashPassword hashes a password using Argon2id.
// The returned string is in the format: $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
func HashPassword(password string) (string, error) {
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	encoded := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argon2Memory, argon2Time, argon2Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)

	return encoded, nil
}

// VerifyPassword verifies a password against an Argon2id hash.
func VerifyPassword(hashedPassword, password string) (bool, error) {
	salt, hash, params, err := decodeArgon2Hash(hashedPassword)
	if err != nil {
		return false, fmt.Errorf("decoding hash: %w", err)
	}

	computedHash := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, params.keyLen)

	return subtle.ConstantTimeCompare(hash, computedHash) == 1, nil
}

type argon2Params struct {
	memory  uint32
	time    uint32
	threads uint8
	keyLen  uint32
}

func decodeArgon2Hash(encoded string) (salt, hash []byte, params argon2Params, err error) {
	var version int
	_, err = fmt.Sscanf(encoded, "$argon2id$v=%d$m=%d,t=%d,p=%d$", &version, &params.memory, &params.time, &params.threads)
	if err != nil {
		return nil, nil, params, fmt.Errorf("parsing argon2id params: %w", err)
	}

	// Extract the salt and hash portions after the last '$' delimiters.
	// Format: $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
	parts := splitHashParts(encoded)
	if len(parts) < 2 {
		return nil, nil, params, fmt.Errorf("invalid hash format: missing salt or hash")
	}

	salt, err = base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, params, fmt.Errorf("decoding salt: %w", err)
	}

	hash, err = base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, params, fmt.Errorf("decoding hash: %w", err)
	}

	params.keyLen = uint32(len(hash))

	// Validate parameter ranges to reject hashes with dangerously weak parameters.
	if params.memory < 16384 {
		return nil, nil, params, fmt.Errorf("argon2id memory parameter too low: %d (minimum 16384)", params.memory)
	}
	if params.time < 1 {
		return nil, nil, params, fmt.Errorf("argon2id time parameter too low: %d (minimum 1)", params.time)
	}
	if params.keyLen < 16 {
		return nil, nil, params, fmt.Errorf("argon2id key length too short: %d (minimum 16)", params.keyLen)
	}

	return salt, hash, params, nil
}

// splitHashParts extracts the base64-encoded salt and hash from the encoded string.
func splitHashParts(encoded string) []string {
	// Count dollar signs to find the last two segments
	count := 0
	lastDollar := -1
	for i := len(encoded) - 1; i >= 0; i-- {
		if encoded[i] == '$' {
			count++
			if count == 1 {
				lastDollar = i
			}
			if count == 2 {
				return []string{encoded[i+1 : lastDollar], encoded[lastDollar+1:]}
			}
		}
	}
	return nil
}

// ValidatePasswordComplexity checks that a password meets complexity requirements:
// - Minimum 8 characters
// - At least one uppercase letter
// - At least one lowercase letter
// - At least one digit
// - At least one special character
func ValidatePasswordComplexity(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("密码长度不能少于8个字符")
	}
	if len(password) > 128 {
		return fmt.Errorf("密码长度不能超过128个字符")
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return fmt.Errorf("密码必须包含至少一个大写字母")
	}
	if !hasLower {
		return fmt.Errorf("密码必须包含至少一个小写字母")
	}
	if !hasDigit {
		return fmt.Errorf("密码必须包含至少一个数字")
	}
	if !hasSpecial {
		return fmt.Errorf("密码必须包含至少一个特殊字符")
	}

	return nil
}
