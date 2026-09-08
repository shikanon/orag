package taskqueue

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/shikanon/orag/internal/platform/apperrors"
	"github.com/shikanon/orag/internal/platform/clock"
	"github.com/shikanon/orag/internal/platform/id"
)

// MemoryQueueRepository is an in-memory implementation of QueueRepository.
type MemoryQueueRepository struct {
	mu              sync.RWMutex
	tasks           map[string]*Task
	idempotencyKeys map[string]string
	events          map[string][]Event
	clock           clock.Clock
}

type resourceLock struct {
	typeName string
	id       string
}

// NewMemoryQueueRepository creates a new in-memory task queue repository.
func NewMemoryQueueRepository() *MemoryQueueRepository {
	return &MemoryQueueRepository{
		tasks:           map[string]*Task{},
		idempotencyKeys: map[string]string{},
		events:          map[string][]Event{},
		clock:           clock.RealClock{},
	}
}

// SetClock sets the clock used by the repository for testing purposes.
func (r *MemoryQueueRepository) SetClock(c clock.Clock) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clock = c
}

// Enqueue adds a new task to the queue.
func (r *MemoryQueueRepository) Enqueue(_ context.Context, task Task) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if task.IdempotencyKey != "" {
		key := task.TenantID + "\x00" + task.IdempotencyKey
		if existingID, ok := r.idempotencyKeys[key]; ok {
			if existingTask, ok := r.tasks[existingID]; ok {
				return *existingTask, nil
			}
		}
	}

	now := r.clock.Now()
	if task.ID == "" {
		task.ID = id.New("task")
	}
	if task.Status == "" {
		task.Status = TaskStatusQueued
	}
	if task.MaxAttempts == 0 {
		task.MaxAttempts = 3
	}
	task.CreatedAt = now
	task.UpdatedAt = now

	taskCopy := task
	r.tasks[task.ID] = &taskCopy
	r.appendEventLocked(task.ID, "enqueued", nil, now)

	if task.IdempotencyKey != "" {
		r.idempotencyKeys[task.TenantID+"\x00"+task.IdempotencyKey] = task.ID
	}

	return taskCopy, nil
}

// Lease acquires tasks from the queue for processing.
func (r *MemoryQueueRepository) Lease(_ context.Context, pool, leaseHolder string, leaseDuration time.Duration, maxTasks int) ([]Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if maxTasks <= 0 {
		maxTasks = 1
	}

	now := r.clock.Now()
	activeResources := make(map[resourceLock]struct{})
	for _, task := range r.tasks {
		resource, ok := taskResourceLock(task)
		if !ok {
			continue
		}

		switch task.Status {
		case TaskStatusLeased, TaskStatusRunning, TaskStatusCancelling:
			if task.LeaseExpiresAt.IsZero() || task.LeaseExpiresAt.After(now) {
				activeResources[resource] = struct{}{}
			}
		}
	}

	var readyTasks []*Task

	for _, task := range r.tasks {
		if task.Pool != pool {
			continue
		}
		if !task.RunAfter.IsZero() && task.RunAfter.After(now) {
			continue
		}

		switch task.Status {
		case TaskStatusQueued:
			readyTasks = append(readyTasks, task)
		case TaskStatusLeased, TaskStatusRunning, TaskStatusCancelling:
			if !task.LeaseExpiresAt.IsZero() && !task.LeaseExpiresAt.After(now) {
				readyTasks = append(readyTasks, task)
			}
		case TaskStatusFailedRetryable:
			readyTasks = append(readyTasks, task)
		}
	}

	sort.Slice(readyTasks, func(i, j int) bool {
		if readyTasks[i].Priority != readyTasks[j].Priority {
			return readyTasks[i].Priority > readyTasks[j].Priority
		}
		if !readyTasks[i].CreatedAt.Equal(readyTasks[j].CreatedAt) {
			return readyTasks[i].CreatedAt.Before(readyTasks[j].CreatedAt)
		}
		return readyTasks[i].ID < readyTasks[j].ID
	})

	var leased []Task
	leaseExpiresAt := now.Add(leaseDuration)
	count := 0
	leasedResources := make(map[resourceLock]struct{})

	for _, task := range readyTasks {
		if count >= maxTasks {
			break
		}

		resource, resourceLocked := taskResourceLock(task)
		if resourceLocked {
			if _, active := activeResources[resource]; active {
				continue
			}
			if _, alreadyLeased := leasedResources[resource]; alreadyLeased {
				continue
			}
		}

		task.Status = TaskStatusLeased
		task.LeaseHolder = leaseHolder
		task.LeaseExpiresAt = leaseExpiresAt
		task.LeaseGeneration++
		task.UpdatedAt = now
		if task.Attempt == 0 {
			task.Attempt = 1
		} else {
			task.Attempt++
		}
		task.StartedAt = now
		task.LastHeartbeatAt = now
		r.appendEventLocked(task.ID, "leased", map[string]string{"lease_holder": leaseHolder}, now)

		leased = append(leased, *task)
		if resourceLocked {
			leasedResources[resource] = struct{}{}
		}
		count++
	}

	return leased, nil
}

func taskResourceLock(task *Task) (resourceLock, bool) {
	if task.LockedResourceType == "" || task.LockedResourceID == "" {
		return resourceLock{}, false
	}
	return resourceLock{typeName: task.LockedResourceType, id: task.LockedResourceID}, true
}

// Heartbeat renews the lease on a task.
func (r *MemoryQueueRepository) Heartbeat(_ context.Context, taskID string, token LeaseToken, leaseDuration time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, ok := r.tasks[taskID]
	if !ok || !ownsLease(task, token) ||
		(task.Status != TaskStatusLeased && task.Status != TaskStatusRunning) {
		return ErrLeaseLost
	}

	now := r.clock.Now()
	task.LeaseExpiresAt = now.Add(leaseDuration)
	task.LastHeartbeatAt = now
	task.UpdatedAt = now

	if task.Status == TaskStatusLeased {
		task.Status = TaskStatusRunning
		r.appendEventLocked(task.ID, "running", nil, now)
	}

	return nil
}

// OwnsActiveLease reports whether token still owns an unexpired execution
// lease. It is used by in-memory durable coordinators to apply the same write
// fencing as the PostgreSQL lease predicates.
func (r *MemoryQueueRepository) OwnsActiveLease(_ context.Context, taskID string, token LeaseToken, allowCancelling bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok || !ownsLease(task, token) || task.LeaseExpiresAt.IsZero() || !task.LeaseExpiresAt.After(r.clock.Now()) {
		return false
	}
	return task.Status == TaskStatusLeased || task.Status == TaskStatusRunning || (allowCancelling && task.Status == TaskStatusCancelling)
}

// Complete marks a task as successfully completed.
func (r *MemoryQueueRepository) Complete(_ context.Context, taskID string, token LeaseToken, _ TaskResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, ok := r.tasks[taskID]
	if !ok || !ownsLease(task, token) || task.Status != TaskStatusRunning {
		return ErrLeaseLost
	}

	now := r.clock.Now()
	task.Status = TaskStatusSucceeded
	task.LeaseHolder = ""
	task.LeaseExpiresAt = time.Time{}
	task.CompletedAt = now
	task.UpdatedAt = now
	r.appendEventLocked(taskID, "succeeded", nil, now)

	return nil
}

// Fail marks a task as failed.
func (r *MemoryQueueRepository) Fail(_ context.Context, taskID string, token LeaseToken, failure TaskFailure) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, ok := r.tasks[taskID]
	if !ok || !ownsLease(task, token) ||
		(task.Status != TaskStatusLeased && task.Status != TaskStatusRunning) {
		return ErrLeaseLost
	}

	now := r.clock.Now()
	task.ErrorCode = failure.ErrorCode
	task.ErrorMessage = failure.Message
	task.UpdatedAt = now

	if !failure.Retryable {
		task.Status = TaskStatusFailedTerminal
		task.CompletedAt = now
		task.LeaseHolder = ""
		task.LeaseExpiresAt = time.Time{}
		r.appendEventLocked(taskID, "failed", nil, now)
		return nil
	}

	if task.Attempt >= task.MaxAttempts {
		task.Status = TaskStatusDeadLetter
		task.CompletedAt = now
		task.LeaseHolder = ""
		task.LeaseExpiresAt = time.Time{}
		r.appendEventLocked(taskID, "dead_letter", nil, now)
		return nil
	}

	task.Status = TaskStatusFailedRetryable
	task.LeaseHolder = ""
	task.LeaseExpiresAt = time.Time{}
	task.RunAfter = now.Add(retryBackoff(task.Attempt))
	r.appendEventLocked(taskID, "failed_retryable", nil, now)
	return nil
}

// Cancel requests cancellation of a task.
func (r *MemoryQueueRepository) Cancel(_ context.Context, taskID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, ok := r.tasks[taskID]
	if !ok {
		return apperrors.New(apperrors.CodeNotFound, "task not found")
	}

	now := r.clock.Now()

	switch task.Status {
	case TaskStatusQueued, TaskStatusFailedRetryable:
		task.Status = TaskStatusCancelled
		task.CompletedAt = now
		task.UpdatedAt = now
		task.LeaseHolder = ""
		task.LeaseExpiresAt = time.Time{}
		r.appendEventLocked(taskID, "cancelled", nil, now)
	case TaskStatusLeased, TaskStatusRunning:
		task.Status = TaskStatusCancelling
		task.UpdatedAt = now
		r.appendEventLocked(taskID, "cancelling", nil, now)
	case TaskStatusCancelling, TaskStatusCancelled, TaskStatusSucceeded, TaskStatusFailedTerminal, TaskStatusDeadLetter:
		return nil
	}

	return nil
}

func (r *MemoryQueueRepository) MarkCancelled(_ context.Context, taskID string, token LeaseToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok || !ownsLease(task, token) || task.Status != TaskStatusCancelling {
		return ErrLeaseLost
	}
	now := r.clock.Now()
	task.Status = TaskStatusCancelled
	task.CompletedAt = now
	task.UpdatedAt = now
	task.LeaseHolder = ""
	task.LeaseExpiresAt = time.Time{}
	r.appendEventLocked(taskID, "cancelled", nil, now)
	return nil
}

func (r *MemoryQueueRepository) Retry(_ context.Context, taskID string) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok {
		return Task{}, apperrors.New(apperrors.CodeNotFound, "task not found")
	}
	switch task.Status {
	case TaskStatusDeadLetter, TaskStatusFailedTerminal, TaskStatusCancelled:
	default:
		return Task{}, apperrors.New(apperrors.CodeConflict, "task is not eligible for manual retry")
	}
	now := r.clock.Now()
	task.Status = TaskStatusQueued
	task.Attempt = 0
	task.RunAfter = now
	task.UpdatedAt = now
	task.CompletedAt = time.Time{}
	task.ErrorCode = ""
	task.ErrorMessage = ""
	r.appendEventLocked(taskID, "manual_retry", nil, now)
	return *task, nil
}

// Get retrieves a task by its ID.
func (r *MemoryQueueRepository) Get(_ context.Context, taskID string) (Task, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	task, ok := r.tasks[taskID]
	if !ok {
		return Task{}, false, nil
	}
	return *task, true, nil
}

// List returns tasks matching the given filter.
func (r *MemoryQueueRepository) List(_ context.Context, filter TaskFilter) ([]Task, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Task
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	skipped := 0
	startFound := filter.Cursor == ""

	var taskIDs []string
	for id := range r.tasks {
		taskIDs = append(taskIDs, id)
	}
	sort.Strings(taskIDs)

	for _, id := range taskIDs {
		task := r.tasks[id]
		if filter.TenantID != "" && task.TenantID != filter.TenantID {
			continue
		}

		if !startFound {
			skipped++
			if id == filter.Cursor {
				startFound = true
			}
			continue
		}

		if filter.Status != "" && task.Status != filter.Status {
			continue
		}
		if filter.Type != "" && task.Type != filter.Type {
			continue
		}
		if filter.Pool != "" && task.Pool != filter.Pool {
			continue
		}
		if filter.ProjectID != "" && task.ProjectID != filter.ProjectID {
			continue
		}

		result = append(result, *task)
		if len(result) >= limit {
			break
		}
	}

	var nextCursor string
	if len(result) == limit {
		nextCursor = result[len(result)-1].ID
	}

	return result, nextCursor, nil
}

func (r *MemoryQueueRepository) ListEvents(_ context.Context, taskID, cursor string, limit int) ([]Event, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.tasks[taskID]; !ok {
		return nil, "", apperrors.New(apperrors.CodeNotFound, "task not found")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	events := r.events[taskID]
	start := 0
	if cursor != "" {
		for i, event := range events {
			if event.ID == cursor {
				start = i + 1
				break
			}
		}
	}
	end := start + limit
	if end > len(events) {
		end = len(events)
	}
	out := append([]Event(nil), events[start:end]...)
	next := ""
	if end < len(events) && len(out) > 0 {
		next = out[len(out)-1].ID
	}
	return out, next, nil
}

func (r *MemoryQueueRepository) Stats(_ context.Context, tenantID string) (map[string]PoolStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	stats := map[string]PoolStats{}
	for _, task := range r.tasks {
		if task.TenantID != tenantID {
			continue
		}
		current := stats[task.Pool]
		switch task.Status {
		case TaskStatusQueued, TaskStatusFailedRetryable:
			current.Queued++
		case TaskStatusLeased, TaskStatusRunning, TaskStatusCancelling:
			current.Running++
		case TaskStatusSucceeded:
			current.Succeeded++
		case TaskStatusDeadLetter:
			current.DeadLetter++
		case TaskStatusFailedTerminal, TaskStatusCancelled:
			current.Failed++
		}
		stats[task.Pool] = current
	}
	return stats, nil
}

func (r *MemoryQueueRepository) appendEventLocked(taskID, eventType string, data any, now time.Time) {
	// Keep the memory implementation intentionally metadata-only. It is used by
	// local development/tests and must follow the same privacy boundary as SQL.
	r.events[taskID] = append(r.events[taskID], Event{ID: id.New("task_event"), TaskID: taskID, Type: eventType, CreatedAt: now})
}

func ownsLease(task *Task, token LeaseToken) bool {
	return task.LeaseHolder == token.Holder &&
		task.LeaseGeneration == token.Generation
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	backoff := 250 * time.Millisecond * time.Duration(1<<min(attempt-1, 12))
	if backoff > time.Hour {
		return time.Hour
	}
	return backoff
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
