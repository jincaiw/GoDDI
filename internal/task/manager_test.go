package task

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
}

func TestNewManager_DefaultWorkers(t *testing.T) {
	m := NewManager(0) // Should default to 4
	defer m.Shutdown(context.Background())

	if m.workers != 4 {
		t.Errorf("workers = %d, want 4", m.workers)
	}
}

func TestNewManager_NegativeWorkers(t *testing.T) {
	m := NewManager(-1)
	defer m.Shutdown(context.Background())

	if m.workers != 4 {
		t.Errorf("workers = %d, want 4", m.workers)
	}
}

func TestSubmitTask(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	id, err := m.SubmitTask("test-task", func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("SubmitTask() error = %v", err)
	}
	if id == "" {
		t.Error("SubmitTask() returned empty ID")
	}

	// Wait for task to complete
	time.Sleep(100 * time.Millisecond)

	status, err := m.GetTaskStatus(id)
	if err != nil {
		t.Fatalf("GetTaskStatus() error = %v", err)
	}
	if status.Status != "completed" {
		t.Errorf("task status = %q, want %q", status.Status, "completed")
	}
	if status.Progress != 100 {
		t.Errorf("task progress = %d, want 100", status.Progress)
	}
}

func TestSubmitTask_Failed(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	id, _ := m.SubmitTask("failing-task", func(ctx context.Context) error {
		return errors.New("task failed")
	})

	// Wait for task to complete
	time.Sleep(100 * time.Millisecond)

	status, _ := m.GetTaskStatus(id)
	if status.Status != "failed" {
		t.Errorf("task status = %q, want %q", status.Status, "failed")
	}
	if status.Error != "task failed" {
		t.Errorf("task error = %q, want %q", status.Error, "task failed")
	}
}

func TestSubmitTaskWithTimeout(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	id, err := m.SubmitTaskWithTimeout("timeout-task", func(ctx context.Context) error {
		// Wait for context cancellation (simulating a long-running task)
		<-ctx.Done()
		return ctx.Err()
	}, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("SubmitTaskWithTimeout() error = %v", err)
	}

	// Wait for timeout and task to process cancellation
	time.Sleep(300 * time.Millisecond)

	status, _ := m.GetTaskStatus(id)
	if status.Status != "failed" && status.Status != "cancelled" {
		t.Errorf("timed out task status = %q, want 'failed' or 'cancelled'", status.Status)
	}
}

func TestGetTaskStatus_NotFound(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	_, err := m.GetTaskStatus("nonexistent")
	if err == nil {
		t.Error("GetTaskStatus() should return error for nonexistent task")
	}
}

func TestCancelTask(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	id, _ := m.SubmitTask("long-task", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	// Give the task a moment to start
	time.Sleep(50 * time.Millisecond)

	err := m.CancelTask(id)
	if err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}

	status, _ := m.GetTaskStatus(id)
	if status.Status != "cancelled" {
		t.Errorf("task status = %q, want %q", status.Status, "cancelled")
	}
}

func TestCancelTask_NotFound(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	err := m.CancelTask("nonexistent")
	if err == nil {
		t.Error("CancelTask() should return error for nonexistent task")
	}
}

func TestCancelTask_AlreadyCompleted(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	id, _ := m.SubmitTask("quick-task", func(ctx context.Context) error {
		return nil
	})

	// Wait for completion
	time.Sleep(100 * time.Millisecond)

	err := m.CancelTask(id)
	if err == nil {
		t.Error("CancelTask() should return error for completed task")
	}
}

func TestListTasks(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	// Submit multiple tasks
	for i := 0; i < 5; i++ {
		m.SubmitTask("task-"+string(rune('A'+i)), func(ctx context.Context) error {
			return nil
		})
	}

	// Wait for tasks to complete
	time.Sleep(200 * time.Millisecond)

	tasks, total, err := m.ListTasks(TaskFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(tasks) != 5 {
		t.Errorf("returned tasks = %d, want 5", len(tasks))
	}
}

func TestListTasks_Pagination(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	for i := 0; i < 5; i++ {
		m.SubmitTask("task", func(ctx context.Context) error {
			time.Sleep(2 * time.Second) // Keep tasks around
			return nil
		})
	}

	time.Sleep(100 * time.Millisecond)

	// Page 1 with page size 2
	tasks, total, _ := m.ListTasks(TaskFilter{Page: 1, PageSize: 2})
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(tasks) != 2 {
		t.Errorf("page 1 tasks = %d, want 2", len(tasks))
	}

	// Page 3 should be empty
	tasks, _, _ = m.ListTasks(TaskFilter{Page: 3, PageSize: 2})
	if len(tasks) != 1 {
		t.Errorf("page 3 tasks = %d, want 1", len(tasks))
	}
}

func TestListTasks_FilterByStatus(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	m.SubmitTask("completed-task", func(ctx context.Context) error {
		return nil
	})

	time.Sleep(100 * time.Millisecond)

	tasks, total, _ := m.ListTasks(TaskFilter{Status: "completed", Page: 1, PageSize: 10})
	if total != 1 {
		t.Errorf("completed tasks total = %d, want 1", total)
	}
	if len(tasks) != 1 {
		t.Errorf("completed tasks = %d, want 1", len(tasks))
	}
}

func TestSetTaskProgress(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	id, _ := m.SubmitTask("progress-task", func(ctx context.Context) error {
		time.Sleep(2 * time.Second)
		return nil
	})

	time.Sleep(50 * time.Millisecond)

	m.SetTaskProgress(id, 50)
	status, _ := m.GetTaskStatus(id)
	if status.Progress != 50 {
		t.Errorf("progress = %d, want 50", status.Progress)
	}
}

func TestSetTaskProgress_Clamped(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	id, _ := m.SubmitTask("progress-task", func(ctx context.Context) error {
		time.Sleep(2 * time.Second)
		return nil
	})

	time.Sleep(50 * time.Millisecond)

	// Test upper bound
	m.SetTaskProgress(id, 150)
	status, _ := m.GetTaskStatus(id)
	if status.Progress != 100 {
		t.Errorf("progress = %d, want 100 (clamped)", status.Progress)
	}

	// Test lower bound
	m.SetTaskProgress(id, -10)
	status, _ = m.GetTaskStatus(id)
	if status.Progress != 0 {
		t.Errorf("progress = %d, want 0 (clamped)", status.Progress)
	}
}

func TestSetTaskResult(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	id, _ := m.SubmitTask("result-task", func(ctx context.Context) error {
		time.Sleep(2 * time.Second)
		return nil
	})

	time.Sleep(50 * time.Millisecond)

	m.SetTaskResult(id, "task completed with 42 items")
	status, _ := m.GetTaskStatus(id)
	if status.Result != "task completed with 42 items" {
		t.Errorf("result = %q, want %q", status.Result, "task completed with 42 items")
	}
}

func TestSetTaskProgress_NonexistentTask(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	// Should not panic
	m.SetTaskProgress("nonexistent", 50)
}

func TestSetTaskResult_NonexistentTask(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	// Should not panic
	m.SetTaskResult("nonexistent", "result")
}

func TestShutdown(t *testing.T) {
	m := NewManager(2)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := m.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestShutdown_WithRunningTasks(t *testing.T) {
	m := NewManager(2)

	// Submit a long-running task
	m.SubmitTask("long-task", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := m.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestListTasks_DefaultPagination(t *testing.T) {
	m := NewManager(2)
	defer m.Shutdown(context.Background())

	// Submit with zero page/page_size - should use defaults
	m.SubmitTask("task", func(ctx context.Context) error {
		time.Sleep(2 * time.Second)
		return nil
	})

	time.Sleep(50 * time.Millisecond)

	tasks, total, _ := m.ListTasks(TaskFilter{Page: 0, PageSize: 0})
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(tasks) != 1 {
		t.Errorf("tasks = %d, want 1", len(tasks))
	}
}
