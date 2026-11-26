package main

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jincaiw/GoDDI/internal/config"
)

func controlCfg(dsn string) *config.Config {
	return &config.Config{Database: config.DatabaseConfig{Driver: "sqlite", DSN: dsn}}
}

func TestServePreflightAllowsFreshControlDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.db")
	cfg := controlCfg(path)
	// The real binaryFS is used by the production call; this test only needs
	// the helper to accept a missing file without creating it.
	if err := checkControlCompatibilityWithFS(cfg, fstest.MapFS{"001_first.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")}}); err != nil {
		t.Fatalf("fresh database refused: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("preflight created the database: %v", err)
	}
}

func TestServePreflightRefusesNewerControlSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ahead.db")
	uri := (&url.URL{Scheme: "file", Path: path}).String()
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY AUTOINCREMENT, version_id INTEGER NOT NULL, is_applied INTEGER NOT NULL);`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO goose_db_version(version_id,is_applied) VALUES (0,1),(2,1)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{"001_first.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")}}
	err = checkControlCompatibilityWithFS(controlCfg(path), fsys)
	if err == nil || !strings.Contains(err.Error(), "比本二进制更新") {
		t.Fatalf("newer schema = %v, want refusal", err)
	}
}

func TestServePreflightRefusesUnreadableControlState(t *testing.T) {
	fsys := fstest.MapFS{"001_first.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")}}
	err := checkControlCompatibilityWithFS(controlCfg("file:mem?mode=memory"), fsys)
	if err == nil || !strings.Contains(err.Error(), "无法读取") {
		t.Fatalf("memory DSN = %v, want refusal", err)
	}
}
