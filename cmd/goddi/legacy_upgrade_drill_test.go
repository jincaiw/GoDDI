package main

// The legacy-baseline upgrade drill.
//
// P0-G1 pins the split: this work tree's baseline is the upstream commit
// 8d665d2, whose last release tags (v0.8.2/v0.9.0) carried **25** control
// migrations. The current tree ships **32**. A real site upgrading runs
// exactly this path: a database carrying the 25-migration baseline and its
// data, then the newer binary's migrations on top.
//
// The drill builds that legacy database (migrations 001–025 from the current
// tree's own migration files — the 026–032 set did not exist at the
// baseline), inserts legacy-shaped rows, applies the remaining migrations,
// and asserts the data survived byte-for-byte in the columns a real site
// would check. It cannot prove anything about data this synthetic fixture
// does not model; the real legacy-data reconciliation stays with the real
// legacy database.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jincaiw/GoDDI"
	"github.com/jincaiw/GoDDI/internal/config"
	"github.com/jincaiw/GoDDI/internal/database"
)

const legacyBaselineMigrations = 25 // v0.8.2/v0.9.0: 001–025

func splitLegacyAndCurrent(t *testing.T) (legacyDir, currentDir string, legacyNames, currentNames []string) {
	t.Helper()
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) != 39 {
		t.Fatalf("this drill pins the migration split: got %d files, want 39 (25 legacy + 14 current)", len(names))
	}
	legacyDir = t.TempDir()
	currentDir = t.TempDir()
	for i, n := range names {
		raw, err := os.ReadFile(filepath.Join("../../migrations", n))
		if err != nil {
			t.Fatal(err)
		}
		dst := currentDir
		if i < legacyBaselineMigrations {
			dst = legacyDir
			legacyNames = append(legacyNames, n)
		} else {
			currentNames = append(currentNames, n)
		}
		if err := os.WriteFile(filepath.Join(dst, n), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if len(legacyNames) != legacyBaselineMigrations || len(currentNames) == 0 {
		t.Fatalf("split produced %d legacy / %d current", len(legacyNames), len(currentNames))
	}
	return legacyDir, currentDir, legacyNames, currentNames
}

func TestLegacyBaselineUpgradeKeepsTheData(t *testing.T) {
	legacyDir, _, legacyNames, currentNames := splitLegacyAndCurrent(t)

	// 1. Build the legacy database: only the baseline migrations run here.
	legacyDSN := filepath.Join(t.TempDir(), "legacy.db")
	dbh, err := database.New(config.DatabaseConfig{Driver: "sqlite", DSN: legacyDSN})
	if err != nil {
		t.Fatal(err)
	}
	if err := dbh.RunMigrations(legacyDir); err != nil {
		t.Fatalf("building the legacy baseline: %v", err)
	}

	// 2. Legacy-shaped data, written the way the legacy binary would have.
	legacyZone := fmt.Sprintf("legacy-site%d.example", legacyBaselineMigrations)
	legacyUser := "legacy-operator"
	if _, err := dbh.Exec(`INSERT INTO dns_zones (id,name,type,soa_mname,soa_rname,serial) VALUES ('zone-legacy-1',?,'primary','ns1.`+legacyZone+`','admin.`+legacyZone+`',1970010101)`, legacyZone); err != nil {
		t.Fatalf("inserting a legacy zone: %v", err)
	}
	if _, err := dbh.Exec(`INSERT INTO dns_records (id,zone_id,name,type,value,ttl) VALUES ('rec-legacy-1','zone-legacy-1','www.` + legacyZone + `','A','192.0.2.77',600)`); err != nil {
		// The exact legacy column set differs across baselines; a failed insert
		// here means the drill's fixture drifted from the baseline schema and
		// must be fixed, not skipped.
		t.Fatalf("inserting a legacy record: %v", err)
	}
	if _, err := dbh.Exec(`INSERT INTO users (id,username,password_hash,enabled) VALUES ('user-legacy-1',?, '$argon2id$v=19$m=65536,t=1,p=4$c2FsdHNhbHQ$c2FsdHNhbHRzYWx0c2FsdHNhbHRzYWx0', 1)`, legacyUser); err != nil {
		t.Fatalf("inserting a legacy user: %v", err)
	}

	// 3. The newer binary applies the remaining migrations (26–032) from its
	// embedded set — goose skips the 25 already applied.
	if err := dbh.RunMigrationsFS(goddiassets.Migrations()); err != nil {
		t.Fatalf("applying the current migrations over the legacy baseline: %v", err)
	}

	// 4. The version table says all 32 applied.
	var version int64
	if err := dbh.QueryRow(`SELECT max(version_id) FROM goose_db_version`).Scan(&version); err != nil {
		t.Fatalf("reading the goose version: %v", err)
	}
	if version < int64(len(legacyNames)+len(currentNames)) {
		t.Fatalf("goose version = %d, want at least %d (25 legacy + current set)", version, len(legacyNames)+len(currentNames))
	}

	// 5. The legacy data survived: same zone name, same record value, same user.
	var name string
	if err := dbh.QueryRow(`SELECT name FROM dns_zones WHERE id='zone-legacy-1'`).Scan(&name); err != nil {
		t.Fatalf("the legacy zone did not survive the upgrade: %v", err)
	}
	if name != legacyZone {
		t.Fatalf("legacy zone name = %q, want %q", name, legacyZone)
	}
	var value string
	if err := dbh.QueryRow(`SELECT value FROM dns_records WHERE id='rec-legacy-1'`).Scan(&value); err != nil {
		t.Fatalf("the legacy record did not survive: %v", err)
	}
	if value != "192.0.2.77" {
		t.Fatalf("legacy record value = %q, want 192.0.2.77", value)
	}
	var enabled bool
	if err := dbh.QueryRow(`SELECT enabled FROM users WHERE username=?`, legacyUser).Scan(&enabled); err != nil {
		t.Fatalf("the legacy user did not survive: %v", err)
	}
	if !enabled {
		t.Fatal("the legacy user must stay enabled through the upgrade")
	}

	// 6. The new-migration objects exist (a spot check from the 026–032 set).
	var newTables int
	if err := dbh.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('dns_zone_transfer','dns_dynamic_update_policies')`).Scan(&newTables); err != nil {
		t.Fatal(err)
	}
	if newTables != 2 {
		t.Fatalf("the newer migrations did not create the expected tables: %d", newTables)
	}
	_ = fmt.Sprint(legacyUser)
}
