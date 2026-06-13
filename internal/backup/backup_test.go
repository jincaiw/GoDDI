package backup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
)

func setupBackupTest(t *testing.T) (*Manager, *database.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := database.New(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(dir, "test.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(filepath.Join("..", "..", "migrations")); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return NewManager(db.DB, dir, "test"), db
}

func TestSecurityBackupRestorePreservesFields(t *testing.T) {
	mgr, db := setupBackupTest(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO dns_block_lists (id,name,type,url,enabled,last_updated,entry_count) VALUES ('l1','list','remote','https://example.test/list',1,'2026-01-01T00:00:00Z',2);
		INSERT INTO dns_block_rules (id,list_id,pattern,match_type,response_type,response_data,enabled) VALUES ('r1','l1','bad.test','suffix','A','0.0.0.0',1);
		INSERT INTO dns_allow_rules (id,pattern,match_type,enabled) VALUES ('a1','good.test','exact',1);
		INSERT INTO dns_client_policies (id,name,source_cidr,action,block_list_ids,allow_rule_ids,priority,enabled) VALUES ('p1','policy','10.0.0.0/8','allow','["l1"]','["a1"]',7,1);
	`)
	if err != nil {
		t.Fatal(err)
	}

	job, err := mgr.CreateBackup(BackupOptions{Type: "security", Description: "security-test"})
	if err != nil || job.Status != "completed" {
		t.Fatalf("CreateBackup() job=%+v err=%v", job, err)
	}
	if _, err := db.Exec(`DELETE FROM dns_block_rules; DELETE FROM dns_allow_rules; DELETE FROM dns_client_policies; DELETE FROM dns_block_lists`); err != nil {
		t.Fatal(err)
	}
	if err := mgr.RestoreBackup(job.ID); err != nil {
		t.Fatal(err)
	}

	var listType, url, matchType, responseType, responseData, action string
	var priority int
	if err := db.QueryRow(`SELECT type,url FROM dns_block_lists WHERE id='l1'`).Scan(&listType, &url); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT match_type,response_type,response_data FROM dns_block_rules WHERE id='r1'`).Scan(&matchType, &responseType, &responseData); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT action,priority FROM dns_client_policies WHERE id='p1'`).Scan(&action, &priority); err != nil {
		t.Fatal(err)
	}
	if listType != "remote" || url != "https://example.test/list" || matchType != "suffix" || responseType != "A" || responseData != "0.0.0.0" || action != "allow" || priority != 7 {
		t.Fatalf("restored fields differ: %q %q %q %q %q %q %d", listType, url, matchType, responseType, responseData, action, priority)
	}
}

func TestRestoreSecurityRollsBackOnInvalidRow(t *testing.T) {
	mgr, db := setupBackupTest(t)
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO dns_allow_rules (id,pattern,match_type,enabled) VALUES ('keep','keep.test','exact',1)`); err != nil {
		t.Fatal(err)
	}
	jobID := "bad-restore"
	backupDir := filepath.Join(mgr.dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(backupDir, "bad.json")
	payload := `{"metadata":{"id":"bad-restore","type":"security","created_at":"2026-01-01T00:00:00Z","version":"test","description":""},"data":{"security":{"allow_rules":[{"id":"a1","pattern":"one.test","enabled":true},{"id":"a1","pattern":"two.test","enabled":true}]}}}`
	if err := os.WriteFile(path, []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO backup_jobs (id,type,status,file_path,description,created_at) VALUES (?,?,?,?,?,?)`, jobID, "security", "completed", path, "bad", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}

	if err := mgr.RestoreBackup(jobID); err == nil {
		t.Fatal("RestoreBackup() succeeded with an invalid required field")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dns_allow_rules WHERE id='keep'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("restore failure did not roll back the original security data")
	}
}
