package configver

import "testing"

func TestRecordStateVersionsAnAppliedRecordChangeWithoutARelease(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	if err := svc.Register(NewDNSRecordsAdapter(nil)); err != nil {
		t.Fatal(err)
	}
	seedZone(t, db, "zone-state", "state.test.")
	seedRecord(t, db, "a-1", "zone-state", "www.state.test.", "A", "192.0.2.1", 0)
	if _, err := svc.RecordCreation(RecordCreationRequest{
		ResourceType: ResourceDNSRecords, ResourceID: "zone-state", Actor: "admin",
		Content: mustReadContent(t, svc, "zone-state"),
	}); err != nil {
		t.Fatalf("record creation: %v", err)
	}
	if _, err := db.Exec(`UPDATE dns_records SET value = ? WHERE id = ?`, "192.0.2.2", "a-1"); err != nil {
		t.Fatal(err)
	}
	rev, err := svc.RecordState(RecordStateRequest{ResourceType: ResourceDNSRecords, ResourceID: "zone-state", Actor: "tester", Note: "record updated"})
	if err != nil {
		t.Fatalf("record state: %v", err)
	}
	if rev.Revision != 2 || rev.BaseRevision != 1 || rev.Status != StatusPersisted {
		t.Fatalf("record state = %+v, want revision 2 based on 1 and persisted", rev)
	}
	changes, err := svc.DiffRevisions(ResourceDNSRecords, "zone-state", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) == 0 {
		t.Fatal("record change is invisible in the revision diff")
	}
	var releases int
	if err := db.QueryRow(`SELECT COUNT(*) FROM config_release_outbox WHERE resource_id = ?`, "zone-state").Scan(&releases); err != nil {
		t.Fatal(err)
	}
	if releases != 0 {
		t.Fatalf("RecordState queued %d releases, want none", releases)
	}
}

func mustReadContent(t *testing.T, svc *Service, id string) []byte {
	t.Helper()
	content, err := ReadContent(svc.DB(), ResourceDNSRecords, id)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
