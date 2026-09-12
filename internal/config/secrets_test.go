package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	goodJWTSecret         = "a-proper-jwt-secret-for-testing"
	goodEncryptionKey     = "a-proper-encryption-key-too"
	shippedConfigFilename = "config.yaml"
)

func secretConfig(t *testing.T) *Config {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Security.JWTSecret = goodJWTSecret
	return cfg
}

// The repository is the first place a secret leaks from. A shipped file with a
// usable value in it is copied into every deployment that never edits it.
func TestTheShippedConfigCarriesNoUsableSecret(t *testing.T) {
	path := filepath.Join("..", "..", shippedConfigFilename)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("%s is not in this tree", path)
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var shipped Config
	if err := yaml.Unmarshal(raw, &shipped); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	if shipped.Security.JWTSecret != "" {
		t.Fatalf("the shipped %s sets security.jwt_secret to %q; a value in the repository is a value every "+
			"deployment inherits", shippedConfigFilename, shipped.Security.JWTSecret)
	}
	if shipped.Security.EncryptionKey != "" {
		t.Fatalf("the shipped %s sets security.encryption_key to a value", shippedConfigFilename)
	}

	// And the file that ships must not be startable as-is: if it were, an
	// operator would get a working instance built on a secret from a public
	// repository.
	previous := os.Getenv("GODDI_ALLOW_DEFAULT_SECRET")
	_ = os.Unsetenv("GODDI_ALLOW_DEFAULT_SECRET")
	t.Cleanup(func() {
		if previous != "" {
			_ = os.Setenv("GODDI_ALLOW_DEFAULT_SECRET", previous)
		}
	})
	if err := Validate(&shipped); err == nil {
		t.Fatal("the shipped configuration validates without any further setup, so an unedited installation " +
			"would run on a secret that is in the repository")
	}
}

func TestProductionRefusesToRunWithoutADedicatedEncryptionKey(t *testing.T) {
	cfg := secretConfig(t)
	cfg.Env = "production"
	cfg.Security.EncryptionKey = ""

	err := Validate(cfg)
	if err == nil {
		t.Fatal("a production deployment started without security.encryption_key")
	}
	if !strings.Contains(err.Error(), "encryption_key") {
		t.Fatalf("the error does not name the setting: %v", err)
	}
}

func TestProductionAcceptsADedicatedEncryptionKey(t *testing.T) {
	cfg := secretConfig(t)
	cfg.Env = "production"
	cfg.Security.EncryptionKey = goodEncryptionKey

	if err := Validate(cfg); err != nil {
		t.Fatalf("a production deployment with a dedicated key was rejected: %v", err)
	}
}

// The fallback is what every installation predating the setting relies on.
// Removing it in development would break them for no security gain, since the
// coupling is only dangerous once the JWT secret is rotated in earnest.
func TestDevelopmentKeepsTheFallbackForTheEncryptionKey(t *testing.T) {
	cfg := secretConfig(t)
	cfg.Env = "development"
	cfg.Security.EncryptionKey = ""

	if err := Validate(cfg); err != nil {
		t.Fatalf("a development deployment was rejected for having no encryption key: %v", err)
	}
}

// Two purposes sharing one secret means a leak of either compromises both, and
// a rotation of one silently breaks the other.
func TestTheEncryptionKeyMustDifferFromTheJWTSecret(t *testing.T) {
	cfg := secretConfig(t)
	cfg.Security.EncryptionKey = goodJWTSecret

	if err := Validate(cfg); err == nil {
		t.Fatal("security.encryption_key was accepted while identical to security.jwt_secret")
	}
}

func TestAShortEncryptionKeyIsRejected(t *testing.T) {
	cfg := secretConfig(t)
	cfg.Security.EncryptionKey = "short"

	if err := Validate(cfg); err == nil {
		t.Fatal("a 5-character encryption key was accepted")
	}
}

// The environment is declared once and read once. Two readers would be two
// answers, and the permissive one wins by accident.
func TestTheEnvironmentComesFromTheConfigAndGODDIENV(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.IsProduction() {
		t.Fatal("an unset env reads as production; it has to default to the permissive side")
	}

	t.Setenv("GODDI_ENV", "production")
	ApplyEnvOverrides(cfg)
	if !cfg.IsProduction() {
		t.Fatal("GODDI_ENV=production did not reach the config")
	}
}

// Only the exact word counts. "Production", "prod" and "PRODUCTION" are typos,
// and a typo must not be what decides whether a deployment is hardened.
func TestIsProductionOnlyMatchesTheExactValue(t *testing.T) {
	for _, value := range []string{"", "dev", "development", "prod", "Production", "PRODUCTION"} {
		cfg := &Config{Env: value}
		if cfg.IsProduction() {
			t.Fatalf("env %q was treated as production", value)
		}
	}
	if !(&Config{Env: "production"}).IsProduction() {
		t.Fatal(`env "production" was not treated as production`)
	}
}

// A nil config must not panic a predicate that a startup path may reach before
// the config is built.
func TestIsProductionOnANilConfig(t *testing.T) {
	var cfg *Config
	if cfg.IsProduction() {
		t.Fatal("a nil config reported itself production")
	}
}
