package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shikanon/orag/internal/taskqueue"
)

var _ taskqueue.QueueRepository = (*TaskQueueRepository)(nil)

type TaskQueueRepository struct {
	pool *pgxpool.Pool
}

func NewTaskQueueRepository(pool *pgxpool.Pool) *TaskQueueRepository {
	return &TaskQueueRepository{pool: pool}
}

const taskQueueSelect = `
	SELECT id, tenant_id, COALESCE(project_id,''), type, pool, status,
	       COALESCE(payload,'{}'::jsonb), COALESCE(idempotency_key,''),
	       attempt, max_attempts, COALESCE(locked_resource_type,''),
	       COALESCE(locked_resource_id,''), COALESCE(trace_id,''),
	       COALESCE(created_by,''), priority, run_after, created_at,
	       updated_at, started_at, completed_at,
	       COALESCE(error_code,''), COALESCE(error_message,''),
	       last_heartbeat_at, lease_expires_at,
	       COALESCE(lease_holder,''), lease_generation
	FROM task_queue`

type taskQueueScanner interface {
	Scan(dest ...any) error
}

func scanTask(row taskQueueScanner) (taskqueue.Task, error) {
	var t taskqueue.Task
	var status string
	var payload []byte
	var startedAt, completedAt, lastHeartbeatAt, leaseExpiresAt *time.Time
	err := row.Scan(
		&t.ID, &t.TenantID, &t.ProjectID, &t.Type, &t.Pool, &status,
		&payload, &t.IdempotencyKey, &t.Attempt, &t.MaxAttempts,
		&t.LockedResourceType, &t.LockedResourceID, &t.TraceID,
		&t.CreatedBy, &t.Priority, &t.RunAfter, &t.CreatedAt,
		&t.UpdatedAt, &startedAt, &completedAt,
		&t.ErrorCode, &t.ErrorMessage, &lastHeartbeatAt,
		&leaseExpiresAt, &t.LeaseHolder, &t.LeaseGeneration,
	)
	if err != nil {
		return t, err
	}
	t.Status = taskqueue.TaskStatus(status)
	if len(payload) > 0 {
		t.Payload = json.RawMessage(payload)
	}
	if startedAt != nil && !isZeroOrInfinity(*startedAt) {
		t.StartedAt = *startedAt
	}
	if completedAt != nil && !isZeroOrInfinity(*completedAt) {
		t.CompletedAt = *completedAt
	}
	if lastHeartbeatAt != nil && !isZeroOrInfinity(*lastHeartbeatAt) {
		t.LastHeartbeatAt = *lastHeartbeatAt
	}
	if leaseExpiresAt != nil && !isZeroOrInfinity(*leaseExpiresAt) {
		t.LeaseExpiresAt = *leaseExpiresAt
	}
	return t, nil
}

func isZeroOrInfinity(t time.Time) bool {
	if t.IsZero() {
		return true
	}
	return t.Year() <= -292277022365 || t.Year() >= 292277026596
}

func isPgUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type taskQueueExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertTaskEvent(ctx context.Context, tx taskQueueExecer, taskID string, eventType string, eventData any) error {
	var data []byte
	if eventData != nil {
		var err error
		data, err = json.Marshal(eventData)
		if err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO task_events(task_id, event_type, event_data)
		VALUES($1, $2, $3)`, taskID, eventType, data)
	return err
}

func insertTask(ctx context.Context, execer taskQueueExecer, task taskqueue.Task) error {
	payload := []byte(task.Payload)
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	_, err := execer.Exec(ctx, `
		INSERT INTO task_queue(
			id, tenant_id, project_id, type, pool, status, payload,
			idempotency_key, attempt, max_attempts, locked_resource_type,
			locked_resource_id, trace_id, created_by, priority,
			run_after, created_at, updated_at
		) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,
			NULLIF($8,''),$9,$10,NULLIF($11,''),
			NULLIF($12,''),NULLIF($13,''),NULLIF($14,''),$15,
			$16,$17,$18)`,
		task.ID, task.TenantID, task.ProjectID, task.Type, task.Pool,
		string(taskqueue.TaskStatusQueued), payload, task.IdempotencyKey,
		task.Attempt, task.MaxAttempts, task.LockedResourceType,
		task.LockedResourceID, task.TraceID, task.CreatedBy,
		task.Priority, task.RunAfter, task.CreatedAt, task.UpdatedAt)
	return err
}

type taskQueueTx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Commit(context.Context) error
	Rollback(context.Context) error
}

// Enqueue adds a new task to the queue. If a task with the same idempotency_key
// already exists for the tenant, the existing task is returned without modification.
func (r *TaskQueueRepository) Enqueue(ctx context.Context, task taskqueue.Task) (taskqueue.Task, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return taskqueue.Task{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = insertTask(ctx, tx, task)
	if err != nil {
		if isPgUniqueViolation(err) && task.IdempotencyKey != "" {
			row := tx.QueryRow(ctx, taskQueueSelect+`
				WHERE tenant_id=$1 AND idempotency_key=$2`, task.TenantID, task.IdempotencyKey)
			existing, scanErr := scanTask(row)
			if scanErr != nil {
				return taskqueue.Task{}, scanErr
			}
			return existing, nil
		}
		return taskqueue.Task{}, err
	}

	if err := insertTaskEvent(ctx, tx, task.ID, "enqueued", map[string]any{
		"type": task.Type,
		"pool": task.Pool,
	}); err != nil {
		return taskqueue.Task{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return taskqueue.Task{}, err
	}

	row := r.pool.QueryRow(ctx, taskQueueSelect+` WHERE id=$1`, task.ID)
	return scanTask(row)
}

// Lease acquires tasks from the queue for processing by a worker.
// It uses SKIP LOCKED to avoid contention and supports resource-level locking
// so that only one task for a given locked_resource_type/locked_resource_id
// pair can be leased at a time.
func (r *TaskQueueRepository) Lease(ctx context.Context, pool, leaseHolder string, leaseDuration time.Duration, maxTasks int) ([]taskqueue.Task, error) {
	if maxTasks <= 0 {
		maxTasks = 1
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	intervalStr := formatInterval(leaseDuration)

	// Reclaim abandoned leases before selecting ready work. This is deliberately
	// transactional so a second worker cannot observe a half-reclaimed task.
	if _, err := tx.Exec(ctx, `
		UPDATE task_queue
		SET status='queued', lease_expires_at=NULL, lease_holder=NULL, updated_at=NOW()
		WHERE pool=$1
		  AND status IN ('leased','running')
		  AND lease_expires_at IS NOT NULL
		  AND lease_expires_at <= NOW()`, pool); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
		WITH ranked AS MATERIALIZED (
			-- Batch de-duplication ranks complete resource keys before LIMIT;
			-- tasks without both key parts remain independent candidates.
			SELECT candidate.id,
			       candidate.locked_resource_type,
			       candidate.locked_resource_id,
			       candidate.priority,
			       candidate.created_at,
			       CASE
			         WHEN candidate.locked_resource_type IS NOT NULL
			          AND candidate.locked_resource_type <> ''
			          AND candidate.locked_resource_id IS NOT NULL
			          AND candidate.locked_resource_id <> ''
			         THEN ROW_NUMBER() OVER (
			           PARTITION BY candidate.locked_resource_type, candidate.locked_resource_id
			           ORDER BY candidate.priority DESC, candidate.created_at, candidate.id
			         )
			         ELSE 1
			       END AS resource_rank
			FROM task_queue candidate
			WHERE candidate.pool=$1
			  AND candidate.status IN ('queued', 'failed_retryable')
			  AND candidate.run_after <= NOW()
			  AND (
				candidate.locked_resource_type IS NULL
				OR candidate.locked_resource_type = ''
				OR candidate.locked_resource_id IS NULL
				OR candidate.locked_resource_id = ''
				OR NOT EXISTS (
					SELECT 1 FROM task_queue active
					WHERE active.locked_resource_type = candidate.locked_resource_type
					  AND active.locked_resource_id = candidate.locked_resource_id
					  AND active.status IN ('leased','running','cancelling')
					  AND active.id != candidate.id
				)
			  )
		)
		SELECT candidate.id,
		       COALESCE(candidate.locked_resource_type, ''),
		       COALESCE(candidate.locked_resource_id, '')
		FROM task_queue candidate
		JOIN ranked ON ranked.id = candidate.id
		WHERE ranked.resource_rank = 1
		ORDER BY ranked.priority DESC, ranked.created_at, ranked.id
		LIMIT $2
		-- Row locks prevent duplicate leasing of the same task while SKIP LOCKED
		-- lets competing workers continue with later eligible rows. LIMIT belongs
		-- after row locking so skipped rows do not consume the batch capacity.
		FOR UPDATE OF candidate SKIP LOCKED`, pool, maxTasks)
	if err != nil {
		return nil, err
	}

	type leaseCandidate struct {
		id           string
		resourceType string
		resourceID   string
	}
	var candidates []leaseCandidate
	for rows.Next() {
		var candidate leaseCandidate
		if err := rows.Scan(&candidate.id, &candidate.resourceType, &candidate.resourceID); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return []taskqueue.Task{}, nil
	}

	var taskIDs []string
	for _, candidate := range candidates {
		if candidate.resourceType != "" && candidate.resourceID != "" {
			var resourceLocked bool
			err := tx.QueryRow(ctx, `
				-- The non-blocking transaction advisory lock serializes workers that
				-- selected different task rows for the same global resource key. A JSON
				-- array unambiguously serializes the two key parts before hashing.
				SELECT pg_try_advisory_xact_lock(
					hashtextextended(jsonb_build_array($1::text, $2::text)::text, 0)
				)`, candidate.resourceType, candidate.resourceID).Scan(&resourceLocked)
			if err != nil {
				return nil, err
			}
			if !resourceLocked {
				continue
			}
		}

		var generation int64
		err := tx.QueryRow(ctx, `
			-- Re-check readiness and the active global resource state after row and
			-- advisory locking so only still-eligible tasks transition to leased.
			UPDATE task_queue AS candidate
			SET status='leased',
			    lease_expires_at = NOW() + $2::interval,
			    lease_holder = $3,
			    updated_at = NOW(),
			    attempt = attempt + 1,
			    lease_generation = candidate.lease_generation + 1,
			    started_at = CASE WHEN status = 'queued' THEN NOW() ELSE started_at END
			WHERE candidate.id=$1
			  AND candidate.status IN ('queued', 'failed_retryable')
			  AND candidate.run_after <= NOW()
			  AND (
				candidate.locked_resource_type IS NULL
				OR candidate.locked_resource_type = ''
				OR candidate.locked_resource_id IS NULL
				OR candidate.locked_resource_id = ''
				OR NOT EXISTS (
					SELECT 1 FROM task_queue active
					WHERE active.locked_resource_type = candidate.locked_resource_type
					  AND active.locked_resource_id = candidate.locked_resource_id
					  AND active.status IN ('leased','running','cancelling')
					  AND active.id != candidate.id
				)
			  )
			RETURNING candidate.lease_generation`, candidate.id, intervalStr, leaseHolder).Scan(&generation)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return nil, err
		}
		taskIDs = append(taskIDs, candidate.id)
		if err := insertTaskEvent(ctx, tx, candidate.id, "leased", map[string]any{
			"lease_holder": leaseHolder,
			"generation":   generation,
		}); err != nil {
			return nil, err
		}
	}
	if len(taskIDs) == 0 {
		return []taskqueue.Task{}, nil
	}

	taskRows, err := tx.Query(ctx, taskQueueSelect+`
		WHERE id = ANY($1)
		ORDER BY priority DESC, created_at, id`, taskIDs)
	if err != nil {
		return nil, err
	}
	defer taskRows.Close()

	var tasks []taskqueue.Task
	for taskRows.Next() {
		t, err := scanTask(taskRows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := taskRows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return tasks, nil
}

func formatInterval(d time.Duration) string {
	seconds := d.Seconds()
	return fmt.Sprintf("%f seconds", seconds)
}

// Heartbeat renews the lease and transitions a newly leased task to running.
func (r *TaskQueueRepository) Heartbeat(ctx context.Context, taskID string, token taskqueue.LeaseToken, leaseDuration time.Duration) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	intervalStr := formatInterval(leaseDuration)
	tag, err := tx.Exec(ctx, `
		UPDATE task_queue
		SET status = 'running',
		    last_heartbeat_at = NOW(),
		    lease_expires_at = NOW() + $4::interval,
		    updated_at = NOW()
		WHERE id=$1
		  AND lease_holder=$2
		  AND lease_generation=$3
		  AND status IN ('leased','running')`,
		taskID, token.Holder, token.Generation, intervalStr)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return taskqueue.ErrLeaseLost
	}

	if err := insertTaskEvent(ctx, tx, taskID, "heartbeat", map[string]any{
		"lease_holder": token.Holder,
		"generation":   token.Generation,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Complete marks a task as successfully completed and stores the result data.
func (r *TaskQueueRepository) Complete(ctx context.Context, taskID string, token taskqueue.LeaseToken, result taskqueue.TaskResult) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var resultData []byte
	if result.Data != nil {
		resultData = []byte(result.Data)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE task_queue
		SET status='succeeded',
		    completed_at = NOW(),
		    result_data = $4,
		    error_code = NULL,
		    error_message = NULL,
		    lease_expires_at = NULL,
		    lease_holder = NULL,
		    updated_at = NOW()
		WHERE id=$1
		  AND lease_holder=$2
		  AND lease_generation=$3
		  AND status='running'`,
		taskID, token.Holder, token.Generation, resultData)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return taskqueue.ErrLeaseLost
	}

	if err := insertTaskEvent(ctx, tx, taskID, "succeeded", map[string]any{
		"has_result": result.Data != nil,
		"generation": token.Generation,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Fail marks a task as failed. If the failure is retryable and the task has
// remaining attempts, it will be scheduled for retry with exponential backoff.
// If retries are exhausted, the task moves to dead letter status.
func (r *TaskQueueRepository) Fail(ctx context.Context, taskID string, token taskqueue.LeaseToken, failure taskqueue.TaskFailure) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx, `
		SELECT attempt, max_attempts
		FROM task_queue
		WHERE id=$1
		  AND lease_holder=$2
		  AND lease_generation=$3
		  AND status IN ('leased','running')
		FOR UPDATE`, taskID, token.Holder, token.Generation)
	var attempt, maxAttempts int
	if err := row.Scan(&attempt, &maxAttempts); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return taskqueue.ErrLeaseLost
		}
		return err
	}

	var newStatus string
	var runAfter time.Time

	if failure.Retryable && attempt < maxAttempts {
		newStatus = string(taskqueue.TaskStatusFailedRetryable)
		backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
		if backoff > 1*time.Hour {
			backoff = 1 * time.Hour
		}
		runAfter = time.Now().Add(backoff)
	} else if failure.Retryable && attempt >= maxAttempts {
		newStatus = string(taskqueue.TaskStatusDeadLetter)
	} else {
		newStatus = string(taskqueue.TaskStatusFailedTerminal)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE task_queue
		SET status=$2,
		    error_code=$3,
		    error_message=$4,
		    updated_at=NOW(),
		    completed_at=CASE WHEN $2 IN ('failed_terminal','dead_letter') THEN NOW() ELSE completed_at END,
		    run_after=CASE WHEN $2='failed_retryable' THEN $5 ELSE run_after END,
		    lease_expires_at=NULL,
		    lease_holder=NULL
		WHERE id=$1
		  AND lease_holder=$6
		  AND lease_generation=$7
		  AND status IN ('leased','running')`,
		taskID, newStatus, failure.ErrorCode, failure.Message, runAfter,
		token.Holder, token.Generation)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return taskqueue.ErrLeaseLost
	}

	eventType := "failed"
	if newStatus == string(taskqueue.TaskStatusDeadLetter) {
		eventType = "dead_letter"
	} else if newStatus == string(taskqueue.TaskStatusFailedRetryable) {
		eventType = "retried"
	}

	if err := insertTaskEvent(ctx, tx, taskID, eventType, map[string]any{
		"retryable":  failure.Retryable,
		"error_code": failure.ErrorCode,
		"message":    failure.Message,
		"attempt":    attempt,
		"generation": token.Generation,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Cancel requests cancellation of a task. Queued or retryable tasks are
// immediately cancelled. Leased or running tasks move to cancelling status
// so the worker can gracefully stop.
func (r *TaskQueueRepository) Cancel(ctx context.Context, taskID string) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx, `
		SELECT status
		FROM task_queue
		WHERE id=$1
		FOR UPDATE`, taskID)
	var currentStatus string
	if err := row.Scan(&currentStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	var newStatus string
	var clearLease bool
	switch currentStatus {
	case "queued", "failed_retryable":
		newStatus = string(taskqueue.TaskStatusCancelled)
		clearLease = true
	case "leased", "running":
		newStatus = string(taskqueue.TaskStatusCancelling)
	default:
		return nil
	}

	query := `
		UPDATE task_queue
		SET status=$2,
		    updated_at=NOW()`
	args := []any{taskID, newStatus}

	if clearLease {
		query += ", lease_expires_at=NULL, lease_holder=NULL, completed_at=NOW()"
	}

	query += " WHERE id=$1"

	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}

	if err := insertTaskEvent(ctx, tx, taskID, "cancelled", map[string]any{
		"from_status": currentStatus,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// MarkCancelled transitions an acknowledged cancellation into its terminal
// state. It is intentionally separate from Cancel so handlers have a chance
// to release downstream resources first.
func (r *TaskQueueRepository) MarkCancelled(ctx context.Context, taskID string, token taskqueue.LeaseToken) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE task_queue
		SET status='cancelled', completed_at=NOW(), lease_expires_at=NULL,
		    lease_holder=NULL, updated_at=NOW()
		WHERE id=$1
		  AND lease_holder=$2
		  AND lease_generation=$3
		  AND status='cancelling'`,
		taskID, token.Holder, token.Generation)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return taskqueue.ErrLeaseLost
	}
	if err := insertTaskEvent(ctx, tx, taskID, "cancelled", map[string]any{
		"generation": token.Generation,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Retry requeues a task after explicit operator action. The attempt counter is
// reset because this is a new manual recovery cycle, while the event timeline
// preserves the previous failure history.
func (r *TaskQueueRepository) Retry(ctx context.Context, taskID string) (taskqueue.Task, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return taskqueue.Task{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE task_queue
		SET status='queued', attempt=0, run_after=NOW(), completed_at=NULL,
		    error_code=NULL, error_message=NULL, lease_expires_at=NULL,
		    lease_holder=NULL, updated_at=NOW()
		WHERE id=$1 AND status IN ('dead_letter','failed_terminal','cancelled')`, taskID)
	if err != nil {
		return taskqueue.Task{}, err
	}
	if tag.RowsAffected() == 0 {
		return taskqueue.Task{}, fmt.Errorf("task is not eligible for manual retry")
	}
	if err := insertTaskEvent(ctx, tx, taskID, "manual_retry", nil); err != nil {
		return taskqueue.Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return taskqueue.Task{}, err
	}
	return r.GetTask(ctx, taskID)
}

// Get retrieves a task by its ID.
func (r *TaskQueueRepository) Get(ctx context.Context, taskID string) (taskqueue.Task, bool, error) {
	row := r.pool.QueryRow(ctx, taskQueueSelect+` WHERE id=$1`, taskID)
	t, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return taskqueue.Task{}, false, nil
		}
		return taskqueue.Task{}, false, err
	}
	return t, true, nil
}

func (r *TaskQueueRepository) GetTask(ctx context.Context, taskID string) (taskqueue.Task, error) {
	row := r.pool.QueryRow(ctx, taskQueueSelect+` WHERE id=$1`, taskID)
	return scanTask(row)
}

// List returns tasks matching the given filter with cursor-based pagination.
// The cursor is opaque and should be passed back from a previous List call.
func (r *TaskQueueRepository) List(ctx context.Context, filter taskqueue.TaskFilter) ([]taskqueue.Task, string, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var conditions []string
	var args []any
	argIdx := 1
	if filter.TenantID != "" {
		conditions = append(conditions, fmt.Sprintf("tenant_id=$%d", argIdx))
		args = append(args, filter.TenantID)
		argIdx++
	}

	if filter.Pool != "" {
		conditions = append(conditions, fmt.Sprintf("pool=$%d", argIdx))
		args = append(args, filter.Pool)
		argIdx++
	}
	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status=$%d", argIdx))
		args = append(args, string(filter.Status))
		argIdx++
	}
	if filter.Type != "" {
		conditions = append(conditions, fmt.Sprintf("type=$%d", argIdx))
		args = append(args, filter.Type)
		argIdx++
	}
	if filter.ProjectID != "" {
		conditions = append(conditions, fmt.Sprintf("project_id=$%d", argIdx))
		args = append(args, filter.ProjectID)
		argIdx++
	}

	var cursorCreatedAt time.Time
	var cursorID string
	if filter.Cursor != "" {
		parts := strings.SplitN(filter.Cursor, ":", 2)
		if len(parts) == 2 {
			if timeBytes, err := base64.URLEncoding.DecodeString(parts[0]); err == nil {
				if t, err := time.Parse(time.RFC3339Nano, string(timeBytes)); err == nil {
					cursorCreatedAt = t
					cursorID = parts[1]
				}
			}
		}
	}

	if !cursorCreatedAt.IsZero() {
		conditions = append(conditions, fmt.Sprintf(
			"(created_at < $%d OR (created_at = $%d AND id < $%d))",
			argIdx, argIdx, argIdx+1,
		))
		args = append(args, cursorCreatedAt, cursorID)
		argIdx += 2
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	fetchLimit := limit + 1
	query := taskQueueSelect + whereClause + fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", argIdx)
	args = append(args, fetchLimit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var tasks []taskqueue.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, "", err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(tasks) > limit {
		tasks = tasks[:limit]
		last := tasks[len(tasks)-1]
		timeStr := last.CreatedAt.Format(time.RFC3339Nano)
		nextCursor = base64.URLEncoding.EncodeToString([]byte(timeStr)) + ":" + last.ID
	}

	return tasks, nextCursor, nil
}

func (r *TaskQueueRepository) ListEvents(ctx context.Context, taskID, cursor string, limit int) ([]taskqueue.Event, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var afterID int64
	if cursor != "" {
		parsed, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid task event cursor")
		}
		if _, err := fmt.Sscan(string(parsed), &afterID); err != nil {
			return nil, "", fmt.Errorf("invalid task event cursor")
		}
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, task_id, event_type, COALESCE(event_data, '{}'::jsonb), created_at
		FROM task_events
		WHERE task_id=$1 AND id>$2
		ORDER BY id ASC
		LIMIT $3`, taskID, afterID, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	events := make([]taskqueue.Event, 0, limit)
	var lastID int64
	for rows.Next() {
		var numericID int64
		var event taskqueue.Event
		var data []byte
		if err := rows.Scan(&numericID, &event.TaskID, &event.Type, &data, &event.CreatedAt); err != nil {
			return nil, "", err
		}
		event.ID = fmt.Sprintf("%d", numericID)
		event.Data = json.RawMessage(data)
		if len(events) == limit {
			return events, base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", lastID))), nil
		}
		events = append(events, event)
		lastID = numericID
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return events, "", nil
}

func (r *TaskQueueRepository) Stats(ctx context.Context, tenantID string) (map[string]taskqueue.PoolStats, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT pool,
		  COUNT(*) FILTER (WHERE status IN ('queued','failed_retryable')),
		  COUNT(*) FILTER (WHERE status IN ('leased','running','cancelling')),
		  COUNT(*) FILTER (WHERE status='succeeded'),
		  COUNT(*) FILTER (WHERE status IN ('failed_terminal','cancelled')),
		  COUNT(*) FILTER (WHERE status='dead_letter')
		FROM task_queue WHERE tenant_id=$1 GROUP BY pool`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := map[string]taskqueue.PoolStats{}
	for rows.Next() {
		var pool string
		var current taskqueue.PoolStats
		if err := rows.Scan(&pool, &current.Queued, &current.Running, &current.Succeeded, &current.Failed, &current.DeadLetter); err != nil {
			return nil, err
		}
		stats[pool] = current
	}
	return stats, rows.Err()
}
