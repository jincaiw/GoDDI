package configver

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// Queryer is the read half of database/sql that an adapter needs.
//
// Both *sql.DB and *sql.Tx satisfy it. Adapters are handed a *sql.Tx while
// applying, and this interface exists so existence checks and reads can be
// issued against either without the adapter knowing which.
type Queryer interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

// Adapter teaches configver how to validate, write and announce one resource
// type. Everything resource-specific lives here; the service only sequences
// the steps, so adding a resource type cannot accidentally skip publishing.
type Adapter interface {
	// Type is the registry key. It must be stable: it is persisted in every
	// revision row.
	Type() ResourceType

	// Validate decides whether content is acceptable. It must not touch the
	// database -- a rejected publish has to fail without taking the write
	// lock, and without leaving a half-written revision behind.
	Validate(content json.RawMessage) error

	// Exists reports whether the resource being published to is present.
	// Publishing to a deleted resource would create a revision that can never
	// be applied, so it is refused up front.
	Exists(q Queryer, id string) (bool, error)

	// Apply writes content into the resource. It runs inside a transaction
	// supplied by the caller and MUST issue every statement through that tx.
	// (The pool is capped at a single connection for SQLite; a statement sent
	// to the *sql.DB while the transaction holds the connection blocks
	// forever rather than failing.)
	//
	// Apply must be idempotent: a release is retried after a failure, and the
	// retry re-applies before re-announcing.
	Apply(tx *sql.Tx, id string, content json.RawMessage) error

	// Notify tells the data plane to serve the new configuration. It runs
	// after the transaction commits, so it may touch in-memory state that
	// cannot participate in a transaction.
	Notify(id string) error
}

// Register adds an adapter. Registering the same type twice is refused rather
// than silently replacing: two adapters for one type means one of them is dead
// code whose Notify never runs, which is exactly the silent-failure class this
// package exists to prevent.
func (s *Service) Register(a Adapter) error {
	if a == nil {
		return fmt.Errorf("configver: nil adapter")
	}
	rt := a.Type()
	if !rt.Valid() {
		return fmt.Errorf("configver: unknown resource type %q", rt)
	}
	if s.adapters == nil {
		s.adapters = make(map[ResourceType]Adapter)
	}
	if _, dup := s.adapters[rt]; dup {
		return fmt.Errorf("configver: adapter already registered for %q", rt)
	}
	s.adapters[rt] = a
	return nil
}

// RegisteredTypes returns the resource types that can be published, sorted for
// stable output. Exposed so the API can report which resources are governed.
func (s *Service) RegisteredTypes() []ResourceType {
	out := make([]ResourceType, 0, len(s.adapters))
	for rt := range s.adapters {
		out = append(out, rt)
	}
	// Small set; insertion sort keeps the import list short and the order
	// deterministic without pulling in sort.Slice for three items.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
