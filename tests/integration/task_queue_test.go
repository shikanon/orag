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

func TestPostgresTaskQueueLeaseFencing(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	repo := postgres.NewTaskQueueRepository(app.Postgres)
	taskID := id.New("task")
	pool := "integration-lease-fencing-" + taskID
	now := time.Now().UTC()

	if _, err := repo.Enqueue(ctx, taskqueue.Task{
		ID:          taskID,
		TenantID:    testTenantID,
		Type:        "integration-lease-fencing",
		Pool:        pool,
		MaxAttempts: 3,
		RunAfter:    now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("enqueue task: %v", err)
	}

	leasedA, err := repo.Lease(ctx, pool, "worker-a", time.Second, 1)
	if err != nil {
		t.Fatalf("worker-a lease: %v", err)
	}
	if len(leasedA) != 1 || leasedA[0].ID != taskID {
		t.Fatalf("worker-a leased tasks = %#v, want task %q", leasedA, taskID)
	}
	tokenA := leasedA[0].LeaseToken()
	if tokenA.Holder != "worker-a" || tokenA.Generation <= 0 {
		t.Fatalf("worker-a token = %#v, want holder worker-a and positive generation", tokenA)
	}
	if err := repo.Heartbeat(ctx, taskID, tokenA, time.Second); err != nil {
		t.Fatalf("worker-a start heartbeat: %v", err)
	}

	runningA, found, err := repo.Get(ctx, taskID)
	if err != nil {
		t.Fatalf("get worker-a task: %v", err)
	}
	if !found {
		t.Fatal("worker-a task not found")
	}
	if runningA.Status != taskqueue.TaskStatusRunning ||
		runningA.LeaseHolder != tokenA.Holder ||
		runningA.LeaseGeneration != tokenA.Generation {
		t.Fatalf("worker-a running task = %#v, want running with token %#v", runningA, tokenA)
	}

	tag, err := app.Postgres.Exec(ctx, `
		UPDATE task_queue
		SET lease_expires_at = NOW() - INTERVAL '1 second'
		WHERE id=$1
		  AND lease_holder=$2
		  AND lease_generation=$3
		  AND status='running'`,
		taskID, tokenA.Holder, tokenA.Generation)
	if err != nil {
		t.Fatalf("expire worker-a lease: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("expired worker-a lease rows = %d, want 1", tag.RowsAffected())
	}

	leasedB, err := repo.Lease(ctx, pool, "worker-b", time.Second, 1)
	if err != nil {
		t.Fatalf("worker-b lease: %v", err)
	}
	if len(leasedB) != 1 || leasedB[0].ID != taskID {
		t.Fatalf("worker-b leased tasks = %#v, want task %q", leasedB, taskID)
	}
	tokenB := leasedB[0].LeaseToken()
	if tokenB.Holder != "worker-b" {
		t.Fatalf("worker-b token holder = %q, want worker-b", tokenB.Holder)
	}
	if tokenB.Generation <= tokenA.Generation {
		t.Fatalf("worker-b generation = %d, want greater than worker-a generation %d",
			tokenB.Generation, tokenA.Generation)
	}

	staleOperations := []struct {
		name string
		run  func() error
	}{
		{
			name: "heartbeat",
			run: func() error {
				return repo.Heartbeat(ctx, taskID, tokenA, time.Second)
			},
		},
		{
			name: "complete",
			run: func() error {
				return repo.Complete(ctx, taskID, tokenA, taskqueue.TaskResult{
					Data: json.RawMessage(`{"worker":"worker-a"}`),
				})
			},
		},
		{
			name: "fail",
			run: func() error {
				return repo.Fail(ctx, taskID, tokenA, taskqueue.TaskFailure{
					ErrorCode: "stale_worker",
					Message:   "worker-a must be fenced",
				})
			},
		},
	}
	for _, operation := range staleOperations {
		if err := operation.run(); !errors.Is(err, taskqueue.ErrLeaseLost) {
			t.Errorf("worker-a %s error = %v, want ErrLeaseLost", operation.name, err)
		}
	}

	ownedByB, found, err := repo.Get(ctx, taskID)
	if err != nil {
		t.Fatalf("get worker-b task after stale writes: %v", err)
	}
	if !found {
		t.Fatal("worker-b task not found after stale writes")
	}
	if ownedByB.Status != taskqueue.TaskStatusLeased ||
		ownedByB.LeaseHolder != tokenB.Holder ||
		ownedByB.LeaseGeneration != tokenB.Generation {
		t.Fatalf("task after stale writes = %#v, want leased with token %#v", ownedByB, tokenB)
	}

	events, nextCursor, err := repo.ListEvents(ctx, taskID, "", 100)
	if err != nil {
		t.Fatalf("list events before worker-b completion: %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("event cursor before worker-b completion = %q, want empty", nextCursor)
	}
	for _, event := range events {
		switch event.Type {
		case "succeeded", "failed", "dead_letter", "cancelled":
			t.Fatalf("stale worker produced terminal event before worker-b completion: %#v", event)
		}
	}

	if err := repo.Heartbeat(ctx, taskID, tokenB, time.Second); err != nil {
		t.Fatalf("worker-b heartbeat: %v", err)
	}
	if err := repo.Complete(ctx, taskID, tokenB, taskqueue.TaskResult{
		Data: json.RawMessage(`{"worker":"worker-b"}`),
	}); err != nil {
		t.Fatalf("worker-b complete: %v", err)
	}

	completed, found, err := repo.Get(ctx, taskID)
	if err != nil {
		t.Fatalf("get completed task: %v", err)
	}
	if !found {
		t.Fatal("completed task not found")
	}
	if completed.Status != taskqueue.TaskStatusSucceeded {
		t.Fatalf("completed task status = %q, want %q", completed.Status, taskqueue.TaskStatusSucceeded)
	}

	events, nextCursor, err = repo.ListEvents(ctx, taskID, "", 100)
	if err != nil {
		t.Fatalf("list final events: %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("final event cursor = %q, want empty", nextCursor)
	}
	succeededEvents := 0
	for _, event := range events {
		if event.Type != "succeeded" {
			continue
		}
		succeededEvents++
		var data struct {
			Generation int64 `json:"generation"`
		}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			t.Fatalf("decode succeeded event data %q: %v", event.Data, err)
		}
		if data.Generation != tokenB.Generation {
			t.Errorf("succeeded event generation = %d, want worker-b generation %d",
				data.Generation, tokenB.Generation)
		}
	}
	if succeededEvents != 1 {
		t.Fatalf("succeeded event count = %d, want 1; events = %#v", succeededEvents, events)
	}
}
