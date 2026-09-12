package configver

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jasonwa/goddi"
)

// newTestDB builds a database with the real migration schema.
//
// Using the migrations rather than a hand-written subset is deliberate: the
// revision and outbox tables are added by migration 019, and a test that
// invented its own schema would keep passing after the migration changed.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Production caps the pool at one connection for SQLite. Keeping that here
	// means an unclosed cursor inside a transaction fails the test instead of
	// hanging forever in production only.
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

	t.Cleanup(func() { db.Close() })
	return db
}

// --- harness ----------------------------------------------------------------

// faultyAdapter wraps a real adapter so a write or announce failure can be
// injected.
//
// The tests deliberately run against the real DHCP scope adapter rather than a
// stand-in: the SQL, the validation rules and the column mapping all matter,
// and a fake adapter would let a schema change pass unnoticed.
type faultyAdapter struct {
	inner       Adapter
	mu          sync.Mutex
	notifyErr   error
	applyErr    error
	notifyCalls int
}

func (a *faultyAdapter) Type() ResourceType               { return a.inner.Type() }
func (a *faultyAdapter) Validate(c json.RawMessage) error { return a.inner.Validate(c) }
func (a *faultyAdapter) Exists(q Queryer, id string) (bool, error) {
	return a.inner.Exists(q, id)
}

func (a *faultyAdapter) Apply(tx *sql.Tx, id string, c json.RawMessage) error {
	a.mu.Lock()
	err := a.applyErr
	a.mu.Unlock()
	if err != nil {
		return err
	}
	return a.inner.Apply(tx, id, c)
}

func (a *faultyAdapter) failApply(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applyErr = err
}

func (a *faultyAdapter) Notify(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.notifyCalls++
	return a.notifyErr
}

func (a *faultyAdapter) calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.notifyCalls
}

func (a *faultyAdapter) failNotify(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.notifyErr = err
}

func (a *faultyAdapter) allowNotify() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.notifyErr = nil
}

func newScopeService(t *testing.T, db *sql.DB) (*Service, *faultyAdapter) {
	t.Helper()
	adapter := &faultyAdapter{inner: NewDHCPScopeAdapter()}
	svc := NewService(db)
	// Registered through the map rather than Register() so the wrapper can
	// stand in for the production adapter of the same type.
	svc.adapters[ResourceDHCPScope] = adapter
	return svc, adapter
}

func seedScope(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO dhcp_scopes
		(id, name, subnet, start_ip, end_ip, lease_time, enabled, dns_updates)
		VALUES (?, 'seed-scope', '192.0.2.0/24', '192.0.2.10', '192.0.2.100', 3600, 1, 0)`,
		id); err != nil {
		t.Fatalf("seed scope: %v", err)
	}
}

// scopeContent builds a complete, valid scope snapshot. Marshalling the real
// content struct keeps the tests honest about the wire shape: a hand-written
// JSON literal would keep passing after a field was renamed.
func scopeContent(t *testing.T, mutate func(*DHCPScopeContent)) json.RawMessage {
	t.Helper()
	c := DHCPScopeContent{
		Name:       "test-scope",
		Subnet:     "192.0.2.0/24",
		StartIP:    "192.0.2.10",
		EndIP:      "192.0.2.100",
		LeaseTime:  3600,
		Enabled:    true,
		DomainName: "example.test",
	}
	if mutate != nil {
		mutate(&c)
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal scope content: %v", err)
	}
	return b
}

func scopeEndIP(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var out string
	if err := db.QueryRow(`SELECT end_ip FROM dhcp_scopes WHERE id = ?`, id).Scan(&out); err != nil {
		t.Fatalf("read scope end_ip: %v", err)
	}
	return out
}

// forceDue makes every pending release immediately eligible, so a retry can be
// exercised without sleeping through the backoff.
func forceDue(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(
		`UPDATE config_release_outbox SET next_attempt_at = datetime('now') WHERE status = 'pending'`,
	); err != nil {
		t.Fatalf("force due: %v", err)
	}
}

func publishScope(t *testing.T, svc *Service, id string, expected int64, content json.RawMessage) *PublishResult {
	t.Helper()
	res, err := svc.Publish(PublishRequest{
		ResourceType:     ResourceDHCPScope,
		ResourceID:       id,
		Content:          content,
		ExpectedRevision: expected,
	})
	if err != nil {
		t.Fatalf("publish scope %s (expected %d): %v", id, expected, err)
	}
	return res
}

// --- publish ----------------------------------------------------------------

func TestDiffRevisions_FirstRevisionReportsAddedFields(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-first-diff")

	publishScope(t, svc, "scope-first-diff", 0, scopeContent(t, nil))
	changes, err := svc.DiffRevisions(ResourceDHCPScope, "scope-first-diff", 0, 1)
	if err != nil {
		t.Fatalf("diff first revision: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("first revision diff is empty; expected added fields")
	}
	for _, change := range changes {
		if change.Old != nil {
			t.Fatalf("change %q has old value %v, want nil", change.Field, change.Old)
		}
	}
}

func TestPublish_FirstRevisionIsNumberedOne(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	res := publishScope(t, svc, "scope-1", 0, scopeContent(t, nil))

	if res.Revision.Revision != 1 {
		t.Fatalf("revision = %d, want 1", res.Revision.Revision)
	}
	if res.Revision.Status != StatusStaged {
		t.Fatalf("status = %s, want %s", res.Revision.Status, StatusStaged)
	}
	// The first publish has nothing to compare against, so every field reads
	// as added rather than the diff being empty.
	if len(res.Changes) == 0 {
		t.Fatalf("changes are empty; a first publish should list every field as added")
	}
	for _, ch := range res.Changes {
		if ch.Old != nil {
			t.Fatalf("change %q has Old=%v, want nil for a first publish", ch.Field, ch.Old)
		}
	}
}

func TestPublish_ContentIsCanonicalised(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	// Key order and whitespace must not affect the stored bytes or the hash.
	// If they did, an idempotent retry would look like a different
	// configuration and "did anything change" would depend on formatting.
	_, h1, err := normaliseContent(json.RawMessage(`{"b":2,"a":1}`))
	if err != nil {
		t.Fatalf("normalise 1: %v", err)
	}
	_, h2, err := normaliseContent(json.RawMessage("{\n  \"a\": 1,\n  \"b\": 2\n}"))
	if err != nil {
		t.Fatalf("normalise 2: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("hashes differ for the same object: %s vs %s", h1, h2)
	}

	snapshot := scopeContent(t, nil)
	want, _, err := normaliseContent(snapshot)
	if err != nil {
		t.Fatalf("normalise snapshot: %v", err)
	}
	first := publishScope(t, svc, "scope-1", 0, snapshot)
	if string(first.Revision.Content) != string(want) {
		t.Fatalf("stored content = %s, want the canonical form %s", first.Revision.Content, want)
	}

	// Republishing the canonical bytes must report no change at all.
	second := publishScope(t, svc, "scope-1", 1, first.Revision.Content)
	if len(second.Changes) != 0 {
		t.Fatalf("changes = %+v, want none for identical content", second.Changes)
	}
}

func TestPublish_ConcurrentWritersOnlyOneWins(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	// Build the payloads before starting the goroutines: t.Fatalf is only
	// valid on the test goroutine.
	const writers = 8
	payloads := make([]json.RawMessage, writers)
	for i := range payloads {
		payloads[i] = scopeContent(t, func(c *DHCPScopeContent) {
			c.Comment = fmt.Sprintf("writer %d", i)
		})
	}

	// Every writer read revision 0 and publishes against it.
	var wg sync.WaitGroup
	results := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.Publish(PublishRequest{
				ResourceType:     ResourceDHCPScope,
				ResourceID:       "scope-1",
				Content:          payloads[i],
				ExpectedRevision: 0,
			})
			results[i] = err
		}(i)
	}
	wg.Wait()

	var conflicts, successes int
	for _, err := range results {
		var conflict *ConflictError
		switch {
		case err == nil:
			successes++
		case errors.As(err, &conflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful publishes = %d, want exactly 1", successes)
	}
	if conflicts != writers-1 {
		t.Fatalf("conflicts = %d, want %d", conflicts, writers-1)
	}
}

func TestPublish_IdempotentReplayReturnsSameRevision(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	req := PublishRequest{
		ResourceType:   ResourceDHCPScope,
		ResourceID:     "scope-1",
		Content:        scopeContent(t, nil),
		IdempotencyKey: "key-1",
	}
	first, err := svc.Publish(req)
	if err != nil {
		t.Fatalf("publish 1: %v", err)
	}

	// A retry carrying the same key must not create revision 2, and must not
	// trip the concurrency check against the revision it itself created.
	second, err := svc.Publish(req)
	if err != nil {
		t.Fatalf("publish 2 (replay): %v", err)
	}
	if !second.Replayed {
		t.Fatalf("replayed = false, want true")
	}
	if second.Revision.ID != first.Revision.ID {
		t.Fatalf("replay returned a different revision: %s vs %s", second.Revision.ID, first.Revision.ID)
	}

	var revisions, releases int
	if err := db.QueryRow(`SELECT COUNT(*) FROM config_revisions WHERE resource_id = 'scope-1'`).Scan(&revisions); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM config_release_outbox WHERE resource_id = 'scope-1'`).Scan(&releases); err != nil {
		t.Fatalf("count releases: %v", err)
	}
	if revisions != 1 {
		t.Fatalf("revision rows = %d, want 1", revisions)
	}
	if releases != 1 {
		t.Fatalf("release rows = %d, want 1 (a replay must not queue a second release)", releases)
	}
}

func TestPublish_IdempotencyKeyCannotBeReusedForAnotherResource(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-1")
	seedScope(t, db, "scope-2")

	if _, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "scope-1",
		Content: scopeContent(t, nil), IdempotencyKey: "shared",
	}); err != nil {
		t.Fatalf("publish scope-1: %v", err)
	}

	// Reusing a key for a different resource must not hand back scope-1's
	// revision as if it were scope-2's result.
	_, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "scope-2",
		Content: scopeContent(t, nil), IdempotencyKey: "shared",
	})
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want *ConflictError", err)
	}
}

func TestPublish_DryRunStoresNothingAndConsumesNoRevision(t *testing.T) {
	db := newTestDB(t)
	svc, adapter := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	res, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "scope-1",
		Content: scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "192.0.2.50" }),
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !res.DryRun {
		t.Fatalf("dry_run = false, want true")
	}
	if res.Revision != nil {
		t.Fatalf("dry run returned a stored revision; a preview must not be written")
	}
	if res.BaseRevision != 0 {
		t.Fatalf("base_revision = %d, want 0 for an unpublished resource", res.BaseRevision)
	}
	// The resource has no prior revision, so every field reads as added --
	// including the end_ip the preview would change.
	found := false
	for _, ch := range res.Changes {
		if ch.Field == "end_ip" && ch.New == "192.0.2.50" {
			found = true
		}
	}
	if !found {
		t.Fatalf("changes = %+v, want end_ip among the previewed fields", res.Changes)
	}

	// A dry run must not touch the resource, queue a release, or leave a row.
	if got := scopeEndIP(t, db, "scope-1"); got != "192.0.2.100" {
		t.Fatalf("scope end_ip = %q, want it untouched", got)
	}
	var revisions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM config_revisions`).Scan(&revisions); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if revisions != 0 {
		t.Fatalf("revision rows = %d, want 0 for a dry run", revisions)
	}
	if applied, err := svc.DrainOutbox(10); err != nil || applied != 0 {
		t.Fatalf("drain applied = %d err = %v, want 0/nil", applied, err)
	}
	if adapter.calls() != 0 {
		t.Fatalf("notify calls = %d, want 0", adapter.calls())
	}

	// A preview must not consume a revision number: a real publish after a dry
	// run still starts at 1. (Storing the preview as revision 1 used to make
	// this insert collide on the unique index.)
	real := publishScope(t, svc, "scope-1", 0, scopeContent(t, nil))
	if real.Revision.Revision != 1 {
		t.Fatalf("revision = %d, want 1 (the dry run must not consume a number)", real.Revision.Revision)
	}
}

func TestPublish_DryRunRejectsStaleAndInvalidInput(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	publishScope(t, svc, "scope-1", 0, scopeContent(t, nil))

	// A preview built on an outdated revision must be rejected, otherwise an
	// operator would review a diff against a state that is no longer current.
	_, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "scope-1",
		Content:          scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "192.0.2.50" }),
		ExpectedRevision: 0,
		DryRun:           true,
	})
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want *ConflictError", err)
	}

	// And an invalid preview is rejected the same way a real publish is.
	_, err = svc.Publish(PublishRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "scope-1",
		Content:          scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "198.51.100.7" }),
		ExpectedRevision: 1,
		DryRun:           true,
	})
	var vErr *ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
}

func TestPublish_RejectsInvalidContentWithoutWriting(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	// end_ip outside the subnet is a real validation failure, not a synthetic
	// one: this is the typo an operator actually makes.
	_, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "scope-1",
		Content: scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "198.51.100.5" }),
	})
	var vErr *ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if vErr.Reason == "" {
		t.Fatalf("validation error carries no reason; the operator needs to know which field is wrong")
	}

	// Nothing may be written: no revision, no release, resource untouched.
	var revisions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM config_revisions`).Scan(&revisions); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if revisions != 0 {
		t.Fatalf("revision rows = %d, want 0", revisions)
	}
	if got := scopeEndIP(t, db, "scope-1"); got != "192.0.2.100" {
		t.Fatalf("scope end_ip = %q, want it untouched", got)
	}
}

func TestPublish_RejectsNonObjectContent(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	_, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "scope-1",
		Content: json.RawMessage(`[1,2,3]`),
	})
	var vErr *ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
}

func TestPublish_RejectsUnknownAndMissingResources(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	// No adapter registered for DNS zones in this service.
	if _, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDNSZone, ResourceID: "zone-1",
		Content: json.RawMessage(`{"name":"example.test."}`),
	}); !errors.Is(err, ErrNoAdapter) {
		t.Fatalf("err = %v, want ErrNoAdapter", err)
	}

	if _, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "missing",
		Content: scopeContent(t, nil),
	}); !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("err = %v, want ErrResourceNotFound", err)
	}
}

// --- outbox -----------------------------------------------------------------

func TestDrainOutbox_AppliesAndMarksApplied(t *testing.T) {
	db := newTestDB(t)
	svc, adapter := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	publishScope(t, svc, "scope-1", 0,
		scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "192.0.2.200" }))

	applied, err := svc.DrainOutbox(10)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}

	if got := scopeEndIP(t, db, "scope-1"); got != "192.0.2.200" {
		t.Fatalf("scope end_ip = %q, want the published content", got)
	}
	if adapter.calls() != 1 {
		t.Fatalf("notify calls = %d, want 1", adapter.calls())
	}

	rev, err := svc.GetByNumber(ResourceDHCPScope, "scope-1", 1)
	if err != nil {
		t.Fatalf("get revision: %v", err)
	}
	if rev.Status != StatusApplied {
		t.Fatalf("status = %s, want %s", rev.Status, StatusApplied)
	}
	if rev.AppliedGeneration != 1 {
		t.Fatalf("applied_generation = %d, want 1", rev.AppliedGeneration)
	}
	if rev.AppliedAt == "" {
		t.Fatalf("applied_at is empty")
	}

	// A second drain must be a no-op: an applied release is not re-applied.
	if applied, err := svc.DrainOutbox(10); err != nil || applied != 0 {
		t.Fatalf("second drain applied = %d err = %v, want 0/nil", applied, err)
	}
	if adapter.calls() != 1 {
		t.Fatalf("notify calls = %d after second drain, want still 1", adapter.calls())
	}
}

func TestDrainOutbox_AppliesNewestLast(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	// Three revisions queued before any drain: the last one applied must be
	// the newest, or the data plane would serve a superseded configuration.
	for i, endIP := range []string{"192.0.2.50", "192.0.2.60", "192.0.2.70"} {
		publishScope(t, svc, "scope-1", int64(i), scopeContent(t, func(c *DHCPScopeContent) {
			c.EndIP = endIP
		}))
	}

	applied, err := svc.DrainOutbox(10)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if applied != 3 {
		t.Fatalf("applied = %d, want 3", applied)
	}
	if got := scopeEndIP(t, db, "scope-1"); got != "192.0.2.70" {
		t.Fatalf("scope end_ip = %q, want the newest revision", got)
	}
}

func TestDrainOutbox_NotifyFailureKeepsRevisionStaged(t *testing.T) {
	db := newTestDB(t)
	svc, adapter := newScopeService(t, db)
	seedScope(t, db, "scope-1")
	adapter.failNotify(errors.New("data plane unavailable"))

	publishScope(t, svc, "scope-1", 0,
		scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "192.0.2.200" }))

	// Drive the release to its terminal state. The backoff is asserted
	// separately, so eligibility is forced rather than waited out.
	for i := 0; i < ReleaseMaxAttempts; i++ {
		forceDue(t, db)
		if _, err := svc.DrainOutbox(10); err != nil {
			t.Fatalf("drain %d: %v", i, err)
		}
	}

	releases, err := svc.ListReleases(ReleaseFailed, 10)
	if err != nil {
		t.Fatalf("list releases: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("failed releases = %d, want 1 (the row must stay visible)", len(releases))
	}
	if releases[0].Attempts < ReleaseMaxAttempts {
		t.Fatalf("attempts = %d, want >= %d", releases[0].Attempts, ReleaseMaxAttempts)
	}
	if releases[0].LastError == "" {
		t.Fatalf("last_error is empty; the reason must be visible to an operator")
	}
	if !releases[0].ResourceApplied {
		t.Fatalf("resource_applied = false, but the write committed before the announcement failed")
	}

	// The resource write succeeded, so the configuration IS in the database.
	// The honest report is "written, not announced": the revision must stay
	// staged rather than claim failed, which would tell an operator that
	// nothing changed when something did.
	if got := scopeEndIP(t, db, "scope-1"); got != "192.0.2.200" {
		t.Fatalf("scope end_ip = %q, want the applied content", got)
	}
	rev, err := svc.GetByNumber(ResourceDHCPScope, "scope-1", 1)
	if err != nil {
		t.Fatalf("get revision: %v", err)
	}
	if rev.Status != StatusStaged {
		t.Fatalf("revision status = %s, want %s (written but never announced)", rev.Status, StatusStaged)
	}
	if rev.AppliedGeneration != 0 {
		t.Fatalf("applied_generation = %d, want 0 while the data plane has not confirmed", rev.AppliedGeneration)
	}

	pending, failed, err := svc.OutboxStats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if pending != 0 || failed != 1 {
		t.Fatalf("stats = (%d pending, %d failed), want (0, 1)", pending, failed)
	}

	// Recovery must not require a database edit.
	adapter.allowNotify()
	n, err := svc.RetryFailed(ResourceDHCPScope, "scope-1")
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("reset releases = %d, want 1", n)
	}
	if applied, err := svc.DrainOutbox(10); err != nil || applied != 1 {
		t.Fatalf("drain after retry = %d err = %v, want 1/nil", applied, err)
	}
	rev, err = svc.GetByNumber(ResourceDHCPScope, "scope-1", 1)
	if err != nil {
		t.Fatalf("get revision: %v", err)
	}
	if rev.Status != StatusApplied {
		t.Fatalf("revision status after retry = %s, want %s", rev.Status, StatusApplied)
	}
	if rev.AppliedGeneration != 1 {
		t.Fatalf("applied_generation = %d, want 1 after the retry succeeded", rev.AppliedGeneration)
	}
}

func TestDrainOutbox_ApplyFailurePreservesResourceAndFailsRevision(t *testing.T) {
	db := newTestDB(t)
	svc, adapter := newScopeService(t, db)
	seedScope(t, db, "scope-1")
	adapter.failApply(errors.New("database is read-only"))

	publishScope(t, svc, "scope-1", 0,
		scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "192.0.2.200" }))

	for i := 0; i < ReleaseMaxAttempts; i++ {
		forceDue(t, db)
		if _, err := svc.DrainOutbox(10); err != nil {
			t.Fatalf("drain %d: %v", i, err)
		}
	}

	// The write never landed, so the last-known-good configuration must still
	// be the one in the table.
	if got := scopeEndIP(t, db, "scope-1"); got != "192.0.2.100" {
		t.Fatalf("scope end_ip = %q, want the previous value preserved", got)
	}

	releases, err := svc.ListReleases(ReleaseFailed, 10)
	if err != nil {
		t.Fatalf("list releases: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("failed releases = %d, want 1", len(releases))
	}
	if releases[0].ResourceApplied {
		t.Fatalf("resource_applied = true, but the write never succeeded")
	}

	rev, err := svc.GetByNumber(ResourceDHCPScope, "scope-1", 1)
	if err != nil {
		t.Fatalf("get revision: %v", err)
	}
	if rev.Status != StatusFailed {
		t.Fatalf("revision status = %s, want %s", rev.Status, StatusFailed)
	}

	// And a revision that never reached the resource must not be offered as a
	// rollback target.
	if _, err := svc.Rollback(RollbackRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "scope-1",
		ToRevision: 1, ExpectedRevision: 1,
	}); !errors.Is(err, ErrInvalidRevision) {
		t.Fatalf("err = %v, want ErrInvalidRevision for a revision that never served", err)
	}
}

func TestDrainOutbox_BacksOffWithoutReapplyingImmediately(t *testing.T) {
	db := newTestDB(t)
	svc, adapter := newScopeService(t, db)
	seedScope(t, db, "scope-1")
	adapter.failNotify(errors.New("data plane unavailable"))

	publishScope(t, svc, "scope-1", 0,
		scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "192.0.2.200" }))

	if _, err := svc.DrainOutbox(10); err != nil {
		t.Fatalf("first drain: %v", err)
	}
	// The entry was backed off into the future, so an immediate second drain
	// must not pick it up again -- otherwise a permanent failure would spin.
	if applied, err := svc.DrainOutbox(10); err != nil || applied != 0 {
		t.Fatalf("immediate second drain = %d err = %v, want 0/nil", applied, err)
	}
	if adapter.calls() != 1 {
		t.Fatalf("notify calls = %d, want 1 (the backoff must hold the retry)", adapter.calls())
	}

	releases, err := svc.ListReleases(ReleasePending, 10)
	if err != nil {
		t.Fatalf("list releases: %v", err)
	}
	if len(releases) != 1 || releases[0].Attempts != 1 {
		t.Fatalf("pending release = %+v, want 1 attempt recorded", releases)
	}
}

func TestReleaseBackoffIsExponentialAndCapped(t *testing.T) {
	if got := releaseBackoff(1); got != ReleaseRetryBase {
		t.Fatalf("backoff(1) = %s, want %s", got, ReleaseRetryBase)
	}
	if got := releaseBackoff(2); got != 2*ReleaseRetryBase {
		t.Fatalf("backoff(2) = %s, want %s", got, 2*ReleaseRetryBase)
	}
	// Far past the cap: must not overflow into a negative or absurd duration.
	if got := releaseBackoff(64); got != ReleaseRetryMax {
		t.Fatalf("backoff(64) = %s, want the cap %s", got, ReleaseRetryMax)
	}
}

// --- diff -------------------------------------------------------------------

func TestDiff_ReportsAddedRemovedAndChanged(t *testing.T) {
	changes, err := Diff(
		json.RawMessage(`{"kept":1,"changed":"a","removed":true}`),
		json.RawMessage(`{"kept":1,"changed":"b","added":42}`),
	)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}

	want := []FieldChange{
		{Field: "added", Old: nil, New: float64(42)},
		{Field: "changed", Old: "a", New: "b"},
		{Field: "removed", Old: true, New: nil},
	}
	if len(changes) != len(want) {
		t.Fatalf("changes = %+v, want %+v", changes, want)
	}
	for i := range want {
		if changes[i].Field != want[i].Field {
			t.Fatalf("changes[%d].Field = %q, want %q", i, changes[i].Field, want[i].Field)
		}
		if !valuesEqual(changes[i].Old, want[i].Old) || !valuesEqual(changes[i].New, want[i].New) {
			t.Fatalf("changes[%d] = %+v, want %+v", i, changes[i], want[i])
		}
	}
}

func TestDiff_IsSortedAndStable(t *testing.T) {
	oldC := json.RawMessage(`{"z":1,"a":1,"m":1}`)
	newC := json.RawMessage(`{"z":2,"a":2,"m":2}`)

	first, err := Diff(oldC, newC)
	if err != nil {
		t.Fatalf("diff 1: %v", err)
	}
	second, err := Diff(oldC, newC)
	if err != nil {
		t.Fatalf("diff 2: %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("changes = %d, want 3", len(first))
	}
	for i, want := range []string{"a", "m", "z"} {
		if first[i].Field != want {
			t.Fatalf("changes[%d].Field = %q, want %q (sorted)", i, first[i].Field, want)
		}
		if first[i].Field != second[i].Field {
			t.Fatalf("diff is not deterministic at index %d", i)
		}
	}
}

func valuesEqual(a, b any) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

// --- rollback ---------------------------------------------------------------

func TestRollback_CreatesNewRevisionWithOldContent(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	publishScope(t, svc, "scope-1", 0,
		scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "192.0.2.200" }))
	publishScope(t, svc, "scope-1", 1,
		scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "192.0.2.99" }))

	if applied, err := svc.DrainOutbox(10); err != nil || applied != 2 {
		t.Fatalf("drain = %d err = %v, want 2/nil", applied, err)
	}
	if got := scopeEndIP(t, db, "scope-1"); got != "192.0.2.99" {
		t.Fatalf("scope end_ip after v2 = %q", got)
	}

	res, err := svc.Rollback(RollbackRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "scope-1",
		ToRevision: 1, ExpectedRevision: 2,
	})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}

	// A rollback is a NEW revision, not a rewrite: the log must still show
	// that the bad configuration was live.
	if res.Revision.Revision != 3 {
		t.Fatalf("rollback revision = %d, want 3", res.Revision.Revision)
	}
	if res.Revision.Note == "" {
		t.Fatalf("rollback note is empty; the reason should be recorded")
	}

	// The old revision must be untouched.
	v2, err := svc.GetByNumber(ResourceDHCPScope, "scope-1", 2)
	if err != nil {
		t.Fatalf("get v2: %v", err)
	}
	var restored DHCPScopeContent
	if err := json.Unmarshal(v2.Content, &restored); err != nil {
		t.Fatalf("decode v2 content: %v", err)
	}
	if restored.EndIP != "192.0.2.99" {
		t.Fatalf("v2 content was rewritten: end_ip = %q", restored.EndIP)
	}

	if applied, err := svc.DrainOutbox(10); err != nil || applied != 1 {
		t.Fatalf("drain after rollback = %d err = %v, want 1/nil", applied, err)
	}
	if got := scopeEndIP(t, db, "scope-1"); got != "192.0.2.200" {
		t.Fatalf("scope end_ip after rollback = %q, want the v1 pool restored", got)
	}

	// And the diff of the rollback must read as the single reverted field, not
	// as a re-creation of everything.
	if len(res.Changes) != 1 || res.Changes[0].Field != "end_ip" {
		t.Fatalf("rollback changes = %+v, want only the end_ip change", res.Changes)
	}
}

func TestRollback_DoesNotReleaseInUseLeases(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	// A client is holding an address from the pool that the rollback will
	// restore. Rolling the configuration back must not hand that address to
	// someone else, and must not delete the lease.
	if _, err := db.Exec(`INSERT INTO dhcp_leases
		(id, scope_id, ip_address, mac_address, hostname, status, generation,
		 lease_start, lease_end, last_seen)
		VALUES ('lease-1', 'scope-1', '192.0.2.150', 'aa:bb:cc:dd:ee:ff', 'host1', 'active', 1,
		        datetime('now'), datetime('now', '+1 hour'), datetime('now'))`); err != nil {
		t.Fatalf("seed lease: %v", err)
	}

	publishScope(t, svc, "scope-1", 0,
		scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "192.0.2.200" }))
	publishScope(t, svc, "scope-1", 1,
		scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "192.0.2.120" }))
	if applied, err := svc.DrainOutbox(10); err != nil || applied != 2 {
		t.Fatalf("drain = %d err = %v", applied, err)
	}

	if _, err := svc.Rollback(RollbackRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "scope-1",
		ToRevision: 1, ExpectedRevision: 2,
	}); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if applied, err := svc.DrainOutbox(10); err != nil || applied != 1 {
		t.Fatalf("drain after rollback = %d err = %v", applied, err)
	}

	// The pool shrank back below the leased address. The lease is still a
	// commitment to the client and must survive: the configuration publish
	// only governs the pool, it does not revoke assignments.
	var status, ip string
	if err := db.QueryRow(`SELECT status, ip_address FROM dhcp_leases WHERE id = 'lease-1'`).Scan(&status, &ip); err != nil {
		t.Fatalf("read lease: %v", err)
	}
	if status != "active" || ip != "192.0.2.150" {
		t.Fatalf("lease = (%s, %s), want it untouched by the rollback", status, ip)
	}
}

func TestRollback_RefusesUnknownTarget(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	publishScope(t, svc, "scope-1", 0, scopeContent(t, nil))

	if _, err := svc.Rollback(RollbackRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "scope-1",
		ToRevision: 99, ExpectedRevision: 1,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}

	// A dry run stores nothing, so it has no revision to roll back to. This
	// guards the property that made previews non-addressable.
	if _, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "scope-1",
		Content:          scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "192.0.2.50" }),
		ExpectedRevision: 1,
		DryRun:           true,
	}); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if _, err := svc.Rollback(RollbackRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "scope-1",
		ToRevision: 2, ExpectedRevision: 1,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (a preview must not be addressable)", err)
	}
}

func TestPruneRevisions_KeepsWindowAppliedAndOutboxReferenced(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-prune")

	for i := 0; i < 4; i++ {
		endIP := fmt.Sprintf("192.0.2.%d", 50+i)
		publishScope(t, svc, "scope-prune", int64(i), scopeContent(t, func(c *DHCPScopeContent) {
			c.EndIP = endIP
		}))
	}
	if applied, err := svc.DrainOutbox(10); err != nil || applied != 4 {
		t.Fatalf("drain = %d err = %v, want 4/nil", applied, err)
	}

	// The outbox normally retains every release row as history. Remove the
	// oldest applied marker to model a separately compacted release queue, then
	// protect revision 2 as if its release were still referenced by an
	// operator-visible pending row. This exercises both retention guards.
	if _, err := db.Exec(`DELETE FROM config_release_outbox WHERE revision = 1`); err != nil {
		t.Fatalf("compact revision 1 release: %v", err)
	}
	if _, err := db.Exec(`UPDATE config_release_outbox SET status = 'pending' WHERE revision = 2`); err != nil {
		t.Fatalf("protect revision 2: %v", err)
	}
	removed, err := svc.PruneRevisions(ResourceDHCPScope, "scope-prune", 2)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1 (only revision 1 is eligible)", removed)
	}

	var revisions []int
	rows, err := db.Query(`SELECT revision FROM config_revisions WHERE resource_id = 'scope-prune' ORDER BY revision`)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	for rows.Next() {
		var revision int
		if err := rows.Scan(&revision); err != nil {
			rows.Close()
			t.Fatalf("scan revision: %v", err)
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close revisions: %v", err)
	}
	if got, want := fmt.Sprint(revisions), "[2 3 4]"; got != want {
		t.Fatalf("remaining revisions = %s, want %s", got, want)
	}
}

func TestPruneRevisions_RejectsInvalidKeepAndType(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	if _, err := svc.PruneRevisions(ResourceDHCPScope, "scope", 0); err == nil {
		t.Fatal("keep=0 was accepted")
	}
	if _, err := svc.PruneRevisions(ResourceType("unknown"), "scope", 1); err == nil {
		t.Fatal("unknown resource type was accepted")
	}
}

func TestRollback_RespectsConcurrencyGuard(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newScopeService(t, db)
	seedScope(t, db, "scope-1")

	publishScope(t, svc, "scope-1", 0, scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "192.0.2.200" }))
	publishScope(t, svc, "scope-1", 1, scopeContent(t, func(c *DHCPScopeContent) { c.EndIP = "192.0.2.99" }))

	// Whoever holds the old revision must not be able to roll the resource
	// back underneath the newer writer.
	_, err := svc.Rollback(RollbackRequest{
		ResourceType: ResourceDHCPScope, ResourceID: "scope-1",
		ToRevision: 1, ExpectedRevision: 1,
	})
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want *ConflictError", err)
	}
}

// --- registry ---------------------------------------------------------------

// unknownTypeAdapter reports a resource type that is not in the enumeration.
// It exists so the registry guard is exercised by a test rather than trusted.
type unknownTypeAdapter struct{ DHCPScopeAdapter }

func (unknownTypeAdapter) Type() ResourceType { return "not_a_resource_type" }

func TestRegister_RefusesDuplicateAndUnknownTypes(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)

	if err := svc.Register(&unknownTypeAdapter{}); err == nil {
		t.Fatalf("registration of an unknown resource type was accepted")
	}
	if err := svc.Register(nil); err == nil {
		t.Fatalf("registration of a nil adapter was accepted")
	}

	if err := svc.Register(NewDHCPScopeAdapter()); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := svc.Register(NewDHCPScopeAdapter()); err == nil {
		t.Fatalf("duplicate registration was accepted; two adapters for one type means one Notify never runs")
	}

	types := svc.RegisteredTypes()
	if len(types) != 1 || types[0] != ResourceDHCPScope {
		t.Fatalf("registered types = %v, want [%s]", types, ResourceDHCPScope)
	}
}
