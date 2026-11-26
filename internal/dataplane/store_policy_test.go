package dataplane

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jincaiw/GoDDI/internal/config"
)

func TestWriterPolicyAfterPhysicalReconnect(t *testing.T) {
	for _, role := range []config.DataPlaneRole{config.DataPlaneLease, config.DataPlaneZone} {
		t.Run(string(role), func(t *testing.T) {
			s := newStore(t, role)
			if _, err := s.Exec(`CREATE TABLE policy_parent(id INTEGER PRIMARY KEY); CREATE TABLE policy_child(parent_id INTEGER REFERENCES policy_parent(id))`); err != nil {
				t.Fatal(err)
			}
			s.SetMaxIdleConns(0)
			for _, tc := range []struct{ query, want string }{
				{"PRAGMA busy_timeout", "5000"}, {"PRAGMA foreign_keys", "1"}, {"PRAGMA journal_mode", "wal"}, {"PRAGMA synchronous", "2"},
			} {
				var got string
				if err := s.QueryRow(tc.query).Scan(&got); err != nil {
					t.Fatal(err)
				}
				if got != tc.want {
					t.Fatalf("%s = %q, want %q", tc.query, got, tc.want)
				}
			}
			if _, err := s.Exec(`INSERT INTO policy_child VALUES (17)`); err == nil {
				t.Fatal("foreign key violation accepted")
			}
		})
	}
}

func TestWriterRejectsNonWALStore(t *testing.T) {
	s, err := Open(config.DataPlaneLease, ":memory:")
	if s != nil {
		s.Close()
		t.Fatal("unexpected usable store")
	}
	if err == nil || !strings.Contains(err.Error(), "journal_mode") {
		t.Fatalf("expected WAL verification error, got %v", err)
	}
}

func TestReadOnlySpecialPathsAndURIParameters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lease space #?%25.db")
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
	for _, dsn := range []string{path, uri, uri + "?mode=rwc&cache=shared&_pragma=journal_mode%28WAL%29"} {
		s, err := OpenReadOnly(config.DataPlaneLease, dsn)
		if err != nil {
			t.Fatalf("open %q: %v", dsn, err)
		}
		var value int
		var mode string
		if err := s.QueryRow(`SELECT id FROM marker`).Scan(&value); err != nil || value != 42 {
			t.Fatalf("%q: value=%d err=%v", dsn, value, err)
		}
		if err := s.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "delete" {
			t.Fatalf("%q: mode=%s err=%v", dsn, mode, err)
		}
		if _, err := s.Exec(`INSERT INTO marker VALUES (99)`); err == nil {
			t.Fatalf("%q allowed write", dsn)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadOnlyRejectsMissingAndMemory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent?#.db")
	for _, dsn := range []string{missing, ":memory:", "file::memory:?cache=shared", "file:memory-name?mode=memory", "file:/tmp/missing?bad=%ZZ", "file://remote/tmp/db"} {
		s, err := OpenReadOnly(config.DataPlaneLease, dsn)
		if s != nil {
			s.Close()
			t.Fatalf("opened %q", dsn)
		}
		if err == nil {
			t.Fatalf("accepted %q", dsn)
		}
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("missing path changed: %v", err)
	}
}

func TestReadOnlyDoesNotApplyWriterPolicyOrMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE original (id INTEGER); INSERT INTO original VALUES (42)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := OpenReadOnly(config.DataPlaneLease, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var mode string
	if err := s.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "delete" {
		t.Fatalf("read-only open changed journal mode: %s", mode)
	}
	var id int
	if err := s.QueryRow(`SELECT id FROM original`).Scan(&id); err != nil || id != 42 {
		t.Fatalf("original data: id=%d err=%v", id, err)
	}
	if _, err := s.Exec(`INSERT INTO original VALUES (43)`); err == nil {
		t.Fatal("read-only store allowed write")
	}
	var migrations int
	if err := s.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='goose_db_version'`).Scan(&migrations); err != nil || migrations != 0 {
		t.Fatalf("read-only open ran migrations: count=%d err=%v", migrations, err)
	}
}
