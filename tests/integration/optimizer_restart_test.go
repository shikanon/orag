package integration

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/optimizer"
	"github.com/shikanon/orag/internal/storage/postgres"
	"github.com/shikanon/orag/internal/taskqueue"
)

const (
	optimizerHelperEnv   = "ORAG_OPTIMIZER_RESTART_HELPER"
	optimizerHelperRunID = "ORAG_OPTIMIZER_RESTART_RUN_ID"
	optimizerHelperMode  = "ORAG_OPTIMIZER_RESTART_MODE"
)

type restartCandidateRunner struct {
	mode string
}

type restartOptimizerHandler struct {
	service *optimizer.Service
}

func (h restartOptimizerHandler) Handle(ctx context.Context, task taskqueue.Task, reporter taskqueue.ProgressReporter) error {
	return h.service.HandleTask(ctx, task, reporter)
}

func (r restartCandidateRunner) RunCandidate(ctx context.Context, req optimizer.CandidateRunRequest) (optimizer.CandidateRunResult, error) {
	if r.mode == "block_second" && req.Candidate.Retrieval.DenseTopK == 2 {
		fmt.Println("CANDIDATE_STARTED")
		select {
		case <-ctx.Done():
			return optimizer.CandidateRunResult{}, ctx.Err()
		}
	}
	return optimizer.CandidateRunResult{
		CandidateID: req.Candidate.ID,
		EvaluationRun: eval.RunResult{
			ID: fmt.Sprintf("eval_%d_%s", os.Getpid(), req.Candidate.ID),
		},
		Metrics:       map[string]float64{"pairwise_accuracy": float64(req.Candidate.Retrieval.DenseTopK) / 10},
		CleanupStatus: optimizer.CleanupNotRequired,
	}, nil
}

// TestOptimizerWorkerHelperProcess is launched by TestOptimizerSurvivesProcessRestart.
// It is inert in the normal test process.
func TestOptimizerWorkerHelperProcess(t *testing.T) {
	if os.Getenv(optimizerHelperEnv) != "1" {
		t.Skip("optimizer restart helper process")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	runID := os.Getenv(optimizerHelperRunID)
	mode := os.Getenv(optimizerHelperMode)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	fmt.Println("READY")
	if mode == "wait" {
		select {}
	}

	runRepo := postgres.NewRepository(pool)
	queueRepo := postgres.NewTaskQueueRepository(pool)
	service := &optimizer.Service{Repository: runRepo, Runner: restartCandidateRunner{mode: mode}}
	worker := taskqueue.NewWorkerPool(queueRepo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	worker.RegisterPool(taskqueue.PoolConfig{Name: optimizer.OptimizationPool, Concurrency: 1, LeaseDuration: time.Second, HeartbeatInterval: 200 * time.Millisecond})
	worker.RegisterHandler(optimizer.OptimizationTaskType, restartOptimizerHandler{service: service})
	worker.Start(ctx)
	defer worker.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
			run, found, err := runRepo.GetOptimizationRun(ctx, testTenantID, runID)
			if err != nil {
				t.Fatal(err)
			}
			if found && (run.Status == optimizer.RunStatusCompleted || run.Status == optimizer.RunStatusFailed ||
				run.Status == optimizer.RunStatusCanceled || run.Status == optimizer.RunStatusBudgetStopped) {
				fmt.Println("TERMINAL")
				return
			}
		}
	}
}

type optimizerHelperProcess struct {
	cmd    *exec.Cmd
	lines  <-chan string
	done   <-chan error
	stderr *bytes.Buffer
}

func startOptimizerHelper(t *testing.T, runID, mode string) optimizerHelperProcess {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestOptimizerWorkerHelperProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), optimizerHelperEnv+"=1", optimizerHelperRunID+"="+runID, optimizerHelperMode+"="+mode)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
	})
	lines := make(chan string, 8)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- strings.TrimSpace(scanner.Text())
		}
	}()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return optimizerHelperProcess{cmd: cmd, lines: lines, done: done, stderr: stderr}
}

func waitOptimizerMarker(t *testing.T, process optimizerHelperProcess, marker string) {
	t.Helper()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line, ok := <-process.lines:
			if ok && line == marker {
				return
			}
			if !ok {
				t.Fatalf("optimizer helper stdout closed before %s; stderr=%s", marker, process.stderr.String())
			}
		case err := <-process.done:
			t.Fatalf("optimizer helper exited before %s: %v stderr=%s", marker, err, process.stderr.String())
		case <-timer.C:
			t.Fatalf("timed out waiting for optimizer helper marker %s; stderr=%s", marker, process.stderr.String())
		}
	}
}

func killOptimizerHelper(t *testing.T, process optimizerHelperProcess) {
	t.Helper()
	if err := process.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-process.done:
	case <-time.After(5 * time.Second):
		t.Fatal("optimizer helper did not exit after kill")
	}
}

func waitOptimizerHelperExit(t *testing.T, process optimizerHelperProcess) {
	t.Helper()
	select {
	case err := <-process.done:
		if err != nil {
			t.Fatalf("optimizer helper exit error: %v stderr=%s", err, process.stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("optimizer helper did not exit after terminal marker")
	}
}

func TestOptimizerSurvivesProcessRestart(t *testing.T) {
	application := newIntegrationApp(t)
	application.TaskWorker.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	runRepo := postgres.NewRepository(application.Postgres)

	for _, test := range []struct {
		name      string
		firstMode string
		killMark  string
	}{
		{name: "queued", firstMode: "wait", killMark: "READY"},
		{name: "running checkpoint", firstMode: "block_second", killMark: "CANDIDATE_STARTED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &optimizer.Service{Repository: runRepo, Runner: restartCandidateRunner{}}
			run, err := service.Submit(ctx, optimizer.SubmitRequest{
				TenantID:  testTenantID,
				Objective: optimizer.ObjectiveSpec{Maximize: "pairwise_accuracy"},
				SearchSpace: optimizer.SearchSpace{Retrieval: optimizer.RetrievalSpace{
					DenseTopK: []int{1, 2},
				}},
				Search:         optimizer.SearchSpec{Strategy: optimizer.SearchStrategyGrid, MaxCandidates: 2},
				SelectionSplit: "eval",
			})
			if err != nil {
				t.Fatal(err)
			}

			first := startOptimizerHelper(t, run.ID, test.firstMode)
			waitOptimizerMarker(t, first, test.killMark)
			var completedEvaluationID string
			if test.firstMode == "block_second" {
				status, found, err := service.Get(ctx, testTenantID, run.ID)
				if err != nil || !found || len(status.Run.Checkpoint.CompletedCandidateIDs) != 1 {
					t.Fatalf("running kill checkpoint = %#v found=%v error=%v", status.Run.Checkpoint, found, err)
				}
				completedID := status.Run.Checkpoint.CompletedCandidateIDs[0]
				for _, candidate := range status.Candidates {
					if candidate.ID == completedID {
						completedEvaluationID = candidate.EvaluationRunID
					}
				}
				if completedEvaluationID == "" {
					t.Fatal("completed candidate evaluation ID was not persisted before kill")
				}
			}
			killOptimizerHelper(t, first)
			if test.firstMode == "block_second" {
				if _, err := application.Postgres.Exec(ctx, `
					UPDATE task_queue SET lease_expires_at=NOW()-INTERVAL '1 second'
					WHERE id=(SELECT current_task_id FROM optimization_runs WHERE id=$1)`, run.ID); err != nil {
					t.Fatal(err)
				}
			}

			restarted := startOptimizerHelper(t, run.ID, "complete")
			waitOptimizerMarker(t, restarted, "READY")
			waitOptimizerMarker(t, restarted, "TERMINAL")
			waitOptimizerHelperExit(t, restarted)

			status, found, err := service.Get(ctx, testTenantID, run.ID)
			if err != nil || !found || status.Run.Status != optimizer.RunStatusCompleted {
				t.Fatalf("run after restart = %#v found=%v error=%v", status.Run, found, err)
			}
			task, found, err := application.TaskQueue.Get(ctx, testTenantID, run.CurrentTaskID)
			if err != nil || !found || task.Status != taskqueue.TaskStatusSucceeded {
				t.Fatalf("task after restart = %#v found=%v error=%v", task, found, err)
			}
			if completedEvaluationID != "" {
				for _, candidate := range status.Candidates {
					if candidate.ID == status.Run.Checkpoint.CompletedCandidateIDs[0] && candidate.EvaluationRunID != completedEvaluationID {
						t.Fatalf("completed candidate reran after restart: evaluation %q -> %q", completedEvaluationID, candidate.EvaluationRunID)
					}
				}
			}
		})
	}
}
