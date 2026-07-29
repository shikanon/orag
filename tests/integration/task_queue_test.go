package integration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/storage/postgres"
	"github.com/shikanon/orag/internal/taskqueue"
)

func TestPostgresTaskQueueRejectsStaleLeaseWrites(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repo := postgres.NewTaskQueueRepository(app.Postgres)
	taskID := id.New("task_stale_lease")
	pool := taskID
	t.Cleanup(func() {
		_, _ = app.Postgres.Exec(context.Background(), `DELETE FROM task_queue WHERE id=$1`, taskID)
	})

	now := time.Now().UTC()
	enqueued, err := repo.Enqueue(ctx, taskqueue.Task{
		ID:          taskID,
		TenantID:    testTenantID,
		Type:        "stale_lease_integration",
		Pool:        pool,
		Payload:     json.RawMessage(`{"source":"integration"}`),
		MaxAttempts: 3,
		RunAfter:    now,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	firstLease, err := repo.Lease(ctx, pool, "worker-a", 10*time.Minute, 1)
	if err != nil || len(firstLease) != 1 {
		t.Fatalf("first Lease() returned %d tasks, error = %v", len(firstLease), err)
	}
	if firstLease[0].ID != enqueued.ID {
		t.Fatalf("first Lease() task = %q, want %q", firstLease[0].ID, enqueued.ID)
	}

	tag, err := app.Postgres.Exec(ctx, `
		UPDATE task_queue
		SET lease_expires_at = NOW() - INTERVAL '1 second'
		WHERE id=$1`, taskID)
	if err != nil {
		t.Fatalf("expire first lease: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("expire first lease affected %d rows, want 1", tag.RowsAffected())
	}

	currentLease, err := repo.Lease(ctx, pool, "worker-b", 10*time.Minute, 1)
	if err != nil || len(currentLease) != 1 {
		t.Fatalf("second Lease() returned %d tasks, error = %v", len(currentLease), err)
	}
	if currentLease[0].LeaseGeneration != firstLease[0].LeaseGeneration+1 {
		t.Fatalf(
			"second lease generation = %d, want %d",
			currentLease[0].LeaseGeneration,
			firstLease[0].LeaseGeneration+1,
		)
	}

	assertCurrentLease := func(t *testing.T) {
		t.Helper()
		got, found, err := repo.Get(ctx, taskID)
		if err != nil || !found {
			t.Fatalf("Get() found = %v, error = %v", found, err)
		}
		if got.Status != taskqueue.TaskStatusLeased ||
			got.LeaseHolder != currentLease[0].LeaseHolder ||
			got.LeaseGeneration != currentLease[0].LeaseGeneration ||
			!got.LeaseExpiresAt.Equal(currentLease[0].LeaseExpiresAt) {
			t.Fatalf("stale write changed current lease: got %+v, want %+v", got, currentLease[0])
		}
	}

	staleWrites := []struct {
		name string
		run  func() error
	}{
		{
			name: "heartbeat",
			run: func() error {
				return repo.Heartbeat(
					ctx,
					taskID,
					firstLease[0].LeaseHolder,
					firstLease[0].LeaseGeneration,
					10*time.Minute,
				)
			},
		},
		{
			name: "complete",
			run: func() error {
				return repo.Complete(
					ctx,
					taskID,
					firstLease[0].LeaseHolder,
					firstLease[0].LeaseGeneration,
					taskqueue.TaskResult{},
				)
			},
		},
		{
			name: "fail",
			run: func() error {
				return repo.Fail(
					ctx,
					taskID,
					firstLease[0].LeaseHolder,
					firstLease[0].LeaseGeneration,
					taskqueue.TaskFailure{Retryable: false, Message: "stale failure"},
				)
			},
		},
	}
	for _, write := range staleWrites {
		t.Run(write.name, func(t *testing.T) {
			if err := write.run(); !errors.Is(err, taskqueue.ErrLeaseLost) {
				t.Fatalf("stale %s error = %v, want ErrLeaseLost", write.name, err)
			}
			assertCurrentLease(t)
		})
	}

	if err := repo.Complete(
		ctx,
		taskID,
		currentLease[0].LeaseHolder,
		currentLease[0].LeaseGeneration,
		taskqueue.TaskResult{},
	); err != nil {
		t.Fatalf("current worker Complete() error = %v", err)
	}
	if err := repo.Fail(
		ctx,
		taskID,
		firstLease[0].LeaseHolder,
		firstLease[0].LeaseGeneration,
		taskqueue.TaskFailure{Retryable: false, Message: "late stale failure"},
	); !errors.Is(err, taskqueue.ErrLeaseLost) {
		t.Fatalf("late stale Fail() error = %v, want ErrLeaseLost", err)
	}

	got, found, err := repo.Get(ctx, taskID)
	if err != nil || !found {
		t.Fatalf("Get() found = %v, error = %v", found, err)
	}
	if got.Status != taskqueue.TaskStatusSucceeded {
		t.Fatalf("final status = %s, want %s", got.Status, taskqueue.TaskStatusSucceeded)
	}

	var terminalEvents int
	if err := app.Postgres.QueryRow(ctx, `
		SELECT count(*)
		FROM task_events
		WHERE task_id=$1
		  AND event_type IN ('succeeded','failed','retried','dead_letter')`,
		taskID,
	).Scan(&terminalEvents); err != nil {
		t.Fatalf("count terminal events: %v", err)
	}
	if terminalEvents != 1 {
		t.Fatalf("terminal event count = %d, want 1", terminalEvents)
	}
}
