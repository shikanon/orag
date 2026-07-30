package taskqueue

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testHandler struct {
	mu           sync.Mutex
	handleFunc   func(ctx context.Context, task Task, reporter ProgressReporter) error
	startedCount int32
	runningCount int32
	maxRunning   int32
}

func newTestHandler(fn func(ctx context.Context, task Task, reporter ProgressReporter) error) *testHandler {
	return &testHandler{
		handleFunc: fn,
	}
}

func (h *testHandler) Handle(ctx context.Context, task Task, reporter ProgressReporter) error {
	atomic.AddInt32(&h.startedCount, 1)
	current := atomic.AddInt32(&h.runningCount, 1)
	defer atomic.AddInt32(&h.runningCount, -1)

	for {
		maxRunning := atomic.LoadInt32(&h.maxRunning)
		if current <= maxRunning {
			break
		}
		if atomic.CompareAndSwapInt32(&h.maxRunning, maxRunning, current) {
			break
		}
	}

	return h.handleFunc(ctx, task, reporter)
}

type retryableErr struct {
	msg       string
	retryable bool
}

type heartbeatFailureRepository struct {
	QueueRepository
	heartbeatErr       error
	failWhenStopped    bool
	heartbeatStarted   chan struct{}
	heartbeatCalls     int32
	completeCalls      int32
	failCalls          int32
	markCancelledCalls int32
}

func (r *heartbeatFailureRepository) Heartbeat(ctx context.Context, taskID string, token LeaseToken, leaseDuration time.Duration) error {
	if atomic.AddInt32(&r.heartbeatCalls, 1) == 2 {
		if r.heartbeatStarted != nil {
			close(r.heartbeatStarted)
		}
		if r.failWhenStopped {
			<-ctx.Done()
		}
		return r.heartbeatErr
	}
	return r.QueueRepository.Heartbeat(ctx, taskID, token, leaseDuration)
}

func (r *heartbeatFailureRepository) Complete(ctx context.Context, taskID string, token LeaseToken, result TaskResult) error {
	atomic.AddInt32(&r.completeCalls, 1)
	return r.QueueRepository.Complete(ctx, taskID, token, result)
}

func (r *heartbeatFailureRepository) Fail(ctx context.Context, taskID string, token LeaseToken, failure TaskFailure) error {
	atomic.AddInt32(&r.failCalls, 1)
	return r.QueueRepository.Fail(ctx, taskID, token, failure)
}

func (r *heartbeatFailureRepository) MarkCancelled(ctx context.Context, taskID string, token LeaseToken) error {
	atomic.AddInt32(&r.markCancelledCalls, 1)
	return r.QueueRepository.MarkCancelled(ctx, taskID, token)
}

func (e *retryableErr) Error() string {
	return e.msg
}

func (e *retryableErr) Retryable() bool {
	return e.retryable
}

func TestWorkerPoolHeartbeatFailureCancelsHandler(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "storage error", err: errors.New("temporary heartbeat failure")},
		{name: "lease lost", err: ErrLeaseLost},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memoryRepo := NewMemoryQueueRepository()
			repo := &heartbeatFailureRepository{
				QueueRepository: memoryRepo,
				heartbeatErr:    tt.err,
			}
			wp := NewWorkerPool(repo, slog.Default())
			pw := &poolWorker{
				config: PoolConfig{
					LeaseDuration:     time.Second,
					HeartbeatInterval: time.Millisecond,
				},
			}

			parentCtx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			enqueued, err := memoryRepo.Enqueue(parentCtx, Task{
				TenantID:    "test-tenant",
				ProjectID:   "test-project",
				Type:        "test-task",
				Pool:        "default",
				MaxAttempts: 3,
			})
			if err != nil {
				t.Fatalf("Enqueue failed: %v", err)
			}
			leased, err := memoryRepo.Lease(parentCtx, "default", "test-worker", time.Second, 1)
			if err != nil {
				t.Fatalf("Lease failed: %v", err)
			}
			if len(leased) != 1 {
				t.Fatalf("Lease returned %d tasks, want 1", len(leased))
			}

			handlerCancelled := make(chan struct{})
			handler := newTestHandler(func(ctx context.Context, task Task, reporter ProgressReporter) error {
				<-ctx.Done()
				close(handlerCancelled)
				return nil
			})

			executeDone := make(chan struct{})
			go func() {
				defer close(executeDone)
				wp.executeTask(parentCtx, leased[0], handler, pw)
			}()

			select {
			case <-handlerCancelled:
			case <-time.After(time.Second):
				t.Fatal("handler did not receive cancellation after heartbeat failure")
			}
			select {
			case <-executeDone:
			case <-time.After(time.Second):
				t.Fatal("executeTask did not exit after cancelling the handler")
			}

			if got := atomic.LoadInt32(&repo.heartbeatCalls); got != 2 {
				t.Errorf("Heartbeat calls = %d, want 2", got)
			}
			if got := atomic.LoadInt32(&repo.completeCalls); got != 0 {
				t.Errorf("Complete calls = %d, want 0", got)
			}
			if got := atomic.LoadInt32(&repo.failCalls); got != 0 {
				t.Errorf("Fail calls = %d, want 0", got)
			}
			if got := atomic.LoadInt32(&repo.markCancelledCalls); got != 0 {
				t.Errorf("MarkCancelled calls = %d, want 0", got)
			}

			got, ok, err := memoryRepo.Get(parentCtx, enqueued.ID)
			if err != nil {
				t.Fatalf("Get failed: %v", err)
			}
			if !ok {
				t.Fatal("task not found")
			}
			if got.Status != TaskStatusRunning {
				t.Errorf("task status = %s, want %s", got.Status, TaskStatusRunning)
			}
		})
	}
}

func TestWorkerPoolHeartbeatFailureRacingHandlerReturnPreventsCompletion(t *testing.T) {
	memoryRepo := NewMemoryQueueRepository()
	repo := &heartbeatFailureRepository{
		QueueRepository:  memoryRepo,
		heartbeatErr:     errors.New("heartbeat failed while handler returned"),
		failWhenStopped:  true,
		heartbeatStarted: make(chan struct{}),
	}
	wp := NewWorkerPool(repo, slog.Default())
	pw := &poolWorker{
		config: PoolConfig{
			LeaseDuration:     time.Second,
			HeartbeatInterval: time.Millisecond,
		},
	}

	ctx := context.Background()
	enqueued, err := memoryRepo.Enqueue(ctx, Task{
		TenantID:    "test-tenant",
		ProjectID:   "test-project",
		Type:        "test-task",
		Pool:        "default",
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	leased, err := memoryRepo.Lease(ctx, "default", "test-worker", time.Second, 1)
	if err != nil || len(leased) != 1 {
		t.Fatalf("Lease returned %d tasks, error=%v", len(leased), err)
	}

	handler := newTestHandler(func(context.Context, Task, ProgressReporter) error {
		<-repo.heartbeatStarted
		return nil
	})
	wp.executeTask(ctx, leased[0], handler, pw)

	if got := atomic.LoadInt32(&repo.completeCalls); got != 0 {
		t.Fatalf("Complete calls = %d, want 0", got)
	}
	got, ok, err := memoryRepo.Get(ctx, enqueued.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if got.Status != TaskStatusRunning {
		t.Fatalf("task status = %s, want %s", got.Status, TaskStatusRunning)
	}
}

func TestWorkerPool_Concurrency(t *testing.T) {
	repo := NewMemoryQueueRepository()
	logger := slog.Default()

	handler := newTestHandler(func(ctx context.Context, task Task, reporter ProgressReporter) error {
		time.Sleep(200 * time.Millisecond)
		return nil
	})

	wp := NewWorkerPool(repo, logger)
	wp.RegisterPool(PoolConfig{
		Name:              "default",
		Concurrency:       2,
		LeaseDuration:     10 * time.Second,
		HeartbeatInterval: 3 * time.Second,
	})
	wp.RegisterHandler("test-task", handler)

	ctx := context.Background()
	taskIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		task := Task{
			TenantID:    "test-tenant",
			ProjectID:   "test-project",
			Type:        "test-task",
			Pool:        "default",
			MaxAttempts: 3,
		}
		enqueued, err := repo.Enqueue(ctx, task)
		if err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
		taskIDs[i] = enqueued.ID
	}

	wp.Start(ctx)
	defer wp.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		allDone := true
		for _, id := range taskIDs {
			task, ok, _ := repo.Get(ctx, id)
			if !ok || task.Status != TaskStatusSucceeded {
				allDone = false
				break
			}
		}
		if allDone {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	maxRunning := atomic.LoadInt32(&handler.maxRunning)
	if maxRunning > 2 {
		t.Errorf("expected max concurrent tasks <= 2, got %d", maxRunning)
	}

	started := atomic.LoadInt32(&handler.startedCount)
	if started != 5 {
		t.Errorf("expected 5 tasks started, got %d", started)
	}

	tasks, _, err := repo.List(ctx, TaskFilter{Pool: "default", Status: TaskStatusSucceeded})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tasks) != 5 {
		t.Errorf("expected 5 succeeded tasks, got %d", len(tasks))
	}
}

func TestWorkerPool_Heartbeat(t *testing.T) {
	repo := NewMemoryQueueRepository()
	logger := slog.Default()

	var taskStartedOnce sync.Once
	taskStarted := make(chan struct{})

	handler := newTestHandler(func(ctx context.Context, task Task, reporter ProgressReporter) error {
		taskStartedOnce.Do(func() {
			close(taskStarted)
		})
		time.Sleep(800 * time.Millisecond)
		return nil
	})

	wp := NewWorkerPool(repo, logger)
	wp.RegisterPool(PoolConfig{
		Name:              "default",
		Concurrency:       1,
		LeaseDuration:     300 * time.Millisecond,
		HeartbeatInterval: 100 * time.Millisecond,
	})
	wp.RegisterHandler("test-task", handler)

	ctx := context.Background()
	task := Task{
		TenantID:    "test-tenant",
		ProjectID:   "test-project",
		Type:        "test-task",
		Pool:        "default",
		MaxAttempts: 3,
	}
	enqueued, err := repo.Enqueue(ctx, task)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	wp.Start(ctx)
	defer wp.Stop()

	select {
	case <-taskStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("task did not start in time")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, ok, _ := repo.Get(ctx, enqueued.ID)
		if ok && got.Status == TaskStatusSucceeded {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	got, ok, err := repo.Get(ctx, enqueued.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if got.Status != TaskStatusSucceeded {
		t.Errorf("expected task status %s, got %s", TaskStatusSucceeded, got.Status)
	}
	if got.Attempt != 1 {
		t.Errorf("expected attempt 1 (no retry due to heartbeat), got %d", got.Attempt)
	}
}

func TestWorkerPool_Retry(t *testing.T) {
	repo := NewMemoryQueueRepository()
	logger := slog.Default()

	attemptCount := int32(0)

	handler := newTestHandler(func(ctx context.Context, task Task, reporter ProgressReporter) error {
		atomic.AddInt32(&attemptCount, 1)
		return &retryableErr{
			msg:       "temporary error",
			retryable: true,
		}
	})

	wp := NewWorkerPool(repo, logger)
	wp.RegisterPool(PoolConfig{
		Name:              "default",
		Concurrency:       1,
		LeaseDuration:     10 * time.Second,
		HeartbeatInterval: 3 * time.Second,
	})
	wp.RegisterHandler("test-task", handler)

	ctx := context.Background()
	task := Task{
		TenantID:    "test-tenant",
		ProjectID:   "test-project",
		Type:        "test-task",
		Pool:        "default",
		MaxAttempts: 2,
	}
	enqueued, err := repo.Enqueue(ctx, task)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	wp.Start(ctx)
	defer wp.Stop()

	deadline := time.Now().Add(5 * time.Second)
	var got Task
	var ok bool
	for time.Now().Before(deadline) {
		got, ok, _ = repo.Get(ctx, enqueued.ID)
		if ok && got.Status == TaskStatusDeadLetter {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !ok {
		t.Fatal("task not found")
	}

	attempts := atomic.LoadInt32(&attemptCount)
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}

	if got.Status != TaskStatusDeadLetter {
		t.Errorf("expected status %s, got %s", TaskStatusDeadLetter, got.Status)
	}
}

func TestWorkerPool_DeadLetter(t *testing.T) {
	repo := NewMemoryQueueRepository()
	logger := slog.Default()

	attemptCount := int32(0)

	handler := newTestHandler(func(ctx context.Context, task Task, reporter ProgressReporter) error {
		atomic.AddInt32(&attemptCount, 1)
		return &retryableErr{
			msg:       "always fail",
			retryable: true,
		}
	})

	wp := NewWorkerPool(repo, logger)
	wp.RegisterPool(PoolConfig{
		Name:              "default",
		Concurrency:       1,
		LeaseDuration:     10 * time.Second,
		HeartbeatInterval: 3 * time.Second,
	})
	wp.RegisterHandler("test-task", handler)

	ctx := context.Background()
	task := Task{
		TenantID:    "test-tenant",
		ProjectID:   "test-project",
		Type:        "test-task",
		Pool:        "default",
		MaxAttempts: 3,
	}
	enqueued, err := repo.Enqueue(ctx, task)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	wp.Start(ctx)
	defer wp.Stop()

	deadline := time.Now().Add(5 * time.Second)
	var got Task
	var ok bool
	for time.Now().Before(deadline) {
		got, ok, _ = repo.Get(ctx, enqueued.ID)
		if ok && got.Status == TaskStatusDeadLetter {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !ok {
		t.Fatal("task not found")
	}

	if got.Status != TaskStatusDeadLetter {
		t.Errorf("expected status %s, got %s", TaskStatusDeadLetter, got.Status)
	}

	attempts := atomic.LoadInt32(&attemptCount)
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestWorkerPool_Cancel(t *testing.T) {
	repo := NewMemoryQueueRepository()
	logger := slog.Default()

	var taskStartedOnce sync.Once
	taskStarted := make(chan struct{})
	var taskCancelledOnce sync.Once
	taskCancelled := make(chan struct{})

	handler := newTestHandler(func(ctx context.Context, task Task, reporter ProgressReporter) error {
		taskStartedOnce.Do(func() {
			close(taskStarted)
		})
		select {
		case <-ctx.Done():
			taskCancelledOnce.Do(func() {
				close(taskCancelled)
			})
			return ctx.Err()
		case <-time.After(5 * time.Second):
			t.Error("task was not cancelled")
			return errors.New("timeout")
		}
	})

	wp := NewWorkerPool(repo, logger)
	wp.RegisterPool(PoolConfig{
		Name:              "default",
		Concurrency:       1,
		LeaseDuration:     10 * time.Second,
		HeartbeatInterval: 3 * time.Second,
	})
	wp.RegisterHandler("test-task", handler)

	ctx := context.Background()
	task := Task{
		TenantID:    "test-tenant",
		ProjectID:   "test-project",
		Type:        "test-task",
		Pool:        "default",
		MaxAttempts: 3,
	}
	enqueued, err := repo.Enqueue(ctx, task)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	wp.Start(ctx)
	defer wp.Stop()

	select {
	case <-taskStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("task did not start in time")
	}

	err = repo.Cancel(ctx, enqueued.ID)
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	select {
	case <-taskCancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("task was not cancelled in time")
	}

	time.Sleep(500 * time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	var got Task
	var ok bool
	for time.Now().Before(deadline) {
		got, ok, _ = repo.Get(ctx, enqueued.ID)
		if ok && got.Status != TaskStatusRunning && got.Status != TaskStatusLeased && got.Status != TaskStatusCancelling {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !ok {
		t.Fatal("task not found")
	}

	if got.Status != TaskStatusFailedTerminal && got.Status != TaskStatusFailedRetryable && got.Status != TaskStatusCancelled && got.Status != TaskStatusDeadLetter {
		t.Errorf("unexpected task status after cancel: %s", got.Status)
	}
}
