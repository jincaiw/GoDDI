package main

import (
	"fmt"
	"time"

	"github.com/jincaiw/GoDDI/internal/auth"
	"github.com/jincaiw/GoDDI/internal/config"
	"github.com/jincaiw/GoDDI/internal/database"
	applog "github.com/jincaiw/GoDDI/internal/log"
)

// runUnlock clears login rate-limit lockouts. It intentionally does NOT need
// HTTP credentials: the whole point is to recover an administrator account
// that is locked out and therefore cannot call the unlock API.
func runUnlock(configPath, username, ip string, all bool) error {
	if !all && username == "" {
		return fmt.Errorf("specify --user <name> (optionally with --ip <addr>) or --all")
	}
	if all && (username != "" || ip != "") {
		return fmt.Errorf("--all cannot be combined with --user/--ip")
	}
	if ip != "" && username == "" {
		return fmt.Errorf("--ip requires --user")
	}

	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	config.ApplyEnvOverrides(cfg)
	if err := rejectEnterprise(cfg); err != nil {
		return err
	}

	applog.InitLogger(cfg.Log.Level)

	db, err := database.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}
	defer db.Close()

	if err := auth.EnsureRateLimitTable(db.DB); err != nil {
		return fmt.Errorf("ensuring rate limit table: %w", err)
	}

	rl := auth.NewRateLimiter(db.DB, cfg.Security.LoginRateLimit, time.Duration(cfg.Security.LoginRateWindow)*time.Second)

	switch {
	case all:
		removed, err := rl.ResetAllLockouts()
		if err != nil {
			return err
		}
		fmt.Printf("已清除所有登录锁定（%d 条记录）\n", removed)
	case ip != "":
		if err := rl.ResetLoginAttempts(username, ip); err != nil {
			return err
		}
		fmt.Printf("已解锁 %s（来源 %s）\n", username, ip)
	default:
		entries, err := rl.ListLockedEntries()
		if err != nil {
			return err
		}
		removed, err := rl.ResetUserAttempts(username)
		if err != nil {
			return err
		}
		fmt.Printf("已解锁 %s（清除 %d 条记录）\n", username, removed)
		for _, e := range entries {
			if e.Username == username {
				fmt.Printf("  - 曾被锁定: %s@%s，尝试 %d 次，锁定至 %s\n",
					e.Username, e.IP, e.Attempts, e.LockedUntil.Format("2006-01-02 15:04:05"))
			}
		}
	}

	// Show remaining lockouts so the operator can confirm the state.
	remaining, err := rl.ListLockedEntries()
	if err == nil && len(remaining) > 0 {
		fmt.Printf("当前仍有 %d 个锁定：\n", len(remaining))
		for _, e := range remaining {
			fmt.Printf("  - %s@%s，锁定至 %s\n", e.Username, e.IP, e.LockedUntil.Format("2006-01-02 15:04:05"))
		}
	}

	return nil
}
