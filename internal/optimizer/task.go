package optimizer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/taskqueue"
)

const (
	OptimizationTaskType       = "optimization.run"
	OptimizationPool           = "optimizer"
	OptimizationResourceType   = "optimization_run"
	optimizationPayloadVersion = 1
)

var ErrOptimizationOwnershipLost = errors.New("optimization execution ownership lost")

type ExecutionTaskPayload struct {
	Version int    `json:"version"`
	RunID   string `json:"run_id"`
}

type ExecutionLease struct {
	TaskID     string
	Holder     string
	Generation int64
}

type terminalExecutionError struct{ error }

func (terminalExecutionError) Retryable() bool { return false }

type executionLeaseContextKey struct{}

func ContextWithExecutionLease(ctx context.Context, lease ExecutionLease) context.Context {
	return context.WithValue(ctx, executionLeaseContextKey{}, lease)
}

func ExecutionLeaseFromContext(ctx context.Context) (ExecutionLease, bool) {
	lease, ok := ctx.Value(executionLeaseContextKey{}).(ExecutionLease)
	return lease, ok
}

type ExecutionRepository interface {
	CreateOptimizationRunWithTask(context.Context, OptimizationRun, []OptimizationCandidate, taskqueue.Task) error
	ResumeOptimizationRunWithTask(context.Context, OptimizationRun, RunStatus, taskqueue.Task) (bool, error)
	CancelOptimizationRunWithTask(context.Context, string, string, string, time.Time) (OptimizationRun, bool, error)
	ClaimOptimizationExecution(context.Context, string, string, string, ExecutionLease) error
}

func NewExecutionTask(run OptimizationRun, now time.Time) (taskqueue.Task, error) {
	payload, err := json.Marshal(ExecutionTaskPayload{Version: optimizationPayloadVersion, RunID: run.ID})
	if err != nil {
		return taskqueue.Task{}, err
	}
	taskID := id.New("task")
	return taskqueue.Task{
		ID:                 taskID,
		TenantID:           run.TenantID,
		ProjectID:          run.ProjectID,
		Type:               OptimizationTaskType,
		Pool:               OptimizationPool,
		Status:             taskqueue.TaskStatusQueued,
		Payload:            payload,
		IdempotencyKey:     "optimization-execution:" + taskID,
		MaxAttempts:        3,
		LockedResourceType: OptimizationResourceType,
		LockedResourceID:   run.ID,
		RunAfter:           now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}, nil
}

func decodeExecutionTask(task taskqueue.Task) (ExecutionTaskPayload, error) {
	if task.Type != OptimizationTaskType || task.Pool != OptimizationPool {
		return ExecutionTaskPayload{}, fmt.Errorf("invalid optimization task type or pool")
	}
	if task.LockedResourceType != OptimizationResourceType {
		return ExecutionTaskPayload{}, fmt.Errorf("invalid optimization task resource type")
	}
	decoder := json.NewDecoder(bytes.NewReader(task.Payload))
	decoder.DisallowUnknownFields()
	var payload ExecutionTaskPayload
	if err := decoder.Decode(&payload); err != nil {
		return ExecutionTaskPayload{}, fmt.Errorf("decode optimization task payload: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ExecutionTaskPayload{}, fmt.Errorf("decode optimization task payload: trailing data")
	}
	if payload.Version != optimizationPayloadVersion || strings.TrimSpace(payload.RunID) == "" {
		return ExecutionTaskPayload{}, fmt.Errorf("unsupported optimization task payload")
	}
	if payload.RunID != task.LockedResourceID {
		return ExecutionTaskPayload{}, fmt.Errorf("optimization task resource does not match payload")
	}
	return payload, nil
}

func (s *Service) HandleTask(ctx context.Context, task taskqueue.Task, _ taskqueue.ProgressReporter) error {
	payload, err := decodeExecutionTask(task)
	if err != nil {
		return terminalExecutionError{err}
	}
	if strings.TrimSpace(task.TenantID) == "" || strings.TrimSpace(task.LeaseHolder) == "" || task.LeaseGeneration <= 0 {
		return fmt.Errorf("optimization task is not leased")
	}
	lease := ExecutionLease{TaskID: task.ID, Holder: task.LeaseHolder, Generation: task.LeaseGeneration}
	repository, ok := s.repo().(ExecutionRepository)
	if !ok {
		return errors.New("optimizer repository does not support durable execution")
	}
	if err := repository.ClaimOptimizationExecution(ctx, task.TenantID, task.ProjectID, payload.RunID, lease); err != nil {
		return err
	}
	executionCtx := ContextWithExecutionLease(ctx, lease)
	err = s.RunPending(executionCtx, task.TenantID, payload.RunID, SubmitRequest{})
	if ctx.Err() != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(executionCtx), 5*time.Second)
		defer cancel()
		_ = s.finalizeTaskCancellation(cleanupCtx, task.TenantID, payload.RunID)
	}
	if err != nil {
		run, found, getErr := s.repo().GetOptimizationRun(context.WithoutCancel(executionCtx), task.TenantID, payload.RunID)
		if getErr == nil && found && run.Status == RunStatusFailed {
			return terminalExecutionError{err}
		}
	}
	return err
}

func (s *Service) finalizeTaskCancellation(ctx context.Context, tenantID, runID string) error {
	run, ok, err := s.repo().GetOptimizationRun(ctx, tenantID, runID)
	if err != nil || !ok {
		return err
	}
	if run.Status != RunStatusCanceling && run.CancelRequestedAt == nil {
		return nil
	}
	_ = s.cleanupPendingCandidates(ctx, &run)
	return s.transitionRun(ctx, &run, RunStatusCanceled, "canceled", run.StatusReason)
}
