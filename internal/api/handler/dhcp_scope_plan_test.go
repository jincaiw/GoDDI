package handler

// The console lets an operator preview the DHCP scope a subnet would produce,
// look at what is in the way, and then create it. Between those two clicks the
// world can move -- somebody else creates a scope over the same addresses,
// somebody fences an address inside the range -- and the plan that was
// approved stops being the plan that would be created.
//
// These cases cover both halves of that check. The refusal is asserted to
// refuse *and to not write*: a guard that answers 409 after creating the scope
// would pass a status-code assertion and be worthless. The acceptance half is
// asserted too, because a check that refuses everything is indistinguishable
// from a check that works when nobody retries.
//
// The remaining cases cover the fail-closed edges. A reference that cannot be
// checked -- half of one, or one presented while the IPAM side is unreachable
// -- must refuse rather than quietly create the scope, or the operator walks
// away believing a verification happened.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jasonwa/goddi"
	"github.com/jasonwa/goddi/internal/dhcp/scope"
	"github.com/jasonwa/goddi/internal/ipam"
	"github.com/jasonwa/goddi/internal/ipam/subnet"
)

// newControlPlaneTestDB opens the real control-plane schema in memory, so a
// case fails for the reason under test rather than because a column was
// mistyped here.
func newControlPlaneTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Production caps the pool at one connection. Keeping that here means an
	// unclosed cursor fails the test instead of hanging only in production.
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
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}

// withIPAMServices installs a container for the duration of one test. It
// assigns the package variable directly rather than calling InitIPAMServices,
// which is guarded by a sync.Once and would ignore every call after the first.
func withIPAMServices(t *testing.T, svc *IPAMServiceContainer) {
	t.Helper()
	previous := IPAMServices
	IPAMServices = svc
	t.Cleanup(func() { IPAMServices = previous })
}

func seedControlSubnet(t *testing.T, db *sql.DB, spaceID, subnetID, cidr string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO ipam_spaces (id, name, description) VALUES (?, ?, '')`,
		spaceID, "space-"+spaceID); err != nil {
		t.Fatalf("seed space: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO ipam_subnets (id, space_id, name, cidr) VALUES (?, ?, ?, ?)`,
		subnetID, spaceID, "office", cidr); err != nil {
		t.Fatalf("seed subnet: %v", err)
	}
}

// wireControlPlane installs the two containers the plan and create handlers
// need, over one database, which is what the running console does.
func wireControlPlane(t *testing.T, db *sql.DB) {
	t.Helper()
	withIPAMServices(t, &IPAMServiceContainer{
		DB:        db,
		SubnetMgr: subnet.NewManager(db),
		Linkage:   ipam.NewLinkage(db),
	})
	withDHCPServices(t, &DHCPServiceContainer{
		DB:       db,
		ScopeMgr: scope.NewManager(db),
	})
}

func planRequest(subnetID string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/ipam/subnets/"+subnetID+"/dhcp-scope-plan", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", subnetID)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// planEnvelope is the part of the preview response these tests read.
type planEnvelope struct {
	Data struct {
		Plan  ipam.DHCPScopePlan `json:"plan"`
		Draft scope.ScopeOptions `json:"draft"`
	} `json:"data"`
}

func previewPlan(t *testing.T, subnetID string) planEnvelope {
	t.Helper()
	rec := httptest.NewRecorder()
	PlanDHCPScope(rec, planRequest(subnetID))
	if rec.Code != http.StatusOK {
		t.Fatalf("previewing = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var env planEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decoding the preview: %v (%s)", err, rec.Body.String())
	}
	if env.Data.Plan.Fingerprint == "" {
		t.Fatal("the preview carries no fingerprint, so nothing can be confirmed against it")
	}
	return env
}

func scopeCreateRequest(t *testing.T, body []byte) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodPost, "/api/v1/dhcp/scopes", bytes.NewReader(body))
}

// scopeCreateBody builds a create request whose scope matches what the preview
// proposes, carrying whatever plan reference the case is about.
func scopeCreateBody(t *testing.T, name string, plan any) []byte {
	t.Helper()
	scope := map[string]any{
		"name":     name,
		"subnet":   "192.0.2.0/24",
		"start_ip": "192.0.2.1",
		"end_ip":   "192.0.2.254",
	}
	if plan != nil {
		scope["plan"] = plan
	}
	body, err := json.Marshal(scope)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return body
}

func countScopesNamed(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_scopes WHERE name = ?`, name).Scan(&n); err != nil {
		t.Fatalf("count scopes: %v", err)
	}
	return n
}

// TestAScopeIsRefusedWhenThePreviewedPlanMoved is the case the fingerprint
// exists for.
func TestAScopeIsRefusedWhenThePreviewedPlanMoved(t *testing.T) {
	db := newControlPlaneTestDB(t)
	seedControlSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	wireControlPlane(t, db)

	previewed := previewPlan(t, "sn1")

	// Somebody else starts serving the same addresses.
	if _, err := db.Exec(`INSERT INTO dhcp_scopes
		(id, name, subnet, start_ip, end_ip, enabled)
		VALUES ('sc-other', 'other', '192.0.2.0/24', '192.0.2.10', '192.0.2.100', 1)`); err != nil {
		t.Fatalf("seed the other scope: %v", err)
	}

	rec := httptest.NewRecorder()
	CreateDHCPScope(rec, scopeCreateRequest(t, scopeCreateBody(t, "office", map[string]string{
		"subnet_id":   "sn1",
		"fingerprint": previewed.Data.Plan.Fingerprint,
	})))

	if rec.Code != http.StatusConflict {
		t.Fatalf("creating against a moved plan = %d, want %d: %s",
			rec.Code, http.StatusConflict, rec.Body.String())
	}
	// The refusal has to be a refusal, not a report after the fact.
	if n := countScopesNamed(t, db, "office"); n != 0 {
		t.Fatalf("%d scope(s) named office exist after a refusal", n)
	}
}

// TestAScopeIsCreatedAgainstAFreshPlan is the other half: the check must let a
// confirmed plan through, or operators stop reading the preview and start
// working around the check.
func TestAScopeIsCreatedAgainstAFreshPlan(t *testing.T) {
	db := newControlPlaneTestDB(t)
	seedControlSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	wireControlPlane(t, db)

	previewed := previewPlan(t, "sn1")
	if previewed.Data.Plan.StartIP != "192.0.2.1" || previewed.Data.Plan.EndIP != "192.0.2.254" {
		t.Fatalf("previewed range = %s..%s",
			previewed.Data.Plan.StartIP, previewed.Data.Plan.EndIP)
	}
	// The draft carries the router the plan named. Without this the created
	// scope hands every client no default route, which is the gap the plan
	// exists to close.
	if previewed.Data.Draft.Router == nil || *previewed.Data.Draft.Router != previewed.Data.Plan.Router {
		t.Fatalf("draft router = %v, plan router = %q", previewed.Data.Draft.Router, previewed.Data.Plan.Router)
	}

	rec := httptest.NewRecorder()
	CreateDHCPScope(rec, scopeCreateRequest(t, scopeCreateBody(t, "office", map[string]string{
		"subnet_id":   "sn1",
		"fingerprint": previewed.Data.Plan.Fingerprint,
	})))

	if rec.Code != http.StatusCreated {
		t.Fatalf("creating against a fresh plan = %d, want %d: %s",
			rec.Code, http.StatusCreated, rec.Body.String())
	}
	if n := countScopesNamed(t, db, "office"); n != 1 {
		t.Fatalf("%d scope(s) named office exist, want 1", n)
	}
}

// TestAPlanSurvivesAChangeThatCannotAlterIt. The dangerous direction of an
// optimistic check is refusing for reasons the operator cannot see: a lease
// taken inside the range is the normal life of a pool, and refusing for it
// would send them round the preview loop until they gave up on the check.
func TestAPlanSurvivesAChangeThatCannotAlterIt(t *testing.T) {
	db := newControlPlaneTestDB(t)
	seedControlSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	wireControlPlane(t, db)

	previewed := previewPlan(t, "sn1")

	// A client takes a lease in the range: a row appears and the pool is
	// still the pool.
	if _, err := db.Exec(`INSERT INTO ipam_addresses
		(id, subnet_id, space_id, ip_address, status, observed_state)
		VALUES ('ad-live', 'sn1', 'sp1', '192.0.2.30', 'dhcp', 'in_use')`); err != nil {
		t.Fatalf("seed a live address: %v", err)
	}

	rec := httptest.NewRecorder()
	CreateDHCPScope(rec, scopeCreateRequest(t, scopeCreateBody(t, "office", map[string]string{
		"subnet_id":   "sn1",
		"fingerprint": previewed.Data.Plan.Fingerprint,
	})))

	if rec.Code != http.StatusCreated {
		t.Fatalf("creating against a plan that still holds = %d, want %d: %s",
			rec.Code, http.StatusCreated, rec.Body.String())
	}
}

// TestAPlanThatIsNamedButAbsentStillChecks. A create that carries a plan
// reference can be asked to verify it by a caller who omitted half of it, or
// in a deployment where the IPAM side is not wired up. Both are refused: the
// caller believes a check happened, and reporting success would leave them
// believing it.
func TestAHalfSpecifiedPlanIsRefused(t *testing.T) {
	db := newControlPlaneTestDB(t)
	seedControlSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	wireControlPlane(t, db)

	cases := map[string]map[string]string{
		"fingerprint only": {"fingerprint": "abc"},
		"subnet only":      {"subnet_id": "sn1"},
		"empty both":       {},
	}
	for name, ref := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			CreateDHCPScope(rec, scopeCreateRequest(t, scopeCreateBody(t, "office", ref)))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("a half-specified plan = %d, want %d: %s",
					rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if n := countScopesNamed(t, db, "office"); n != 0 {
				t.Fatalf("%d scope(s) were created without the check running", n)
			}
		})
	}
}

// TestAPlanCannotBeCheckedWithoutTheIPAMSide. Unreachable is not the same as
// verified.
func TestAPlanCannotBeCheckedWithoutTheIPAMSide(t *testing.T) {
	db := newControlPlaneTestDB(t)
	seedControlSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	withDHCPServices(t, &DHCPServiceContainer{DB: db, ScopeMgr: scope.NewManager(db)})
	// The IPAM container is present but has no linkage, which is what a
	// deployment with the IPAM half disabled looks like.
	withIPAMServices(t, &IPAMServiceContainer{DB: db, SubnetMgr: subnet.NewManager(db)})

	rec := httptest.NewRecorder()
	CreateDHCPScope(rec, scopeCreateRequest(t, scopeCreateBody(t, "office", map[string]string{
		"subnet_id":   "sn1",
		"fingerprint": "whatever",
	})))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("an unverifiable plan = %d, want %d: %s",
			rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if n := countScopesNamed(t, db, "office"); n != 0 {
		t.Fatalf("%d scope(s) were created without the check running", n)
	}
}

// TestAScopeCanStillBeCreatedWithoutAPlan. The fingerprint is an addition to
// the create path, not a requirement: a caller that never previewed -- the CLI,
// an import -- must keep working.
func TestAScopeCanStillBeCreatedWithoutAPlan(t *testing.T) {
	db := newControlPlaneTestDB(t)
	seedControlSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	wireControlPlane(t, db)

	rec := httptest.NewRecorder()
	CreateDHCPScope(rec, scopeCreateRequest(t, scopeCreateBody(t, "office", nil)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("creating without a plan = %d, want %d: %s",
			rec.Code, http.StatusCreated, rec.Body.String())
	}
	// And the reference is not stored as part of the scope: it describes a
	// conversation, not the scope.
	var comment sql.NullString
	if err := db.QueryRow(`SELECT comment FROM dhcp_scopes WHERE name = 'office'`).Scan(&comment); err != nil {
		t.Fatalf("read back the scope: %v", err)
	}
	if comment.Valid && comment.String != "" {
		t.Errorf("the created scope carries the plan reference in its comment: %q", comment.String)
	}
}

// TestAPlanForAnUnknownSubnetIsNotFound. A preview for a subnet that does not
// exist is a 404, not a 500 that reads like a bug in the server.
func TestAPlanForAnUnknownSubnetIsNotFound(t *testing.T) {
	db := newControlPlaneTestDB(t)
	wireControlPlane(t, db)

	rec := httptest.NewRecorder()
	PlanDHCPScope(rec, planRequest("nope"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("previewing a missing subnet = %d, want %d: %s",
			rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestAPlanForAnIPv6SubnetIsARefusal. DHCPv6 is out of scope, so the answer is
// a 400 that says so rather than an IPv4 plan computed from an IPv6 address.
func TestAPlanForAnIPv6SubnetIsARefusal(t *testing.T) {
	db := newControlPlaneTestDB(t)
	seedControlSubnet(t, db, "sp1", "sn1", "2001:db8::/64")
	wireControlPlane(t, db)

	rec := httptest.NewRecorder()
	PlanDHCPScope(rec, planRequest("sn1"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("previewing an IPv6 subnet = %d, want %d: %s",
			rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
