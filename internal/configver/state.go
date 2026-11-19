package configver

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// RecordStateRequest records the state a resource already has after an
// operator-facing write that applied the change through its own path.
type RecordStateRequest struct {
	ResourceType ResourceType
	ResourceID   string
	Actor        string
	Note         string
}

// RecordState writes a new persisted revision from the resource's current
// publishable content. It deliberately queues no release: the caller is a
// write path that has already applied and published the change through the
// resource's own mechanism. The revision still gives operators a durable,
// restorable record and makes the change visible in diffs.
//
// A missing revision is accepted: if the creation hook failed, the first
// recorded state is better than no history. A concurrent writer that inserts
// the same revision number is retried against the state that won, so the
// log always records what is actually stored.
func (s *Service) RecordState(req RecordStateRequest) (*Revision, error) {
	if !req.ResourceType.Valid() {
		return nil, fmt.Errorf("configver: unknown resource type %q", req.ResourceType)
	}
	if strings.TrimSpace(req.ResourceID) == "" {
		return nil, fmt.Errorf("configver: resource id is required")
	}
	adapter, ok := s.adapters[req.ResourceType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoAdapter, req.ResourceType)
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		rev, err := s.recordStateOnce(adapter, req)
		if err == nil {
			return rev, nil
		}
		var conflict *ConflictError
		if !asConflict(err, &conflict) {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("configver: record state after concurrent writes: %w", lastErr)
}

func (s *Service) recordStateOnce(adapter Adapter, req RecordStateRequest) (*Revision, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("configver: begin record state: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

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
	content, err := ReadContent(tx, req.ResourceType, req.ResourceID)
	if err != nil {
		return nil, fmt.Errorf("configver: read current state: %w", err)
	}
	canonical, hash, err := normaliseContent(content)
	if err != nil {
		return nil, &ValidationError{ResourceType: req.ResourceType, ResourceID: req.ResourceID, Reason: err.Error()}
	}
	if err := adapter.Validate(canonical); err != nil {
		return nil, &ValidationError{ResourceType: req.ResourceType, ResourceID: req.ResourceID, Reason: err.Error()}
	}
	note := req.Note
	if note == "" {
		note = "applied change"
	}
	rev := &Revision{
		ID: uuid.New().String(), ResourceType: req.ResourceType, ResourceID: req.ResourceID,
		Revision: current + 1, BaseRevision: current, Status: StatusPersisted,
		Content: canonical, ContentHash: hash, Actor: req.Actor, Note: note,
	}
	if err := insertRevision(tx, rev); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("configver: commit recorded state: %w", err)
	}
	return rev, nil
}

func asConflict(err error, target **ConflictError) bool {
	if c, ok := err.(*ConflictError); ok {
		*target = c
		return true
	}
	return false
}
