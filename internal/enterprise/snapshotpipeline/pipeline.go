// Package snapshotpipeline combines snapshot transport and NDJSON validation.
// It does not install a snapshot, authenticate a peer, or grant authority.
package snapshotpipeline

import (
	"errors"
	"fmt"
	"sync"

	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotrows"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotverify"
)

var ErrInvalid = errors.New("snapshotpipeline: invalid state or configuration")
var ErrDirty = errors.New("snapshotpipeline: pipeline permanently failed")

// SchemaTable comes from the local trusted schema, never from the manifest.
// ValidateRow is trusted caller code; if it performs I/O it may write only to
// disposable staging. This package cannot enforce or undo callback side effects.
// A callback must not reenter the same pipeline. Any error makes all staging
// unusable: the caller must discard it and must not commit or activate it.
type SchemaTable struct {
	Name        string
	ValidateRow snapshotrows.ValidateRow
}
type Result struct {
	Transport snapshotverify.TransportResult
	Rows      snapshotrows.Result
	// BusinessComplete is always false: row callbacks alone cannot establish all
	// database-wide invariants, source authority or installation readiness.
	BusinessComplete bool
}
type Pipeline struct {
	mu              sync.Mutex
	transport       *snapshotverify.Verifier
	rows            *snapshotrows.Validator
	dirty, finished bool
}

// New requires the manifest to list every table in the exact local schema order,
// including zero-count tables. All expected counts come from the manifest, not
// caller-supplied row totals. The whitelist is derived solely from local schema.
func New(m snapshotverify.Manifest, trust snapshotverify.Trust, transportLimits snapshotverify.Limits, rowLimits snapshotrows.Limits, schema []SchemaTable, upperMS int64) (*Pipeline, error) {
	if len(schema) == 0 || len(schema) != len(m.Tables) || len(schema) > snapshotrows.MaxTables {
		return nil, ErrInvalid
	}
	tables := make([]snapshotrows.Table, len(schema))
	allowed := make(map[string]bool, len(schema))
	for i, table := range schema {
		if table.Name == "" || allowed[table.Name] || m.Tables[i].Name != table.Name {
			return nil, ErrInvalid
		}
		allowed[table.Name] = true
		tables[i] = snapshotrows.Table{Name: table.Name, Count: m.Tables[i].Rows, ValidateRow: table.ValidateRow}
	}
	transportLimits.Tables = allowed
	transport, err := snapshotverify.New(m, trust, transportLimits, upperMS)
	if err != nil {
		return nil, err
	}
	if rowLimits.TotalBytes < m.TotalBytes {
		return nil, ErrInvalid
	}
	rows, err := snapshotrows.New(tables, rowLimits)
	if err != nil {
		return nil, err
	}
	return &Pipeline{transport: transport, rows: rows}, nil
}

// Feed verifies identity-bound index, time, size and hash before the independent
// chunk copy is parsed or passed to callbacks. Callback work is provisional even
// on success and must be discarded if any later Feed or Finish fails.
func (p *Pipeline) Feed(index uint32, raw []byte, upperMS int64) (err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	completed := false
	defer func() {
		if !completed {
			_ = recover()
			p.dirty = true
			err = ErrInvalid
		}
	}()
	err = func() error {
		if p.dirty {
			return ErrDirty
		}
		if p.finished {
			p.dirty = true
			return ErrInvalid
		}
		verified, err := p.transport.Feed(index, raw, upperMS)
		if err != nil {
			p.dirty = true
			return fmt.Errorf("snapshotpipeline: transport: %w", err)
		}
		if err = p.rows.Feed(verified); err != nil {
			p.dirty = true
			return fmt.Errorf("snapshotpipeline: rows: %w", err)
		}
		return nil
	}()
	completed = true
	return err
}

// Finish confirms transport, structure and declared counts only. It never
// commits staging or establishes business, database, or role readiness.
func (p *Pipeline) Finish(upperMS int64) (Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dirty {
		return Result{}, ErrDirty
	}
	if p.finished {
		p.dirty = true
		return Result{}, ErrInvalid
	}
	transport, err := p.transport.VerifyComplete(upperMS)
	if err != nil {
		p.dirty = true
		return Result{}, fmt.Errorf("snapshotpipeline: transport: %w", err)
	}
	rows, err := p.rows.Finish()
	if err != nil {
		p.dirty = true
		return Result{}, fmt.Errorf("snapshotpipeline: rows: %w", err)
	}
	if !rows.StructureAndCountsValid || rows.Bytes != transport.Bytes || rows.Rows != transport.DeclaredRows {
		p.dirty = true
		return Result{}, ErrInvalid
	}
	p.finished = true
	return Result{Transport: transport, Rows: rows, BusinessComplete: false}, nil
}
