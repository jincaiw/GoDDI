package dataplane

import (
	"database/sql"
	"testing"
)

// revision reads the control-plane change counter for a domain.
func revision(t *testing.T, db *sql.DB, domain string) int64 {
	t.Helper()
	var rev int64
	if err := db.QueryRow(`SELECT revision FROM dataplane_revision WHERE domain = ?`, domain).Scan(&rev); err != nil {
		t.Fatalf("read %s revision: %v", domain, err)
	}
	return rev
}

// TestControlRevisionBumpsOnEveryWritePath is what tells a data plane its copy
// is stale. The counter is maintained by triggers rather than by the API
// handlers precisely so that a write path added later cannot forget to bump it
// -- a data plane serving configuration changed hours ago, with nothing
// anywhere to show it, is the failure this avoids.
func TestControlRevisionBumpsOnEveryWritePath(t *testing.T) {
	db := newControlDB(t)

	dhcpBefore, dnsBefore := revision(t, db, "dhcp"), revision(t, db, "dns")
	if dhcpBefore != 0 || dnsBefore != 0 {
		t.Fatalf("revisions start at dhcp=%d dns=%d, want 0 and 0", dhcpBefore, dnsBefore)
	}

	// --- DHCP configuration ---

	if _, err := db.Exec(`INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip)
		VALUES ('s1', 'lan', '10.0.0.0/24', '10.0.0.10', '10.0.0.20')`); err != nil {
		t.Fatalf("insert scope: %v", err)
	}
	if got := revision(t, db, "dhcp"); got != dhcpBefore+1 {
		t.Errorf("after inserting a scope the dhcp revision = %d, want %d", got, dhcpBefore+1)
	}

	if _, err := db.Exec(`UPDATE dhcp_scopes SET name = 'lan2' WHERE id = 's1'`); err != nil {
		t.Fatalf("update scope: %v", err)
	}
	if got := revision(t, db, "dhcp"); got != dhcpBefore+2 {
		t.Errorf("after updating a scope the dhcp revision = %d, want %d", got, dhcpBefore+2)
	}

	if _, err := db.Exec(`INSERT INTO dhcp_reservations (id, scope_id, ip_address, mac_address)
		VALUES ('r1', 's1', '10.0.0.15', 'aa:bb:cc:dd:ee:01')`); err != nil {
		t.Fatalf("insert reservation: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO dhcp_options (id, scope_id, code, value)
		VALUES ('o1', 's1', 6, '10.0.0.1')`); err != nil {
		t.Fatalf("insert option: %v", err)
	}
	if got := revision(t, db, "dhcp"); got != dhcpBefore+4 {
		t.Errorf("after a reservation and an option the dhcp revision = %d, want %d", got, dhcpBefore+4)
	}

	// --- DNS configuration ---

	if _, err := db.Exec(`INSERT INTO dns_zones (id, name, type, soa_mname, soa_rname, serial)
		VALUES ('z1', 'example.test.', 'master', 'ns1.example.test.', 'hostmaster.example.test.', 1)`); err != nil {
		t.Fatalf("insert zone: %v", err)
	}
	if got := revision(t, db, "dns"); got != dnsBefore+1 {
		t.Errorf("after inserting a zone the dns revision = %d, want %d", got, dnsBefore+1)
	}

	if _, err := db.Exec(`INSERT INTO dns_records (id, zone_id, name, type, value)
		VALUES ('rec1', 'z1', 'host.example.test.', 'A', '10.0.0.5')`); err != nil {
		t.Fatalf("insert record: %v", err)
	}
	if _, err := db.Exec(`UPDATE dns_records SET value = '10.0.0.6' WHERE id = 'rec1'`); err != nil {
		t.Fatalf("update record: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM dns_records WHERE id = 'rec1'`); err != nil {
		t.Fatalf("delete record: %v", err)
	}
	if got := revision(t, db, "dns"); got != dnsBefore+4 {
		t.Errorf("after three record writes the dns revision = %d, want %d", got, dnsBefore+4)
	}

	// Deletes count: a data plane that missed a deletion keeps serving
	// configuration that no longer exists.
	if _, err := db.Exec(`DELETE FROM dns_zones WHERE id = 'z1'`); err != nil {
		t.Fatalf("delete zone: %v", err)
	}
	if got := revision(t, db, "dns"); got != dnsBefore+5 {
		t.Errorf("after deleting a zone the dns revision = %d, want %d", got, dnsBefore+5)
	}
}

// TestControlRevisionIsPerDomain keeps the two data planes from waking each
// other up: a DHCP scope change is nothing to a DNS node, and on a busy server
// a shared counter would push a full zone reload every time a lease was
// reserved.
func TestControlRevisionIsPerDomain(t *testing.T) {
	db := newControlDB(t)

	if _, err := db.Exec(`INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip)
		VALUES ('s1', 'lan', '10.0.0.0/24', '10.0.0.10', '10.0.0.20')`); err != nil {
		t.Fatalf("insert scope: %v", err)
	}
	if got := revision(t, db, "dns"); got != 0 {
		t.Errorf("a DHCP change moved the dns revision to %d, want 0", got)
	}

	if _, err := db.Exec(`INSERT INTO dns_zones (id, name, type, soa_mname, soa_rname, serial)
		VALUES ('z1', 'example.test.', 'master', 'ns1.example.test.', 'hostmaster.example.test.', 1)`); err != nil {
		t.Fatalf("insert zone: %v", err)
	}
	if got := revision(t, db, "dhcp"); got != 1 {
		t.Errorf("a DNS change moved the dhcp revision to %d, want 1 (only the scope)", got)
	}
}

// TestControlRevisionDoesNotFollowLeaseWrites pins the direction of the lease
// replication. Leases are authored on the data plane and copied *up*, so a
// write to the control database's lease replica -- which is what that copy
// does -- must not look like a configuration change and send the data plane
// back to fetch what it just sent.
func TestControlRevisionDoesNotFollowLeaseWrites(t *testing.T) {
	db := newControlDB(t)

	if _, err := db.Exec(`INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip)
		VALUES ('s1', 'lan', '10.0.0.0/24', '10.0.0.10', '10.0.0.20')`); err != nil {
		t.Fatalf("insert scope: %v", err)
	}
	before := revision(t, db, "dhcp")

	if _, err := db.Exec(`INSERT INTO dhcp_leases
		(id, scope_id, ip_address, mac_address, hostname, client_id,
		 lease_start, lease_end, status, last_seen, generation)
		VALUES ('l1', 's1', '10.0.0.10', 'aa:bb:cc:dd:ee:01', 'h', '',
		        datetime('now'), datetime('now', '+1 hour'), 'active', datetime('now'), 1)`); err != nil {
		t.Fatalf("insert lease replica: %v", err)
	}
	if got := revision(t, db, "dhcp"); got != before {
		t.Errorf("writing the lease replica moved the dhcp revision from %d to %d", before, got)
	}
}
