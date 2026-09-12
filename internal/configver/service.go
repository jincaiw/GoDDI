package configver

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ErrResourceNotFound means the resource named by a publish does not exist.
//
// It is separate from ErrNotFound (a missing revision) because the remedy is
// different: one means "create the resource first", the other "pick a revision
// that exists".
var ErrResourceNotFound = errors.New("configver: resource not found")

// Service is the entry point for reading and publishing configuration
// revisions. It is safe for concurrent use; the underlying database is the
// only serialisation point.
type Service struct {
	db       *sql.DB
	adapters map[ResourceType]Adapter
}

// NewService creates a service backed by db. Register adapters before
// publishing anything; an unregistered type is refused rather than written.
func NewService(db *sql.DB) *Service {
	return &Service{db: db, adapters: make(map[ResourceType]Adapter)}
}

// DB exposes the underlying handle for callers that need to run maintenance
// queries (outbox stats). It is not used for publishing.
func (s *Service) DB() *sql.DB { return s.db }

// PublishRequest is a request to publish (or preview) a configuration.
type PublishRequest struct {
	ResourceType ResourceType
	ResourceID   string
	Content      json.RawMessage

	// ExpectedRevision is the revision the caller based its change on.
	// 0 means "this resource has never been published". A mismatch is
	// rejected with *ConflictError; it is never silently rebased, because a
	// silent rebase would apply a change designed for a different base.
	ExpectedRevision int64

	// IdempotencyKey makes a retried publish safe. When a revision with the
	// same key already exists, it is returned instead of creating a second
	// one -- so a client that retries after a timeout cannot produce two
	// revisions for one intended change.
	IdempotencyKey string

	Actor string
	Note  string

	// DryRun validates and records the content for review without queuing a
	// release. The resource is left untouched.
	DryRun bool
}

// PublishResult is the outcome of Publish.
type PublishResult struct {
	// Revision is nil for a dry run, which stores nothing.
	Revision *Revision `json:"revision,omitempty"`
	// Changes is the diff against the revision this publish was based on.
	// For a first publish it lists every field as added.
	Changes []FieldChange `json:"changes"`
	// BaseRevision is the revision the change was computed against, echoed so
	// a dry run can be turned into a real publish without re-reading it.
	BaseRevision int64 `json:"base_revision"`
	// Replayed is true when an idempotency key matched an existing revision
	// and nothing new was created.
	Replayed bool `json:"replayed"`
	DryRun   bool `json:"dry_run"`
}

// normaliseContent canonicalises content and returns its hash.
//
// The stored bytes are re-encoded from the decoded value so that two requests
// carrying the same object with different key order or whitespace produce the
// same hash. Without this, an idempotent retry could look like a different
// configuration, and "did anything change" would depend on formatting.
func normaliseContent(content json.RawMessage) (json.RawMessage, string, error) {
	if len(content) == 0 {
		content = json.RawMessage(`{}`)
	}
	obj, err := decodeObject(content)
	if err != nil {
		return nil, "", fmt.Errorf("content must be a JSON object: %w", err)
	}
	canonical, err := json.Marshal(obj)
	if err != nil {
		return nil, "", fmt.Errorf("re-encode content: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(sum[:]), nil
}

// Publish validates content, records it as a revision, and (unless dry-run)
// queues a release for the data plane.
//
// The revision row and the release event are written in one transaction. That
// is the whole point: a configuration that is durable but whose reload is not
// recorded would survive a restart looking correct while the data plane served
// the old value.
func (s *Service) Publish(req PublishRequest) (*PublishResult, error) {
	if !req.ResourceType.Valid() {
		return nil, fmt.Errorf("configver: unknown resource type %q", req.ResourceType)
	}
	if strings.TrimSpace(req.ResourceID) == "" {
		return nil, fmt.Errorf("configver: resource id is required")
	}
	if req.ExpectedRevision < 0 {
		return nil, fmt.Errorf("configver: expected revision must not be negative")
	}
	adapter, ok := s.adapters[req.ResourceType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoAdapter, req.ResourceType)
	}

	content, hash, err := normaliseContent(req.Content)
	if err != nil {
		return nil, &ValidationError{
			ResourceType: req.ResourceType,
			ResourceID:   req.ResourceID,
			Reason:       err.Error(),
		}
	}

	// Validate before opening the transaction. Validation is pure, and a
	// rejected publish must not hold the single SQLite connection.
	if err := adapter.Validate(content); err != nil {
		return nil, &ValidationError{
			ResourceType: req.ResourceType,
			ResourceID:   req.ResourceID,
			Reason:       err.Error(),
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("configver: begin publish: %w", err)
	}
	// Rollback is a no-op after a successful commit.
	defer func() { _ = tx.Rollback() }()

	// Idempotent replay. Checked first so a retry never trips the concurrency
	// check with a revision number that this very retry was supposed to have
	// created.
	if req.IdempotencyKey != "" {
		existing, err := revisionByIdempotencyKey(tx, req.IdempotencyKey)
		switch {
		case err == nil:
			if existing.ResourceType != req.ResourceType || existing.ResourceID != req.ResourceID {
				return nil, &ConflictError{
					ResourceType:     req.ResourceType,
					ResourceID:       req.ResourceID,
					ExpectedRevision: req.ExpectedRevision,
					ActualRevision:   existing.Revision,
				}
			}
			changes, err := s.diffAgainstBase(tx, existing)
			if err != nil {
				return nil, err
			}
			return &PublishResult{
				Revision:     existing,
				Changes:      changes,
				BaseRevision: existing.BaseRevision,
				Replayed:     true,
			}, nil
		case errors.Is(err, ErrNotFound):
			// No prior attempt; continue.
		default:
			return nil, err
		}
	}

	exists, err := adapter.Exists(tx, req.ResourceID)
	if err != nil {
		return nil, fmt.Errorf("configver: check resource: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: %s %s", ErrResourceNotFound, req.ResourceType, req.ResourceID)
	}

	current, err := currentRevision(tx, req.ResourceType, req.ResourceID)
	if err != nil {
		return nil, err
	}
	if current != req.ExpectedRevision {
		return nil, &ConflictError{
			ResourceType:     req.ResourceType,
			ResourceID:       req.ResourceID,
			ExpectedRevision: req.ExpectedRevision,
			ActualRevision:   current,
		}
	}

	changes, err := s.diffFromRevision(tx, req.ResourceType, req.ResourceID, current, content)
	if err != nil {
		return nil, err
	}

	// A dry run stops here: the content is valid and the diff is known, and
	// nothing is written. Storing a preview would consume a revision number
	// that the concurrency baseline does not count, so the next real publish
	// would collide with it.
	if req.DryRun {
		return &PublishResult{Changes: changes, BaseRevision: current, DryRun: true}, nil
	}

	rev := &Revision{
		ID:             uuid.New().String(),
		ResourceType:   req.ResourceType,
		ResourceID:     req.ResourceID,
		Revision:       current + 1,
		BaseRevision:   current,
		ContentHash:    hash,
		IdempotencyKey: req.IdempotencyKey,
		Actor:          req.Actor,
		Note:           req.Note,
		Content:        content,
	}

	// `persisted` and `staged` are both reached inside this transaction. The
	// stored status is the further one: the content is durable AND the release
	// is queued, or neither.
	rev.Status = StatusStaged
	if err := insertRevision(tx, rev); err != nil {
		return nil, err
	}
	if err := insertRelease(tx, rev); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("configver: commit publish: %w", err)
	}

	return &PublishResult{Revision: rev, Changes: changes, BaseRevision: current}, nil
}

// RollbackRequest asks for an earlier revision's content to be published again.
type RollbackRequest struct {
	ResourceType ResourceType
	ResourceID   string
	// ToRevision is the revision whose content should be restored.
	ToRevision int64
	// ExpectedRevision guards the rollback itself against a concurrent
	// publish, exactly like PublishRequest.ExpectedRevision.
	ExpectedRevision int64
	IdempotencyKey   string
	Actor            string
	Note             string
}

// Rollback republishes the content of an earlier revision.
//
// It deliberately does not rewind the log. The result is a NEW revision whose
// content equals the old one, so the history still shows that the reverted
// configuration was live and for how long. Rewriting history would erase the
// evidence an operator needs while investigating, and would make the revision
// numbers that other clients hold as their expected_revision meaningless.
func (s *Service) Rollback(req RollbackRequest) (*PublishResult, error) {
	if req.ToRevision <= 0 {
		return nil, fmt.Errorf("%w: revision must be positive, got %d", ErrInvalidRevision, req.ToRevision)
	}

	target, err := s.GetByNumber(req.ResourceType, req.ResourceID, req.ToRevision)
	if err != nil {
		return nil, err
	}

	// Only a revision that actually reached the resource can be rolled back
	// to. A failed release was never serving traffic, so "restoring" it would
	// publish a configuration nobody ever ran.
	if !target.Status.restorable() {
		return nil, fmt.Errorf(
			"%w: revision %d is %s and was never serving traffic",
			ErrInvalidRevision, req.ToRevision, target.Status)
	}

	note := req.Note
	if note == "" {
		note = fmt.Sprintf("rollback to revision %d", req.ToRevision)
	}

	return s.Publish(PublishRequest{
		ResourceType:     req.ResourceType,
		ResourceID:       req.ResourceID,
		Content:          target.Content,
		ExpectedRevision: req.ExpectedRevision,
		IdempotencyKey:   req.IdempotencyKey,
		Actor:            req.Actor,
		Note:             note,
	})
}

// Get returns a revision by its id.
func (s *Service) Get(id string) (*Revision, error) {
	return scanRevision(s.db.QueryRow(revisionSelect+` WHERE id = ?`, id))
}

// GetByNumber returns a specific revision number of a resource.
func (s *Service) GetByNumber(rt ResourceType, id string, revision int64) (*Revision, error) {
	return scanRevision(s.db.QueryRow(
		revisionSelect+` WHERE resource_type = ? AND resource_id = ? AND revision = ?`,
		rt, id, revision))
}

// CurrentRevision returns the newest released revision number, or 0 when the
// resource has never been published.
func (s *Service) CurrentRevision(rt ResourceType, id string) (int64, error) {
	return currentRevision(s.db, rt, id)
}

// Current returns the newest released revision, or ErrNotFound.
func (s *Service) Current(rt ResourceType, id string) (*Revision, error) {
	n, err := currentRevision(s.db, rt, id)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, fmt.Errorf("%w: %s %s has no published revision", ErrNotFound, rt, id)
	}
	return s.GetByNumber(rt, id, n)
}

// ListFilter narrows a revision listing.
type ListFilter struct {
	ResourceType ResourceType
	ResourceID   string
	Status       Status
	Page         int
	PageSize     int
}

// List returns revisions newest first, with the total count for pagination.
func (s *Service) List(f ListFilter) ([]Revision, int64, error) {
	var where []string
	var args []any

	if f.ResourceType != "" {
		where = append(where, "resource_type = ?")
		args = append(args, f.ResourceType)
	}
	if f.ResourceID != "" {
		where = append(where, "resource_id = ?")
		args = append(args, f.ResourceID)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	if err := s.db.QueryRow("SELECT COUNT(*) FROM config_revisions"+clause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("configver: count revisions: %w", err)
	}

	page, pageSize := f.Page, f.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	offset := (page - 1) * pageSize

	rows, err := s.db.Query(
		revisionSelect+clause+" ORDER BY resource_type, resource_id, revision DESC LIMIT ? OFFSET ?",
		append(args, pageSize, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("configver: list revisions: %w", err)
	}
	defer rows.Close()

	var out []Revision
	for rows.Next() {
		rev, err := scanRevisionRows(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *rev)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// DiffRevisions compares two revisions of the same resource.
func (s *Service) DiffRevisions(rt ResourceType, id string, from, to int64) ([]FieldChange, error) {
	oldRev, err := s.GetByNumber(rt, id, from)
	if err != nil {
		return nil, err
	}
	newRev, err := s.GetByNumber(rt, id, to)
	if err != nil {
		return nil, err
	}
	return Diff(oldRev.Content, newRev.Content)
}

// diffAgainstBase diffs a revision's content against the content of the
// revision it was based on.
func (s *Service) diffAgainstBase(q Queryer, rev *Revision) ([]FieldChange, error) {
	return s.diffFromRevision(q, rev.ResourceType, rev.ResourceID, rev.BaseRevision, rev.Content)
}

func (s *Service) diffFromRevision(q Queryer, rt ResourceType, id string, base int64, content json.RawMessage) ([]FieldChange, error) {
	if base <= 0 {
		// First publish: every field is new.
		return Diff(nil, content)
	}
	baseRev, err := scanRevision(q.QueryRow(
		revisionSelect+` WHERE resource_type = ? AND resource_id = ? AND revision = ?`,
		rt, id, base))
	if err != nil {
		return nil, err
	}
	return Diff(baseRev.Content, content)
}

// revisionSelect is the shared column list. Kept in one place so the scanner
// below cannot drift out of step with it.
const revisionSelect = `SELECT id, resource_type, resource_id, revision, status,
	content, content_hash, base_revision, idempotency_key, actor, note, error,
	applied_generation, created_at, updated_at, COALESCE(applied_at, '')
	FROM config_revisions`

// rowScanner is the subset shared by *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanRevision(row *sql.Row) (*Revision, error) {
	rev, err := scanRevisionRows(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	return rev, err
}

func scanRevisionRows(row rowScanner) (*Revision, error) {
	var rev Revision
	var content string
	var idem, actor, note, errText, appliedAt sql.NullString

	if err := row.Scan(
		&rev.ID, &rev.ResourceType, &rev.ResourceID, &rev.Revision, &rev.Status,
		&content, &rev.ContentHash, &rev.BaseRevision, &idem, &actor, &note, &errText,
		&rev.AppliedGeneration, &rev.CreatedAt, &rev.UpdatedAt, &appliedAt,
	); err != nil {
		return nil, err
	}

	rev.Content = json.RawMessage(content)
	rev.IdempotencyKey = idem.String
	rev.Actor = actor.String
	rev.Note = note.String
	rev.Error = errText.String
	rev.AppliedAt = appliedAt.String
	return &rev, nil
}

// currentRevision returns the newest revision number for a resource.
//
// Every stored row counts, because every stored row holds a revision number:
// the unique index is on (resource_type, resource_id, revision), so a row that
// were excluded here would still be collided with on the next insert.
func currentRevision(q Queryer, rt ResourceType, id string) (int64, error) {
	var current sql.NullInt64
	err := q.QueryRow(`SELECT MAX(revision) FROM config_revisions
		WHERE resource_type = ? AND resource_id = ?`, rt, id).Scan(&current)
	if err != nil {
		return 0, fmt.Errorf("configver: read current revision: %w", err)
	}
	if !current.Valid {
		return 0, nil
	}
	return current.Int64, nil
}

func revisionByIdempotencyKey(q Queryer, key string) (*Revision, error) {
	return scanRevision(q.QueryRow(revisionSelect+` WHERE idempotency_key = ?`, key))
}

func insertRevision(tx *sql.Tx, rev *Revision) error {
	_, err := tx.Exec(`INSERT INTO config_revisions
		(id, resource_type, resource_id, revision, status, content, content_hash,
		 base_revision, idempotency_key, actor, note, error, applied_generation)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 0)`,
		rev.ID, rev.ResourceType, rev.ResourceID, rev.Revision, rev.Status,
		string(rev.Content), rev.ContentHash, rev.BaseRevision,
		nullIfEmpty(rev.IdempotencyKey), rev.Actor, rev.Note)
	if err != nil {
		// A unique-index violation here means another writer inserted the same
		// revision number between our read and our write. Translate it into the
		// concurrency error the caller already knows how to handle, instead of
		// leaking a driver-specific constraint message.
		if isUniqueViolation(err) {
			return &ConflictError{
				ResourceType:     rev.ResourceType,
				ResourceID:       rev.ResourceID,
				ExpectedRevision: rev.BaseRevision,
				ActualRevision:   rev.Revision,
			}
		}
		return fmt.Errorf("configver: insert revision: %w", err)
	}
	return nil
}

func insertRelease(tx *sql.Tx, rev *Revision) error {
	_, err := tx.Exec(`INSERT INTO config_release_outbox
		(revision_id, resource_type, resource_id, revision, status)
		VALUES (?, ?, ?, ?, 'pending')`,
		rev.ID, rev.ResourceType, rev.ResourceID, rev.Revision)
	if err != nil {
		return fmt.Errorf("configver: queue release: %w", err)
	}
	return nil
}

// setRevisionStatus moves a revision through the state machine, refusing an
// illegal transition. Returning an error instead of writing an impossible row
// is what keeps the state machine meaningful rather than decorative.
func setRevisionStatus(tx *sql.Tx, id string, from, to Status, errText string) error {
	if from != to && !from.CanTransitionTo(to) {
		return fmt.Errorf("configver: illegal revision transition %s -> %s", from, to)
	}
	if to == StatusApplied {
		_, err := tx.Exec(`UPDATE config_revisions
			SET status = ?, error = ?, applied_at = datetime('now'), updated_at = datetime('now'),
			    applied_generation = revision
			WHERE id = ?`, to, errText, id)
		return err
	}
	_, err := tx.Exec(`UPDATE config_revisions
		SET status = ?, error = ?, updated_at = datetime('now')
		WHERE id = ?`, to, errText, id)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// isUniqueViolation reports whether err is a SQLite uniqueness violation.
//
// The driver does not export a typed error for this, so the message is
// inspected. Constraint names are matched too, so a future unrelated unique
// index does not get misreported as a publish conflict.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if !strings.Contains(msg, "UNIQUE constraint failed") {
		return false
	}
	return strings.Contains(msg, "idx_config_revisions_version") ||
		strings.Contains(msg, "config_revisions.resource_type")
}
