package taskqueue

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/shikanon/orag/internal/platform/clock"
	"github.com/shikanon/orag/internal/platform/id"
)

// PoolConfig defines the configuration for a worker pool.
type PoolConfig struct {
	Name              string
	Concurrency       int
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
}

// WorkerPool manages a collection of worker pools that process tasks from a queue repository.
type WorkerPool struct {
	repo     QueueRepository
	handlers map[string]Handler
	pools    map[string]*poolWorker
	clock    clock.Clock
	logger   *slog.Logger
	wg       sync.WaitGroup
	mu       sync.RWMutex
	running  bool
	workerID string
	cancel   context.CancelFunc
}

// poolWorker manages the scheduler and concurrency for a single pool.
type poolWorker struct {
	config PoolConfig
	stopCh chan struct{}
	sem    chan struct{}
}

type taskProgressReporter struct {
	repo QueueRepository
}

// NewWorkerPool creates a new WorkerPool instance.
func NewWorkerPool(repo QueueRepository, logger *slog.Logger) *WorkerPool {
	if logger == nil {
		logger = slog.Default()
	}
	return &WorkerPool{
		repo:     repo,
		handlers: make(map[string]Handler),
		pools:    make(map[string]*poolWorker),
		clock:    clock.RealClock{},
		logger:   logger,
		workerID: id.New("worker"),
	}
}

// SetClock sets the clock used by the worker pool for testing purposes.
func (wp *WorkerPool) SetClock(c clock.Clock) {
	wp.clock = c
}

// RegisterHandler registers a task handler for the given task type.
func (wp *WorkerPool) RegisterHandler(taskType string, handler Handler) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.handlers[taskType] = handler
}

// RegisterPool registers a worker pool with the given configuration.
func (wp *WorkerPool) RegisterPool(config PoolConfig) {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if config.Concurrency <= 0 {
		config.Concurrency = 1
	}
	if config.LeaseDuration <= 0 {
		config.LeaseDuration = 30 * time.Second
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = config.LeaseDuration / 3
	}

	wp.pools[config.Name] = &poolWorker{
		config: config,
		stopCh: make(chan struct{}),
		sem:    make(chan struct{}, config.Concurrency),
	}
}

// Start starts all registered worker pools.
func (wp *WorkerPool) Start(ctx context.Context) {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.running {
		return
	}
	wp.running = true
	runCtx, cancel := context.WithCancel(ctx)
	wp.cancel = cancel

	for name, pw := range wp.pools {
		wp.wg.Add(1)
		go wp.runScheduler(runCtx, name, pw)
	}
}

// Stop stops all worker pools and waits for running tasks to complete.
func (wp *WorkerPool) Stop() {
	wp.mu.Lock()
	if !wp.running {
		wp.mu.Unlock()
		return
	}
	wp.running = false
	if wp.cancel != nil {
		wp.cancel()
		wp.cancel = nil
	}

	for _, pw := range wp.pools {
		close(pw.stopCh)
	}
	wp.mu.Unlock()

	wp.wg.Wait()
}

func (wp *WorkerPool) runScheduler(ctx context.Context, poolName string, pw *poolWorker) {
	defer wp.wg.Done()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pw.stopCh:
			return
		case <-ticker.C:
			wp.leaseAndDispatch(ctx, poolName, pw)
		}
	}
}

func (wp *WorkerPool) leaseAndDispatch(ctx context.Context, poolName string, pw *poolWorker) {
	availableSlots := cap(pw.sem) - len(pw.sem)
	if availableSlots <= 0 {
		return
	}

	leasedTasks, err := wp.repo.Lease(ctx, poolName, wp.workerID, pw.config.LeaseDuration, availableSlots)
	if err != nil {
		wp.logger.Error("failed to lease tasks", "pool", poolName, "error", err)
		return
	}

	for _, task := range leasedTasks {
		task := task

		wp.mu.RLock()
		handler, ok := wp.handlers[task.Type]
		wp.mu.RUnlock()

		if !ok {
			wp.logger.Warn("no handler registered for task type", "task_type", task.Type, "task_id", task.ID)
			wp.repo.Fail(ctx, task.ID, task.LeaseToken(), TaskFailure{
				Retryable: false,
				ErrorCode: "no_handler",
				Message:   "no handler registered for task type: " + task.Type,
			})
			continue
		}

		select {
		case pw.sem <- struct{}{}:
		default:
			continue
		}

		wp.wg.Add(1)
		go func(t Task, h Handler) {
			defer wp.wg.Done()
			defer func() { <-pw.sem }()
			wp.executeTask(ctx, t, h, pw)
		}(task, handler)
	}
}

func (wp *WorkerPool) executeTask(parentCtx context.Context, task Task, handler Handler, pw *poolWorker) {
	taskCtx, taskCancel := context.WithCancel(parentCtx)
	defer taskCancel()
	heartbeatCtx, heartbeatCancel := context.WithCancel(taskCtx)
	defer heartbeatCancel()
	token := task.LeaseToken()
	// Transition to running before invoking the handler. Waiting for the first
	// periodic heartbeat leaves a short but observable leased-only window and
	// makes cancellation/timeline semantics needlessly ambiguous.
	if err := wp.repo.Heartbeat(taskCtx, task.ID, token, pw.config.LeaseDuration); err != nil {
		wp.logger.Error("failed to start task lease", "task_id", task.ID, "error", err)
		wp.handleCancelledTask(parentCtx, task.ID, token)
		return
	}

	heartbeatResult := make(chan error, 1)
	go func() {
		heartbeatResult <- wp.runHeartbeat(heartbeatCtx, taskCancel, task.ID, token, pw.config.LeaseDuration, pw.config.HeartbeatInterval)
	}()

	cancelMonitorDone := make(chan struct{})
	go func() {
		defer close(cancelMonitorDone)
		wp.monitorCancellation(taskCtx, task.ID, taskCancel)
	}()

	reporter := &taskProgressReporter{repo: wp.repo}

	wp.logger.Debug("task started", "task_id", task.ID, "task_type", task.Type, "pool", task.Pool)

	handlerErr := handler.Handle(taskCtx, task, reporter)

	// Stop heartbeats independently so an in-flight heartbeat can report a
	// storage or lease-loss error before the terminal state is committed.
	heartbeatCancel()
	heartbeatErr := <-heartbeatResult
	isCancelled := taskCtx.Err() != nil || heartbeatErr != nil
	taskCancel()
	<-cancelMonitorDone

	if isCancelled {
		wp.handleCancelledTask(parentCtx, task.ID, token)
		return
	}

	if handlerErr != nil {
		wp.logger.Error("task failed", "task_id", task.ID, "error", handlerErr)

		retryable := isRetryableError(handlerErr)
		failure := TaskFailure{
			Retryable: retryable,
			Message:   handlerErr.Error(),
		}

		if err := wp.repo.Fail(parentCtx, task.ID, token, failure); err != nil {
			wp.logger.Error("failed to mark task as failed", "task_id", task.ID, "error", err)
		}
		return
	}

	if err := wp.repo.Complete(parentCtx, task.ID, token, TaskResult{}); err != nil {
		wp.logger.Error("failed to mark task as complete", "task_id", task.ID, "error", err)
	}
	wp.logger.Debug("task completed", "task_id", task.ID)
}

func (wp *WorkerPool) monitorCancellation(ctx context.Context, taskID string, cancelFunc context.CancelFunc) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			task, ok, err := wp.repo.Get(ctx, taskID)
			if err != nil || !ok {
				return
			}
			if task.Status == TaskStatusCancelling {
				wp.logger.Debug("task cancellation detected", "task_id", taskID)
				cancelFunc()
				return
			}
		}
	}
}

func (wp *WorkerPool) handleCancelledTask(ctx context.Context, taskID string, token LeaseToken) {
	task, ok, err := wp.repo.Get(ctx, taskID)
	if err != nil || !ok {
		return
	}

	if task.Status == TaskStatusCancelling && task.LeaseToken() == token {
		if err := wp.repo.MarkCancelled(ctx, taskID, token); err != nil {
			wp.logger.Error("failed to mark task as cancelled", "task_id", taskID, "error", err)
		}
	}
}

func (wp *WorkerPool) runHeartbeat(ctx context.Context, cancel context.CancelFunc, taskID string, token LeaseToken, leaseDuration, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			err := wp.repo.Heartbeat(ctx, taskID, token, leaseDuration)
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
					return nil
				}
				wp.logger.Error("heartbeat failed", "task_id", taskID, "error", err)
				cancel()
				return err
			}
		}
	}
}

func (r *taskProgressReporter) ReportProgress(ctx context.Context, taskID string, progress int, message string) error {
	return nil
}

type retryableError interface {
	Retryable() bool
}

func isRetryableError(err error) bool {
	if re, ok := err.(retryableError); ok {
		return re.Retryable()
	}
	return true
}
