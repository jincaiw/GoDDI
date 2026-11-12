package main

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestMigrationStatusUsesReadOnlyFileWithoutDSNPragmas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db space #?%25.sqlite")
	uri := (&url.URL{Scheme: "file", Path: path}).String()
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE marker (id INTEGER); INSERT INTO marker VALUES (42)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	migrations := fstest.MapFS{"001_first.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")}}
	for _, dsn := range []string{path, uri + "?mode=rwc&_pragma=journal_mode%28WAL%29"} {
		status := migrationStatusOfStore(migrateStore{set: "leases", dsn: dsn, fsys: migrations})
		if status.Err != nil || !status.Configured || !status.FilePresent || status.Path != path || status.TablePresent || len(status.Pending) != 1 {
			t.Fatalf("status for %q: %+v", dsn, status)
		}
	}
	check, err := sql.Open("sqlite", uri+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var mode string
	var value, tables int
	if err := check.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if err := check.QueryRow("SELECT id FROM marker").Scan(&value); err != nil {
		t.Fatal(err)
	}
	if err := check.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='goose_db_version'").Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if mode != "delete" || value != 42 || tables != 0 {
		t.Fatalf("database changed: mode=%s value=%d migrations=%d", mode, value, tables)
	}
}

func TestMigrationStatusMissingAndUnconfigured(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing #?%.sqlite")
	uri := (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=rwc&_pragma=journal_mode%28WAL%29"}).String()
	migrations := fstest.MapFS{"001_first.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")}}
	for _, dsn := range []string{path, uri} {
		status := migrationStatusOfStore(migrateStore{set: "leases", dsn: dsn, fsys: migrations})
		if status.Err != nil || !status.Configured || status.FilePresent || status.Path != path || len(status.Pending) != 1 {
			t.Fatalf("missing %q: %+v", dsn, status)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing file created: %v", err)
	}
	status := migrationStatusOfStore(migrateStore{set: "leases", dsn: " ", fsys: migrations})
	if status.Err != nil || status.Configured || status.FilePresent || len(status.Pending) != 1 {
		t.Fatalf("unconfigured: %+v", status)
	}
	status = migrationStatusOfStore(migrateStore{set: "leases", dsn: "file:mem?mode=memory", fsys: migrations})
	if status.Err == nil || !status.Configured || status.FilePresent {
		t.Fatalf("memory: %+v", status)
	}
}
