package handler

// The configuration version API is the only operator-facing surface of the
// publishing mechanism, and until now it had no test at all. That mattered
// because everything the surface promises -- a history you can read, a diff you
// can review, a rollback that adds rather than rewrites, and a queue that says
// what never reached the data plane -- is a claim about the HTTP layer's shape,
// not about the service's: the status codes are what tell a console whether to
// fix a document or reload it.
//
// These cases drive the real handlers against the real schema. The service is
// installed once per process because the production wiring is too
// (InitConfigVersionService is a sync.Once), so the cases share a database and
// keep to their own resource ids.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jasonwa/goddi"
	"github.com/jasonwa/goddi/internal/configver"
)

// One service and one database for the whole file, because production has one
// of each and the wiring that installs them is a sync.Once. Sharing them is
// also what keeps a case honest: a resource seeded in one database is not
// visible to a service built over another, and an in-memory SQLite per case
// would quietly test a service that cannot see the rows the case seeded.
var (
	configVersionEnvOnce sync.Once
	configVersionSvc     *configver.Service
	configVersionDB      *sql.DB
)

// openMigratedTestDB opens the real control-plane schema in memory and never
// closes it. See configVersionEnv for why it has to outlive one test.
func openMigratedTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	goose.SetBaseFS(goddiassets.Migrations())
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}

// configVersionEnv installs the service once, against the real control-plane
// schema, with every adapter production registers.
func configVersionEnv(t *testing.T) (*configver.Service, *sql.DB) {
	t.Helper()
	configVersionEnvOnce.Do(func() {
		// Not newControlPlaneTestDB: that helper closes the database when the
		// test that created it returns, and every case here shares one service
		// and one database across several tests. The database is left open for
		// the life of the binary, which in a test process is exactly long
		// enough and costs one in-memory database.
		db := openMigratedTestDB(t)
		svc := configver.NewService(db)
		for _, adapter := range []configver.Adapter{
			configver.NewDHCPScopeAdapter(),
			configver.NewDNSZoneAdapter(nil),
			configver.NewDNSRecordsAdapter(nil),
			configver.NewIPAMSubnetAdapter(),
		} {
			if err := svc.Register(adapter); err != nil {
				t.Fatalf("register %s: %v", adapter.Type(), err)
			}
		}
		configVersionDB, configVersionSvc = db, svc
		InitConfigVersionService(svc)
	})
	return configVersionSvc, configVersionDB
}

// call runs a handler and returns the status and the decoded envelope.
func call(t *testing.T, fn http.HandlerFunc, method, target string, body string) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	rec := httptest.NewRecorder()
	fn(rec, httptest.NewRequest(method, target, reader))

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s %s: the body is not JSON (%q): %v", method, target, rec.Body.String(), err)
	}
	return rec.Code, out
}

func dataField(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("the response carries no data object: %+v", body)
	}
	return data
}

func seedConfigZone(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO dns_zones
		(id, name, type, enabled, default_ttl, soa_mname, soa_rname, serial,
		 refresh, retry, expire, minimum)
		VALUES (?, ?, 'primary', 1, 3600, 'ns1.example.test.', 'hostmaster.example.test.', 1,
		        3600, 600, 86400, 300)`, id, name); err != nil {
		t.Fatalf("seed zone: %v", err)
	}
}

// TestTheConfigTypesEndpointNamesWhatIsGoverned is what a console asks first:
// which resources it can publish at all. A type the API would refuse appearing
// here is a page that fails on the operator's second click.
func TestTheConfigTypesEndpointNamesWhatIsGoverned(t *testing.T) {
	configVersionEnv(t)

	code, body := call(t, ListConfigTypes, http.MethodGet, "/api/v1/config/types", "")
	if code != http.StatusOK {
		t.Fatalf("GET /config/types = %d, want 200", code)
	}
	types, ok := dataField(t, body)["resource_types"].([]any)
	if !ok {
		t.Fatalf("resource_types is missing or not a list: %+v", body)
	}
	want := map[string]bool{"dns_zone": false, "dhcp_scope": false, "ipam_subnet": false, "dns_records": false}
	for _, item := range types {
		name, _ := item.(string)
		if _, tracked := want[name]; tracked {
			want[name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("%q is not listed as publishable", name)
		}
	}
}

// TestADryRunPublishStoresNothing: a preview that consumed a revision number
// would make the next real publish collide with it, so the dry run has to be
// observable as a diff and invisible to the history.
func TestADryRunPublishStoresNothing(t *testing.T) {
	svc, db := configVersionEnv(t)
	seedConfigZone(t, db, "zone-dry", "dry.test.")

	content := `{"schema":1,"records":[{"name":"www","type":"A","value":"192.0.2.1","ttl":300,"enabled":true}]}`
	code, body := call(t, PublishConfigRevision, http.MethodPost, "/api/v1/config/publish",
		`{"resource_type":"dns_records","resource_id":"zone-dry","content":`+content+`,"dry_run":true}`)
	if code != http.StatusOK {
		t.Fatalf("a dry-run publish = %d, want 200: %+v", code, body)
	}
	data := dataField(t, body)
	if data["dry_run"] != true {
		t.Errorf("the response does not say it was a dry run: %+v", data)
	}
	if rev, ok := data["revision"]; ok && rev != nil {
		t.Errorf("a dry run returned a revision: %+v", rev)
	}

	n, err := svc.CurrentRevision(configver.ResourceDNSRecords, "zone-dry")
	if err != nil {
		t.Fatalf("read the current revision: %v", err)
	}
	if n != 0 {
		t.Fatalf("after a dry run the resource is at revision %d, want 0", n)
	}
}

// TestAPublishWithAStaleBaselineIsAConflict is the status code that tells a
// console to reload rather than to fix its document. Reporting it as a
// validation failure would send the operator editing a document that is fine.
func TestAPublishWithAStaleBaselineIsAConflict(t *testing.T) {
	_, db := configVersionEnv(t)
	seedConfigZone(t, db, "zone-conflict", "conflict.test.")

	content := `{"schema":1,"records":[]}`
	if code, body := call(t, PublishConfigRevision, http.MethodPost, "/api/v1/config/publish",
		`{"resource_type":"dns_records","resource_id":"zone-conflict","content":`+content+
			`,"expected_revision":7}`); code != http.StatusConflict {
		t.Fatalf("a publish citing revision 7 on a resource at 0 = %d, want 409: %+v", code, body)
	}
}

// TestAPublishOfContentTheResourceWouldRefuseIsRejected: the governed path must
// not be a way around validation.
func TestAPublishOfContentTheResourceWouldRefuseIsRejected(t *testing.T) {
	_, db := configVersionEnv(t)
	seedConfigZone(t, db, "zone-invalid", "invalid.test.")

	code, body := call(t, PublishConfigRevision, http.MethodPost, "/api/v1/config/publish",
		`{"resource_type":"dns_records","resource_id":"zone-invalid","content":{"schema":1,`+
			`"records":[{"name":"www","type":"A","value":"not-an-address","ttl":300,"enabled":true}]}}`)
	if code != http.StatusBadRequest {
		t.Fatalf("publishing an A record whose value is a hostname = %d, want 400: %+v", code, body)
	}
	if msg, _ := body["message"].(string); msg == "" {
		t.Error("the refusal carries no message: an operator is told something is wrong and not what")
	}
}

// TestAPublishToAResourceThatDoesNotExistIsNotFound: the remedy is "create it
// first", which is a different remedy from "pick a revision that exists", so it
// is a different status.
func TestAPublishToAResourceThatDoesNotExistIsNotFound(t *testing.T) {
	configVersionEnv(t)
	code, body := call(t, PublishConfigRevision, http.MethodPost, "/api/v1/config/publish",
		`{"resource_type":"dns_records","resource_id":"no-such-zone","content":{"schema":1,"records":[]}}`)
	if code != http.StatusNotFound {
		t.Fatalf("publishing to a missing zone = %d, want 404: %+v", code, body)
	}
}

// TestTheRollbackAddsARevision is the promise the exit condition is about: a
// rollback is a NEW revision, so the history still shows the configuration that
// was wrong and how long it was live.
func TestTheRollbackAddsARevision(t *testing.T) {
	svc, db := configVersionEnv(t)
	seedConfigZone(t, db, "zone-rollback", "rollback.test.")

	good := `{"schema":1,"records":[{"name":"www","type":"A","value":"192.0.2.1","ttl":300,"enabled":true}]}`
	bad := `{"schema":1,"records":[{"name":"www","type":"A","value":"198.51.100.9","ttl":300,"enabled":true}]}`

	for _, step := range []struct {
		content  string
		expected int
	}{
		{good, 0},
		{bad, 1},
	} {
		body := `{"resource_type":"dns_records","resource_id":"zone-rollback","content":` +
			step.content + `,"expected_revision":` + strconv.Itoa(step.expected) + `}`
		if code, out := call(t, PublishConfigRevision, http.MethodPost, "/api/v1/config/publish", body); code != http.StatusCreated {
			t.Fatalf("publishing %s = %d, want 201: %+v", step.content, code, out)
		}
		if _, err := svc.DrainOutbox(10); err != nil {
			t.Fatalf("drain the outbox: %v", err)
		}
	}

	code, out := call(t, RollbackConfigRevision, http.MethodPost, "/api/v1/config/rollback",
		`{"resource_type":"dns_records","resource_id":"zone-rollback","to_revision":1,"expected_revision":2}`)
	if code != http.StatusCreated {
		t.Fatalf("the rollback = %d, want 201: %+v", code, out)
	}
	revision, ok := dataField(t, out)["revision"].(map[string]any)
	if !ok {
		t.Fatalf("the rollback returned no revision: %+v", out)
	}
	if n, _ := revision["revision"].(float64); int64(n) != 3 {
		t.Errorf("the rollback produced revision %v, want 3: history is not rewritten, it is added to", n)
	}
	if note, _ := revision["note"].(string); note != "rollback to revision 1" {
		t.Errorf("the rollback note is %q, want it to name what it restored", note)
	}
	// The content is the whole point of a rollback, so it is re-encoded rather
	// than compared field by field: whatever the envelope hands back has to
	// carry revision 1's record, not a summary of it.
	content, err := json.Marshal(revision["content"])
	if err != nil {
		t.Fatalf("re-encode the rolled-back content: %v", err)
	}
	if !bytes.Contains(content, []byte("192.0.2.1")) {
		t.Errorf("the new revision does not carry revision 1's content: %s", content)
	}
}

// TestTheReleaseQueueSaysWhatHasNotReachedTheDataPlane: the queue is the answer
// to "is a configuration stuck?", which is the one question that used to be
// unanswerable.
func TestTheReleaseQueueSaysWhatHasNotReachedTheDataPlane(t *testing.T) {
	svc, db := configVersionEnv(t)
	seedConfigZone(t, db, "zone-queue", "queue.test.")

	if _, err := svc.RetryFailed(configver.ResourceDNSRecords, "zone-queue"); err != nil {
		t.Fatalf("clearing the queue before the case: %v", err)
	}
	body := `{"resource_type":"dns_records","resource_id":"zone-queue","content":{"schema":1,"records":[]}}`
	if code, out := call(t, PublishConfigRevision, http.MethodPost, "/api/v1/config/publish", body); code != http.StatusCreated {
		t.Fatalf("publishing into the queue case = %d, want 201: %+v", code, out)
	}

	code, out := call(t, ListConfigReleases, http.MethodGet, "/api/v1/config/releases", "")
	if code != http.StatusOK {
		t.Fatalf("GET /config/releases = %d, want 200: %+v", code, out)
	}
	data := dataField(t, out)
	stats, ok := data["stats"].(map[string]any)
	if !ok {
		t.Fatalf("the release listing carries no stats: %+v", data)
	}
	if pending, _ := stats["pending"].(float64); pending < 1 {
		t.Errorf("a published revision is not pending release: %+v", stats)
	}

	// Draining is the worker's job in production; here it is what makes the
	// queue empty, and an empty queue has to be reported as empty.
	if _, err := svc.DrainOutbox(10); err != nil {
		t.Fatalf("drain the outbox: %v", err)
	}
	if _, after := call(t, ListConfigReleases, http.MethodGet, "/api/v1/config/releases", ""); true {
		afterStats, _ := dataField(t, after)["stats"].(map[string]any)
		if pending, _ := afterStats["pending"].(float64); pending != 0 {
			t.Errorf("after every release was applied the queue still reports %v pending", pending)
		}
	}

	// The retry endpoint is the operator's way back from `failed`, and it is a
	// write because it changes what the worker will do.
	if code, out := call(t, RetryConfigReleases, http.MethodPost, "/api/v1/config/releases/retry", ""); code != http.StatusOK {
		t.Fatalf("POST /config/releases/retry = %d, want 200: %+v", code, out)
	}
}

// TestTheListingAndTheDiffAgree: the console reads a list, then asks for one
// revision and then for a diff between two. The three have to describe the same
// history or the page contradicts itself.
func TestTheListingAndTheDiffAgree(t *testing.T) {
	_, db := configVersionEnv(t)
	seedConfigZone(t, db, "zone-list", "list.test.")

	first := `{"schema":1,"records":[{"name":"www","type":"A","value":"192.0.2.1","ttl":300,"enabled":true}]}`
	second := `{"schema":1,"records":[{"name":"www","type":"A","value":"192.0.2.2","ttl":300,"enabled":true}]}`
	if code, out := call(t, PublishConfigRevision, http.MethodPost, "/api/v1/config/publish",
		`{"resource_type":"dns_records","resource_id":"zone-list","content":`+first+`}`); code != http.StatusCreated {
		t.Fatalf("publishing the first revision = %d, want 201: %+v", code, out)
	}
	if code, out := call(t, PublishConfigRevision, http.MethodPost, "/api/v1/config/publish",
		`{"resource_type":"dns_records","resource_id":"zone-list","content":`+second+
			`,"expected_revision":1}`); code != http.StatusCreated {
		t.Fatalf("publishing the second revision = %d, want 201: %+v", code, out)
	}

	listed, body2 := call(t, ListConfigRevisions, http.MethodGet,
		"/api/v1/config/revisions?resource_type=dns_records&resource_id=zone-list", "")
	if listed != http.StatusOK {
		t.Fatalf("GET /config/revisions = %d, want 200: %+v", listed, body2)
	}
	// The paginated envelope puts the collection straight into `data`; a
	// listing that silently changed shape is a console page that renders
	// nothing, so the shape is asserted rather than decoded leniently.
	revisions, ok := body2["data"].([]any)
	if !ok {
		t.Fatalf("the listing carries no list: %+v", body2)
	}
	if len(revisions) < 2 {
		t.Fatalf("the listing shows %d revision(s), want both of them: %+v", len(revisions), body2)
	}

	code, out := call(t, DiffConfigRevisions, http.MethodGet,
		"/api/v1/config/diff?resource_type=dns_records&resource_id=zone-list&from=1&to=2", "")
	if code != http.StatusOK {
		t.Fatalf("GET /config/diff = %d, want 200: %+v", code, out)
	}
	changes, ok := dataField(t, out)["changes"].([]any)
	if !ok || len(changes) == 0 {
		t.Fatalf("a publish that changed one record reports no change: %+v", dataField(t, out))
	}
}
