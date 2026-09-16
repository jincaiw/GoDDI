package ipam

import (
	"context"
	"errors"
	"sync"

	"github.com/jasonwa/goddi/internal/facts"
)

// FactsPipelineOptions is the explicit rollout switch for the migration-stage
// unified facts projection. Disabled is the safe default: no consumer is
// constructed, started, or wired into the process.
type FactsPipelineOptions struct {
	Enabled         bool
	ConsumerOptions FactsConsumerOptions
	// BeforeStart is an optional producer-side readiness gate. The caller uses
	// it to finish lease/WAL recovery before the control-side consumer starts.
	// A non-nil error keeps the pipeline unstarted and fail-closed.
	BeforeStart func(context.Context) error
}

// FactsPipeline is the opt-in assembly boundary for the unified facts
// projection. Producer-side lease mutation wiring remains owned by the caller;
// this object owns only the control-side consumer lifecycle and its rollback
// boundary. It is intentionally not used by the default GoDDI process.
type FactsPipeline struct {
	enabled         bool
	linkage         *Linkage
	outbox          *facts.ObservationOutbox
	consumerOptions FactsConsumerOptions
	consumer        *FactsConsumer
	consumerUsed    bool
	beforeStart     func(context.Context) error
	startMu         sync.Mutex
	started         bool
}

// FactsPipelineStatus reports whether the migration pipeline is enabled and,
// when enabled, the consumer's durable lifecycle/backlog state.
type FactsPipelineStatus struct {
	Enabled  bool
	Consumer FactsConsumerStatus
}

// NewFactsPipeline constructs the migration assembly. Disabled construction is
// a deliberate no-op and does not validate or touch consumer dependencies;
// callers can keep the legacy path assembled without unified-facts tables.
func NewFactsPipeline(linkage *Linkage, outbox *facts.ObservationOutbox, options FactsPipelineOptions) (*FactsPipeline, error) {
	pipeline := &FactsPipeline{
		enabled:         options.Enabled,
		linkage:         linkage,
		outbox:          outbox,
		consumerOptions: options.ConsumerOptions,
		beforeStart:     options.BeforeStart,
	}
	if !options.Enabled {
		return pipeline, nil
	}
	consumer, err := NewFactsConsumerWithOptions(linkage, outbox, options.ConsumerOptions)
	if err != nil {
		return nil, err
	}
	pipeline.consumer = consumer
	return pipeline, nil
}

// Enabled reports whether this explicit migration assembly was requested.
func (p *FactsPipeline) Enabled() bool {
	return p != nil && p.enabled
}

// Start starts the consumer only after the caller has completed producer-side
// startup/recovery checks. A disabled pipeline is a safe no-op.
func (p *FactsPipeline) Start(ctx context.Context) error {
	if p == nil {
		return errors.New("ipam facts pipeline: nil pipeline")
	}
	if !p.enabled {
		return nil
	}
	if ctx == nil {
		return errors.New("ipam facts pipeline: nil start context")
	}
	p.startMu.Lock()
	defer p.startMu.Unlock()
	if p.started {
		return errors.New("ipam facts pipeline: already started")
	}
	if p.beforeStart != nil {
		if err := p.beforeStart(ctx); err != nil {
			return err
		}
	}
	if p.consumerUsed {
		consumer, err := NewFactsConsumerWithOptions(p.linkage, p.outbox, p.consumerOptions)
		if err != nil {
			return err
		}
		p.consumer = consumer
	}
	if err := p.consumer.Start(ctx); err != nil {
		return err
	}
	p.consumerUsed = true
	p.started = true
	return nil
}

// Stop is the rollback/shutdown boundary. It first stops new consumer replay
// and waits for any in-flight projection transaction; a timeout is returned and
// never converted into a successful drain claim. A disabled pipeline is a
// safe no-op.
func (p *FactsPipeline) Stop(ctx context.Context) error {
	if p == nil {
		return errors.New("ipam facts pipeline: nil pipeline")
	}
	if !p.enabled {
		return nil
	}
	if ctx == nil {
		return errors.New("ipam facts pipeline: nil stop context")
	}
	p.startMu.Lock()
	defer p.startMu.Unlock()
	if !p.started {
		return nil
	}
	err := p.consumer.Stop(ctx)
	if errors.Is(err, ErrFactsConsumerFailed) {
		// The failed runner has already exited. Clear only the wrapper state so
		// the next Start can rebuild a fresh consumer after the durable fault is
		// repaired; a timeout/cancellation keeps the active runner owned here.
		p.started = false
	}
	if err != nil {
		return err
	}
	p.started = false
	return nil
}

// Wake requests prompt replay after the producer commits a durable event.
// Producers may call it after commit; it never blocks and is a no-op when the
// migration assembly is disabled.
func (p *FactsPipeline) Wake() {
	if p == nil || !p.enabled {
		return
	}
	p.consumer.Wake()
}

// Status returns the durable consumer status. Disabled pipelines report the
// explicit unconfigured state rather than pretending that legacy mode is
// healthy unified-facts projection.
func (p *FactsPipeline) Status(ctx context.Context) (FactsPipelineStatus, error) {
	if p == nil {
		return FactsPipelineStatus{}, errors.New("ipam facts pipeline: nil pipeline")
	}
	if !p.enabled {
		return FactsPipelineStatus{Enabled: false, Consumer: FactsConsumerStatus{State: FactsConsumerIdle}}, nil
	}
	status, err := p.consumer.Status(ctx)
	return FactsPipelineStatus{Enabled: true, Consumer: status}, err
}

// FactsPipelineLifecycle is the explicit rollout lifecycle surface. It keeps
// the pipeline wrapper's enabled/status envelope separate from the raw
// FactsConsumerLifecycle contract.
type FactsPipelineLifecycle interface {
	Start(context.Context) error
	Wake()
	Stop(context.Context) error
	Status(context.Context) (FactsPipelineStatus, error)
}

var _ FactsPipelineLifecycle = (*FactsPipeline)(nil)
