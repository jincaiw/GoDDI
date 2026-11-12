package lease

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrWALPipelineConfig = errors.New("lease wal pipeline: invalid config")
	ErrWALPipelineClosed = errors.New("lease wal pipeline: closed")
)

// WALPipeline is the single opt-in assembly of the event-sourced lease write
// path. It exists to fix, in one place, the ordering N5 depends on:
//
//  1. open the WAL;
//  2. replay every durable event newer than the SQLite projection watermark
//     *before* any lease is admitted (recovery is fail-closed);
//  3. start the ordered asynchronous projection applier at that watermark;
//  4. hand out a write bridge whose admissions cross the WAL durable boundary
//     and are then projected, and a lifecycle that owns the shutdown cutoff.
//
// The default DHCP process never constructs this component (it keeps the
// SQLite-backed *Manager). This type only makes the pieces that already exist
// (OpenWAL, RecoverSQLiteProjection, NewSQLiteProjectionApplier,
// NewWALLifecycle, MemoryWriteBridge) compose into one Start/Stop unit, so the
// assembly contract can be exercised by tests instead of described in prose.
type WALPipeline struct {
	wal       *WALFile
	applier   *AsyncApplier
	lifecycle *WALLifecycle
	index     *MemoryIndex
	bridge    *MemoryWriteBridge
}

// WALPipelineConfig configures the assembly.
type WALPipelineConfig struct {
	// Path is the WAL file. It is opened or created with owner-only perms.
	Path string
	// DB is the SQLite projection database. Required.
	DB *sql.DB
	// Capacity bounds the asynchronous projection queue. Required (>0).
	Capacity int
	// NextOp builds the event shape for a lease value. Required.
	NextOp func(*Lease) WALEvent
	// Apply overrides the default SQLite projection applier. When nil the
	// pipeline uses NewSQLiteProjectionApplier (lease snapshot only). A store
	// that must also enqueue a fact envelope in the same projection
	// transaction supplies its own applier here.
	Apply ApplyFunc
}

// lifecycleSink adapts WALLifecycle to the MemoryWriteBridge durable boundary.
// AppendDurable already appends to the WAL *and* submits the canonical event to
// the applier under the lifecycle's admission lock, so the bridge's separate
// submitter becomes a no-op.
type lifecycleSink struct{ lifecycle *WALLifecycle }

func (s lifecycleSink) DurableEvent(_ context.Context, event WALEvent) (WALEvent, error) {
	return s.lifecycle.AppendDurable(event)
}

type noopSubmitter struct{}

func (noopSubmitter) Submit(WALEvent) error { return nil }

// OpenWALPipeline assembles and starts the pipeline. Recovery runs before the
// projection applier starts, so a caller may treat a nil error as "the
// projection is caught up and admission may begin".
func OpenWALPipeline(ctx context.Context, cfg WALPipelineConfig) (*WALPipeline, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is nil", ErrWALPipelineConfig)
	}
	if cfg.Path == "" {
		return nil, fmt.Errorf("%w: WAL path is empty", ErrWALPipelineConfig)
	}
	if cfg.DB == nil {
		return nil, fmt.Errorf("%w: database is nil", ErrWALPipelineConfig)
	}
	if cfg.Capacity <= 0 {
		return nil, fmt.Errorf("%w: capacity must be positive", ErrWALPipelineConfig)
	}
	if cfg.NextOp == nil {
		return nil, fmt.Errorf("%w: event builder is nil", ErrWALPipelineConfig)
	}

	wal, err := OpenWAL(cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("open WAL: %w", err)
	}
	// Recovery is fail-closed and synchronous: a failure leaves the pipeline
	// unbuilt so the caller keeps the data plane unready. When a custom applier
	// is supplied it must drive recovery too, so facts derived from unprojected
	// durable events are enqueued exactly like the live path (a crash that left
	// a fact owed must not lose it).
	if cfg.Apply != nil {
		if err := EnsureAppliedWatermark(ctx, cfg.DB); err != nil {
			_ = wal.Close()
			return nil, fmt.Errorf("recover projection: %w", err)
		}
		tx, rerr := cfg.DB.BeginTx(ctx, nil)
		if rerr != nil {
			_ = wal.Close()
			return nil, fmt.Errorf("recover projection: %w", rerr)
		}
		wm, rerr := ReadAppliedWatermark(ctx, tx)
		_ = tx.Rollback()
		if rerr != nil {
			_ = wal.Close()
			return nil, fmt.Errorf("recover projection: %w", rerr)
		}
		if _, rerr := wal.Replay(wm.Applied, func(e WALEvent) error { return cfg.Apply(ctx, e) }); rerr != nil {
			_ = wal.Close()
			return nil, fmt.Errorf("recover projection: %w", rerr)
		}
	} else if _, err := RecoverSQLiteProjection(ctx, wal, cfg.DB); err != nil {
		_ = wal.Close()
		return nil, fmt.Errorf("recover projection: %w", err)
	}
	var applier *AsyncApplier
	if cfg.Apply != nil {
		if err := EnsureAppliedWatermark(ctx, cfg.DB); err != nil {
			_ = wal.Close()
			return nil, fmt.Errorf("ensure projection watermark: %w", err)
		}
		tx, err := cfg.DB.BeginTx(ctx, nil)
		if err != nil {
			_ = wal.Close()
			return nil, fmt.Errorf("read projection watermark: %w", err)
		}
		wm, rerr := ReadAppliedWatermark(ctx, tx)
		_ = tx.Rollback()
		if rerr != nil {
			_ = wal.Close()
			return nil, fmt.Errorf("read projection watermark: %w", rerr)
		}
		applier, err = NewAsyncApplier(ctx, cfg.Capacity, wm.Applied, cfg.Apply)
		if err != nil {
			_ = wal.Close()
			return nil, fmt.Errorf("start projection applier: %w", err)
		}
	} else {
		applier, err = NewSQLiteProjectionApplier(ctx, cfg.DB, cfg.Capacity)
		if err != nil {
			_ = wal.Close()
			return nil, fmt.Errorf("start projection applier: %w", err)
		}
	}
	lifecycle, err := NewWALLifecycle(wal, applier)
	if err != nil {
		_ = applier.Close()
		_ = wal.Close()
		return nil, fmt.Errorf("build lifecycle: %w", err)
	}
	index := NewMemoryIndex()
	bridge, err := NewMemoryWriteBridge(index, lifecycleSink{lifecycle: lifecycle}, noopSubmitter{}, cfg.NextOp)
	if err != nil {
		_ = lifecycle.Close(context.Background())
		return nil, fmt.Errorf("build write bridge: %w", err)
	}
	return &WALPipeline{wal: wal, applier: applier, lifecycle: lifecycle, index: index, bridge: bridge}, nil
}

// Admit crosses the durable boundary for one lease value: memory is updated
// first, the event is appended and synced to the WAL, then projected
// asynchronously. A durability failure restores the previous memory value and
// submits nothing, matching MemoryWriteBridge's contract.
func (p *WALPipeline) Admit(ctx context.Context, value Lease) error {
	if p == nil || p.bridge == nil {
		return ErrWALPipelineClosed
	}
	if err := p.bridge.Commit(ctx, value); err != nil {
		return err
	}
	return nil
}

// admitEvent is the store's admission entry: it applies the already-built
// event to memory, crosses the WAL durable boundary, and projects. It mirrors
// MemoryWriteBridge.Commit but accepts a full event (with mutation/before/space)
// so facts can be derived from the durable record.
func (p *WALPipeline) admitEvent(ctx context.Context, event WALEvent) error {
	if p == nil || p.index == nil || p.lifecycle == nil {
		return ErrWALPipelineClosed
	}
	if event.Lease == nil || event.Lease.ID == "" {
		return ErrWALCorrupt
	}
	previous, existed := p.index.Get(event.Lease.ID)
	p.index.Upsert(*event.Lease)
	canonical, err := p.lifecycle.AppendDurable(event)
	if err != nil {
		if existed && previous != nil {
			p.index.Upsert(*previous)
		} else {
			p.index.Remove(event.Lease.ID)
		}
		return err
	}
	_ = canonical
	return nil
}

// Index exposes the read index the pipeline keeps in step with the write path.
func (p *WALPipeline) Index() *MemoryIndex {
	if p == nil {
		return nil
	}
	return p.index
}

// Applied is the last sequence the SQLite projection has durably applied.
func (p *WALPipeline) Applied() int64 {
	if p == nil || p.applier == nil {
		return 0
	}
	return p.applier.Applied()
}

// Drain waits until every event admitted before the call has been projected to
// SQLite. It is the read-your-writes barrier a store needs when a caller wants
// the projection caught up (tests, canary verification); the hot path itself
// never blocks on it.
func (p *WALPipeline) Drain(ctx context.Context) error {
	if p == nil || p.applier == nil {
		return ErrWALPipelineClosed
	}
	return p.applier.Wait(ctx)
}

// Close rejects new admissions, syncs the WAL, drains accepted projection work,
// syncs again, and only then closes the WAL. On a cancelled or timed-out ctx it
// returns the context error and leaves the durable resources open so a later
// startup replays the unprojected events.
func (p *WALPipeline) Close(ctx context.Context) error {
	if p == nil || p.lifecycle == nil {
		return ErrWALPipelineClosed
	}
	return p.lifecycle.Close(ctx)
}
