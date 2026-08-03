package integration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/optimizer"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/storage/postgres"
	"github.com/shikanon/orag/internal/taskqueue"
)

type recoveryCandidateRunner struct {
	mu    sync.Mutex
	calls map[string]int
}

func (r *recoveryCandidateRunner) RunCandidate(_ context.Context, req optimizer.CandidateRunRequest) (optimizer.CandidateRunResult, error) {
	r.mu.Lock()
	r.calls[req.Candidate.ID]++
	r.mu.Unlock()
	return optimizer.CandidateRunResult{
		CandidateID:   req.Candidate.ID,
		EvaluationRun: eval.RunResult{ID: "eval_" + req.Candidate.ID},
		Metrics:       map[string]float64{"pairwise_accuracy": float64(req.Candidate.Retrieval.DenseTopK) / 10},
		CleanupStatus: optimizer.CleanupNotRequired,
	}, nil
}

func (r *recoveryCandidateRunner) count(candidateID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[candidateID]
}

func TestPostgresOptimizerExpiredLeaseRecoveredOnce(t *testing.T) {
	application := newIntegrationApp(t)
	application.TaskWorker.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runRepo := postgres.NewRepository(application.Postgres)
	queueRepo := postgres.NewTaskQueueRepository(application.Postgres)
	now := time.Now().UTC()
	runID := id.New("opt_recovery")
	taskID := id.New("task_recovery")
	firstID := id.New("cand_done")
	secondID := id.New("cand_pending")
	run := optimizer.OptimizationRun{
		ID: runID, TenantID: testTenantID, Objective: optimizer.ObjectiveSpec{Maximize: "pairwise_accuracy"},
		Config: optimizer.RunConfig{Objective: optimizer.ObjectiveSpec{Maximize: "pairwise_accuracy"}, SelectionSplit: "eval"},
		Status: optimizer.RunStatusQueued, CurrentTaskID: taskID, SamplingStrategy: optimizer.SearchStrategyGrid,
		SearchSpaceSize: 2, SampledCandidateCount: 2, Checkpoint: optimizer.Checkpoint{Stage: "submitted"},
		CreatedAt: now, UpdatedAt: now,
	}
	candidates := []optimizer.OptimizationCandidate{
		{ID: firstID, OptimizationRunID: runID, Config: optimizer.CandidateConfig{ID: firstID, Retrieval: optimizer.RetrievalCandidate{DenseTopK: 5}}, Status: optimizer.CandidateStatusQueued, CleanupStatus: optimizer.CleanupNotRequired, CreatedAt: now, UpdatedAt: now},
		{ID: secondID, OptimizationRunID: runID, Config: optimizer.CandidateConfig{ID: secondID, Retrieval: optimizer.RetrievalCandidate{DenseTopK: 9}}, Status: optimizer.CandidateStatusQueued, CleanupStatus: optimizer.CleanupNotRequired, CreatedAt: now, UpdatedAt: now},
	}
	task := taskqueue.Task{
		ID: taskID, TenantID: testTenantID, Type: optimizer.OptimizationTaskType, Pool: optimizer.OptimizationPool,
		Payload: []byte(fmt.Sprintf(`{"version":1,"run_id":%q}`, runID)), MaxAttempts: 3,
		LockedResourceType: optimizer.OptimizationResourceType, LockedResourceID: runID,
		RunAfter: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := runRepo.CreateOptimizationRunWithTask(ctx, run, candidates, task); err != nil {
		t.Fatalf("create durable optimizer fixture: %v", err)
	}

	leasedA, err := queueRepo.Lease(ctx, optimizer.OptimizationPool, "worker-a", time.Minute, 1)
	if err != nil || len(leasedA) != 1 {
		t.Fatalf("worker A lease = %#v error=%v", leasedA, err)
	}
	if err := queueRepo.Heartbeat(ctx, taskID, leasedA[0].LeaseToken(), time.Minute); err != nil {
		t.Fatal(err)
	}
	leaseA := optimizer.ExecutionLease{TaskID: taskID, Holder: "worker-a", Generation: leasedA[0].LeaseGeneration}
	if err := runRepo.ClaimOptimizationExecution(ctx, testTenantID, "", runID, leaseA); err != nil {
		t.Fatal(err)
	}
	ctxA := optimizer.ContextWithExecutionLease(ctx, leaseA)
	stored, _, _ := runRepo.GetOptimizationRun(ctxA, testTenantID, runID)
	stored.Status = optimizer.RunStatusRunning
	if swapped, err := runRepo.CompareAndSwapOptimizationRun(ctxA, stored, optimizer.RunStatusQueued); err != nil || !swapped {
		t.Fatalf("claim run = %v error=%v", swapped, err)
	}
	candidates[0].Status = optimizer.CandidateStatusScored
	candidates[0].Metrics = map[string]float64{"pairwise_accuracy": 0.5}
	if err := runRepo.UpdateOptimizationCandidate(ctxA, candidates[0]); err != nil {
		t.Fatal(err)
	}
	candidates[1].Status = optimizer.CandidateStatusRunning
	if err := runRepo.UpdateOptimizationCandidate(ctxA, candidates[1]); err != nil {
		t.Fatal(err)
	}
	stored.CompletedCandidateCount = 1
	stored.Checkpoint.CompletedCandidateIDs = []string{firstID}
	if err := runRepo.UpdateOptimizationRun(ctxA, stored); err != nil {
		t.Fatal(err)
	}

	if leased, err := queueRepo.Lease(ctx, optimizer.OptimizationPool, "worker-before-expiry", time.Minute, 1); err != nil || len(leased) != 0 {
		t.Fatalf("lease before expiry = %#v error=%v", leased, err)
	}
	if _, err := application.Postgres.Exec(ctx, `UPDATE task_queue SET lease_expires_at=NOW()-INTERVAL '1 second' WHERE id=$1`, taskID); err != nil {
		t.Fatal(err)
	}
	candidates[1].Error = "stale"
	if err := runRepo.UpdateOptimizationCandidate(ctxA, candidates[1]); !errors.Is(err, optimizer.ErrOptimizationOwnershipLost) {
		t.Fatalf("stale worker update error = %v, want ownership lost", err)
	}

	start := make(chan struct{})
	results := make(chan []taskqueue.Task, 2)
	errs := make(chan error, 2)
	for _, worker := range []string{"worker-b", "worker-c"} {
		worker := worker
		go func() {
			<-start
			leased, err := queueRepo.Lease(ctx, optimizer.OptimizationPool, worker, time.Minute, 1)
			results <- leased
			errs <- err
		}()
	}
	close(start)
	var recovered taskqueue.Task
	leaseCount := 0
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		leased := <-results
		leaseCount += len(leased)
		if len(leased) == 1 {
			recovered = leased[0]
		}
	}
	if leaseCount != 1 || recovered.LeaseGeneration <= leaseA.Generation {
		t.Fatalf("recovery leases = %d task=%#v", leaseCount, recovered)
	}
	if err := queueRepo.Heartbeat(ctx, taskID, recovered.LeaseToken(), time.Minute); err != nil {
		t.Fatal(err)
	}
	leaseB := optimizer.ExecutionLease{TaskID: taskID, Holder: recovered.LeaseHolder, Generation: recovered.LeaseGeneration}
	if err := runRepo.ClaimOptimizationExecution(ctx, testTenantID, "", runID, leaseB); err != nil {
		t.Fatal(err)
	}

	runner := &recoveryCandidateRunner{calls: map[string]int{}}
	service := &optimizer.Service{Repository: runRepo, Runner: runner}
	ctxB := optimizer.ContextWithExecutionLease(ctx, leaseB)
	if err := service.RunPending(ctxB, testTenantID, runID, optimizer.SubmitRequest{}); err != nil {
		t.Fatalf("recovered optimizer run: %v", err)
	}
	if err := queueRepo.Complete(ctx, taskID, recovered.LeaseToken(), taskqueue.TaskResult{}); err != nil {
		t.Fatal(err)
	}
	finalRun, found, err := runRepo.GetOptimizationRun(ctx, testTenantID, runID)
	if err != nil || !found || finalRun.Status != optimizer.RunStatusCompleted {
		t.Fatalf("final run = %#v found=%v error=%v", finalRun, found, err)
	}
	finalTask, found, err := queueRepo.Get(ctx, taskID)
	if err != nil || !found || finalTask.Status != taskqueue.TaskStatusSucceeded {
		t.Fatalf("final task = %#v found=%v error=%v", finalTask, found, err)
	}
	if runner.count(firstID) != 0 || runner.count(secondID) != 1 {
		t.Fatalf("runner calls done/pending = %d/%d", runner.count(firstID), runner.count(secondID))
	}
}
