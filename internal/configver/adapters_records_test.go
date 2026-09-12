package configver

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jasonwa/goddi/internal/dhcp/scope"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/jasonwa/goddi/internal/ipam/subnet"
)

// seedRecord inserts one dns_records row with the given authorship.
func seedRecord(t *testing.T, db *sql.DB, id, zoneID, name, rtype, value string, authoredLocally int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO dns_records
		(id, zone_id, name, type, value, ttl, enabled, authored_locally)
		VALUES (?, ?, ?, ?, ?, 300, 1, ?)`, id, zoneID, name, rtype, value, authoredLocally); err != nil {
		t.Fatalf("seed record %s: %v", id, err)
	}
}

func countRecords(t *testing.T, db *sql.DB, zoneID string, authoredLocally int) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM dns_records WHERE zone_id = ? AND authored_locally = ?`,
		zoneID, authoredLocally).Scan(&n); err != nil {
		t.Fatalf("count records: %v", err)
	}
	return n
}

// TestTheRecordSetLeavesTheRecordsADataPlaneAuthoredAlone is the load-bearing
// test for this adapter.
//
// dns_records is the one table with two authors: an operator's records and the
// rows a data plane pushed up from a DHCP binding or a dynamic update. A
// whole-set replace that ignores the distinction deletes records its author
// still believes it owns -- which surfaces later as an address that is leased
// and has no name, long after the publish that caused it.
func TestTheRecordSetLeavesTheRecordsADataPlaneAuthoredAlone(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	if err := svc.Register(NewDNSRecordsAdapter(nil)); err != nil {
		t.Fatalf("register: %v", err)
	}
	seedZone(t, db, "zone-1", "example.test.")
	seedRecord(t, db, "authored-1", "zone-1", "www.example.test.", "A", "192.0.2.1", 0)
	seedRecord(t, db, "dynamic-1", "zone-1", "host1.example.test.", "A", "192.0.2.50", 1)

	content := json.RawMessage(`{"schema":1,"records":[
		{"name":"www","type":"A","value":"192.0.2.2","ttl":300,"enabled":true},
		{"name":"mail","type":"A","value":"192.0.2.3","ttl":300,"enabled":true}]}`)

	res, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDNSRecords,
		ResourceID:   "zone-1",
		Content:      content,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if res.Revision == nil || res.Revision.Revision != 1 {
		t.Fatalf("first publish did not create revision 1: %+v", res.Revision)
	}
	// Applying is the release worker's job, not the publish's: the revision and
	// the release are written in one transaction and the write happens after
	// it. A test that asserted the rows after Publish alone would be asserting
	// that nothing had happened.
	drainOutbox(t, svc)

	// The operator's set is exactly what was published: the old authored row is
	// gone, both new rows are there, and the names were normalised.
	if n := countRecords(t, db, "zone-1", 0); n != 2 {
		t.Errorf("control-authored records = %d, want 2", n)
	}
	if n := countRecords(t, db, "zone-1", 1); n != 1 {
		t.Errorf("data-plane-authored records = %d, want 1: the replace took a row it does not own", n)
	}
	var value string
	if err := db.QueryRow(`SELECT value FROM dns_records WHERE id = 'dynamic-1'`).Scan(&value); err != nil {
		t.Fatalf("the data-plane row is gone: %v", err)
	}
	if value != "192.0.2.50" {
		t.Errorf("the data-plane row now holds %q, want its own value", value)
	}
	var name string
	if err := db.QueryRow(
		`SELECT name FROM dns_records WHERE zone_id = 'zone-1' AND value = '192.0.2.2'`).Scan(&name); err != nil {
		t.Fatalf("the published record is missing: %v", err)
	}
	if name != "www.example.test." {
		t.Errorf("stored name = %q, want the normalised absolute form", name)
	}
}

// TestARecordSetTheConsoleWouldRefuseIsRefusedHere covers validation.
//
// The publish path writes the same rows the record API writes. If it validated
// less, it would be the way around validation rather than the way to record it.
func TestARecordSetTheConsoleWouldRefuseIsRefusedHere(t *testing.T) {
	adapter := NewDNSRecordsAdapter(nil)

	cases := []struct {
		name    string
		content string
	}{
		{"no schema", `{"records":[]}`},
		{"unknown schema", `{"schema":99,"records":[]}`},
		{"a name that is not one", `{"schema":1,"records":[{"name":"not a name","type":"A","value":"192.0.2.1","ttl":300,"enabled":true}]}`},
		{"a type that does not exist", `{"schema":1,"records":[{"name":"www","type":"NOPE","value":"x","ttl":300,"enabled":true}]}`},
		{"an A record with a hostname in it", `{"schema":1,"records":[{"name":"www","type":"A","value":"example.test","ttl":300,"enabled":true}]}`},
		{"an A record with an IPv6 address", `{"schema":1,"records":[{"name":"www","type":"A","value":"2001:db8::1","ttl":300,"enabled":true}]}`},
		{"MX without a priority", `{"schema":1,"records":[{"name":"example.test","type":"MX","value":"mail.example.test","ttl":300,"enabled":true}]}`},
		{"a negative ttl", `{"schema":1,"records":[{"name":"www","type":"A","value":"192.0.2.1","ttl":-1,"enabled":true}]}`},
		{"an expiry that is not a time", `{"schema":1,"records":[{"name":"www","type":"A","value":"192.0.2.1","ttl":300,"enabled":true,"expires_at":"soon"}]}`},
		{"the same id twice", `{"schema":1,"records":[
			{"id":"r1","name":"www","type":"A","value":"192.0.2.1","ttl":300,"enabled":true},
			{"id":"r1","name":"mail","type":"A","value":"192.0.2.2","ttl":300,"enabled":true}]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := adapter.Validate(json.RawMessage(tc.content)); err == nil {
				t.Fatalf("Validate accepted %s", tc.content)
			}
		})
	}

	valid := []string{
		`{"schema":1,"records":[]}`,
		`{"schema":1,"records":[{"name":"www","type":"A","value":"192.0.2.1","ttl":300,"enabled":true}]}`,
		`{"schema":1,"records":[{"name":"@","type":"MX","value":"mail.example.test.","ttl":300,"enabled":true,"priority":10}]}`,
	}
	for _, content := range valid {
		if err := adapter.Validate(json.RawMessage(content)); err != nil {
			t.Errorf("Validate rejected valid content %s: %v", content, err)
		}
	}
}

// TestReadingTheRecordSetRepublishesItUnchanged pins the round trip that makes
// a diff readable.
//
// A content that differs from the stored one only in how it was assembled hashes
// differently and diffs as every record changed. ReadContent exists to prevent
// that, so the property worth asserting is that publishing what it returns is a
// no-op change.
func TestReadingTheRecordSetRepublishesItUnchanged(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	if err := svc.Register(NewDNSRecordsAdapter(nil)); err != nil {
		t.Fatalf("register: %v", err)
	}
	seedZone(t, db, "zone-1", "example.test.")
	seedRecord(t, db, "r-mx", "zone-1", "example.test.", "MX", "mail.example.test.", 0)
	seedRecord(t, db, "r-a", "zone-1", "www.example.test.", "A", "192.0.2.1", 0)
	// A row with no priority and one with a priority of zero have to stay
	// distinguishable, or a republish silently drops the zero.
	if _, err := db.Exec(`UPDATE dns_records SET priority = 0 WHERE id = 'r-mx'`); err != nil {
		t.Fatalf("set priority: %v", err)
	}

	content, err := ReadContent(db, ResourceDNSRecords, "zone-1")
	if err != nil {
		t.Fatalf("read content: %v", err)
	}

	first, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDNSRecords, ResourceID: "zone-1", Content: content,
	})
	if err != nil {
		t.Fatalf("publish the read content: %v", err)
	}
	if first.Revision == nil {
		t.Fatal("publishing the read content stored nothing")
	}
	drainOutbox(t, svc)

	second, err := svc.Publish(PublishRequest{
		ResourceType:     ResourceDNSRecords,
		ResourceID:       "zone-1",
		Content:          content,
		ExpectedRevision: 1,
		DryRun:           true,
	})
	if err != nil {
		t.Fatalf("dry run on the same content: %v", err)
	}
	if len(second.Changes) != 0 {
		t.Fatalf("the same content diffed against itself: %+v", second.Changes)
	}

	// And the priority of zero survived the trip: the reader must not turn a
	// set value into an absent one, or every MX republished through this path
	// would lose it.
	var priority sql.NullInt64
	if err := db.QueryRow(`SELECT priority FROM dns_records WHERE id = 'r-mx'`).Scan(&priority); err != nil {
		t.Fatalf("read priority: %v", err)
	}
	if !priority.Valid || priority.Int64 != 0 {
		t.Errorf("mx priority = %+v, want a set 0", priority)
	}
}

// TestARollbackOfARecordSetIsANewRevision covers the recovery path end to end.
func TestARollbackOfARecordSetIsANewRevision(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	if err := svc.Register(NewDNSRecordsAdapter(nil)); err != nil {
		t.Fatalf("register: %v", err)
	}
	seedZone(t, db, "zone-1", "example.test.")

	good := json.RawMessage(`{"schema":1,"records":[{"name":"www","type":"A","value":"192.0.2.1","ttl":300,"enabled":true}]}`)
	if _, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDNSRecords, ResourceID: "zone-1", Content: good,
	}); err != nil {
		t.Fatalf("publish the good set: %v", err)
	}
	drainOutbox(t, svc)

	bad := json.RawMessage(`{"schema":1,"records":[{"name":"www","type":"A","value":"198.51.100.9","ttl":300,"enabled":true}]}`)
	if _, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDNSRecords, ResourceID: "zone-1", Content: bad, ExpectedRevision: 1,
	}); err != nil {
		t.Fatalf("publish the bad set: %v", err)
	}
	drainOutbox(t, svc)

	rolled, err := svc.Rollback(RollbackRequest{
		ResourceType: ResourceDNSRecords, ResourceID: "zone-1",
		ToRevision: 1, ExpectedRevision: 2,
	})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if rolled.Revision.Revision != 3 {
		t.Errorf("rollback created revision %d, want a NEW revision 3", rolled.Revision.Revision)
	}
	if rolled.Revision.Note != "rollback to revision 1" {
		t.Errorf("rollback note = %q", rolled.Revision.Note)
	}
	drainOutbox(t, svc)

	var value string
	if err := db.QueryRow(
		`SELECT value FROM dns_records WHERE zone_id = 'zone-1' AND authored_locally = 0`).Scan(&value); err != nil {
		t.Fatalf("read the restored record: %v", err)
	}
	if value != "192.0.2.1" {
		t.Errorf("after the rollback the record holds %q, want the revision-1 value", value)
	}
	if n := countRecords(t, db, "zone-1", 0); n != 1 {
		t.Errorf("the zone holds %d authored records after the rollback, want 1", n)
	}
}

// TestARecordSetForAZoneThatDoesNotExistIsRefused is the precondition, and it
// is what makes the new type's creation story coherent: records are governed
// under their zone, so there is nothing to publish to before the zone exists.
func TestARecordSetForAZoneThatDoesNotExistIsRefused(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	if err := svc.Register(NewDNSRecordsAdapter(nil)); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDNSRecords,
		ResourceID:   "no-such-zone",
		Content:      json.RawMessage(`{"schema":1,"records":[]}`),
	})
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("Publish to a missing zone = %v, want ErrResourceNotFound", err)
	}
}

// TestACreationIsTheFirstRevision covers the half of the history that the
// publish path cannot reach.
func TestACreationIsTheFirstRevision(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	if err := svc.Register(NewDNSRecordsAdapter(nil)); err != nil {
		t.Fatalf("register: %v", err)
	}
	seedZone(t, db, "zone-1", "example.test.")
	// The zone's create path seeds nothing, but a record set that arrives with
	// the zone is the case that makes recording it worthwhile.
	seedRecord(t, db, "r1", "zone-1", "www.example.test.", "A", "192.0.2.1", 0)

	content, err := ReadContent(db, ResourceDNSRecords, "zone-1")
	if err != nil {
		t.Fatalf("read content: %v", err)
	}
	rev, err := svc.RecordCreation(RecordCreationRequest{
		ResourceType: ResourceDNSRecords,
		ResourceID:   "zone-1",
		Content:      content,
		Actor:        "operator",
	})
	if err != nil {
		t.Fatalf("RecordCreation: %v", err)
	}
	if rev.Revision != 1 || rev.BaseRevision != 0 {
		t.Fatalf("the creation was recorded as revision %d based on %d", rev.Revision, rev.BaseRevision)
	}
	if rev.Status != StatusPersisted {
		t.Errorf("the creation is %s, want %s: nothing confirmed anything to a data plane",
			rev.Status, StatusPersisted)
	}
	if rev.Note != "created" {
		t.Errorf("note = %q, want a note saying what it is", rev.Note)
	}
	if !rev.Status.restorable() {
		t.Error("the creation cannot be rolled back to, which is the only reason to record it")
	}

	// The baseline moves: a publish now has to cite revision 1.
	if _, err := svc.Publish(PublishRequest{
		ResourceType:     ResourceDNSRecords,
		ResourceID:       "zone-1",
		Content:          content,
		ExpectedRevision: 0,
	}); !isConflict(err) {
		t.Fatalf("a publish that cites revision 0 after a creation = %v, want a conflict", err)
	}
	if _, err := svc.Publish(PublishRequest{
		ResourceType:     ResourceDNSRecords,
		ResourceID:       "zone-1",
		Content:          json.RawMessage(`{"schema":1,"records":[]}`),
		ExpectedRevision: 1,
	}); err != nil {
		t.Fatalf("a publish that cites the creation: %v", err)
	}

	// Recording the same creation twice is refused rather than ignored: the
	// second call means the caller has lost track of what it wrote.
	if _, err := svc.RecordCreation(RecordCreationRequest{
		ResourceType: ResourceDNSRecords, ResourceID: "zone-1", Content: content,
	}); !isConflict(err) {
		t.Fatalf("a second RecordCreation = %v, want a conflict", err)
	}

	// And a resource that does not exist has no creation to record.
	if _, err := svc.RecordCreation(RecordCreationRequest{
		ResourceType: ResourceDNSRecords,
		ResourceID:   "no-such-zone",
		Content:      json.RawMessage(`{"schema":1,"records":[]}`),
	}); !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("RecordCreation for a missing zone = %v, want ErrResourceNotFound", err)
	}
}

// drainOutbox runs the release worker to completion. It is what production's
// Service.Run does on its interval, called directly so the test does not have to
// sleep through one.
func drainOutbox(t *testing.T, svc *Service) {
	t.Helper()
	if _, err := svc.DrainOutbox(10); err != nil {
		t.Fatalf("drain the outbox: %v", err)
	}
}

func isConflict(err error) bool {
	var conflict *ConflictError
	return errors.As(err, &conflict)
}

// TestTheConstructorsProduceContentTheAdaptersAccept covers the mapping from a
// domain object to publishable content.
//
// The constructors exist so that the create paths record what they created
// without hand-assembling JSON, and so that the field names live in one place.
// What still can go wrong is a field the adapter requires and the constructor
// leaves empty, which no compiler catches.
func TestTheConstructorsProduceContentTheAdaptersAccept(t *testing.T) {
	z := &zone.Zone{
		ID: "zone-1", Name: "example.test.", Type: "primary", Enabled: true,
		DefaultTTL: 3600, SOA_MName: "ns1.example.test.", SOA_RName: "hostmaster.example.test.",
		Refresh: 3600, Retry: 600, Expire: 86400, Minimum: 300,
		TransferPolicy: `["192.0.2.0/24"]`, UpdatePolicy: `["192.0.2.0/24"]`,
	}
	if err := NewDNSZoneAdapter(nil).Validate(mustJSON(t, DNSZoneContentFromZone(z))); err != nil {
		t.Errorf("the zone constructor's content is rejected by its own adapter: %v", err)
	}

	s := &scope.Scope{
		ID: "scope-1", Name: "office", Subnet: "192.0.2.0/24",
		StartIP: "192.0.2.10", EndIP: "192.0.2.100", LeaseTime: 3600, Enabled: true,
	}
	if err := NewDHCPScopeAdapter().Validate(mustJSON(t, DHCPScopeContentFromScope(s))); err != nil {
		t.Errorf("the scope constructor's content is rejected by its own adapter: %v", err)
	}

	sub := &subnet.Subnet{ID: "subnet-1", SpaceID: "space-1", Name: "office", CIDR: "192.0.2.0/24", VLANID: 10}
	if err := NewIPAMSubnetAdapter().Validate(mustJSON(t, IPAMSubnetContentFromSubnet(sub))); err != nil {
		t.Errorf("the subnet constructor's content is rejected by its own adapter: %v", err)
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("encode %T: %v", v, err)
	}
	return encoded
}
