package backup

import (
	"errors"
	"testing"
)

// Regression tests for the backup 404 handling fix: "no such backup" must
// surface as ErrNotFound so the HTTP layer can map it to 404 instead of 500.

func TestGetBackup_NotFound(t *testing.T) {
	mgr, db := setupBackupTest(t)
	defer db.Close()

	_, err := mgr.GetBackup("00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("GetBackup(nonexistent) = nil error, want ErrNotFound")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetBackup(nonexistent) error = %v, want ErrNotFound", err)
	}
}

func TestRestoreBackup_NotFound(t *testing.T) {
	mgr, db := setupBackupTest(t)
	defer db.Close()

	err := mgr.RestoreBackup("00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("RestoreBackup(nonexistent) = nil error, want error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("RestoreBackup(nonexistent) error = %v, want wrapped ErrNotFound", err)
	}
}

func TestDeleteBackup_NotFound(t *testing.T) {
	mgr, db := setupBackupTest(t)
	defer db.Close()

	err := mgr.DeleteBackup("00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("DeleteBackup(nonexistent) = nil error, want error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteBackup(nonexistent) error = %v, want wrapped ErrNotFound", err)
	}
}

func TestCreateBackup_UnknownType(t *testing.T) {
	mgr, db := setupBackupTest(t)
	defer db.Close()

	job, err := mgr.CreateBackup(BackupOptions{Type: "bogus"})
	if err != nil {
		t.Fatalf("CreateBackup returned transport error: %v", err)
	}
	if job == nil || job.Status != "failed" {
		t.Fatalf("CreateBackup(bogus type) job = %+v, want failed status", job)
	}
}
