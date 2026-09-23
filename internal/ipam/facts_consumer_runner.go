package ipam

import (
	"context"
	"errors"
	"time"
)

const defaultFactsConsumerPollInterval = time.Second

// FactsConsumerState describes the lifecycle of an opt-in facts consumer.
type FactsConsumerState string

const (
	FactsConsumerIdle    FactsConsumerState = "idle"
	FactsConsumerRunning FactsConsumerState = "running"
	FactsConsumerStopped FactsConsumerState = "stopped"
	FactsConsumerFailed  FactsConsumerState = "failed"
)

const (
	FactsReadinessUnconfigured = "unconfigured"
	FactsReadinessOK           = "ok"
	FactsReadinessDegraded     = "degraded"
	FactsReadinessFailing      = "failing"
	FactsReadinessStopped      = "stopped"
)

// FactsConsumerStatus is a point-in-time lifecycle and backlog snapshot.
// LastApplied is the durable projection watermark. NextExpectedSequence is the
// next contiguous sequence required by the consumer. Lag is the number of
// durable sequences above LastApplied, including failed/outstanding events;
// Gap is true only when the first outstanding event is already beyond the next
// expected sequence. Unknown runtime failures are retained in LastError;
// callers must not infer health from a zero-valued status when the consumer has
// not been started.
type FactsConsumerStatus struct {
	State                    FactsConsumerState
	Started                  bool
	Running                  bool
	LastError                string
	LastApplied              int64
	NextExpectedSequence     int64
	HeadSequence             int64
	FirstOutstandingSequence int64
	Pending                  int64
	Failed                   int64
	Lag                      int64
	Gap                      bool
}

// Readiness returns a conservative opt-in status for the facts projection.
// An unstarted consumer is not reported as healthy; a failed event or sequence
// gap is failing; otherwise a running consumer with backlog is degraded.
// This value is suitable for an opt-in metrics provider, but is not wired into
// the default /ready endpoint.
func (s FactsConsumerStatus) Readiness() string {
	if s.State == FactsConsumerStopped {
		return FactsReadinessStopped
	}
	if !s.Started {
		return FactsReadinessUnconfigured
	}
	if s.State == FactsConsumerFailed || s.Gap || s.Failed > 0 {
		return FactsReadinessFailing
	}
	if s.State == FactsConsumerStopped {
		return FactsReadinessStopped
	}
	if s.Pending > 0 || s.Lag > 0 {
		return FactsReadinessDegraded
	}
	return FactsReadinessOK
}

// FactsConsumerOptions controls the optional background replay loop.
type FactsConsumerOptions struct {
	PollInterval time.Duration
	BatchSize    int
}

// FactsConsumerLifecycle is the consumer lifecycle surface. A caller owns
// construction, start, wake, and stop ordering.
type FactsConsumerLifecycle interface {
	Start(context.Context) error
	Wake()
	Stop(context.Context) error
	Status(context.Context) (FactsConsumerStatus, error)
}

// lifecycle fields are kept on FactsConsumer so ProcessOne/Replay remain the
// single projection implementation and the runner cannot accidentally create a
// second ordering or watermark path.
func (c *FactsConsumer) initLifecycle() {
	c.lifecycleOnce.Do(func() {
		c.wake = make(chan struct{}, 1)
		c.state = FactsConsumerIdle
	})
}

// Start launches one background replay loop. A consumer instance has one
// lifecycle: after Stop or a fail-closed projection error, construct a fresh
// consumer against the same durable outbox to replay again after correction or
// restart. Start is deliberately explicit and never called by NewFactsConsumer.
func (c *FactsConsumer) Start(parent context.Context) error {
	if c == nil || c.linkage == nil || c.outbox == nil {
		return ErrFactsConsumerClosed
	}
	if parent == nil {
		return errors.New("ipam facts consumer: nil start context")
	}
	c.initLifecycle()
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.started {
		if c.state == FactsConsumerFailed {
			return ErrFactsConsumerFailed
		}
		return errors.New("ipam facts consumer: already started")
	}
	ctx, cancel := context.WithCancel(parent)
	c.ctx = ctx
	c.cancel = cancel
	c.started = true
	c.state = FactsConsumerRunning
	c.done = make(chan struct{})
	go c.runLifecycle(ctx)
	return nil
}

// Wake coalesces a prompt replay request. It never blocks the producer and is
// safe before Start, during a replay, and after Stop.
func (c *FactsConsumer) Wake() {
	if c == nil {
		return
	}
	c.initLifecycle()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// Stop prevents new replay passes and waits for the current transaction to
// finish. A timeout reports to the caller; it does not abandon the goroutine or
// claim that the consumer drained successfully.
func (c *FactsConsumer) Stop(ctx context.Context) error {
	if c == nil {
		return ErrFactsConsumerClosed
	}
	if ctx == nil {
		return errors.New("ipam facts consumer: nil stop context")
	}
	c.initLifecycle()
	c.lifecycleMu.Lock()
	if !c.started {
		c.state = FactsConsumerStopped
		c.lifecycleMu.Unlock()
		return nil
	}
	cancel := c.cancel
	done := c.done
	c.lifecycleMu.Unlock()
	cancel()
	select {
	case <-done:
		c.lifecycleMu.Lock()
		err := c.lifecycleErr
		c.lifecycleMu.Unlock()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Status reports lifecycle state plus durable projection/backlog counters.
func (c *FactsConsumer) Status(ctx context.Context) (FactsConsumerStatus, error) {
	if c == nil || c.linkage == nil || c.outbox == nil {
		return FactsConsumerStatus{}, ErrFactsConsumerClosed
	}
	c.initLifecycle()
	c.lifecycleMu.Lock()
	status := FactsConsumerStatus{State: c.state, Started: c.started, Running: c.state == FactsConsumerRunning}
	if c.lifecycleErr != nil {
		status.LastError = c.lifecycleErr.Error()
	}
	c.lifecycleMu.Unlock()
	var err error
	status.LastApplied, err = c.Applied(ctx)
	if err != nil {
		return status, err
	}
	outboxStatus, err := c.outbox.Status(ctx)
	if err != nil {
		return status, err
	}
	status.Pending = outboxStatus.Pending
	status.Failed = outboxStatus.Failed
	status.HeadSequence = outboxStatus.HeadSequence
	status.FirstOutstandingSequence = outboxStatus.FirstOutstandingSequence
	status.NextExpectedSequence = status.LastApplied + 1
	if status.HeadSequence > status.LastApplied {
		status.Lag = status.HeadSequence - status.LastApplied
	}
	if status.FirstOutstandingSequence > 0 {
		status.Gap = status.FirstOutstandingSequence > status.NextExpectedSequence
	}
	return status, nil
}

func (c *FactsConsumer) runLifecycle(ctx context.Context) {
	defer close(c.done)
	defer c.cancel()
	for {
		applied, err := c.Replay(ctx, c.batchSize())
		if err != nil {
			if ctx.Err() != nil {
				c.finishLifecycle(FactsConsumerStopped, nil)
			} else {
				c.finishLifecycle(FactsConsumerFailed, errors.Join(ErrFactsConsumerFailed, err))
			}
			return
		}
		if applied > 0 {
			continue
		}
		interval := c.pollInterval()
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			stopTimer(timer)
			c.finishLifecycle(FactsConsumerStopped, nil)
			return
		case <-c.wake:
			stopTimer(timer)
		case <-timer.C:
		}
	}
}

func stopTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	_ = timer.Stop()
}

func (c *FactsConsumer) finishLifecycle(state FactsConsumerState, err error) {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	c.state = state
	c.lifecycleErr = err
}

func (c *FactsConsumer) pollInterval() time.Duration {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.options.PollInterval > 0 {
		return c.options.PollInterval
	}
	return defaultFactsConsumerPollInterval
}

func (c *FactsConsumer) batchSize() int {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.options.BatchSize > 0 {
		return c.options.BatchSize
	}
	return 100
}

var _ FactsConsumerLifecycle = (*FactsConsumer)(nil)
