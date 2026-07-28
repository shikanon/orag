package integration

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/storage/postgres"
	"github.com/shikanon/orag/internal/taskqueue"
)

func TestPostgresTaskQueueLeaseTransferFencesStaleLease(t *testing.T) {
	if os.Getenv("ORAG_INTEGRATION_TESTS") != "1" {
		t.Skip("integration tests require ORAG_INTEGRATION_TESTS=1 and docker compose test services")
	}

	setenvDefault(t, "DATABASE_URL", "postgres://orag:orag@localhost:55432/orag_test?sslmode=disable")
	databaseURL := os.Getenv("DATABASE_URL")
	waitForPostgres(t, databaseURL)
	migrate(t, databaseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	repo := postgres.NewTaskQueueRepository(pool)
	service := taskqueue.NewService(repo, nil)
	taskID := id.New("task")
	task, err := service.Enqueue(ctx, testTenantID, taskqueue.Task{
		ID:          taskID,
		Type:        "lease.transfer.integration",
		Pool:        "lease_transfer_" + taskID,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("enqueue task: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM task_queue WHERE id=$1`, task.ID)
	})

	leaseA := leaseOneIntegrationTask(t, ctx, repo, task.Pool, "shared-worker")
	if _, err := pool.Exec(ctx, `
		UPDATE task_queue
		SET lease_expires_at=NOW() - INTERVAL '1 second'
		WHERE id=$1 AND lease_generation=$2`,
		task.ID, leaseA.LeaseGeneration); err != nil {
		t.Fatalf("expire lease A: %v", err)
	}

	leaseB := leaseOneIntegrationTask(t, ctx, repo, task.Pool, "shared-worker")
	if leaseB.LeaseGeneration <= leaseA.LeaseGeneration {
		t.Fatalf("lease generation did not advance: A=%d B=%d", leaseA.LeaseGeneration, leaseB.LeaseGeneration)
	}
	if leaseB.Attempt <= leaseA.Attempt {
		t.Fatalf("attempt did not advance: A=%d B=%d", leaseA.Attempt, leaseB.Attempt)
	}

	eventsBefore, _, err := repo.ListEvents(ctx, task.ID, "", 100)
	if err != nil {
		t.Fatalf("list events before stale mutations: %v", err)
	}
	staleCalls := []struct {
		name string
		call func() error
	}{
		{
			name: "heartbeat",
			call: func() error {
				return repo.Heartbeat(ctx, task.ID, leaseA.LeaseHolder, leaseA.LeaseGeneration, leaseA.Attempt, time.Minute)
			},
		},
		{
			name: "complete",
			call: func() error {
				return repo.Complete(ctx, task.ID, leaseA.LeaseHolder, leaseA.LeaseGeneration, leaseA.Attempt, taskqueue.TaskResult{})
			},
		},
		{
			name: "fail",
			call: func() error {
				return repo.Fail(ctx, task.ID, leaseA.LeaseHolder, leaseA.LeaseGeneration, leaseA.Attempt, taskqueue.TaskFailure{
					ErrorCode: "stale_worker",
					Message:   "stale worker failed",
				})
			},
		},
	}
	for _, tc := range staleCalls {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if !errors.Is(err, taskqueue.ErrLeaseLost) {
				t.Fatalf("error = %v, want ErrLeaseLost", err)
			}
			var leaseErr *taskqueue.LeaseLostError
			if !errors.As(err, &leaseErr) || leaseErr.TaskID != task.ID {
				t.Fatalf("error = %#v, want LeaseLostError for task %s", err, task.ID)
			}
			assertIntegrationLease(t, ctx, repo, leaseB)
		})
	}

	eventsAfter, _, err := repo.ListEvents(ctx, task.ID, "", 100)
	if err != nil {
		t.Fatalf("list events after stale mutations: %v", err)
	}
	if len(eventsAfter) != len(eventsBefore) {
		t.Fatalf("stale mutations appended events: before=%d after=%d", len(eventsBefore), len(eventsAfter))
	}

	if err := repo.Complete(ctx, task.ID, leaseB.LeaseHolder, leaseB.LeaseGeneration, leaseB.Attempt, taskqueue.TaskResult{}); err != nil {
		t.Fatalf("complete lease B: %v", err)
	}
	completed, found, err := repo.Get(ctx, task.ID)
	if err != nil || !found {
		t.Fatalf("get completed task: found=%v err=%v", found, err)
	}
	if completed.Status != taskqueue.TaskStatusSucceeded ||
		completed.LeaseGeneration != leaseB.LeaseGeneration ||
		completed.Attempt != leaseB.Attempt ||
		completed.LeaseHolder != "" {
		t.Fatalf("lease B terminal state = %#v", completed)
	}
}

func leaseOneIntegrationTask(
	t *testing.T,
	ctx context.Context,
	repo *postgres.TaskQueueRepository,
	pool string,
	leaseHolder string,
) taskqueue.Task {
	t.Helper()
	leased, err := repo.Lease(ctx, pool, leaseHolder, time.Minute, 1)
	if err != nil {
		t.Fatalf("lease task: %v", err)
	}
	if len(leased) != 1 {
		t.Fatalf("leased task count = %d, want 1", len(leased))
	}
	return leased[0]
}

func assertIntegrationLease(
	t *testing.T,
	ctx context.Context,
	repo *postgres.TaskQueueRepository,
	want taskqueue.Task,
) {
	t.Helper()
	got, found, err := repo.Get(ctx, want.ID)
	if err != nil || !found {
		t.Fatalf("get leased task: found=%v err=%v", found, err)
	}
	if got.Status != taskqueue.TaskStatusLeased ||
		got.LeaseHolder != want.LeaseHolder ||
		got.LeaseGeneration != want.LeaseGeneration ||
		got.Attempt != want.Attempt ||
		!got.LeaseExpiresAt.Equal(want.LeaseExpiresAt) {
		t.Fatalf("stale mutation changed lease B: got %#v, want %#v", got, want)
	}
}
