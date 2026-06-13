package task

import (
	"context"
	"time"
)

// TaskFunc is the function signature for a background task.
type TaskFunc func(ctx context.Context) error

// TaskStatus represents the current status of a task.
type TaskStatus struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"` // pending, running, completed, failed, cancelled
	Progress    int        `json:"progress"`
	Result      string     `json:"result,omitempty"`
	Error       string     `json:"error,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// TaskFilter specifies filter criteria for listing tasks.
type TaskFilter struct {
	Status   string `json:"status"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

// ProgressReporter allows tasks to report their progress.
type ProgressReporter interface {
	SetProgress(percent int)
	SetResult(result string)
}
