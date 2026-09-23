package zone

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dataplane"
	"github.com/miekg/dns"
)

func TestCAARecordMetadataPersistsAcrossCreateUpdateAndJournal(t *testing.T) {
	store, err := dataplane.Open(config.DataPlaneZone, filepath.Join(t.TempDir(), "zones.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	zoneStore := NewStore(store.DB)
	defer zoneStore.Close()
	zoneManager := NewZoneManager(store.DB, zoneStore)
	z, err := zoneManager.CreateZone(ZoneOptions{Name: "example.test", Type: string(ZoneTypePrimary)})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewRecordManager(store.DB, zoneStore, zoneManager)
	flag := 128
	rec, err := manager.CreateRecord(z.ID, RecordOptions{
		Name: "www", Type: "CAA", Value: "ca.example.test", Flag: &flag, Tag: "issue",
	})
	if err != nil {
		t.Fatalf("create CAA record: %v", err)
	}
	if rec.Tag != "issue" || rec.Flag != 128 {
		t.Fatalf("created record metadata = flag %d tag %q", rec.Flag, rec.Tag)
	}

	updated, err := manager.UpdateRecord(rec.ID, RecordOptions{Value: "ca2.example.test"})
	if err != nil {
		t.Fatalf("update CAA value without resending unchanged metadata: %v", err)
	}
	if updated.Tag != "issue" || updated.Flag != 128 {
		t.Fatalf("updated record metadata = flag %d tag %q", updated.Flag, updated.Tag)
	}

	var storedFlag int
	var storedTag string
	if err := store.QueryRow(`SELECT flag, tag FROM dns_records WHERE id = ?`, rec.ID).Scan(&storedFlag, &storedTag); err != nil {
		t.Fatal(err)
	}
	if storedFlag != 128 || storedTag != "issue" {
		t.Fatalf("database metadata = flag %d tag %q", storedFlag, storedTag)
	}

	var deletes, adds int
	var historyTag string
	if err := store.QueryRow(`SELECT COUNT(*) FROM dns_zone_changes WHERE zone_id = ? AND change_type = 'delete' AND type = 'CAA' AND flag = 128 AND tag = 'issue'`, z.ID).Scan(&deletes); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(`SELECT COUNT(*), MAX(tag) FROM dns_zone_changes WHERE zone_id = ? AND change_type = 'add' AND type = 'CAA' AND flag = 128`, z.ID).Scan(&adds, &historyTag); err != nil {
		t.Fatal(err)
	}
	if deletes != 1 || adds != 2 || historyTag != "issue" {
		t.Fatalf("CAA journal = deletes %d, adds %d, latest tag %q", deletes, adds, historyTag)
	}
	entries, _, err := zoneManager.ListZoneHistory(z.ID, 1, 20)
	if err != nil {
		t.Fatalf("list zone history: %v", err)
	}
	foundMetadata := false
	for _, entry := range entries {
		if entry.ChangeType == "add" && entry.Type == "CAA" && entry.Value == "ca2.example.test" {
			foundMetadata = entry.Flag == 128 && entry.Tag == "issue"
		}
	}
	if !foundMetadata {
		t.Fatalf("zone history did not expose CAA flag/tag: %+v", entries)
	}

	zoneStore.ReloadNow()
	_, answer, ok := zoneStore.Lookup("www.example.test.", dns.TypeCAA)
	if !ok || len(answer) != 1 {
		t.Fatalf("authoritative CAA answer = (%v, %v), want one record", ok, answer)
	}
	caa, ok := answer[0].(*dns.CAA)
	if !ok || caa.Tag != "issue" || caa.Flag != 128 || caa.Value != "ca2.example.test" {
		t.Fatalf("authoritative CAA answer = %#v", answer[0])
	}

	csvData, err := manager.ExportRecordsCSV(z.ID)
	if err != nil {
		t.Fatalf("export CAA CSV: %v", err)
	}
	csvZone, err := zoneManager.CreateZone(ZoneOptions{Name: "csv.example.test", Type: string(ZoneTypePrimary)})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ImportRecordsCSV(csvZone.ID, csvData); err != nil {
		t.Fatalf("import CAA CSV: %v", err)
	}
	assertImportedCAA(t, store.DB, csvZone.ID)

	zoneFileZone, err := zoneManager.CreateZone(ZoneOptions{Name: "zonefile.example.test", Type: string(ZoneTypePrimary)})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ImportZoneFile(zoneFileZone.ID, `policy 300 IN CAA 128 issue "ca-zonefile.example.test"`); err != nil {
		t.Fatalf("import CAA zone file: %v", err)
	}
	var importedFlag int
	var importedTag, importedValue string
	if err := store.QueryRow(`SELECT flag, tag, value FROM dns_records WHERE zone_id = ?`, zoneFileZone.ID).
		Scan(&importedFlag, &importedTag, &importedValue); err != nil {
		t.Fatal(err)
	}
	if importedFlag != 128 || importedTag != "issue" || importedValue != "ca-zonefile.example.test" {
		t.Fatalf("zone-file CAA fields = flag %d, tag %q, value %q", importedFlag, importedTag, importedValue)
	}
}

func TestImportsRejectMalformedAndPreserveNAPTR(t *testing.T) {
	store, err := dataplane.Open(config.DataPlaneZone, filepath.Join(t.TempDir(), "zones.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	zoneStore := NewStore(store.DB)
	defer zoneStore.Close()
	zoneManager := NewZoneManager(store.DB, zoneStore)
	manager := NewRecordManager(store.DB, zoneStore, zoneManager)

	t.Run("CSV unsupported type rolls back entire import", func(t *testing.T) {
		z, err := zoneManager.CreateZone(ZoneOptions{Name: "csv-fail.test", Type: string(ZoneTypePrimary)})
		if err != nil {
			t.Fatal(err)
		}
		data := []byte("name,type,value,ttl,priority,weight,port,flag,tag\nwww,A,192.0.2.8,300,,,,,\nbad,TYPE65000,\\# 1 00,300,,,,,\n")
		if err := manager.ImportRecordsCSV(z.ID, data); err == nil {
			t.Fatal("import with unsupported type unexpectedly succeeded")
		}
		var count int
		if err := store.QueryRow(`SELECT COUNT(*) FROM dns_records WHERE zone_id = ?`, z.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("failed CSV import left %d records", count)
		}
	})

	t.Run("CSV malformed row is rejected", func(t *testing.T) {
		z, err := zoneManager.CreateZone(ZoneOptions{Name: "csv-malformed.test", Type: string(ZoneTypePrimary)})
		if err != nil {
			t.Fatal(err)
		}
		if err := manager.ImportRecordsCSV(z.ID, []byte("name,type,value\nwww,A,192.0.2.8\nbad,\"A,broken\n")); err == nil {
			t.Fatal("malformed CSV unexpectedly succeeded")
		}
	})

	t.Run("zone-file out-of-zone owner is rejected", func(t *testing.T) {
		z, err := zoneManager.CreateZone(ZoneOptions{Name: "owner-boundary.test", Type: string(ZoneTypePrimary)})
		if err != nil {
			t.Fatal(err)
		}
		if err := manager.ImportZoneFile(z.ID, `outside.other.test. 300 IN A 192.0.2.9`); err == nil {
			t.Fatal("out-of-zone owner unexpectedly imported")
		}
		var count int
		if err := store.QueryRow(`SELECT COUNT(*) FROM dns_records WHERE zone_id = ?`, z.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rejected zone-file import left %d records", count)
		}
	})

	t.Run("NAPTR metadata round trips through API and imports", func(t *testing.T) {
		z, err := zoneManager.CreateZone(ZoneOptions{Name: "naptr.test", Type: string(ZoneTypePrimary)})
		if err != nil {
			t.Fatal(err)
		}
		priority, weight := 100, 10
		value := `"s" "SIP+D2U" "" _sip._udp.naptr.test.`
		if _, err := manager.CreateRecord(z.ID, RecordOptions{
			Name: "@", Type: "NAPTR", Value: value, Priority: &priority, Weight: &weight,
		}); err != nil {
			t.Fatalf("create NAPTR record: %v", err)
		}
		assertNAPTRRecord(t, zoneStore, z.Name, "_sip._udp.naptr.test.")

		csvData, err := manager.ExportRecordsCSV(z.ID)
		if err != nil {
			t.Fatalf("export NAPTR CSV: %v", err)
		}
		csvZone, err := zoneManager.CreateZone(ZoneOptions{Name: "naptr-csv.test", Type: string(ZoneTypePrimary)})
		if err != nil {
			t.Fatal(err)
		}
		if err := manager.ImportRecordsCSV(csvZone.ID, csvData); err != nil {
			t.Fatalf("import NAPTR CSV: %v", err)
		}
		assertNAPTRRecord(t, zoneStore, csvZone.Name, "_sip._udp.naptr.test.")

		zoneFile, err := manager.ExportZoneFile(z.ID)
		if err != nil {
			t.Fatalf("export NAPTR zone file: %v", err)
		}
		zoneFileZone, err := zoneManager.CreateZone(ZoneOptions{Name: "naptr-zonefile.test", Type: string(ZoneTypePrimary)})
		if err != nil {
			t.Fatal(err)
		}
		zoneFile = strings.Replace(zoneFile, "$ORIGIN "+z.Name, "$ORIGIN "+zoneFileZone.Name, 1)
		if err := manager.ImportZoneFile(zoneFileZone.ID, zoneFile); err != nil {
			t.Fatalf("import NAPTR zone file: %v", err)
		}
		assertNAPTRRecord(t, zoneStore, zoneFileZone.Name, "_sip._udp.naptr.test.")

		legacyZone, err := zoneManager.CreateZone(ZoneOptions{Name: "naptr-legacy.test", Type: string(ZoneTypePrimary)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Exec(`INSERT INTO dns_records (id, zone_id, name, type, value, ttl, priority, weight, enabled)
			VALUES ('legacy-naptr', ?, ?, 'NAPTR', ?, 300, 100, 10, 1)`,
			legacyZone.ID, legacyZone.Name, "_legacy._tcp.naptr-legacy.test."); err != nil {
			t.Fatalf("insert legacy NAPTR fixture: %v", err)
		}
		legacyRecords, _, err := manager.ListRecords(RecordFilter{ZoneID: legacyZone.ID, PageSize: 10})
		if err != nil {
			t.Fatalf("list legacy NAPTR record: %v", err)
		}
		if len(legacyRecords) != 1 || legacyRecords[0].Value != `"" "" "" _legacy._tcp.naptr-legacy.test.` {
			t.Fatalf("legacy NAPTR record representation = %+v", legacyRecords)
		}
		zoneStore.ReloadNow()
		_, answer, ok := zoneStore.Lookup(legacyZone.Name, dns.TypeNAPTR)
		if !ok || len(answer) != 1 {
			t.Fatalf("legacy authoritative NAPTR answer = (%v, %v), want one record", ok, answer)
		}
		legacy, ok := answer[0].(*dns.NAPTR)
		if !ok || legacy.Flags != "" || legacy.Service != "" || legacy.Regexp != "" || legacy.Replacement != "_legacy._tcp.naptr-legacy.test." {
			t.Fatalf("legacy authoritative NAPTR answer = %#v", answer[0])
		}
	})
}

func TestCAAFlagMustFitWireOctet(t *testing.T) {
	for _, flag := range []int{-1, 256} {
		if err := validateRecordValue("CAA", "ca.example.test", nil, nil, nil, "issue", &flag); err == nil {
			t.Errorf("CAA flag %d unexpectedly accepted", flag)
		}
	}
}

func assertImportedCAA(t *testing.T, db *sql.DB, zoneID string) {
	t.Helper()
	var flag int
	var tag string
	if err := db.QueryRow(`SELECT flag, tag FROM dns_records WHERE zone_id = ?`, zoneID).Scan(&flag, &tag); err != nil {
		t.Fatal(err)
	}
	if flag != 128 || tag != "issue" {
		t.Fatalf("imported CAA metadata = flag %d, tag %q", flag, tag)
	}
}

func assertNAPTRRecord(t *testing.T, store *Store, zoneName, replacement string) {
	t.Helper()
	store.ReloadNow()
	_, answer, ok := store.Lookup(dns.Fqdn(zoneName), dns.TypeNAPTR)
	if !ok || len(answer) != 1 {
		t.Fatalf("authoritative NAPTR answer = (%v, %v), want one record", ok, answer)
	}
	rec, ok := answer[0].(*dns.NAPTR)
	if !ok || rec.Order != 100 || rec.Preference != 10 || rec.Flags != "s" || rec.Service != "SIP+D2U" || rec.Regexp != "" || rec.Replacement != replacement {
		t.Fatalf("authoritative NAPTR answer = %#v", answer[0])
	}
}
