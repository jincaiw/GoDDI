package main

import (
	"fmt"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
	"github.com/jasonwa/goddi/internal/dns/transfer"
	applog "github.com/jasonwa/goddi/internal/log"
	"github.com/jasonwa/goddi/internal/secretbox"
)

// runMigrations runs database migrations only.
// runRekey moves stored secrets onto security.encryption_key.
//
// The previous key material is the JWT secret: that is what this build used
// before a dedicated key existed, and it is the only other key an operator
// could have had. When the two are the same there is nothing to move, and the
// command says so instead of reporting a successful no-op.
func runRekey(configPath string, dryRun bool) error {
	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	config.ApplyEnvOverrides(cfg)
	// Even dry-run currently opens a potentially creating/writing database handle.
	if err := rejectEnterprise(cfg); err != nil {
		return err
	}
	applog.InitLogger(cfg.Log.Level)

	if cfg.Security.EncryptionKey == "" {
		return fmt.Errorf("security.encryption_key is not set, so there is nothing to rekey onto (set GODDI_SECURITY_ENCRYPTION_KEY)")
	}
	if cfg.Security.EncryptionKey == cfg.Security.JWTSecret {
		return fmt.Errorf("security.encryption_key is identical to the JWT secret; set it to a distinct value before rekeying")
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}
	defer db.Close()

	targets := secretbox.ControlDatabaseSecrets()
	to := secretbox.New(secretbox.LabelTSIG, cfg.Security.EncryptionKey)

	if dryRun {
		outcomes, err := secretbox.RekeyPlan(db.DB, cfg.Security.JWTSecret, cfg.Security.EncryptionKey, targets)
		if err != nil {
			return fmt.Errorf("planning rekey: %w", err)
		}
		plaintext, err := transfer.CountPlaintextTSIGSecrets(db.DB)
		if err != nil {
			return err
		}
		reportRekey(outcomes, true)
		if plaintext > 0 {
			fmt.Printf("另有 %d 条 TSIG 密钥以明文存储，将在下次启动或本次执行时加密\n", plaintext)
		}
		return nil
	}

	outcomes, err := secretbox.Rekey(db.DB, cfg.Security.JWTSecret, cfg.Security.EncryptionKey, targets)
	if err != nil {
		return err
	}
	plaintext, err := transfer.SealPlaintextTSIGSecrets(db.DB, to)
	if err != nil {
		return err
	}
	reportRekey(outcomes, false)
	if plaintext > 0 {
		fmt.Printf("另有 %d 条 TSIG 密钥从明文转为加密存储\n", plaintext)
	}
	return nil
}

func reportRekey(outcomes []secretbox.RekeyOutcome, dryRun bool) {
	moved := 0
	for _, o := range outcomes {
		verb := "已迁移"
		if dryRun {
			verb = "待迁移"
		}
		fmt.Printf("%s %s：重加密 %d 条，明文加密 %d 条，已是最新 %d 条，空值 %d 条\n",
			verb, o.Table, o.Resealed, o.SealedPlaintext, o.AlreadyCurrent, o.Empty)
		moved += o.Resealed + o.SealedPlaintext
	}
	if dryRun {
		if moved == 0 {
			fmt.Println("没有需要迁移的密文；当前密钥可以读取全部已存储的密钥材料")
		} else {
			fmt.Printf("共 %d 条需要迁移；去掉 --dry-run 执行\n", moved)
		}
		return
	}
	fmt.Printf("完成：共迁移 %d 条\n", moved)
}

// --- backup and restore ---
