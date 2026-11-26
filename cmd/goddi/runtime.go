package main

import (
	"errors"
	"log/slog"
	"os"
	"strconv"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dataplane"
	"github.com/jasonwa/goddi/internal/dhcp/ha"
	"github.com/jasonwa/goddi/internal/system"
)

func loadRuntimeConfig(configPath string) (*config.Config, error) {
	cfg, err := config.LoadFromYAML(configPath)
	if err == nil {
		return cfg, nil
	}
	if configPath == "/etc/goddi/config.yaml" && errors.Is(err, os.ErrNotExist) {
		slog.Info("default config file not found; using built-in defaults and environment overrides")
		return config.DefaultConfig(), nil
	}
	return nil, err
}

// currentECSMode reads the persisted ECS mode setting, falling back to
// "strip" when unset. Used when applying any of the three ECS keys so the
// other members of the config unit keep their stored values.
func currentECSMode(mgr *system.Manager) string {
	if mgr == nil {
		return "strip"
	}
	if v, err := mgr.GetSetting("dns_ecs_mode"); err == nil && v != "" {
		return v
	}
	return "strip"
}

// parseIntOr parses value as an int, returning fallback on error.
func parseIntOr(value string, fallback int) int {
	if n, err := strconv.Atoi(value); err == nil {
		return n
	}
	return fallback
}

// cacheSettingOverrides mirrors the DNS cache keys in config.yaml into the
// values used to seed system_settings on a fresh database. The keys must match
// the ones applySetting reads when it replays settings onto dnsCache.
func cacheSettingOverrides(c config.CacheConfig) map[string]string {
	return map[string]string{
		"dns_cache_serve_stale": strconv.FormatBool(c.ServeStale),
		"dns_cache_stale_ttl":   strconv.Itoa(c.StaleTTL),
		"dns_cache_prefetch":    strconv.FormatBool(c.Prefetch),
		"dns_cache_min_ttl":     strconv.Itoa(c.MinTTL),
		"dns_cache_max_ttl":     strconv.Itoa(c.MaxTTL),
	}
}

// dataPlaneQuota resolves the configured bounds into the shape the data plane
// checks against. A bound the operator disabled resolves to zero, which is
// how the data plane is told not to look at it.
// securityKeyMaterial resolves what sealed values are encrypted with.
//
// A dedicated security.encryption_key is the intended configuration. Falling
// back to the JWT secret keeps an installation that predates the setting
// running unchanged, and costs coupling: rotating the JWT secret would then
// make every sealed value unreadable, and the only way back is a rekey while
// both keys are known. Validate() refuses the fallback once the deployment
// declares itself production, so the coupling survives only where it is cheap
// to repair.
func securityKeyMaterial(cfg *config.Config) string {
	if cfg.Security.EncryptionKey != "" {
		return cfg.Security.EncryptionKey
	}
	slog.Warn("security.encryption_key is not set; stored secrets are sealed with the JWT secret as key material. Set security.encryption_key and run `goddi rekey` to decouple them.")
	return cfg.Security.JWTSecret
}

func dataPlaneQuota(cfg *config.Config) dataplane.Quota {
	q := cfg.EffectiveQuota()
	return dataplane.Quota{
		MaxLeases:    q.MaxLeases,
		MaxFileBytes: q.MaxStoreBytes,
		MaxBacklog:   q.MaxBacklog,
	}
}

// currentHAState is the redundancy state of this process, derived when it is
// asked rather than recorded at each transition.
//
// One function rather than one per consumer: the probe and the metric answer
// different questions about the same fact, and a second derivation of it is how
// the two come to report different states for one node.
func currentHAState(role string, repl *ha.Replicator, mirror *ha.Mirror) ha.State {
	switch {
	case repl != nil:
		return repl.State()
	case mirror != nil:
		return mirror.State()
	case role == ha.RoleFenced:
		return ha.StateFenced
	default:
		return ha.StateSolo
	}
}
