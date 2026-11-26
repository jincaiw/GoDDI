package main

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	goddiassets "github.com/jincaiw/GoDDI"
	"github.com/jincaiw/GoDDI/internal/auth"
	"github.com/jincaiw/GoDDI/internal/config"
	"github.com/jincaiw/GoDDI/internal/database"
	"github.com/jincaiw/GoDDI/internal/dataplane"
	applog "github.com/jincaiw/GoDDI/internal/log"
	"github.com/jincaiw/GoDDI/internal/rbac"
)

func runMigrations(configPath string) error {
	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	config.ApplyEnvOverrides(cfg)

	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if err := rejectEnterprise(cfg); err != nil {
		return err
	}

	applog.InitLogger(cfg.Log.Level)

	// The control database lives under the data directory, and SQLite cannot
	// create a file in a directory that is not there -- it reports only
	// "unable to open database file". runServer creates that directory, so a
	// host that has been started once never shows the gap; this command is the
	// documented way to prepare a database, so it has to create it too.
	if dir := strings.TrimSpace(cfg.Server.DataDir); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating data directory %s: %w", dir, err)
		}
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}
	defer db.Close()

	if err := db.RunMigrationsFS(goddiassets.Migrations()); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// Also initialize RBAC data during migration.
	rbacMgr := rbac.NewRBACManager(db.DB)
	if err := rbacMgr.InitializePredefinedData(); err != nil {
		return fmt.Errorf("initializing RBAC data: %w", err)
	}

	// Auto-initialize admin user if needed.
	if err := auth.AutoInitializeAdmin(db.DB); err != nil {
		slog.Warn("auto-initialize admin failed", "error", err)
	}

	slog.Info("migrations completed successfully")
	return nil
}

// migrateStore is one database whose schema this deployment owns and whose
// migrations the migrate command has to be able to report on.
type migrateStore struct {
	set  string
	dsn  string
	fsys fs.FS
}

// migrationStores lists the databases this deployment owns.
//
// It mirrors snapshotTargets deliberately: the three stores the archiver
// copies are the three whose schemas have to line up with the binary, and a
// store that could be restored but not reported on would be exactly the gap
// this command exists to close.
func migrationStores(cfg *config.Config) []migrateStore {
	return []migrateStore{
		{set: "control", dsn: cfg.Database.DSN, fsys: goddiassets.Migrations()},
		{set: "dnsdata", dsn: cfg.DataPlaneDSN(config.DataPlaneZone), fsys: dataplane.Migrations()},
		{set: "leases", dsn: cfg.DataPlaneDSN(config.DataPlaneLease), fsys: dataplane.Migrations()},
	}
}

// runMigrationStatus reports every store's migration state and, with check,
// fails when there is anything to apply or anything this build cannot place.
func runMigrationStatus(configPath string, check bool) error {
	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	config.ApplyEnvOverrides(cfg)
	if err := config.ValidateEdition(cfg); err != nil {
		return err
	}

	// No logger is initialized. This command's output is the report, and log
	// lines interleaved with it would make the report unreadable to both the
	// operator and any script reading it.
	//
	// config.Validate is deliberately not called either, for the same reason
	// the snapshot command skips it: the state of the schema is most worth
	// inspecting on a deployment whose configuration is already suspect, and
	// refusing to look because something unrelated is wrong is the wrong way
	// round.
	fmt.Printf("迁移状态（本二进制 %s）\n", Version)

	toApply, unrecognised, unreadable := 0, 0, 0
	for _, store := range migrationStores(cfg) {
		status := migrationStatusOfStore(store)
		location := status.Path
		if location == "" {
			location = "无存放位置"
		}
		fmt.Printf("  %s  %s\n", status.Set, location)
		fmt.Printf("    %s\n", status.Summary())
		toApply += len(status.Pending)
		unrecognised += len(status.Withdrawn)
		if status.Err != nil {
			unreadable++
		}
	}
	fmt.Printf("\n待应用合计：%d\n", toApply)
	if unrecognised > 0 {
		fmt.Printf("本二进制不认识的版本合计：%d\n", unrecognised)
	}
	if unreadable > 0 {
		fmt.Printf("读取失败的存储合计：%d\n", unreadable)
	}

	if !check {
		return nil
	}
	switch {
	case unreadable > 0:
		return fmt.Errorf("有 %d 个存储无法读取，无法判断迁移状态；请先解决读取失败再运行 migrate", unreadable)
	case unrecognised > 0:
		return fmt.Errorf("有 %d 个已应用的迁移不在本二进制自带之列：该数据库比这个二进制更新，用它继续运行会把 schema 回退到本二进制理解的版本，请改用与其匹配的二进制", unrecognised)
	case toApply > 0:
		// `migrate` deliberately prepares only the control store (see
		// docs/实施计划-v0.6.0.md W12-b): --status/--check must be able to
		// inspect a deployment without creating databases. The data-plane
		// stores are created and migrated by `serve` on its first start.
		// Without this note the message sends an operator on a fresh install
		// back to `migrate`, which cannot apply those pending migrations.
		return fmt.Errorf("有 %d 个迁移待应用；请运行 goddi migrate 应用它们；若待应用项来自 dnsdata/leases 数据面库，它们由首次启动 goddi serve 创建并迁移（migrate 不会创建数据面库）", toApply)
	}
	fmt.Println("migrate --check：每个存储都不缺迁移，且没有本二进制不认识的版本。")
	return nil
}

func checkControlCompatibilityWithFS(cfg *config.Config, fsys fs.FS) error {
	store := migrateStore{set: "control", dsn: cfg.Database.DSN, fsys: fsys}
	status := migrationStatusOfStore(store)
	switch {
	case status.Err != nil:
		return fmt.Errorf("控制库迁移状态无法读取，拒绝启动：%w", status.Err)
	case len(status.Withdrawn) > 0:
		return fmt.Errorf("控制库已应用的迁移版本 %v 不在本二进制之列；该数据库比本二进制更新，拒绝启动", status.Withdrawn)
	case status.FilePresent && !status.TablePresent && len(status.Pending) > 0:
		return fmt.Errorf("控制库文件已存在但没有迁移记录，且本二进制有 %d 个待应用迁移；拒绝在未知 schema 上运行", len(status.Pending))
	}
	return nil
}

func requireControlMigrationCompatibility(cfg *config.Config) error {
	if !strings.EqualFold(strings.TrimSpace(cfg.Database.Driver), "sqlite") {
		return nil // this preflight reads a SQLite file without opening it for writes
	}
	return checkControlCompatibilityWithFS(cfg, goddiassets.Migrations())
}

// migrationStatusOfStore reads one store's migration state without creating or
// changing anything.
func migrationStatusOfStore(store migrateStore) database.MigrationStatus {
	if strings.TrimSpace(store.dsn) == "" {
		status := database.PendingMigrationStatus(store.fsys, store.set)
		status.Configured = false
		return status
	}
	path, err := dataplane.ReadOnlyPath(store.dsn)
	if err != nil {
		status := database.PendingMigrationStatus(store.fsys, store.set)
		status.Configured = true
		status.Err = err
		return status
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			// Never opened: opening a SQLite path creates the file, and a
			// report that brings the database into existence has answered a
			// different question than the one it was asked.
			status := database.PendingMigrationStatus(store.fsys, store.set)
			status.Configured = true
			status.Path = path
			return status
		}
		status := database.PendingMigrationStatus(store.fsys, store.set)
		status.Configured = true
		status.Path = path
		status.Err = err
		return status
	}

	conn, err := dataplane.OpenReadOnly(config.DataPlaneRole(store.set), store.dsn)
	if err != nil {
		status := database.PendingMigrationStatus(store.fsys, store.set)
		status.Configured = true
		status.Path = path
		if !os.IsNotExist(err) {
			status.Err = err
		}
		return status
	}
	defer conn.Close()

	status := database.MigrationStatusOf(conn.DB, store.fsys, store.set)
	status.Configured = true
	status.Path = path
	return status
}

// knownSchemaVersions is the highest migration this binary carries for each
// store it knows how to restore.
//
// It returns an error rather than an empty map when a migration set cannot be
// read: a ceiling that is missing because the list failed to load would make
// the restore gate below describe every archive as acceptable, and a gate that
// passes because it could not look is worse than no gate.
func knownSchemaVersions() (map[string]int64, error) {
	sets := []struct {
		set  string
		fsys fs.FS
	}{
		{set: "control", fsys: goddiassets.Migrations()},
		{set: "dnsdata", fsys: dataplane.Migrations()},
		{set: "leases", fsys: dataplane.Migrations()},
	}
	ceiling := make(map[string]int64, len(sets))
	for _, entry := range sets {
		versions, err := database.ShippedMigrations(entry.fsys)
		if err != nil {
			return nil, fmt.Errorf("读取 %s 库自带的迁移列表：%w", entry.set, err)
		}
		if len(versions) == 0 {
			continue
		}
		ceiling[entry.set] = versions[len(versions)-1]
	}
	return ceiling, nil
}
