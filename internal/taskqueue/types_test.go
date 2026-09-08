package taskqueue

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/platform/clock"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t time.Time) *fakeClock {
	return &fakeClock{now: t}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *fakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

var _ clock.Clock = (*fakeClock)(nil)

func newTestTask(pool, idempotencyKey string) Task {
	return Task{
		TenantID:       "test-tenant",
		ProjectID:      "test-project",
		Type:           "test-type",
		Pool:           pool,
		IdempotencyKey: idempotencyKey,
		MaxAttempts:    3,
		Priority:       0,
	}
}

func TestTaskStatusStateMachine_Success(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	task, err := repo.Enqueue(ctx, newTestTask("default", ""))
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if task.Status != TaskStatusQueued {
		t.Errorf("expected status %s, got %s", TaskStatusQueued, task.Status)
	}

	leased, err := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)
	if err != nil {
		t.Fatalf("Lease failed: %v", err)
	}
	if len(leased) != 1 {
		t.Fatalf("expected 1 leased task, got %d", len(leased))
	}
	if leased[0].Status != TaskStatusLeased {
		t.Errorf("expected status %s, got %s", TaskStatusLeased, leased[0].Status)
	}
	if leased[0].Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", leased[0].Attempt)
	}

	token := leased[0].LeaseToken()
	err = repo.Heartbeat(ctx, leased[0].ID, token, 30*time.Second)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
	got, _, _ := repo.Get(ctx, leased[0].ID)
	if got.Status != TaskStatusRunning {
		t.Errorf("expected status %s after heartbeat, got %s", TaskStatusRunning, got.Status)
	}

	err = repo.Complete(ctx, leased[0].ID, token, TaskResult{})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	got, _, _ = repo.Get(ctx, leased[0].ID)
	if got.Status != TaskStatusSucceeded {
		t.Errorf("expected status %s, got %s", TaskStatusSucceeded, got.Status)
	}
	if got.CompletedAt.IsZero() {
		t.Error("expected CompletedAt to be set")
	}
}

func TestTaskStatusStateMachine_FailedRetryable(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	_, _ = repo.Enqueue(ctx, newTestTask("default", ""))
	leased, _ := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)
	if len(leased) != 1 {
		t.Fatalf("expected 1 leased task")
	}

	err := repo.Fail(ctx, leased[0].ID, leased[0].LeaseToken(), TaskFailure{
		Retryable: true,
		ErrorCode: "temporary_error",
		Message:   "temporary failure",
	})
	if err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	got, _, _ := repo.Get(ctx, leased[0].ID)
	if got.Status != TaskStatusFailedRetryable {
		t.Errorf("expected status %s, got %s", TaskStatusFailedRetryable, got.Status)
	}
	if got.ErrorCode != "temporary_error" {
		t.Errorf("expected error code temporary_error, got %s", got.ErrorCode)
	}
	if got.ErrorMessage != "temporary failure" {
		t.Errorf("expected error message temporary failure, got %s", got.ErrorMessage)
	}
	if got.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", got.Attempt)
	}
}

func TestTaskStatusStateMachine_FailedTerminal(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	task, _ := repo.Enqueue(ctx, newTestTask("default", ""))
	leased, _ := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)

	err := repo.Fail(ctx, leased[0].ID, leased[0].LeaseToken(), TaskFailure{
		Retryable: false,
		ErrorCode: "permanent_error",
		Message:   "permanent failure",
	})
	if err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	got, _, _ := repo.Get(ctx, task.ID)
	if got.Status != TaskStatusFailedTerminal {
		t.Errorf("expected status %s, got %s", TaskStatusFailedTerminal, got.Status)
	}
}

func TestTaskStatusStateMachine_DeadLetter(t *testing.T) {
	repo := NewMemoryQueueRepository()
	fc := newFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	repo.SetClock(fc)
	ctx := context.Background()

	task := newTestTask("default", "")
	task.MaxAttempts = 2
	task, _ = repo.Enqueue(ctx, task)

	leased, _ := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)
	repo.Fail(ctx, leased[0].ID, leased[0].LeaseToken(), TaskFailure{Retryable: true, Message: "fail 1"})
	fc.Add(300 * time.Millisecond)

	leased, _ = repo.Lease(ctx, "default", "worker-2", 30*time.Second, 1)
	if len(leased) != 1 {
		t.Fatalf("expected 1 leased task on retry")
	}
	if leased[0].Attempt != 2 {
		t.Errorf("expected attempt 2, got %d", leased[0].Attempt)
	}

	repo.Fail(ctx, leased[0].ID, leased[0].LeaseToken(), TaskFailure{Retryable: true, Message: "fail 2"})

	got, _, _ := repo.Get(ctx, task.ID)
	if got.Status != TaskStatusDeadLetter {
		t.Errorf("expected status %s, got %s", TaskStatusDeadLetter, got.Status)
	}
}

func TestLeaseConcurrency(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	task := newTestTask("default", "")
	task.Priority = 100
	task, _ = repo.Enqueue(ctx, task)

	var wg sync.WaitGroup
	results := make(chan string, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(workerID string) {
			defer wg.Done()
			leased, err := repo.Lease(ctx, "default", workerID, 30*time.Second, 1)
			if err != nil {
				return
			}
			if len(leased) > 0 {
				results <- workerID
			}
		}(string(rune('A' + i)))
	}

	wg.Wait()
	close(results)

	var winners []string
	for w := range results {
		winners = append(winners, w)
	}

	if len(winners) != 1 {
		t.Errorf("expected exactly 1 winner, got %d: %v", len(winners), winners)
	}
}

func TestMemoryQueueRepositoryResourceLockSelectsFirstTask(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fc := newFakeClock(startTime)
	repo := NewMemoryQueueRepository()
	repo.SetClock(fc)
	ctx := context.Background()

	for _, taskID := range []string{"task-b", "task-a"} {
		task := newTestTask("default", taskID)
		task.ID = taskID
		task.LockedResourceType = "document"
		task.LockedResourceID = "document-1"
		if _, err := repo.Enqueue(ctx, task); err != nil {
			t.Fatalf("Enqueue(%s) failed: %v", taskID, err)
		}
	}

	leased, err := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 2)
	if err != nil {
		t.Fatalf("Lease failed: %v", err)
	}
	if len(leased) != 1 {
		t.Fatalf("leased %d tasks, want 1", len(leased))
	}
	if leased[0].ID != "task-a" {
		t.Fatalf("leased task %q, want deterministic first task %q", leased[0].ID, "task-a")
	}
}

func TestMemoryQueueRepositoryResourceLockConcurrentLeaseHasOneWinner(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	for _, taskID := range []string{"task-a", "task-b"} {
		task := newTestTask("default", taskID)
		task.ID = taskID
		task.LockedResourceType = "document"
		task.LockedResourceID = "document-1"
		if _, err := repo.Enqueue(ctx, task); err != nil {
			t.Fatalf("Enqueue(%s) failed: %v", taskID, err)
		}
	}

	const workers = 10
	results := make(chan int, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			leased, err := repo.Lease(ctx, "default", string(rune('A'+worker)), 30*time.Second, 1)
			if err != nil {
				t.Errorf("Lease failed: %v", err)
				return
			}
			results <- len(leased)
		}(i)
	}
	wg.Wait()
	close(results)

	total := 0
	for leased := range results {
		total += leased
	}
	if total != 1 {
		t.Fatalf("concurrent leases returned %d tasks in total, want 1", total)
	}
}

func TestMemoryQueueRepositoryResourceLockReleasedAfterCompletion(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	for _, taskID := range []string{"task-a", "task-b"} {
		task := newTestTask("default", taskID)
		task.ID = taskID
		task.LockedResourceType = "document"
		task.LockedResourceID = "document-1"
		if _, err := repo.Enqueue(ctx, task); err != nil {
			t.Fatalf("Enqueue(%s) failed: %v", taskID, err)
		}
	}

	first, err := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 2)
	if err != nil || len(first) != 1 {
		t.Fatalf("first Lease returned %d tasks, error=%v; want 1 task", len(first), err)
	}
	if err := repo.Heartbeat(ctx, first[0].ID, first[0].LeaseToken(), 30*time.Second); err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
	blocked, err := repo.Lease(ctx, "default", "worker-2", 30*time.Second, 1)
	if err != nil || len(blocked) != 0 {
		t.Fatalf("Lease while first task is running returned %d tasks, error=%v; want 0", len(blocked), err)
	}
	if err := repo.Complete(ctx, first[0].ID, first[0].LeaseToken(), TaskResult{}); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	second, err := repo.Lease(ctx, "default", "worker-2", 30*time.Second, 1)
	if err != nil || len(second) != 1 {
		t.Fatalf("Lease after completion returned %d tasks, error=%v; want 1", len(second), err)
	}
	if second[0].ID != "task-b" {
		t.Fatalf("leased task %q after completion, want %q", second[0].ID, "task-b")
	}
}

func TestMemoryQueueRepositoryResourceLockBatchIncludesIndependentTasks(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	tasks := []Task{
		{ID: "resource-a-first", LockedResourceType: "document", LockedResourceID: "a"},
		{ID: "resource-a-second", LockedResourceType: "document", LockedResourceID: "a"},
		{ID: "resource-b", LockedResourceType: "document", LockedResourceID: "b"},
		{ID: "unlocked", LockedResourceType: "", LockedResourceID: ""},
		{ID: "type-only", LockedResourceType: "document", LockedResourceID: ""},
		{ID: "id-only", LockedResourceType: "", LockedResourceID: "c"},
	}
	for i := range tasks {
		task := newTestTask("default", tasks[i].ID)
		task.ID = tasks[i].ID
		task.LockedResourceType = tasks[i].LockedResourceType
		task.LockedResourceID = tasks[i].LockedResourceID
		if _, err := repo.Enqueue(ctx, task); err != nil {
			t.Fatalf("Enqueue(%s) failed: %v", task.ID, err)
		}
	}

	leased, err := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 5)
	if err != nil {
		t.Fatalf("Lease failed: %v", err)
	}
	if len(leased) != 5 {
		t.Fatalf("leased %d tasks, want a full batch of 5", len(leased))
	}
	leasedIDs := make(map[string]bool, len(leased))
	for _, task := range leased {
		leasedIDs[task.ID] = true
	}
	for _, taskID := range []string{"resource-a-first", "resource-b", "unlocked", "type-only", "id-only"} {
		if !leasedIDs[taskID] {
			t.Errorf("task %q was not leased", taskID)
		}
	}
	if leasedIDs["resource-a-second"] {
		t.Error("second task for resource a was leased in the same batch")
	}
}

func TestMemoryQueueRepositoryResourceLockActiveStatusesBlock(t *testing.T) {
	for _, status := range []TaskStatus{TaskStatusLeased, TaskStatusRunning, TaskStatusCancelling} {
		t.Run(string(status), func(t *testing.T) {
			repo := NewMemoryQueueRepository()
			ctx := context.Background()

			blocker := newTestTask("default", "blocker")
			blocker.ID = "blocker"
			blocker.LockedResourceType = "document"
			blocker.LockedResourceID = "document-1"
			if _, err := repo.Enqueue(ctx, blocker); err != nil {
				t.Fatalf("Enqueue blocker failed: %v", err)
			}
			leased, err := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)
			if err != nil || len(leased) != 1 {
				t.Fatalf("Lease blocker returned %d tasks, error=%v; want 1", len(leased), err)
			}
			if status == TaskStatusRunning {
				if err := repo.Heartbeat(ctx, blocker.ID, leased[0].LeaseToken(), 30*time.Second); err != nil {
					t.Fatalf("Heartbeat failed: %v", err)
				}
			}
			if status == TaskStatusCancelling {
				if err := repo.Cancel(ctx, blocker.ID); err != nil {
					t.Fatalf("Cancel failed: %v", err)
				}
			}

			waiting := newTestTask("default", "waiting")
			waiting.ID = "waiting"
			waiting.LockedResourceType = "document"
			waiting.LockedResourceID = "document-1"
			if _, err := repo.Enqueue(ctx, waiting); err != nil {
				t.Fatalf("Enqueue waiting task failed: %v", err)
			}
			got, err := repo.Lease(ctx, "default", "worker-2", 30*time.Second, 1)
			if err != nil {
				t.Fatalf("Lease waiting task failed: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("Lease returned %d tasks while resource holder is %s, want 0", len(got), status)
			}
		})
	}
}

func TestMemoryQueueRepositoryResourceLockExpiredLeaseDoesNotBlock(t *testing.T) {
	for _, status := range []TaskStatus{TaskStatusLeased, TaskStatusRunning} {
		t.Run(string(status), func(t *testing.T) {
			startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			fc := newFakeClock(startTime)
			repo := NewMemoryQueueRepository()
			repo.SetClock(fc)
			ctx := context.Background()

			blocker := newTestTask("default", "blocker")
			blocker.ID = "blocker"
			blocker.Priority = 1
			blocker.LockedResourceType = "document"
			blocker.LockedResourceID = "document-1"
			if _, err := repo.Enqueue(ctx, blocker); err != nil {
				t.Fatalf("Enqueue blocker failed: %v", err)
			}
			leased, err := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)
			if err != nil || len(leased) != 1 {
				t.Fatalf("Lease blocker returned %d tasks, error=%v; want 1", len(leased), err)
			}
			if status == TaskStatusRunning {
				if err := repo.Heartbeat(ctx, blocker.ID, leased[0].LeaseToken(), 30*time.Second); err != nil {
					t.Fatalf("Heartbeat failed: %v", err)
				}
			}

			waiting := newTestTask("default", "waiting")
			waiting.ID = "waiting"
			waiting.Priority = 2
			waiting.LockedResourceType = "document"
			waiting.LockedResourceID = "document-1"
			if _, err := repo.Enqueue(ctx, waiting); err != nil {
				t.Fatalf("Enqueue waiting task failed: %v", err)
			}
			fc.Add(31 * time.Second)

			got, err := repo.Lease(ctx, "default", "worker-2", 30*time.Second, 2)
			if err != nil || len(got) != 1 {
				t.Fatalf("Lease after expiry returned %d tasks, error=%v; want 1", len(got), err)
			}
			if got[0].ID != waiting.ID {
				t.Fatalf("leased task %q after expiry, want higher-priority task %q", got[0].ID, waiting.ID)
			}
		})
	}
}

func TestLeaseExpiry(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fc := newFakeClock(startTime)
	repo := NewMemoryQueueRepository()
	repo.SetClock(fc)
	ctx := context.Background()

	_, _ = repo.Enqueue(ctx, newTestTask("default", ""))

	leased, err := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)
	if err != nil || len(leased) != 1 {
		t.Fatalf("first lease failed")
	}

	leased2, err := repo.Lease(ctx, "default", "worker-2", 30*time.Second, 1)
	if err != nil {
		t.Fatalf("second lease failed: %v", err)
	}
	if len(leased2) != 0 {
		t.Errorf("expected 0 tasks before expiry, got %d", len(leased2))
	}

	fc.Add(31 * time.Second)

	leased3, err := repo.Lease(ctx, "default", "worker-2", 30*time.Second, 1)
	if err != nil {
		t.Fatalf("third lease failed: %v", err)
	}
	if len(leased3) != 1 {
		t.Errorf("expected 1 task after expiry, got %d", len(leased3))
	}
	if leased3[0].LeaseHolder != "worker-2" {
		t.Errorf("expected lease holder worker-2, got %s", leased3[0].LeaseHolder)
	}
	if leased3[0].Attempt != 2 {
		t.Errorf("expected attempt 2, got %d", leased3[0].Attempt)
	}
}

func TestLeaseGenerationMonotonicAcrossRetries(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fc := newFakeClock(startTime)
	repo := NewMemoryQueueRepository()
	repo.SetClock(fc)
	ctx := context.Background()

	task, err := repo.Enqueue(ctx, newTestTask("default", "lease-generation"))
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	first, err := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first Lease failed: tasks=%d err=%v", len(first), err)
	}
	if first[0].LeaseGeneration != 1 || first[0].Attempt != 1 {
		t.Fatalf("first lease: generation=%d attempt=%d, want generation=1 attempt=1", first[0].LeaseGeneration, first[0].Attempt)
	}

	fc.Add(31 * time.Second)
	second, err := repo.Lease(ctx, "default", "worker-2", 30*time.Second, 1)
	if err != nil || len(second) != 1 {
		t.Fatalf("Lease after expiry failed: tasks=%d err=%v", len(second), err)
	}
	if second[0].LeaseGeneration != first[0].LeaseGeneration+1 || second[0].Attempt != 2 {
		t.Fatalf("lease after expiry: generation=%d attempt=%d, want generation=%d attempt=2", second[0].LeaseGeneration, second[0].Attempt, first[0].LeaseGeneration+1)
	}

	if err := repo.Fail(ctx, task.ID, second[0].LeaseToken(), TaskFailure{Retryable: false, Message: "operator retry"}); err != nil {
		t.Fatalf("Fail failed: %v", err)
	}
	retried, err := repo.Retry(ctx, task.ID)
	if err != nil {
		t.Fatalf("Retry failed: %v", err)
	}
	if retried.LeaseGeneration != second[0].LeaseGeneration || retried.Attempt != 0 {
		t.Fatalf("manual retry: generation=%d attempt=%d, want generation=%d attempt=0", retried.LeaseGeneration, retried.Attempt, second[0].LeaseGeneration)
	}

	third, err := repo.Lease(ctx, "default", "worker-3", 30*time.Second, 1)
	if err != nil || len(third) != 1 {
		t.Fatalf("Lease after manual retry failed: tasks=%d err=%v", len(third), err)
	}
	if third[0].LeaseGeneration != second[0].LeaseGeneration+1 || third[0].Attempt != 1 {
		t.Fatalf("lease after manual retry: generation=%d attempt=%d, want generation=%d attempt=1", third[0].LeaseGeneration, third[0].Attempt, second[0].LeaseGeneration+1)
	}
}

func TestMemoryQueueRepositoryLeaseFencing(t *testing.T) {
	type leaseOperation func(context.Context, *MemoryQueueRepository, string, LeaseToken) error
	tests := []struct {
		name       string
		prepare    leaseOperation
		operation  leaseOperation
		wantStatus TaskStatus
	}{
		{
			name: "heartbeat",
			operation: func(ctx context.Context, repo *MemoryQueueRepository, taskID string, token LeaseToken) error {
				return repo.Heartbeat(ctx, taskID, token, 30*time.Second)
			},
			wantStatus: TaskStatusRunning,
		},
		{
			name: "complete",
			prepare: func(ctx context.Context, repo *MemoryQueueRepository, taskID string, token LeaseToken) error {
				return repo.Heartbeat(ctx, taskID, token, 30*time.Second)
			},
			operation: func(ctx context.Context, repo *MemoryQueueRepository, taskID string, token LeaseToken) error {
				return repo.Complete(ctx, taskID, token, TaskResult{})
			},
			wantStatus: TaskStatusSucceeded,
		},
		{
			name: "fail",
			operation: func(ctx context.Context, repo *MemoryQueueRepository, taskID string, token LeaseToken) error {
				return repo.Fail(ctx, taskID, token, TaskFailure{Retryable: false, Message: "terminal"})
			},
			wantStatus: TaskStatusFailedTerminal,
		},
		{
			name: "mark cancelled",
			prepare: func(ctx context.Context, repo *MemoryQueueRepository, taskID string, _ LeaseToken) error {
				return repo.Cancel(ctx, taskID)
			},
			operation: func(ctx context.Context, repo *MemoryQueueRepository, taskID string, token LeaseToken) error {
				return repo.MarkCancelled(ctx, taskID, token)
			},
			wantStatus: TaskStatusCancelled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			fc := newFakeClock(startTime)
			repo := NewMemoryQueueRepository()
			repo.SetClock(fc)

			task, err := repo.Enqueue(ctx, newTestTask("default", "fence-"+tt.name))
			if err != nil {
				t.Fatalf("Enqueue failed: %v", err)
			}
			first, err := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)
			if err != nil || len(first) != 1 {
				t.Fatalf("first Lease tasks=%d error=%v", len(first), err)
			}

			fc.Add(31 * time.Second)
			second, err := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)
			if err != nil || len(second) != 1 {
				t.Fatalf("second Lease tasks=%d error=%v", len(second), err)
			}
			oldToken := first[0].LeaseToken()
			newToken := second[0].LeaseToken()
			if oldToken.Holder != newToken.Holder || oldToken.Generation == newToken.Generation {
				t.Fatalf("tokens do not isolate attempts: old=%+v new=%+v", oldToken, newToken)
			}

			if tt.prepare != nil {
				if err := tt.prepare(ctx, repo, task.ID, newToken); err != nil {
					t.Fatalf("prepare failed: %v", err)
				}
			}
			before, _, err := repo.Get(ctx, task.ID)
			if err != nil {
				t.Fatalf("Get before stale operation failed: %v", err)
			}
			eventsBefore, _, err := repo.ListEvents(ctx, task.ID, "", 100)
			if err != nil {
				t.Fatalf("ListEvents before stale operation failed: %v", err)
			}

			if err := tt.operation(ctx, repo, task.ID, oldToken); !errors.Is(err, ErrLeaseLost) {
				t.Fatalf("stale operation error=%v, want ErrLeaseLost", err)
			}
			after, _, err := repo.Get(ctx, task.ID)
			if err != nil {
				t.Fatalf("Get after stale operation failed: %v", err)
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("stale operation changed task:\nbefore=%+v\nafter=%+v", before, after)
			}
			eventsAfter, _, err := repo.ListEvents(ctx, task.ID, "", 100)
			if err != nil {
				t.Fatalf("ListEvents after stale operation failed: %v", err)
			}
			if len(eventsAfter) != len(eventsBefore) {
				t.Fatalf("stale operation appended event: before=%d after=%d", len(eventsBefore), len(eventsAfter))
			}

			if err := tt.operation(ctx, repo, task.ID, newToken); err != nil {
				t.Fatalf("current operation failed: %v", err)
			}
			got, _, err := repo.Get(ctx, task.ID)
			if err != nil {
				t.Fatalf("Get after current operation failed: %v", err)
			}
			if got.Status != tt.wantStatus {
				t.Fatalf("current operation status=%s, want %s", got.Status, tt.wantStatus)
			}
		})
	}
}

func TestCancelQueuedTask(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	task, _ := repo.Enqueue(ctx, newTestTask("default", ""))

	err := repo.Cancel(ctx, task.ID)
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	got, _, _ := repo.Get(ctx, task.ID)
	if got.Status != TaskStatusCancelled {
		t.Errorf("expected status %s, got %s", TaskStatusCancelled, got.Status)
	}

	leased, _ := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)
	if len(leased) != 0 {
		t.Errorf("cancelled task should not be leasable, got %d", len(leased))
	}
}

func TestCancelRunningTask(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	task, _ := repo.Enqueue(ctx, newTestTask("default", ""))
	leased, _ := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)
	repo.Heartbeat(ctx, leased[0].ID, leased[0].LeaseToken(), 30*time.Second)

	err := repo.Cancel(ctx, task.ID)
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	got, _, _ := repo.Get(ctx, task.ID)
	if got.Status != TaskStatusCancelling {
		t.Errorf("expected status %s, got %s", TaskStatusCancelling, got.Status)
	}
}

func TestIdempotencyKey(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	task1, err := repo.Enqueue(ctx, newTestTask("default", "idem-key-1"))
	if err != nil {
		t.Fatalf("first Enqueue failed: %v", err)
	}

	task2, err := repo.Enqueue(ctx, newTestTask("default", "idem-key-1"))
	if err != nil {
		t.Fatalf("second Enqueue failed: %v", err)
	}

	if task1.ID != task2.ID {
		t.Errorf("expected same task ID for idempotent enqueue, got %s and %s", task1.ID, task2.ID)
	}

	tasks, _, _ := repo.List(ctx, TaskFilter{Pool: "default"})
	if len(tasks) != 1 {
		t.Errorf("expected 1 task with idempotency key, got %d", len(tasks))
	}
}

func TestPriorityOrdering(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	taskLow := newTestTask("default", "low")
	taskLow.Priority = 1
	repo.Enqueue(ctx, taskLow)

	taskHigh := newTestTask("default", "high")
	taskHigh.Priority = 10
	repo.Enqueue(ctx, taskHigh)

	taskMid := newTestTask("default", "mid")
	taskMid.Priority = 5
	repo.Enqueue(ctx, taskMid)

	leased, _ := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 3)
	if len(leased) != 3 {
		t.Fatalf("expected 3 leased tasks, got %d", len(leased))
	}

	if leased[0].Priority != 10 {
		t.Errorf("expected first task priority 10, got %d", leased[0].Priority)
	}
	if leased[1].Priority != 5 {
		t.Errorf("expected second task priority 5, got %d", leased[1].Priority)
	}
	if leased[2].Priority != 1 {
		t.Errorf("expected third task priority 1, got %d", leased[2].Priority)
	}
}

func TestPoolIsolation(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	repo.Enqueue(ctx, newTestTask("pool-a", "a"))
	repo.Enqueue(ctx, newTestTask("pool-b", "b"))

	leasedA, _ := repo.Lease(ctx, "pool-a", "worker-1", 30*time.Second, 10)
	if len(leasedA) != 1 {
		t.Errorf("expected 1 task in pool-a, got %d", len(leasedA))
	}

	leasedB, _ := repo.Lease(ctx, "pool-b", "worker-1", 30*time.Second, 10)
	if len(leasedB) != 1 {
		t.Errorf("expected 1 task in pool-b, got %d", len(leasedB))
	}
}

func TestListWithFilter(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	task1, _ := repo.Enqueue(ctx, newTestTask("default", "t1"))
	task2, _ := repo.Enqueue(ctx, newTestTask("default", "t2"))
	task3 := newTestTask("other", "t3")
	task3.Type = "other-type"
	repo.Enqueue(ctx, task3)

	tasks, _, err := repo.List(ctx, TaskFilter{Pool: "default"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks in default pool, got %d", len(tasks))
	}

	tasks2, _, _ := repo.List(ctx, TaskFilter{Status: TaskStatusQueued})
	if len(tasks2) != 3 {
		t.Errorf("expected 3 queued tasks, got %d", len(tasks2))
	}

	_ = task1
	_ = task2
}

func TestRunAfter(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fc := newFakeClock(startTime)
	repo := NewMemoryQueueRepository()
	repo.SetClock(fc)
	ctx := context.Background()

	task := newTestTask("default", "delayed")
	task.RunAfter = startTime.Add(1 * time.Hour)
	repo.Enqueue(ctx, task)

	leased, _ := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)
	if len(leased) != 0 {
		t.Errorf("expected 0 tasks before run_after, got %d", len(leased))
	}

	fc.Add(61 * time.Minute)

	leased2, _ := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)
	if len(leased2) != 1 {
		t.Errorf("expected 1 task after run_after, got %d", len(leased2))
	}
}

func TestHeartbeatWrongHolder(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	task, _ := repo.Enqueue(ctx, newTestTask("default", ""))
	leased, _ := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)

	wrongToken := leased[0].LeaseToken()
	wrongToken.Holder = "worker-2"
	err := repo.Heartbeat(ctx, leased[0].ID, wrongToken, 30*time.Second)
	if err == nil {
		t.Error("expected error for wrong lease holder")
	}

	_ = task
}
