package database

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jincaiw/GoDDI/internal/config"
)

func TestSQLiteConnectionPolicySurvivesReconnect(t *testing.T) {
	db, err := New(config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "control.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE parent (id INTEGER PRIMARY KEY); CREATE TABLE child (parent_id INTEGER REFERENCES parent(id))`); err != nil {
		t.Fatal(err)
	}
	// Closing all idle connections forces the next query to use a fresh physical connection.
	db.SetMaxIdleConns(0)
	for _, check := range []struct{ query, want string }{
		{"PRAGMA busy_timeout", "5000"}, {"PRAGMA foreign_keys", "1"},
		{"PRAGMA journal_mode", "wal"}, {"PRAGMA synchronous", "2"},
	} {
		var got string
		if err := db.QueryRow(check.query).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != check.want {
			t.Fatalf("%s = %q, want %q", check.query, got, check.want)
		}
	}
	if _, err := db.Exec(`INSERT INTO child(parent_id) VALUES (123)`); err == nil {
		t.Fatal("foreign key violation accepted after reconnect")
	}
}

func TestSQLiteConnectionRejectsNonWALDatabase(t *testing.T) {
	db, err := New(config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"})
	if db != nil {
		db.Close()
		t.Fatal("unexpected usable database")
	}
	if err == nil || !strings.Contains(err.Error(), "journal_mode") {
		t.Fatalf("expected WAL verification failure, got %v", err)
	}
}
