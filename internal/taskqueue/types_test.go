package taskqueue

import (
	"context"
	"errors"
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
	if leased[0].LeaseGeneration != 1 {
		t.Errorf("expected lease generation 1, got %d", leased[0].LeaseGeneration)
	}

	err = repo.Heartbeat(ctx, leased[0].ID, "worker-1", leased[0].LeaseGeneration, leased[0].Attempt, 30*time.Second)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
	got, _, _ := repo.Get(ctx, leased[0].ID)
	if got.Status != TaskStatusRunning {
		t.Errorf("expected status %s after heartbeat, got %s", TaskStatusRunning, got.Status)
	}

	err = repo.Complete(ctx, leased[0].ID, "worker-1", leased[0].LeaseGeneration, leased[0].Attempt, TaskResult{})
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

	err := repo.Fail(ctx, leased[0].ID, "worker-1", leased[0].LeaseGeneration, leased[0].Attempt, TaskFailure{
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

	err := repo.Fail(ctx, leased[0].ID, "worker-1", leased[0].LeaseGeneration, leased[0].Attempt, TaskFailure{
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
	repo.Fail(ctx, leased[0].ID, "worker-1", leased[0].LeaseGeneration, leased[0].Attempt, TaskFailure{Retryable: true, Message: "fail 1"})
	fc.Add(300 * time.Millisecond)

	leased, _ = repo.Lease(ctx, "default", "worker-2", 30*time.Second, 1)
	if len(leased) != 1 {
		t.Fatalf("expected 1 leased task on retry")
	}
	if leased[0].Attempt != 2 {
		t.Errorf("expected attempt 2, got %d", leased[0].Attempt)
	}

	repo.Fail(ctx, leased[0].ID, "worker-2", leased[0].LeaseGeneration, leased[0].Attempt, TaskFailure{Retryable: true, Message: "fail 2"})

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
	if leased3[0].LeaseGeneration != 2 {
		t.Errorf("expected lease generation 2, got %d", leased3[0].LeaseGeneration)
	}
}

func TestLeaseGenerationSurvivesManualAttemptReset(t *testing.T) {
	repo := NewMemoryQueueRepository()
	ctx := context.Background()

	task, err := repo.Enqueue(ctx, newTestTask("default", ""))
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	leased, err := repo.Lease(ctx, "default", "worker-1", 30*time.Second, 1)
	if err != nil || len(leased) != 1 {
		t.Fatalf("first Lease = %#v, %v", leased, err)
	}
	if err := repo.Fail(ctx, task.ID, "worker-1", leased[0].LeaseGeneration, leased[0].Attempt, TaskFailure{Message: "terminal"}); err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	retried, err := repo.Retry(ctx, task.ID)
	if err != nil {
		t.Fatalf("Retry failed: %v", err)
	}
	if retried.Attempt != 0 {
		t.Fatalf("attempt after Retry = %d, want 0", retried.Attempt)
	}
	if retried.LeaseGeneration != 1 {
		t.Fatalf("lease generation after Retry = %d, want 1", retried.LeaseGeneration)
	}

	leased, err = repo.Lease(ctx, "default", "worker-2", 30*time.Second, 1)
	if err != nil || len(leased) != 1 {
		t.Fatalf("second Lease = %#v, %v", leased, err)
	}
	if leased[0].Attempt != 1 {
		t.Errorf("attempt after second Lease = %d, want 1", leased[0].Attempt)
	}
	if leased[0].LeaseGeneration != 2 {
		t.Errorf("lease generation after second Lease = %d, want 2", leased[0].LeaseGeneration)
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
	repo.Heartbeat(ctx, leased[0].ID, "worker-1", leased[0].LeaseGeneration, leased[0].Attempt, 30*time.Second)

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

	err := repo.Heartbeat(ctx, leased[0].ID, "worker-2", leased[0].LeaseGeneration, leased[0].Attempt, 30*time.Second)
	if !errors.Is(err, ErrLeaseLost) {
		t.Errorf("error = %v, want ErrLeaseLost", err)
	}

	_ = task
}

func TestLeaseGenerationCASRejectsStaleOwner(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fc := newFakeClock(startTime)
	repo := NewMemoryQueueRepository()
	repo.SetClock(fc)
	ctx := context.Background()

	task, err := repo.Enqueue(ctx, newTestTask("default", ""))
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	leaseA, err := repo.Lease(ctx, "default", "shared-worker", 30*time.Second, 1)
	if err != nil || len(leaseA) != 1 {
		t.Fatalf("lease A = %#v, %v", leaseA, err)
	}

	fc.Add(31 * time.Second)
	leaseB, err := repo.Lease(ctx, "default", "shared-worker", 30*time.Second, 1)
	if err != nil || len(leaseB) != 1 {
		t.Fatalf("lease B = %#v, %v", leaseB, err)
	}
	if leaseA[0].LeaseGeneration == leaseB[0].LeaseGeneration {
		t.Fatalf("lease generation was reused: %d", leaseA[0].LeaseGeneration)
	}

	eventsBefore, _, err := repo.ListEvents(ctx, task.ID, "", 100)
	if err != nil {
		t.Fatalf("ListEvents before stale mutations failed: %v", err)
	}
	staleCalls := []struct {
		name string
		call func() error
	}{
		{
			name: "heartbeat",
			call: func() error {
				return repo.Heartbeat(ctx, task.ID, leaseA[0].LeaseHolder, leaseA[0].LeaseGeneration, leaseA[0].Attempt, 30*time.Second)
			},
		},
		{
			name: "complete",
			call: func() error {
				return repo.Complete(ctx, task.ID, leaseA[0].LeaseHolder, leaseA[0].LeaseGeneration, leaseA[0].Attempt, TaskResult{})
			},
		},
		{
			name: "fail",
			call: func() error {
				return repo.Fail(ctx, task.ID, leaseA[0].LeaseHolder, leaseA[0].LeaseGeneration, leaseA[0].Attempt, TaskFailure{Message: "stale failure"})
			},
		},
	}
	for _, tc := range staleCalls {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if !errors.Is(err, ErrLeaseLost) {
				t.Fatalf("error = %v, want ErrLeaseLost", err)
			}
			var leaseErr *LeaseLostError
			if !errors.As(err, &leaseErr) || leaseErr.TaskID != task.ID {
				t.Fatalf("error = %#v, want LeaseLostError for task %s", err, task.ID)
			}
		})
	}

	got, ok, err := repo.Get(ctx, task.ID)
	if err != nil || !ok {
		t.Fatalf("Get after stale mutations = %#v, %v", got, err)
	}
	if got.Status != TaskStatusLeased ||
		got.LeaseHolder != leaseB[0].LeaseHolder ||
		got.LeaseGeneration != leaseB[0].LeaseGeneration ||
		got.Attempt != leaseB[0].Attempt ||
		!got.LeaseExpiresAt.Equal(leaseB[0].LeaseExpiresAt) {
		t.Fatalf("stale mutations changed lease B: got %#v, want %#v", got, leaseB[0])
	}
	eventsAfter, _, err := repo.ListEvents(ctx, task.ID, "", 100)
	if err != nil {
		t.Fatalf("ListEvents after stale mutations failed: %v", err)
	}
	if len(eventsAfter) != len(eventsBefore) {
		t.Fatalf("stale mutations appended events: before=%d after=%d", len(eventsBefore), len(eventsAfter))
	}

	if err := repo.Complete(ctx, task.ID, leaseB[0].LeaseHolder, leaseB[0].LeaseGeneration, leaseB[0].Attempt, TaskResult{}); err != nil {
		t.Fatalf("lease B Complete failed: %v", err)
	}
	got, ok, err = repo.Get(ctx, task.ID)
	if err != nil || !ok || got.Status != TaskStatusSucceeded {
		t.Fatalf("lease B terminal state = %#v, found=%v, err=%v", got, ok, err)
	}
}
