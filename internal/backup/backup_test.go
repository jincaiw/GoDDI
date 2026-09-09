package backup

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
)

func TestDNSBackupPreservesNullableNumbers(t *testing.T) {
	mgr, db := setupBackupTest(t)
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO dns_zones (id,name,type,soa_mname,soa_rname,serial) VALUES ('nullable-zone','nullable.example','primary','ns.nullable.example','admin.nullable.example',1); INSERT INTO dns_records (id,zone_id,name,type,value,priority,port,weight) VALUES ('nullable-record','nullable-zone','www','A','192.0.2.42',NULL,NULL,NULL)`); err != nil {
		t.Fatal(err)
	}
	job, err := mgr.CreateBackup(BackupOptions{Type: "dns"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.RestoreBackup(job.ID); err != nil {
		t.Fatal(err)
	}
	var priority, port, weight sql.NullInt64
	if err := db.QueryRow(`SELECT priority,port,weight FROM dns_records WHERE id='nullable-record'`).Scan(&priority, &port, &weight); err != nil {
		t.Fatal(err)
	}
	if priority.Valid || port.Valid || weight.Valid {
		t.Fatal("NULL numeric fields changed during restore")
	}
	info, err := os.Stat(job.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("backup permissions: %v", info.Mode())
	}
}

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

func TestDNSBackupRestorePreservesACLAndZonePermissions(t *testing.T) {
	mgr, db := setupBackupTest(t)
	defer db.Close()

	acl := `{"query":["10.0.0.0/8"],"transfer":["192.0.2.0/24"],"update":["198.51.100.7"]}`
	if _, err := db.Exec(`
		INSERT INTO dns_zones (id,name,type,soa_mname,soa_rname,serial,acl) VALUES ('secured-zone','secured.example','primary','ns.secured.example','admin.secured.example',1,?);
		INSERT INTO dns_zone_permissions (id,zone_id,principal_type,principal_id,can_view,can_modify,can_delete) VALUES ('permission-user','secured-zone','user','user-1',1,1,0);
		INSERT INTO dns_zone_permissions (id,zone_id,principal_type,principal_id,can_view,can_modify,can_delete) VALUES ('permission-group','secured-zone','group','group-1',1,0,1);
	`, acl); err != nil {
		t.Fatal(err)
	}

	job, err := mgr.CreateBackup(BackupOptions{Type: "dns"})
	if err != nil || job.Status != "completed" {
		t.Fatalf("CreateBackup() job=%+v err=%v", job, err)
	}
	if _, err := db.Exec(`DELETE FROM dns_zone_permissions; DELETE FROM dns_zones`); err != nil {
		t.Fatal(err)
	}
	if err := mgr.RestoreBackup(job.ID); err != nil {
		t.Fatal(err)
	}

	var restoredACL string
	if err := db.QueryRow(`SELECT acl FROM dns_zones WHERE id = 'secured-zone'`).Scan(&restoredACL); err != nil {
		t.Fatal(err)
	}
	if restoredACL != acl {
		t.Fatalf("ACL changed during restore: got %q want %q", restoredACL, acl)
	}
	var canView, canModify, canDelete bool
	if err := db.QueryRow(`SELECT can_view,can_modify,can_delete FROM dns_zone_permissions WHERE id = 'permission-user'`).Scan(&canView, &canModify, &canDelete); err != nil {
		t.Fatal(err)
	}
	if !canView || !canModify || canDelete {
		t.Fatalf("user permission changed during restore: %t %t %t", canView, canModify, canDelete)
	}
	if err := db.QueryRow(`SELECT can_view,can_modify,can_delete FROM dns_zone_permissions WHERE id = 'permission-group'`).Scan(&canView, &canModify, &canDelete); err != nil {
		t.Fatal(err)
	}
	if !canView || canModify || !canDelete {
		t.Fatalf("group permission changed during restore: %t %t %t", canView, canModify, canDelete)
	}
}

func TestRestoreLegacyDNSBackupPreservesExistingRestrictions(t *testing.T) {
	mgr, db := setupBackupTest(t)
	defer db.Close()

	acl := `{"query":["10.0.0.0/8"]}`
	if _, err := db.Exec(`
		INSERT INTO dns_zones (id,name,type,soa_mname,soa_rname,serial,acl) VALUES ('existing-zone','existing.example','primary','ns.existing.example','admin.existing.example',1,?);
		INSERT INTO dns_zone_permissions (id,zone_id,principal_type,principal_id,can_view,can_modify,can_delete) VALUES ('existing-permission','existing-zone','user','user-1',1,0,0);
	`, acl); err != nil {
		t.Fatal(err)
	}

	jobID := "legacy-dns"
	backupDir := filepath.Join(mgr.dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(backupDir, "legacy-dns.json")
	payload := `{"metadata":{"id":"legacy-dns","type":"dns","created_at":"2026-01-01T00:00:00Z","version":"old","description":""},"data":{"dns":{"zones":[{"id":"legacy-zone","name":"legacy.example","type":"primary","enabled":true,"dnssec_enabled":false,"default_ttl":3600,"soa_mname":"ns.legacy.example","soa_rname":"admin.legacy.example","serial":1,"refresh":3600,"retry":600,"expire":86400,"minimum":300}],"records":[],"forwarders":[],"conditional_forwarders":[]}}}`
	if err := os.WriteFile(path, []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO backup_jobs (id,type,status,file_path,description,created_at) VALUES (?,?,?,?,?,?)`, jobID, "dns", "completed", path, "legacy", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}

	err := mgr.RestoreBackup(jobID)
	if err == nil || !strings.Contains(err.Error(), "lacks zone_permissions") {
		t.Fatalf("RestoreBackup() error = %v, want missing zone_permissions compatibility error", err)
	}
	var restoredACL string
	if err := db.QueryRow(`SELECT acl FROM dns_zones WHERE id = 'existing-zone'`).Scan(&restoredACL); err != nil {
		t.Fatal(err)
	}
	if restoredACL != acl {
		t.Fatalf("legacy restore changed ACL: got %q want %q", restoredACL, acl)
	}
	var permissions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dns_zone_permissions WHERE zone_id = 'existing-zone'`).Scan(&permissions); err != nil {
		t.Fatal(err)
	}
	if permissions != 1 {
		t.Fatalf("legacy restore changed zone permissions: got %d want 1", permissions)
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
