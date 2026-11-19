package backup

import "testing"

// TestFullBackupRestoreRestoresDNSDHCPIPAM proves a "full" backup restores the
// three core domains after the live rows are deleted: the restore path deletes
// and re-inserts each section, so this exercises the real recovery case rather
// than a no-op re-write (D-12).
func TestFullBackupRestoreRestoresDNSDHCPIPAM(t *testing.T) {
	mgr, db := setupBackupTest(t)
	defer db.Close()

	if _, err := db.Exec(`
		INSERT INTO dns_zones (id,name,type,soa_mname,soa_rname,serial) VALUES ('z1','restore.example','primary','ns.restore.example','admin.restore.example',7);
		INSERT INTO dns_records (id,zone_id,name,type,value) VALUES ('rec1','z1','www','A','192.0.2.7');
		INSERT INTO dhcp_scopes (id,name,subnet,start_ip,end_ip,enabled) VALUES ('sc1','restore','10.9.0.0/24','10.9.0.10','10.9.0.20',1);
		INSERT INTO dhcp_reservations (id,scope_id,ip_address,mac_address,hostname,enabled) VALUES ('res1','sc1','10.9.0.15','aa:bb:cc:dd:ee:77','printer',1);
		INSERT INTO ipam_spaces (id,name) VALUES ('sp1','restore-space');
		INSERT INTO ipam_subnets (id,space_id,name,cidr) VALUES ('sn1','sp1','restore-subnet','10.9.0.0/24');
		INSERT INTO ipam_addresses (id,subnet_id,space_id,ip_address,status,observed_state,observed_source) VALUES ('addr1','sn1','sp1','10.9.0.12','used','in_use','dhcp');
	`); err != nil {
		t.Fatal(err)
	}

	job, err := mgr.CreateBackup(BackupOptions{Type: "full", Description: "restore-test"})
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("backup status = %q, want completed", job.Status)
	}

	// Simulate data loss.
	if _, err := db.Exec(`
		DELETE FROM dns_records; DELETE FROM dns_zones;
		DELETE FROM dhcp_reservations; DELETE FROM dhcp_scopes;
		DELETE FROM ipam_addresses; DELETE FROM ipam_subnets; DELETE FROM ipam_spaces;
	`); err != nil {
		t.Fatal(err)
	}

	if err := mgr.RestoreBackup(job.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}

	assertRow := func(table, id string) {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE id = ?`, id).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 1 {
			t.Fatalf("%s id=%s restored rows = %d, want 1", table, id, n)
		}
	}
	assertRow("dns_zones", "z1")
	assertRow("dns_records", "rec1")
	assertRow("dhcp_scopes", "sc1")
	assertRow("dhcp_reservations", "res1")
	assertRow("ipam_spaces", "sp1")
	assertRow("ipam_subnets", "sn1")
	assertRow("ipam_addresses", "addr1")
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ipam_address_observations WHERE address_id = ?`, "addr1").Scan(&n); err != nil {
		t.Fatalf("count observations: %v", err)
	}
	if n != 1 {
		t.Fatalf("observation restored rows = %d, want 1", n)
	}
}
