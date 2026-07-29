package taskqueue

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrLeaseLost indicates that a worker no longer owns the lease generation
// required to mutate a task.
var ErrLeaseLost = errors.New("task queue lease lost")

// TaskStatus represents the current state of a task in the queue.
type TaskStatus string

const (
	// TaskStatusQueued means the task is waiting to be leased.
	TaskStatusQueued TaskStatus = "queued"

	// TaskStatusLeased means the task has been leased to a worker but not yet started.
	TaskStatusLeased TaskStatus = "leased"

	// TaskStatusRunning means the task is currently being processed.
	TaskStatusRunning TaskStatus = "running"

	// TaskStatusSucceeded means the task completed successfully.
	TaskStatusSucceeded TaskStatus = "succeeded"

	// TaskStatusFailedRetryable means the task failed but can be retried.
	TaskStatusFailedRetryable TaskStatus = "failed_retryable"

	// TaskStatusFailedTerminal means the task failed permanently and should not be retried.
	TaskStatusFailedTerminal TaskStatus = "failed_terminal"

	// TaskStatusCancelling means a cancellation has been requested for a running task.
	TaskStatusCancelling TaskStatus = "cancelling"

	// TaskStatusCancelled means the task was cancelled before completion.
	TaskStatusCancelled TaskStatus = "cancelled"

	// TaskStatusDeadLetter means the task has exhausted all retries and moved to dead letter queue.
	TaskStatusDeadLetter TaskStatus = "dead_letter"
)

// Task represents a unit of work in the task queue.
type Task struct {
	ID                 string          `json:"id"`
	TenantID           string          `json:"tenant_id"`
	ProjectID          string          `json:"project_id,omitempty"`
	Type               string          `json:"type"`
	Pool               string          `json:"pool"`
	Status             TaskStatus      `json:"status"`
	Payload            json.RawMessage `json:"payload,omitempty"`
	IdempotencyKey     string          `json:"idempotency_key,omitempty"`
	Attempt            int             `json:"attempt"`
	MaxAttempts        int             `json:"max_attempts"`
	LockedResourceType string          `json:"locked_resource_type,omitempty"`
	LockedResourceID   string          `json:"locked_resource_id,omitempty"`
	TraceID            string          `json:"trace_id,omitempty"`
	CreatedBy          string          `json:"created_by,omitempty"`
	Priority           int             `json:"priority"`
	RunAfter           time.Time       `json:"run_after,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	StartedAt          time.Time       `json:"started_at,omitempty"`
	CompletedAt        time.Time       `json:"completed_at,omitempty"`
	ErrorCode          string          `json:"error_code,omitempty"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	LastHeartbeatAt    time.Time       `json:"last_heartbeat_at,omitempty"`
	LeaseExpiresAt     time.Time       `json:"lease_expires_at,omitempty"`
	LeaseHolder        string          `json:"lease_holder,omitempty"`
	LeaseGeneration    int64           `json:"lease_generation"`
}

// TaskResult contains the result of a successfully completed task.
type TaskResult struct {
	Data json.RawMessage `json:"data,omitempty"`
}

// TaskFailure contains information about a task failure.
type TaskFailure struct {
	Retryable bool   `json:"retryable"`
	ErrorCode string `json:"error_code,omitempty"`
	Message   string `json:"message"`
}

// TaskFilter provides filtering criteria for listing tasks.
type TaskFilter struct {
	TenantID  string     `json:"-"`
	Status    TaskStatus `json:"status,omitempty"`
	Type      string     `json:"type,omitempty"`
	Pool      string     `json:"pool,omitempty"`
	ProjectID string     `json:"project_id,omitempty"`
	Limit     int        `json:"limit,omitempty"`
	Cursor    string     `json:"cursor,omitempty"`
}

// Event is an immutable state-transition or progress record for a task.
// Event data deliberately contains operational metadata only; task payloads
// and document/model content remain on the task and are not copied here.
type Event struct {
	ID        string          `json:"id"`
	TaskID    string          `json:"task_id"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type PoolStats struct {
	Queued     int `json:"queued"`
	Running    int `json:"running"`
	Succeeded  int `json:"succeeded"`
	Failed     int `json:"failed"`
	DeadLetter int `json:"dead_letter"`
}

// QueueRepository defines the interface for task queue storage operations.
type QueueRepository interface {
	// Enqueue adds a new task to the queue.
	Enqueue(ctx context.Context, task Task) (Task, error)

	// Lease acquires a task from the queue for processing.
	Lease(ctx context.Context, pool, leaseHolder string, leaseDuration time.Duration, maxTasks int) ([]Task, error)

	// Heartbeat renews the lease on a task to indicate it's still being processed.
	Heartbeat(ctx context.Context, taskID, leaseHolder string, leaseGeneration int64, leaseDuration time.Duration) error

	// Complete marks a task as successfully completed.
	Complete(ctx context.Context, taskID, leaseHolder string, leaseGeneration int64, result TaskResult) error

	// Fail marks a task as failed.
	Fail(ctx context.Context, taskID, leaseHolder string, leaseGeneration int64, failure TaskFailure) error

	// Cancel requests cancellation of a task.
	Cancel(ctx context.Context, taskID string) error

	// MarkCancelled finalizes a task after its handler acknowledges cancellation.
	MarkCancelled(ctx context.Context, taskID, leaseHolder string, leaseGeneration int64) error

	// Retry manually returns a terminal task to the queue.
	Retry(ctx context.Context, taskID string) (Task, error)

	// Get retrieves a task by its ID.
	Get(ctx context.Context, taskID string) (Task, bool, error)

	// List returns tasks matching the given filter.
	List(ctx context.Context, filter TaskFilter) ([]Task, string, error)

	// ListEvents returns the append-only task timeline.
	ListEvents(ctx context.Context, taskID, cursor string, limit int) ([]Event, string, error)

	// Stats returns aggregate queue counts for a tenant.
	Stats(ctx context.Context, tenantID string) (map[string]PoolStats, error)
}

// Handler defines the interface for task execution handlers.
type Handler interface {
	// Handle processes a task and returns an error if processing fails.
	Handle(ctx context.Context, task Task, reporter ProgressReporter) error
}

// ProgressReporter defines the interface for reporting task progress.
type ProgressReporter interface {
	// ReportProgress reports the current progress of a task.
	ReportProgress(ctx context.Context, taskID string, progress int, message string) error
}
