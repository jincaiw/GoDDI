package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dataplane"
	"github.com/jasonwa/goddi/internal/dns/zone"
)

// These tests pin the console's write guard on records the data plane owns.
//
// When the DNS role runs in its own process it is the author of the names it
// publishes -- the ones DHCP hands out, the ones RFC 2136 clients update, the
// ones an inbound transfer brings in -- and the control database holds a copy
// so the console can show them. Editing that copy reports success and changes
// nothing about the name being served: the owner is never asked, and the next
// push puts the row back. The operator is left believing a name they can still
// resolve was changed, which is worse than a refusal.
//
// The guard is only worth having if something fails when it is removed, so
// both halves are asserted: refused when the row belongs to the data plane,
// written when it belongs to the operator. It is also asserted to be per row
// rather than per zone, because a zone normally holds both kinds at once and a
// zone-wide switch would either refuse the operator's own edits or silently
// accept edits to the published ones. All three write paths -- update, delete,
// batch delete -- are covered, since each one has its own call to the guard.
//
// The tables come from the real data-plane schema rather than a hand-written
// one, so this test fails for the reason under test and not because a column
// was mistyped here.

const (
	recordTestZoneID = "z1"

	// publishedRecordID is written where the name is served from: a DHCP
	// binding publishing its hostname, an RFC 2136 update, a transfer. The
	// console must not touch it.
	publishedRecordID = "r-published"
	// consoleRecordIDA and consoleRecordIDB are the operator's own records in
	// the same zone. They must stay writable, or the guard has cost more than
	// it bought.
	consoleRecordIDA = "r-www"
	consoleRecordIDB = "r-mail"
)

// newRecordOwnershipTestDB opens a real zone-plane store holding one zone with
// one published record and two of the operator's own. Every case below acts on
// one of them.
func newRecordOwnershipTestDB(t *testing.T) *sql.DB {
	t.Helper()
	store, err := dataplane.Open(config.DataPlaneZone, filepath.Join(t.TempDir(), "zones.db"))
	if err != nil {
		t.Fatalf("opening the zone store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	seed := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := store.DB.Exec(query, args...); err != nil {
			t.Fatalf("seeding with %q: %v", query, err)
		}
	}

	seed(`INSERT INTO dns_zones (id, name, type, soa_mname, soa_rname, serial)
		VALUES (?, 'example.com', 'primary', 'ns1.example.com.', 'hostmaster.example.com.', 1)`,
		recordTestZoneID)

	insertRecord := `INSERT INTO dns_records
		(id, zone_id, name, type, value, ttl, enabled, authored_locally)
		VALUES (?, ?, ?, 'A', ?, 300, 1, ?)`
	seed(insertRecord, publishedRecordID, recordTestZoneID, "dhcp-1.example.com.", "10.0.0.10", 1)
	seed(insertRecord, consoleRecordIDA, recordTestZoneID, "www.example.com.", "10.0.0.5", 0)
	seed(insertRecord, consoleRecordIDB, recordTestZoneID, "mail.example.com.", "10.0.0.6", 0)

	return store.DB
}

func recordValueIn(t *testing.T, db *sql.DB, id string) (string, bool) {
	t.Helper()
	var value string
	err := db.QueryRow(`SELECT value FROM dns_records WHERE id = ?`, id).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		t.Fatalf("reading record %s: %v", id, err)
	}
	return value, true
}

func countRecordsIn(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dns_records`).Scan(&n); err != nil {
		t.Fatalf("counting records: %v", err)
	}
	return n
}

// withDNSRecordServices installs a DNS container for the duration of one test.
//
// It assigns the container directly rather than going through InitDNSServices,
// which is guarded by a sync.Once and would therefore ignore every call after
// the first. The managers have their own sync.Once, and firing it with nothing
// keeps the first getRecordManager call from rebuilding them over whatever
// database a previously run test left behind; the cached values are restored
// on cleanup so a later test starts where this one did.
func withDNSRecordServices(t *testing.T, db *sql.DB) {
	t.Helper()
	previousServices := DNSServices
	previousZoneMgr, previousRecordMgr := cachedZoneMgr, cachedRecordMgr

	zoneStore := zone.NewStore(db)
	DNSServices = &DNSServiceContainer{DB: db, ZoneStore: zoneStore}
	cachedZoneMgr = zone.NewZoneManager(db, zoneStore)
	cachedRecordMgr = zone.NewRecordManager(db, zoneStore, cachedZoneMgr)
	cacheInitOnce.Do(func() {})

	t.Cleanup(func() {
		// A write path asks the store to reload, which it debounces by half a
		// second. Draining that here stops it from running against a database
		// this test has already closed.
		zoneStore.ReloadNow()
		DNSServices = previousServices
		cachedZoneMgr, cachedRecordMgr = previousZoneMgr, previousRecordMgr
	})
}

// recordRequest builds a zone-scoped record request with the route parameters
// the handlers read.
func recordRequest(method, id string, body string) *http.Request {
	target := "/api/v1/dns/zones/" + recordTestZoneID + "/records/" + id
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("zoneId", recordTestZoneID)
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func batchDeleteRequest(ids ...string) *http.Request {
	quoted := make([]string, 0, len(ids))
	for _, id := range ids {
		quoted = append(quoted, `"`+id+`"`)
	}
	body := `{"ids":[` + strings.Join(quoted, ",") + `]}`
	return httptest.NewRequest(http.MethodDelete, "/api/v1/dns/records/batch", strings.NewReader(body))
}

func TestTheConsoleRefusesToEditARecordTheDataPlaneOwns(t *testing.T) {
	db := newRecordOwnershipTestDB(t)
	withDNSRecordServices(t, db)

	rec := httptest.NewRecorder()
	UpdateDNSRecord(rec, recordRequest(http.MethodPut, publishedRecordID, `{"value":"10.0.0.99"}`))

	if rec.Code != http.StatusConflict {
		t.Fatalf("editing a published record = %d, want %d: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if got, _ := recordValueIn(t, db, publishedRecordID); got != "10.0.0.10" {
		t.Fatalf("published record value after a refused edit = %q, want %q: the write went through anyway",
			got, "10.0.0.10")
	}
	if body := rec.Body.String(); !strings.Contains(body, "数据面") {
		t.Fatalf("refusal does not say where the record lives: %s", body)
	}
}

func TestTheConsoleRefusesToDeleteARecordTheDataPlaneOwns(t *testing.T) {
	db := newRecordOwnershipTestDB(t)
	withDNSRecordServices(t, db)

	before := countRecordsIn(t, db)

	rec := httptest.NewRecorder()
	DeleteDNSRecord(rec, recordRequest(http.MethodDelete, publishedRecordID, ""))

	if rec.Code != http.StatusConflict {
		t.Fatalf("deleting a published record = %d, want %d: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if _, exists := recordValueIn(t, db, publishedRecordID); !exists {
		t.Fatalf("the published record is gone after a refused delete: the name is still being served")
	}
	if after := countRecordsIn(t, db); after != before {
		t.Fatalf("record count after a refused delete = %d, want %d", after, before)
	}
}

// TestTheConsoleRefusesABatchThatMixesInAPublishedRecord checks that the
// refusal is complete: the operator's own records in the same batch must
// survive, because a half-applied batch is the failure mode where the console
// reports one outcome and the database holds another.
func TestTheConsoleRefusesABatchThatMixesInAPublishedRecord(t *testing.T) {
	db := newRecordOwnershipTestDB(t)
	withDNSRecordServices(t, db)

	before := countRecordsIn(t, db)

	rec := httptest.NewRecorder()
	BatchDeleteRecords(rec, batchDeleteRequest(consoleRecordIDA, publishedRecordID))

	if rec.Code != http.StatusConflict {
		t.Fatalf("batch delete including a published record = %d, want %d: %s",
			rec.Code, http.StatusConflict, rec.Body.String())
	}
	if _, exists := recordValueIn(t, db, consoleRecordIDA); !exists {
		t.Fatalf("the operator's own record was deleted by a batch that was refused")
	}
	if after := countRecordsIn(t, db); after != before {
		t.Fatalf("record count after a refused batch = %d, want %d: the batch was applied in part", after, before)
	}
}

func TestTheConsoleEditsARecordOfItsOwn(t *testing.T) {
	db := newRecordOwnershipTestDB(t)
	withDNSRecordServices(t, db)

	rec := httptest.NewRecorder()
	UpdateDNSRecord(rec, recordRequest(http.MethodPut, consoleRecordIDA, `{"value":"10.0.0.7"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("editing an owned record = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got, _ := recordValueIn(t, db, consoleRecordIDA); got != "10.0.0.7" {
		t.Fatalf("owned record value after an edit = %q, want %q", got, "10.0.0.7")
	}
}

func TestTheConsoleDeletesARecordOfItsOwn(t *testing.T) {
	db := newRecordOwnershipTestDB(t)
	withDNSRecordServices(t, db)

	rec := httptest.NewRecorder()
	DeleteDNSRecord(rec, recordRequest(http.MethodDelete, consoleRecordIDA, ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("deleting an owned record = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if _, exists := recordValueIn(t, db, consoleRecordIDA); exists {
		t.Fatalf("the owned record is still there after a successful delete")
	}
}

func TestTheConsoleBatchDeletesRecordsOfItsOwn(t *testing.T) {
	db := newRecordOwnershipTestDB(t)
	withDNSRecordServices(t, db)

	rec := httptest.NewRecorder()
	BatchDeleteRecords(rec, batchDeleteRequest(consoleRecordIDA, consoleRecordIDB))

	if rec.Code != http.StatusOK {
		t.Fatalf("batch deleting owned records = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	for _, id := range []string{consoleRecordIDA, consoleRecordIDB} {
		if _, exists := recordValueIn(t, db, id); exists {
			t.Fatalf("record %s is still there after a successful batch delete", id)
		}
	}
	if _, exists := recordValueIn(t, db, publishedRecordID); !exists {
		t.Fatalf("the published record was deleted by a batch that did not name it")
	}
}

// TestTheConsoleStillListsRecordsItCannotEdit guards the other half of the
// decision: reads are not refused. A console that cannot show the names DHCP
// is publishing would hide exactly the addresses an operator needs to see.
func TestTheConsoleStillListsRecordsItCannotEdit(t *testing.T) {
	db := newRecordOwnershipTestDB(t)
	withDNSRecordServices(t, db)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/dns/zones/"+recordTestZoneID+"/records", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("zoneId", recordTestZoneID)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	ListDNSRecords(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("listing records = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	for _, name := range []string{"dhcp-1.example.com", "www.example.com", "mail.example.com"} {
		if !strings.Contains(rec.Body.String(), name) {
			t.Fatalf("the list does not contain %s, which exists: %s", name, rec.Body.String())
		}
	}
}
