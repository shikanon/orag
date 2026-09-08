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

func TestPostgresTaskQueueResourceLockLease(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	repo := postgres.NewTaskQueueRepository(app.Postgres)
	enqueue := func(t *testing.T, pool, taskID, resourceType, resourceID string, priority int) {
		t.Helper()
		now := time.Now().UTC().Add(-time.Minute)
		if _, err := repo.Enqueue(ctx, taskqueue.Task{
			ID:                 taskID,
			TenantID:           testTenantID,
			Type:               "integration-resource-lock",
			Pool:               pool,
			MaxAttempts:        3,
			LockedResourceType: resourceType,
			LockedResourceID:   resourceID,
			Priority:           priority,
			RunAfter:           now,
			CreatedAt:          now,
			UpdatedAt:          now,
		}); err != nil {
			t.Fatalf("enqueue task %q: %v", taskID, err)
		}
	}

	t.Run("batch keeps one task per resource", func(t *testing.T) {
		scenarioID := id.New("resource_lock_batch")
		pool := "integration-" + scenarioID
		resourceID := "resource-" + scenarioID
		firstID := id.New("task")
		secondID := id.New("task")
		enqueue(t, pool, firstID, "document", resourceID, 2)
		enqueue(t, pool, secondID, "document", resourceID, 1)

		leased, err := repo.Lease(ctx, pool, "batch-worker-"+scenarioID, time.Minute, 2)
		if err != nil {
			t.Fatalf("lease batch: %v", err)
		}
		if len(leased) != 1 {
			t.Fatalf("leased %d tasks for one resource with maxTasks=2, want 1", len(leased))
		}
		if leased[0].ID != firstID {
			t.Fatalf("leased task %q, want higher-priority task %q", leased[0].ID, firstID)
		}
	})

	t.Run("locked rows do not consume batch capacity", func(t *testing.T) {
		scenarioID := id.New("resource_lock_skip_locked")
		pool := "integration-" + scenarioID
		lockedIDs := []string{id.New("task"), id.New("task")}
		availableIDs := []string{id.New("task"), id.New("task")}
		for i, taskID := range append(lockedIDs, availableIDs...) {
			enqueue(t, pool, taskID, "", "", 4-i)
		}

		tx, err := app.Postgres.Begin(ctx)
		if err != nil {
			t.Fatalf("begin row-lock transaction: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		rows, err := tx.Query(ctx, `
			SELECT id
			FROM task_queue
			WHERE id = ANY($1)
			FOR UPDATE`, lockedIDs)
		if err != nil {
			t.Fatalf("lock leading task rows: %v", err)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatalf("read locked task rows: %v", err)
		}

		leased, err := repo.Lease(ctx, pool, "skip-locked-worker-"+scenarioID, time.Minute, 2)
		if err != nil {
			t.Fatalf("lease behind locked rows: %v", err)
		}
		if len(leased) != len(availableIDs) {
			t.Fatalf("leased %d tasks behind locked rows, want %d", len(leased), len(availableIDs))
		}
		leasedIDs := make(map[string]bool, len(leased))
		for _, task := range leased {
			leasedIDs[task.ID] = true
		}
		for _, taskID := range availableIDs {
			if !leasedIDs[taskID] {
				t.Errorf("available task %q was not leased", taskID)
			}
		}
	})

	t.Run("concurrent workers have one winner", func(t *testing.T) {
		const iterations = 8
		for iteration := 0; iteration < iterations; iteration++ {
			scenarioID := id.New("resource_lock_concurrent")
			pools := []string{
				"integration-a-" + scenarioID,
				"integration-b-" + scenarioID,
			}
			resourceID := "resource-" + scenarioID
			enqueue(t, pools[0], id.New("task"), "document", resourceID, 0)
			enqueue(t, pools[1], id.New("task"), "document", resourceID, 0)

			type leaseResult struct {
				tasks []taskqueue.Task
				err   error
			}
			start := make(chan struct{})
			ready := make(chan struct{}, 2)
			results := make(chan leaseResult, 2)
			for _, workerPool := range pools {
				go func() {
					ready <- struct{}{}
					<-start
					tasks, err := repo.Lease(
						ctx,
						workerPool,
						id.New("concurrent_worker"),
						time.Minute,
						1,
					)
					results <- leaseResult{tasks: tasks, err: err}
				}()
			}
			<-ready
			<-ready
			close(start)

			totalLeased := 0
			var leaseErrors []error
			for worker := 0; worker < 2; worker++ {
				result := <-results
				totalLeased += len(result.tasks)
				if result.err != nil {
					leaseErrors = append(leaseErrors, result.err)
				}
			}
			if err := errors.Join(leaseErrors...); err != nil {
				t.Fatalf("iteration %d concurrent lease errors: %v", iteration, err)
			}
			if totalLeased != 1 {
				t.Fatalf("iteration %d concurrent workers leased %d tasks total, want 1", iteration, totalLeased)
			}
		}
	})

	t.Run("completion releases resource", func(t *testing.T) {
		scenarioID := id.New("resource_lock_release")
		pool := "integration-" + scenarioID
		resourceID := "resource-" + scenarioID
		firstID := id.New("task")
		secondID := id.New("task")
		enqueue(t, pool, firstID, "document", resourceID, 2)
		enqueue(t, pool, secondID, "document", resourceID, 1)

		first, err := repo.Lease(ctx, pool, "first-worker-"+scenarioID, time.Minute, 2)
		if err != nil {
			t.Fatalf("lease first task: %v", err)
		}
		if len(first) != 1 || first[0].ID != firstID {
			t.Fatalf("first lease = %#v, want task %q", first, firstID)
		}
		if err := repo.Heartbeat(ctx, first[0].ID, first[0].LeaseToken(), time.Minute); err != nil {
			t.Fatalf("heartbeat first task: %v", err)
		}
		if err := repo.Complete(ctx, first[0].ID, first[0].LeaseToken(), taskqueue.TaskResult{}); err != nil {
			t.Fatalf("complete first task: %v", err)
		}

		second, err := repo.Lease(ctx, pool, "second-worker-"+scenarioID, time.Minute, 1)
		if err != nil {
			t.Fatalf("lease second task: %v", err)
		}
		if len(second) != 1 || second[0].ID != secondID {
			t.Fatalf("lease after completion = %#v, want task %q", second, secondID)
		}
	})

	t.Run("independent tasks remain leasable", func(t *testing.T) {
		scenarioID := id.New("resource_lock_compatibility")
		pool := "integration-" + scenarioID
		firstID := id.New("task")
		independentIDs := []string{id.New("task"), id.New("task"), id.New("task"), id.New("task")}
		enqueue(t, pool, firstID, "document", "resource-a-"+scenarioID, 2)
		enqueue(t, pool, independentIDs[0], "document", "resource-b-"+scenarioID, 1)
		enqueue(t, pool, independentIDs[1], "", "", 1)
		enqueue(t, pool, independentIDs[2], "document", "", 1)
		enqueue(t, pool, independentIDs[3], "", "resource-c-"+scenarioID, 1)

		first, err := repo.Lease(ctx, pool, "compatibility-worker-a-"+scenarioID, time.Minute, 1)
		if err != nil {
			t.Fatalf("lease first resource: %v", err)
		}
		if len(first) != 1 || first[0].ID != firstID {
			t.Fatalf("first compatibility lease = %#v, want task %q", first, firstID)
		}

		independent, err := repo.Lease(ctx, pool, "compatibility-worker-b-"+scenarioID, time.Minute, len(independentIDs))
		if err != nil {
			t.Fatalf("lease independent tasks: %v", err)
		}
		if len(independent) != len(independentIDs) {
			t.Fatalf("leased %d independent tasks while another resource is active, want %d", len(independent), len(independentIDs))
		}
		leasedIDs := make(map[string]bool, len(independent))
		for _, task := range independent {
			leasedIDs[task.ID] = true
		}
		for _, taskID := range independentIDs {
			if !leasedIDs[taskID] {
				t.Errorf("independent task %q was not leased", taskID)
			}
		}
	})
}

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
