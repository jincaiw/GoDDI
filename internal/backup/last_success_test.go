package backup

// The backup-age gauge is what raises "backups have stopped", so the query
// behind it has to distinguish states a single timestamp cannot: no backup of a
// type has ever succeeded, one succeeded and here is when, and a row exists but
// its timestamp cannot be read.

import (
	"testing"
	"time"
)

func insertBackupJob(t *testing.T, mgr *Manager, id, backupType, status, completedAt string) {
	t.Helper()
	var completed any
	if completedAt != "" {
		completed = completedAt
	}
	if _, err := mgr.db.Exec(`
		INSERT INTO backup_jobs (id, type, status, completed_at, created_at)
		VALUES (?, ?, ?, ?, datetime('now'))
	`, id, backupType, status, completed); err != nil {
		t.Fatalf("seed backup job: %v", err)
	}
}

// TestNoSuccessfulBackupIsAbsentNotZero is the case that decides the alert
// shape. Reporting the zero time would make "never backed up" and "backed up in
// 1970" the same value, and every "older than a day" expression matches both.
func TestNoSuccessfulBackupIsAbsentNotZero(t *testing.T) {
	mgr, db := setupBackupTest(t)
	defer db.Close()

	last, err := mgr.LastSuccessfulBackups()
	if err != nil {
		t.Fatalf("LastSuccessfulBackups on an empty table: %v", err)
	}
	if len(last) != 0 {
		t.Fatalf("reported backups that do not exist: %v", last)
	}

	// A failed job is not a successful one. Counting it would date the last
	// good backup to the moment backups stopped working.
	insertBackupJob(t, mgr, "job-failed", "full", "failed", time.Now().Format(time.RFC3339))

	last, err = mgr.LastSuccessfulBackups()
	if err != nil {
		t.Fatalf("LastSuccessfulBackups with only failed jobs: %v", err)
	}
	if len(last) != 0 {
		t.Fatalf("a failed job was counted as a successful backup: %v", last)
	}
}

// TestEachBackupTypeCarriesItsOwnAge is why the metric is a vector. A type that
// has never succeeded must not be dated by another type's success -- an
// installation taking daily DNS backups and no full backup has a working
// schedule for one and none at all for the other.
func TestEachBackupTypeCarriesItsOwnAge(t *testing.T) {
	mgr, db := setupBackupTest(t)
	defer db.Close()

	// Both layouts the column holds: the RFC3339 string the backup path
	// writes, and SQLite's own datetime('now') form written by anything that
	// touches the row directly.
	insertBackupJob(t, mgr, "dns-older", "dns", "completed", "2026-09-01T04:00:00Z")
	insertBackupJob(t, mgr, "dns-newer", "dns", "completed", "2026-09-09 06:30:00")
	insertBackupJob(t, mgr, "full-newest", "full", "completed", "2026-09-10T02:00:00Z")
	insertBackupJob(t, mgr, "ipam-failed", "ipam", "failed", "2026-09-10T02:00:00Z")

	last, err := mgr.LastSuccessfulBackups()
	if err != nil {
		t.Fatalf("LastSuccessfulBackups: %v", err)
	}

	if _, ok := last["dns"]; !ok {
		t.Fatal("no dns backup reported, want the 2026-09-09 job")
	}
	if want := time.Date(2026, 9, 9, 6, 30, 0, 0, time.UTC); !last["dns"].Equal(want) {
		t.Errorf("dns = %v, want %v (the newest of that type)", last["dns"].UTC(), want)
	}
	if want := time.Date(2026, 9, 10, 2, 0, 0, 0, time.UTC); !last["full"].Equal(want) {
		t.Errorf("full = %v, want %v", last["full"].UTC(), want)
	}
	if _, ok := last["ipam"]; ok {
		t.Error("a type whose only job failed was reported as backed up")
	}
}

// TestAnUnreadableTimestampIsTreatedAsUnknown keeps a hand-edited row from
// becoming an alarm about a backup that is decades overdue. SQLite's datetime()
// returns NULL for a value it cannot parse, and a NULL drops out of the
// aggregate -- so the type reads as "no successful backup known", which is the
// honest description of a row nobody can interpret.
func TestAnUnreadableTimestampIsTreatedAsUnknown(t *testing.T) {
	mgr, db := setupBackupTest(t)
	defer db.Close()

	insertBackupJob(t, mgr, "job-bad-date", "full", "completed", "last tuesday")

	last, err := mgr.LastSuccessfulBackups()
	if err != nil {
		t.Fatalf("LastSuccessfulBackups: %v", err)
	}
	if at, ok := last["full"]; ok {
		t.Errorf("an unparsable completed_at became %v, want no entry", at)
	}
}
