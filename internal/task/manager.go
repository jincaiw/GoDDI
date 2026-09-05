package task

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// taskEntry holds a task with its status and cancellation.
type taskEntry struct {
	mu       sync.Mutex
	status   *TaskStatus
	cancel   context.CancelFunc
	deadline time.Time
}

// Manager manages background tasks with a worker pool.
type Manager struct {
	tasks        sync.Map // map[string]*taskEntry
	workers      int
	taskCh       chan *taskSubmission
	stopCh       chan struct{}
	shutdownOnce sync.Once
	wg           sync.WaitGroup
}

// taskSubmission represents a submitted task waiting for a worker.
type taskSubmission struct {
	id   string
	name string
	fn   TaskFunc
	ctx  context.Context
}

// NewManager creates a new task manager with the specified number of workers.
func NewManager(workers int) *Manager {
	if workers < 1 {
		workers = 4
	}
	m := &Manager{
		workers: workers,
		taskCh:  make(chan *taskSubmission, 100),
		stopCh:  make(chan struct{}),
	}

	// Start worker goroutines.
	for i := 0; i < workers; i++ {
		m.wg.Add(1)
		go m.worker(i)
	}

	// Start cleanup goroutine.
	m.wg.Add(1)
	go m.cleanupLoop()

	slog.Info("task manager initialized", "workers", workers)
	return m
}

// worker processes tasks from the task channel.
func (m *Manager) worker(id int) {
	defer m.wg.Done()
	for {
		select {
		case <-m.stopCh:
			return
		case sub, ok := <-m.taskCh:
			if !ok {
				return
			}
			m.executeTask(sub)
		}
	}
}

// executeTask runs a task and updates its status.
func (m *Manager) executeTask(sub *taskSubmission) {
	entry, ok := m.tasks.Load(sub.id)
	if !ok {
		return
	}

	te := entry.(*taskEntry)
	now := time.Now()
	te.mu.Lock()
	te.status.Status = "running"
	te.status.StartedAt = &now
	te.mu.Unlock()

	// A panicking task must not take down the whole process: recover,
	// mark the task as failed, and let the worker pick up the next one.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("task panicked", "id", sub.id, "name", sub.name, "panic", r, "stack", string(debug.Stack()))
			completedAt := time.Now()
			te.mu.Lock()
			te.status.CompletedAt = &completedAt
			te.status.Status = "failed"
			te.status.Error = fmt.Sprintf("internal panic: %v", r)
			te.mu.Unlock()
		}
	}()

	// Execute the task function.
	err := sub.fn(sub.ctx)

	te.mu.Lock()
	defer te.mu.Unlock()

	// Do not overwrite a terminal status already recorded (e.g. the task was
	// cancelled while running and CancelTask set "cancelled").
	if te.status.Status == "cancelled" {
		return
	}

	completedAt := time.Now()
	te.status.CompletedAt = &completedAt

	if err != nil {
		if err == context.Canceled {
			te.status.Status = "cancelled"
		} else {
			te.status.Status = "failed"
			te.status.Error = err.Error()
		}
	} else {
		te.status.Status = "completed"
		te.status.Progress = 100
	}
}

// cleanupLoop periodically removes old completed tasks.
func (m *Manager) cleanupLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.cleanupOldTasks()
		}
	}
}

// cleanupOldTasks removes completed tasks older than 24 hours.
func (m *Manager) cleanupOldTasks() {
	cutoff := time.Now().Add(-24 * time.Hour)
	m.tasks.Range(func(key, value interface{}) bool {
		te := value.(*taskEntry)
		te.mu.Lock()
		shouldDelete := (te.status.Status == "completed" || te.status.Status == "failed" || te.status.Status == "cancelled") &&
			te.status.CompletedAt != nil && te.status.CompletedAt.Before(cutoff)
		te.mu.Unlock()
		if shouldDelete {
			m.tasks.Delete(key)
		}
		return true
	})
}

// SubmitTask submits a new background task.
func (m *Manager) SubmitTask(name string, fn TaskFunc) (string, error) {
	return m.SubmitTaskWithTimeout(name, fn, 0)
}

// SubmitTaskWithTimeout submits a new background task with a timeout.
func (m *Manager) SubmitTaskWithTimeout(name string, fn TaskFunc, timeout time.Duration) (string, error) {
	id := uuid.New().String()
	now := time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	var deadline time.Time

	if timeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, timeout)
		deadline = now.Add(timeout)
		// Wrap cancel to also call timeout cancel.
		origCancel := cancel
		cancel = func() {
			origCancel()
			timeoutCancel()
		}
	}

	status := &TaskStatus{
		ID:        id,
		Name:      name,
		Status:    "pending",
		Progress:  0,
		CreatedAt: now,
	}

	entry := &taskEntry{
		status:   status,
		cancel:   cancel,
		deadline: deadline,
	}
	m.tasks.Store(id, entry)

	// Submit to worker pool.
	select {
	case m.taskCh <- &taskSubmission{
		id:   id,
		name: name,
		fn:   fn,
		ctx:  ctx,
	}:
		slog.Info("task submitted", "id", id, "name", name)
	default:
		// Worker pool is full, mark as failed (under the entry lock so this
		// cannot race with GetTaskStatus or the worker's status updates).
		te := entry
		te.mu.Lock()
		te.status.Status = "failed"
		te.status.Error = "task queue is full"
		now := time.Now()
		te.status.CompletedAt = &now
		te.mu.Unlock()
		cancel()
		return id, fmt.Errorf("task queue is full")
	}

	return id, nil
}

// GetTaskStatus returns the status of a task.
func (m *Manager) GetTaskStatus(taskID string) (*TaskStatus, error) {
	entry, ok := m.tasks.Load(taskID)
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	te := entry.(*taskEntry)
	te.mu.Lock()
	defer te.mu.Unlock()
	// Return a deep copy to prevent race conditions.
	s := *te.status
	if te.status.StartedAt != nil {
		t := *te.status.StartedAt
		s.StartedAt = &t
	}
	if te.status.CompletedAt != nil {
		t := *te.status.CompletedAt
		s.CompletedAt = &t
	}
	return &s, nil
}

// ListTasks lists tasks with optional filtering.
func (m *Manager) ListTasks(filter TaskFilter) ([]TaskStatus, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}

	var all []TaskStatus
	m.tasks.Range(func(key, value interface{}) bool {
		te := value.(*taskEntry)
		te.mu.Lock()
		if filter.Status != "" && te.status.Status != filter.Status {
			te.mu.Unlock()
			return true
		}
		s := *te.status
		if te.status.StartedAt != nil {
			t := *te.status.StartedAt
			s.StartedAt = &t
		}
		if te.status.CompletedAt != nil {
			t := *te.status.CompletedAt
			s.CompletedAt = &t
		}
		te.mu.Unlock()
		all = append(all, s)
		return true
	})

	// Sort by creation time for stable pagination.
	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.Before(all[j].CreatedAt)
	})

	total := int64(len(all))

	// Paginate.
	start := (filter.Page - 1) * filter.PageSize
	if start >= len(all) {
		return []TaskStatus{}, total, nil
	}
	end := start + filter.PageSize
	if end > len(all) {
		end = len(all)
	}

	return all[start:end], total, nil
}

// CancelTask cancels a running task.
func (m *Manager) CancelTask(taskID string) error {
	entry, ok := m.tasks.Load(taskID)
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	te := entry.(*taskEntry)
	te.mu.Lock()
	defer te.mu.Unlock()
	if te.status.Status == "running" || te.status.Status == "pending" {
		te.cancel()
		te.status.Status = "cancelled"
		now := time.Now()
		te.status.CompletedAt = &now
		return nil
	}

	return fmt.Errorf("task %s is not cancellable (status: %s)", taskID, te.status.Status)
}

// Shutdown stops the task manager and waits for workers to finish.
// Safe to call multiple times: subsequent invocations return nil without
// touching the (already closed) channels. This prevents the
// "close of closed channel" panic that would otherwise occur if the
// caller or a test invoked Shutdown more than once.
func (m *Manager) Shutdown(ctx context.Context) error {
	slog.Info("shutting down task manager...")

	// Cancel all running tasks. We do this before signalling the channels
	// so workers that wake up from the cancellation observe a closed
	// taskCh and exit cleanly.
	m.tasks.Range(func(key, value interface{}) bool {
		te := value.(*taskEntry)
		te.mu.Lock()
		if te.status.Status == "running" || te.status.Status == "pending" {
			te.cancel()
			te.status.Status = "cancelled"
		}
		te.mu.Unlock()
		return true
	})

	// Close the stop channel and the task channel under a guard so we
	// only close them once across all Shutdown calls.
	m.shutdownOnce.Do(func() {
		close(m.stopCh)
		close(m.taskCh)
	})

	// Wait for workers with timeout.
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("task manager stopped gracefully")
		return nil
	case <-ctx.Done():
		slog.Warn("task manager shutdown timed out")
		return ctx.Err()
	}
}

// SetTaskProgress updates the progress of a task.
func (m *Manager) SetTaskProgress(taskID string, progress int) {
	entry, ok := m.tasks.Load(taskID)
	if !ok {
		return
	}
	te := entry.(*taskEntry)
	te.mu.Lock()
	defer te.mu.Unlock()
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	te.status.Progress = progress
}

// SetTaskResult sets the result of a task.
func (m *Manager) SetTaskResult(taskID string, result string) {
	entry, ok := m.tasks.Load(taskID)
	if !ok {
		return
	}
	te := entry.(*taskEntry)
	te.mu.Lock()
	defer te.mu.Unlock()
	te.status.Result = result
}
