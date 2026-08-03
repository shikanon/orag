package optimizer

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/shikanon/orag/internal/taskqueue"
)

type MemoryRepository struct {
	mu         sync.RWMutex
	runs       map[string]OptimizationRun
	candidates map[string]OptimizationCandidate
	harness    []HarnessRunRecord
	taskQueue  taskqueue.QueueRepository
}

func (r *MemoryRepository) SetTaskQueue(queue taskqueue.QueueRepository) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.taskQueue = queue
}

func (r *MemoryRepository) TaskQueue() taskqueue.QueueRepository {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.taskQueue
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		runs:       map[string]OptimizationRun{},
		candidates: map[string]OptimizationCandidate{},
		taskQueue:  taskqueue.NewMemoryQueueRepository(),
	}
}

func (r *MemoryRepository) CreateOptimizationRun(_ context.Context, run OptimizationRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = run
	return nil
}

func (r *MemoryRepository) CreateOptimizationRunWithCandidates(_ context.Context, run OptimizationRun, candidates []OptimizationCandidate) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	copies := make([]OptimizationCandidate, len(candidates))
	for i, candidate := range candidates {
		candidate.Config.ID = candidate.ID
		copies[i] = candidate
	}
	r.runs[run.ID] = run
	for _, candidate := range copies {
		r.candidates[candidate.ID] = candidate
	}
	return nil
}

func (r *MemoryRepository) CreateOptimizationRunWithTask(ctx context.Context, run OptimizationRun, candidates []OptimizationCandidate, task taskqueue.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.taskQueue == nil {
		return errors.New("optimizer task queue is not configured")
	}
	if _, err := r.taskQueue.Enqueue(ctx, task); err != nil {
		return err
	}
	r.runs[run.ID] = run
	for _, candidate := range candidates {
		candidate.Config.ID = candidate.ID
		r.candidates[candidate.ID] = candidate
	}
	return nil
}

func (r *MemoryRepository) ResumeOptimizationRunWithTask(ctx context.Context, run OptimizationRun, expectedStatus RunStatus, task taskqueue.Task) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.runs[run.ID]
	if !ok || current.TenantID != run.TenantID || current.Status != expectedStatus {
		return false, nil
	}
	if r.taskQueue == nil {
		return false, errors.New("optimizer task queue is not configured")
	}
	if _, err := r.taskQueue.Enqueue(ctx, task); err != nil {
		return false, err
	}
	r.runs[run.ID] = run
	return true, nil
}

func (r *MemoryRepository) CancelOptimizationRunWithTask(ctx context.Context, tenantID, runID, reason string, now time.Time) (OptimizationRun, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID {
		return OptimizationRun{}, false, nil
	}
	if run.Status == RunStatusCanceled || run.Status == RunStatusCompleted || run.Status == RunStatusFailed || run.Status == RunStatusBudgetStopped {
		return run, true, nil
	}
	if r.taskQueue == nil {
		return OptimizationRun{}, false, errors.New("optimizer task queue is not configured")
	}
	task, found, err := r.taskQueue.Get(ctx, run.CurrentTaskID)
	if err != nil {
		return OptimizationRun{}, false, err
	}
	run.Status = RunStatusCanceling
	if !found || task.Status == taskqueue.TaskStatusQueued || task.Status == taskqueue.TaskStatusFailedRetryable ||
		task.Status == taskqueue.TaskStatusLeased || task.Status == taskqueue.TaskStatusCancelled ||
		task.Status == taskqueue.TaskStatusFailedTerminal || task.Status == taskqueue.TaskStatusDeadLetter {
		run.Status = RunStatusCanceled
	}
	run.StatusReason = reason
	run.CancelRequestedAt = &now
	run.Checkpoint.CancelRequestedAt = &now
	run.Checkpoint.StatusReason = reason
	run.Checkpoint.Stage = string(run.Status)
	run.UpdatedAt = now
	if found {
		if err := r.taskQueue.Cancel(ctx, task.ID); err != nil {
			return OptimizationRun{}, false, err
		}
		if task.Status == taskqueue.TaskStatusLeased {
			if err := r.taskQueue.MarkCancelled(ctx, task.ID, task.LeaseToken()); err != nil {
				return OptimizationRun{}, false, err
			}
		}
	}
	r.runs[runID] = run
	return run, true, nil
}

func (r *MemoryRepository) ClaimOptimizationExecution(ctx context.Context, tenantID, projectID, runID string, lease ExecutionLease) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.taskQueue == nil {
		return errors.New("optimizer task queue is not configured")
	}
	task, ok, err := r.taskQueue.Get(ctx, lease.TaskID)
	if err != nil {
		return err
	}
	if !ok || task.TenantID != tenantID || task.ProjectID != projectID || task.LeaseToken() != (taskqueue.LeaseToken{Holder: lease.Holder, Generation: lease.Generation}) ||
		(task.Status != taskqueue.TaskStatusLeased && task.Status != taskqueue.TaskStatusRunning) {
		return ErrOptimizationOwnershipLost
	}
	if validator, ok := r.taskQueue.(interface {
		OwnsActiveLease(context.Context, string, taskqueue.LeaseToken, bool) bool
	}); ok && !validator.OwnsActiveLease(ctx, lease.TaskID, task.LeaseToken(), false) {
		return ErrOptimizationOwnershipLost
	}
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID || run.ProjectID != projectID || run.CurrentTaskID != lease.TaskID || run.ExecutionGeneration >= lease.Generation {
		return ErrOptimizationOwnershipLost
	}
	switch run.Status {
	case RunStatusQueued, RunStatusCanceling:
	case RunStatusRunning:
		completed := run.Checkpoint.completedSet()
		for candidateID, candidate := range r.candidates {
			if candidate.OptimizationRunID == run.ID &&
				(candidate.Status == CandidateStatusRunning || candidate.Status == CandidateStatusEvaluated || candidate.Status == CandidateStatusJudged) {
				if _, done := completed[candidate.ID]; !done {
					candidate.Status = CandidateStatusQueued
					candidate.UpdatedAt = time.Now().UTC()
					r.candidates[candidateID] = candidate
				}
			}
		}
		run.Status = RunStatusQueued
		run.Checkpoint.Stage = "recovered"
	default:
		return ErrOptimizationOwnershipLost
	}
	run.ExecutionGeneration = lease.Generation
	run.UpdatedAt = time.Now().UTC()
	r.runs[run.ID] = run
	return nil
}

func (r *MemoryRepository) GetOptimizationRun(_ context.Context, tenantID, runID string) (OptimizationRun, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID {
		return OptimizationRun{}, false, nil
	}
	return run, true, nil
}

func (r *MemoryRepository) GetOptimizationRunInProject(_ context.Context, tenantID, projectID, runID string) (OptimizationRun, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID || run.ProjectID != projectID {
		return OptimizationRun{}, false, nil
	}
	return run, true, nil
}

func (r *MemoryRepository) UpdateOptimizationRun(ctx context.Context, run OptimizationRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ownsExecutionLocked(ctx, run.ID) {
		return ErrOptimizationOwnershipLost
	}
	r.runs[run.ID] = run
	return nil
}

func (r *MemoryRepository) CompareAndSwapOptimizationRun(ctx context.Context, run OptimizationRun, expectedStatus RunStatus) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ownsExecutionLocked(ctx, run.ID) {
		return false, ErrOptimizationOwnershipLost
	}
	current, ok := r.runs[run.ID]
	if !ok || current.TenantID != run.TenantID || current.Status != expectedStatus {
		return false, nil
	}
	r.runs[run.ID] = run
	return true, nil
}

func (r *MemoryRepository) CreateOptimizationCandidate(_ context.Context, candidate OptimizationCandidate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	candidate.Config.ID = candidate.ID
	r.candidates[candidate.ID] = candidate
	return nil
}

func (r *MemoryRepository) UpdateOptimizationCandidate(ctx context.Context, candidate OptimizationCandidate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ownsExecutionLocked(ctx, candidate.OptimizationRunID) {
		return ErrOptimizationOwnershipLost
	}
	r.candidates[candidate.ID] = candidate
	return nil
}

func (r *MemoryRepository) CompareAndSwapOptimizationCandidate(ctx context.Context, candidate OptimizationCandidate, expectedStatus CandidateStatus) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ownsExecutionLocked(ctx, candidate.OptimizationRunID) {
		return false, ErrOptimizationOwnershipLost
	}
	current, ok := r.candidates[candidate.ID]
	if !ok || current.OptimizationRunID != candidate.OptimizationRunID || current.Status != expectedStatus {
		return false, nil
	}
	r.candidates[candidate.ID] = candidate
	return true, nil
}

func (r *MemoryRepository) ownsExecutionLocked(ctx context.Context, runID string) bool {
	lease, fenced := ExecutionLeaseFromContext(ctx)
	if !fenced {
		return true
	}
	run, ok := r.runs[runID]
	if !ok || run.CurrentTaskID != lease.TaskID || run.ExecutionGeneration != lease.Generation || r.taskQueue == nil {
		return false
	}
	task, found, err := r.taskQueue.Get(ctx, lease.TaskID)
	if err != nil || !found || task.LeaseToken() != (taskqueue.LeaseToken{Holder: lease.Holder, Generation: lease.Generation}) {
		return false
	}
	if validator, ok := r.taskQueue.(interface {
		OwnsActiveLease(context.Context, string, taskqueue.LeaseToken, bool) bool
	}); ok {
		return validator.OwnsActiveLease(ctx, lease.TaskID, task.LeaseToken(), true)
	}
	return task.Status == taskqueue.TaskStatusLeased || task.Status == taskqueue.TaskStatusRunning || task.Status == taskqueue.TaskStatusCancelling
}

func (r *MemoryRepository) ListOptimizationCandidates(_ context.Context, tenantID, runID string) ([]OptimizationCandidate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID {
		return nil, nil
	}
	out := make([]OptimizationCandidate, 0, len(r.candidates))
	for _, candidate := range r.candidates {
		if candidate.OptimizationRunID == runID {
			out = append(out, candidate)
		}
	}
	sortOptimizationCandidates(out)
	return out, nil
}

func (r *MemoryRepository) StoreHarnessRun(_ context.Context, run HarnessRunRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.harness = append(r.harness, run)
	return nil
}

func sortOptimizationCandidates(candidates []OptimizationCandidate) {
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].ID < candidates[i].ID {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
}
