package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shikanon/orag/internal/optimizer"
	"github.com/shikanon/orag/internal/taskqueue"
)

func (r *Repository) CreateOptimizationRun(ctx context.Context, run optimizer.OptimizationRun) error {
	return insertOptimizationRun(ctx, r.evaluationQueryer(), run)
}

func (r *Repository) CreateOptimizationRunWithCandidates(ctx context.Context, run optimizer.OptimizationRun, candidates []optimizer.OptimizationCandidate) error {
	tx, err := r.evaluationTxBeginner().BeginEvaluationTx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	if err := insertOptimizationRun(ctx, tx, run); err != nil {
		return err
	}
	for _, candidate := range candidates {
		if err := insertOptimizationCandidate(ctx, tx, candidate); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

func (r *Repository) CreateOptimizationRunWithTask(ctx context.Context, run optimizer.OptimizationRun, candidates []optimizer.OptimizationCandidate, task taskqueue.Task) error {
	tx, err := r.evaluationTxBeginner().BeginEvaluationTx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	if err := insertTask(ctx, tx, task); err != nil {
		return err
	}
	if err := insertTaskEvent(ctx, tx, task.ID, "enqueued", map[string]any{"type": task.Type, "pool": task.Pool}); err != nil {
		return err
	}
	if err := insertOptimizationRun(ctx, tx, run); err != nil {
		return err
	}
	for _, candidate := range candidates {
		if err := insertOptimizationCandidate(ctx, tx, candidate); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

type optimizationExecer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func insertOptimizationRun(ctx context.Context, execer optimizationExecer, run optimizer.OptimizationRun) error {
	objective, err := json.Marshal(run.Objective)
	if err != nil {
		return err
	}
	searchSpace, err := json.Marshal(run.SearchSpace)
	if err != nil {
		return err
	}
	runner, err := json.Marshal(run.RunnerWithConfig())
	if err != nil {
		return err
	}
	checkpoint, err := json.Marshal(run.Checkpoint)
	if err != nil {
		return err
	}
	tokenUsage, err := json.Marshal(run.TokenUsage)
	if err != nil {
		return err
	}
	_, err = execer.Exec(ctx, `
		INSERT INTO optimization_runs(
			id, tenant_id, project_id, dataset_id, knowledge_base_id, objective, search_space, runner,
			status, status_reason, best_candidate_id, holdout_candidate_id, sampling_strategy,
			search_space_size, sampled_candidate_count, completed_candidate_count,
			checkpoint, token_usage, cost_usd, cost_budget_usd, cancel_requested_at,
			current_task_id, execution_generation, created_at, updated_at
		)
		VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,NULLIF($22,''),$23,$24,$25)`,
		run.ID, run.TenantID, run.ProjectID, run.DatasetID, run.KnowledgeBaseID, objective, searchSpace, runner,
		run.Status, run.StatusReason, run.BestCandidateID, run.HoldoutCandidateID, run.SamplingStrategy,
		run.SearchSpaceSize, run.SampledCandidateCount, run.CompletedCandidateCount,
		checkpoint, tokenUsage, run.CostUSD, run.CostBudgetUSD, run.CancelRequestedAt,
		run.CurrentTaskID, run.ExecutionGeneration, run.CreatedAt, run.UpdatedAt)
	return err
}

func (r *Repository) GetOptimizationRun(ctx context.Context, tenantID, runID string) (optimizer.OptimizationRun, bool, error) {
	row := r.evaluationQueryer().QueryRow(ctx, `
		SELECT id, tenant_id, COALESCE(project_id,''), dataset_id, knowledge_base_id, objective, search_space, runner,
			status, status_reason, best_candidate_id, holdout_candidate_id, sampling_strategy,
			search_space_size, sampled_candidate_count, completed_candidate_count,
			checkpoint, token_usage, cost_usd, cost_budget_usd, cancel_requested_at,
			COALESCE(current_task_id,''), execution_generation, created_at, updated_at
		FROM optimization_runs
		WHERE tenant_id=$1 AND id=$2`, tenantID, runID)
	run, err := scanOptimizationRun(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return optimizer.OptimizationRun{}, false, nil
		}
		return optimizer.OptimizationRun{}, false, err
	}
	return run, true, nil
}

func (r *Repository) GetOptimizationRunInProject(ctx context.Context, tenantID, projectID, runID string) (optimizer.OptimizationRun, bool, error) {
	row := r.evaluationQueryer().QueryRow(ctx, `
		SELECT id, tenant_id, COALESCE(project_id,''), dataset_id, knowledge_base_id, objective, search_space, runner,
			status, status_reason, best_candidate_id, holdout_candidate_id, sampling_strategy,
			search_space_size, sampled_candidate_count, completed_candidate_count,
			checkpoint, token_usage, cost_usd, cost_budget_usd, cancel_requested_at,
			COALESCE(current_task_id,''), execution_generation, created_at, updated_at
		FROM optimization_runs
		WHERE tenant_id=$1 AND project_id=$2 AND id=$3`, tenantID, projectID, runID)
	run, err := scanOptimizationRun(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return optimizer.OptimizationRun{}, false, nil
		}
		return optimizer.OptimizationRun{}, false, err
	}
	return run, true, nil
}

func (r *Repository) UpdateOptimizationRun(ctx context.Context, run optimizer.OptimizationRun) error {
	objective, err := json.Marshal(run.Objective)
	if err != nil {
		return err
	}
	searchSpace, err := json.Marshal(run.SearchSpace)
	if err != nil {
		return err
	}
	runner, err := json.Marshal(run.RunnerWithConfig())
	if err != nil {
		return err
	}
	checkpoint, err := json.Marshal(run.Checkpoint)
	if err != nil {
		return err
	}
	tokenUsage, err := json.Marshal(run.TokenUsage)
	if err != nil {
		return err
	}
	query := `
		UPDATE optimization_runs
		SET project_id=NULLIF($3,''), dataset_id=$4, knowledge_base_id=$5, objective=$6, search_space=$7, runner=$8,
			status=$9, status_reason=$10, best_candidate_id=$11, holdout_candidate_id=$12,
			sampling_strategy=$13, search_space_size=$14, sampled_candidate_count=$15,
			completed_candidate_count=$16, checkpoint=$17, token_usage=$18, cost_usd=$19,
			cost_budget_usd=$20, cancel_requested_at=$21, current_task_id=NULLIF($22,''),
			execution_generation=$23, updated_at=$24
		WHERE tenant_id=$1 AND id=$2`
	args := []any{
		run.TenantID, run.ID, run.ProjectID, run.DatasetID, run.KnowledgeBaseID, objective, searchSpace, runner, run.Status, run.StatusReason,
		run.BestCandidateID, run.HoldoutCandidateID, run.SamplingStrategy,
		run.SearchSpaceSize, run.SampledCandidateCount, run.CompletedCandidateCount,
		checkpoint, tokenUsage, run.CostUSD, run.CostBudgetUSD, run.CancelRequestedAt, run.CurrentTaskID, run.ExecutionGeneration, run.UpdatedAt,
	}
	lease, fenced := optimizer.ExecutionLeaseFromContext(ctx)
	if fenced {
		query += ` AND current_task_id=$25 AND execution_generation=$27
			AND EXISTS (SELECT 1 FROM task_queue t WHERE t.id=$25 AND t.lease_holder=$26
				AND t.lease_generation=$27 AND t.status IN ('leased','running','cancelling')
				AND t.lease_expires_at > NOW())`
		args = append(args, lease.TaskID, lease.Holder, lease.Generation)
	}
	tag, err := r.evaluationQueryer().Exec(ctx, query, args...)
	if err == nil && fenced && tag.RowsAffected() == 0 {
		return optimizer.ErrOptimizationOwnershipLost
	}
	return err
}

func (r *Repository) CompareAndSwapOptimizationRun(ctx context.Context, run optimizer.OptimizationRun, expectedStatus optimizer.RunStatus) (bool, error) {
	objective, err := json.Marshal(run.Objective)
	if err != nil {
		return false, err
	}
	searchSpace, err := json.Marshal(run.SearchSpace)
	if err != nil {
		return false, err
	}
	runner, err := json.Marshal(run.RunnerWithConfig())
	if err != nil {
		return false, err
	}
	checkpoint, err := json.Marshal(run.Checkpoint)
	if err != nil {
		return false, err
	}
	tokenUsage, err := json.Marshal(run.TokenUsage)
	if err != nil {
		return false, err
	}
	query := `
		UPDATE optimization_runs
		SET project_id=NULLIF($3,''), dataset_id=$4, knowledge_base_id=$5, objective=$6, search_space=$7, runner=$8,
			status=$9, status_reason=$10, best_candidate_id=$11, holdout_candidate_id=$12,
			sampling_strategy=$13, search_space_size=$14, sampled_candidate_count=$15,
			completed_candidate_count=$16, checkpoint=$17, token_usage=$18, cost_usd=$19,
			cost_budget_usd=$20, cancel_requested_at=$21, current_task_id=NULLIF($22,''),
			execution_generation=$23, updated_at=$24
		WHERE tenant_id=$1 AND id=$2 AND status=$25`
	args := []any{
		run.TenantID, run.ID, run.ProjectID, run.DatasetID, run.KnowledgeBaseID, objective, searchSpace, runner, run.Status, run.StatusReason,
		run.BestCandidateID, run.HoldoutCandidateID, run.SamplingStrategy,
		run.SearchSpaceSize, run.SampledCandidateCount, run.CompletedCandidateCount,
		checkpoint, tokenUsage, run.CostUSD, run.CostBudgetUSD, run.CancelRequestedAt, run.CurrentTaskID, run.ExecutionGeneration, run.UpdatedAt, expectedStatus,
	}
	lease, fenced := optimizer.ExecutionLeaseFromContext(ctx)
	if fenced {
		query += ` AND current_task_id=$26 AND execution_generation=$28
			AND EXISTS (SELECT 1 FROM task_queue t WHERE t.id=$26 AND t.lease_holder=$27
				AND t.lease_generation=$28 AND t.status IN ('leased','running','cancelling')
				AND t.lease_expires_at > NOW())`
		args = append(args, lease.TaskID, lease.Holder, lease.Generation)
	}
	tag, err := r.evaluationQueryer().Exec(ctx, query, args...)
	if err != nil {
		return false, err
	}
	if fenced && tag.RowsAffected() == 0 {
		return false, optimizer.ErrOptimizationOwnershipLost
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repository) ResumeOptimizationRunWithTask(ctx context.Context, run optimizer.OptimizationRun, expectedStatus optimizer.RunStatus, task taskqueue.Task) (bool, error) {
	objective, err := json.Marshal(run.Objective)
	if err != nil {
		return false, err
	}
	searchSpace, err := json.Marshal(run.SearchSpace)
	if err != nil {
		return false, err
	}
	runner, err := json.Marshal(run.RunnerWithConfig())
	if err != nil {
		return false, err
	}
	checkpoint, err := json.Marshal(run.Checkpoint)
	if err != nil {
		return false, err
	}
	tx, err := r.evaluationTxBeginner().BeginEvaluationTx(ctx)
	if err != nil {
		return false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	if err := insertTask(ctx, tx, task); err != nil {
		return false, err
	}
	if err := insertTaskEvent(ctx, tx, task.ID, "enqueued", map[string]any{"type": task.Type, "pool": task.Pool}); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE optimization_runs
		SET project_id=NULLIF($3,''), dataset_id=$4, knowledge_base_id=$5,
			objective=$6, search_space=$7, runner=$8, status=$9, status_reason=$10,
			cancel_requested_at=NULL, checkpoint=$11, current_task_id=$12,
			execution_generation=0, updated_at=$13
		WHERE tenant_id=$1 AND id=$2 AND status=$14`,
		run.TenantID, run.ID, run.ProjectID, run.DatasetID, run.KnowledgeBaseID,
		objective, searchSpace, runner, run.Status, run.StatusReason, checkpoint,
		run.CurrentTaskID, run.UpdatedAt, expectedStatus)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	committed = true
	return true, nil
}

func (r *Repository) CancelOptimizationRunWithTask(ctx context.Context, tenantID, runID, reason string, now time.Time) (optimizer.OptimizationRun, bool, error) {
	tx, err := r.evaluationTxBeginner().BeginEvaluationTx(ctx)
	if err != nil {
		return optimizer.OptimizationRun{}, false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	row := tx.QueryRow(ctx, `
		SELECT id, tenant_id, COALESCE(project_id,''), dataset_id, knowledge_base_id, objective, search_space, runner,
			status, status_reason, best_candidate_id, holdout_candidate_id, sampling_strategy,
			search_space_size, sampled_candidate_count, completed_candidate_count,
			checkpoint, token_usage, cost_usd, cost_budget_usd, cancel_requested_at,
			COALESCE(current_task_id,''), execution_generation, created_at, updated_at
		FROM optimization_runs WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, runID)
	run, err := scanOptimizationRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return optimizer.OptimizationRun{}, false, nil
	}
	if err != nil {
		return optimizer.OptimizationRun{}, false, err
	}
	if run.Status == optimizer.RunStatusCanceled || run.Status == optimizer.RunStatusCompleted ||
		run.Status == optimizer.RunStatusFailed || run.Status == optimizer.RunStatusBudgetStopped {
		if err := tx.Commit(ctx); err != nil {
			return optimizer.OptimizationRun{}, false, err
		}
		committed = true
		return run, true, nil
	}
	var taskStatus taskqueue.TaskStatus
	err = tx.QueryRow(ctx, `SELECT status FROM task_queue WHERE id=$1 FOR UPDATE`, run.CurrentTaskID).Scan(&taskStatus)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return optimizer.OptimizationRun{}, false, err
	}
	immediate := errors.Is(err, pgx.ErrNoRows) || taskStatus == taskqueue.TaskStatusQueued || taskStatus == taskqueue.TaskStatusFailedRetryable ||
		taskStatus == taskqueue.TaskStatusLeased || taskStatus == taskqueue.TaskStatusCancelled ||
		taskStatus == taskqueue.TaskStatusFailedTerminal || taskStatus == taskqueue.TaskStatusDeadLetter
	if err == nil {
		newTaskStatus := taskqueue.TaskStatusCancelling
		if immediate {
			newTaskStatus = taskqueue.TaskStatusCancelled
		}
		_, err = tx.Exec(ctx, `
			UPDATE task_queue SET status=$2, updated_at=$3,
				completed_at=CASE WHEN $2='cancelled' THEN $3 ELSE completed_at END,
				lease_holder=CASE WHEN $2='cancelled' THEN NULL ELSE lease_holder END,
				lease_expires_at=CASE WHEN $2='cancelled' THEN NULL ELSE lease_expires_at END
			WHERE id=$1`, run.CurrentTaskID, newTaskStatus, now)
		if err != nil {
			return optimizer.OptimizationRun{}, false, err
		}
		if err := insertTaskEvent(ctx, tx, run.CurrentTaskID, "cancelled", map[string]any{"from_status": taskStatus}); err != nil {
			return optimizer.OptimizationRun{}, false, err
		}
	}
	run.Status = optimizer.RunStatusCanceling
	if immediate {
		run.Status = optimizer.RunStatusCanceled
	}
	run.StatusReason = reason
	run.CancelRequestedAt = &now
	run.Checkpoint.CancelRequestedAt = &now
	run.Checkpoint.StatusReason = reason
	run.Checkpoint.Stage = string(run.Status)
	run.UpdatedAt = now
	checkpoint, err := json.Marshal(run.Checkpoint)
	if err != nil {
		return optimizer.OptimizationRun{}, false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE optimization_runs SET status=$3, status_reason=$4, cancel_requested_at=$5,
			checkpoint=$6, updated_at=$7 WHERE tenant_id=$1 AND id=$2`,
		tenantID, runID, run.Status, run.StatusReason, run.CancelRequestedAt, checkpoint, now); err != nil {
		return optimizer.OptimizationRun{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return optimizer.OptimizationRun{}, false, err
	}
	committed = true
	return run, true, nil
}

func (r *Repository) ClaimOptimizationExecution(ctx context.Context, tenantID, projectID, runID string, lease optimizer.ExecutionLease) error {
	tx, err := r.evaluationTxBeginner().BeginEvaluationTx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	var status optimizer.RunStatus
	var currentGeneration int64
	err = tx.QueryRow(ctx, `
		SELECT r.status, r.execution_generation
		FROM optimization_runs r
		JOIN task_queue t ON t.id=r.current_task_id
		WHERE r.tenant_id=$1 AND COALESCE(r.project_id,'')=$2 AND r.id=$3
			AND r.current_task_id=$4 AND t.lease_holder=$5 AND t.lease_generation=$6
			AND t.status IN ('leased','running') AND t.lease_expires_at > NOW()
		FOR UPDATE OF r, t`, tenantID, projectID, runID, lease.TaskID, lease.Holder, lease.Generation).Scan(&status, &currentGeneration)
	if errors.Is(err, pgx.ErrNoRows) {
		return optimizer.ErrOptimizationOwnershipLost
	}
	if err != nil {
		return err
	}
	if currentGeneration >= lease.Generation {
		return optimizer.ErrOptimizationOwnershipLost
	}
	switch status {
	case optimizer.RunStatusQueued, optimizer.RunStatusCanceling:
	case optimizer.RunStatusRunning:
		if _, err := tx.Exec(ctx, `
			UPDATE optimization_candidates c SET status='queued', updated_at=NOW()
			FROM optimization_runs r
			WHERE c.optimization_run_id=r.id AND r.id=$1 AND c.status IN ('running','evaluated','judged')
				AND NOT (COALESCE(r.checkpoint->'completed_candidate_ids','[]'::jsonb) ? c.id)`, runID); err != nil {
			return err
		}
		status = optimizer.RunStatusQueued
	default:
		return optimizer.ErrOptimizationOwnershipLost
	}
	tag, err := tx.Exec(ctx, `
		UPDATE optimization_runs SET status=$4, execution_generation=$5,
			checkpoint=jsonb_set(COALESCE(checkpoint,'{}'::jsonb), '{stage}', to_jsonb($6::text), true), updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2 AND current_task_id=$3 AND execution_generation < $5`,
		tenantID, runID, lease.TaskID, status, lease.Generation, map[bool]string{true: "recovered", false: "leased"}[status == optimizer.RunStatusQueued && currentGeneration > 0])
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return optimizer.ErrOptimizationOwnershipLost
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

func (r *Repository) CreateOptimizationCandidate(ctx context.Context, candidate optimizer.OptimizationCandidate) error {
	return insertOptimizationCandidate(ctx, r.evaluationQueryer(), candidate)
}

func insertOptimizationCandidate(ctx context.Context, execer optimizationExecer, candidate optimizer.OptimizationCandidate) error {
	config, confidence, metrics, tokenUsage, artifacts, namespaces, err := encodeOptimizationCandidate(candidate)
	if err != nil {
		return err
	}
	_, err = execer.Exec(ctx, `
		INSERT INTO optimization_candidates(
			id, optimization_run_id, config, status, evaluation_run_id, judge_run_id,
			objective_score, holdout_score, confidence, metrics, token_usage, cost_usd,
			artifacts, temp_namespaces, cleanup_status, expires_at, error, created_at, updated_at
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		ON CONFLICT (id) DO NOTHING`,
		candidate.ID, candidate.OptimizationRunID, config, candidate.Status, candidate.EvaluationRunID, candidate.JudgeRunID,
		candidate.ObjectiveScore, candidate.HoldoutScore, confidence, metrics, tokenUsage, candidate.CostUSD,
		artifacts, namespaces, candidate.CleanupStatus, candidate.ExpiresAt, candidate.Error, candidate.CreatedAt, candidate.UpdatedAt)
	return err
}

func (r *Repository) UpdateOptimizationCandidate(ctx context.Context, candidate optimizer.OptimizationCandidate) error {
	config, confidence, metrics, tokenUsage, artifacts, namespaces, err := encodeOptimizationCandidate(candidate)
	if err != nil {
		return err
	}
	query := `
		UPDATE optimization_candidates
		SET config=$3, status=$4, evaluation_run_id=$5, judge_run_id=$6,
			objective_score=$7, holdout_score=$8, confidence=$9, metrics=$10,
			token_usage=$11, cost_usd=$12, artifacts=$13, temp_namespaces=$14,
			cleanup_status=$15, expires_at=$16, error=$17, updated_at=$18
		WHERE optimization_run_id=$1 AND id=$2`
	args := []any{
		candidate.OptimizationRunID, candidate.ID, config, candidate.Status, candidate.EvaluationRunID, candidate.JudgeRunID,
		candidate.ObjectiveScore, candidate.HoldoutScore, confidence, metrics, tokenUsage, candidate.CostUSD,
		artifacts, namespaces, candidate.CleanupStatus, candidate.ExpiresAt, candidate.Error, candidate.UpdatedAt,
	}
	lease, fenced := optimizer.ExecutionLeaseFromContext(ctx)
	if fenced {
		query += ` AND EXISTS (SELECT 1 FROM optimization_runs r JOIN task_queue t ON t.id=r.current_task_id
			WHERE r.id=$1 AND r.current_task_id=$19 AND r.execution_generation=$21
				AND t.lease_holder=$20 AND t.lease_generation=$21
				AND t.status IN ('leased','running','cancelling') AND t.lease_expires_at > NOW())`
		args = append(args, lease.TaskID, lease.Holder, lease.Generation)
	}
	tag, err := r.evaluationQueryer().Exec(ctx, query, args...)
	if err == nil && fenced && tag.RowsAffected() == 0 {
		return optimizer.ErrOptimizationOwnershipLost
	}
	return err
}

func (r *Repository) CompareAndSwapOptimizationCandidate(ctx context.Context, candidate optimizer.OptimizationCandidate, expectedStatus optimizer.CandidateStatus) (bool, error) {
	config, confidence, metrics, tokenUsage, artifacts, namespaces, err := encodeOptimizationCandidate(candidate)
	if err != nil {
		return false, err
	}
	query := `
		UPDATE optimization_candidates
		SET config=$3, status=$4, evaluation_run_id=$5, judge_run_id=$6,
			objective_score=$7, holdout_score=$8, confidence=$9, metrics=$10,
			token_usage=$11, cost_usd=$12, artifacts=$13, temp_namespaces=$14,
			cleanup_status=$15, expires_at=$16, error=$17, updated_at=$18
		WHERE optimization_run_id=$1 AND id=$2 AND status=$19`
	args := []any{
		candidate.OptimizationRunID, candidate.ID, config, candidate.Status, candidate.EvaluationRunID, candidate.JudgeRunID,
		candidate.ObjectiveScore, candidate.HoldoutScore, confidence, metrics, tokenUsage, candidate.CostUSD,
		artifacts, namespaces, candidate.CleanupStatus, candidate.ExpiresAt, candidate.Error, candidate.UpdatedAt, expectedStatus,
	}
	lease, fenced := optimizer.ExecutionLeaseFromContext(ctx)
	if fenced {
		query += ` AND EXISTS (SELECT 1 FROM optimization_runs r JOIN task_queue t ON t.id=r.current_task_id
			WHERE r.id=$1 AND r.current_task_id=$20 AND r.execution_generation=$22
				AND t.lease_holder=$21 AND t.lease_generation=$22
				AND t.status IN ('leased','running','cancelling') AND t.lease_expires_at > NOW())`
		args = append(args, lease.TaskID, lease.Holder, lease.Generation)
	}
	tag, err := r.evaluationQueryer().Exec(ctx, query, args...)
	if err != nil {
		return false, err
	}
	if fenced && tag.RowsAffected() == 0 {
		return false, optimizer.ErrOptimizationOwnershipLost
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repository) ListOptimizationCandidates(ctx context.Context, tenantID, runID string) ([]optimizer.OptimizationCandidate, error) {
	rows, err := r.evaluationQueryer().Query(ctx, `
		SELECT c.id, c.optimization_run_id, c.config, c.status, c.evaluation_run_id, c.judge_run_id,
			c.objective_score, c.holdout_score, c.confidence, c.metrics, c.token_usage,
			c.cost_usd, c.artifacts, c.temp_namespaces, c.cleanup_status, c.expires_at,
			c.error, c.created_at, c.updated_at
		FROM optimization_candidates c
		JOIN optimization_runs r ON r.id = c.optimization_run_id
		WHERE r.tenant_id=$1 AND c.optimization_run_id=$2
		ORDER BY c.created_at, c.id`, tenantID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []optimizer.OptimizationCandidate
	for rows.Next() {
		candidate, err := scanOptimizationCandidate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, candidate)
	}
	return out, rows.Err()
}

func (r *Repository) StoreHarnessRun(ctx context.Context, run optimizer.HarnessRunRecord) error {
	argv, err := json.Marshal(run.Argv)
	if err != nil {
		return err
	}
	env, err := json.Marshal(run.EnvRedacted)
	if err != nil {
		return err
	}
	parsed, err := json.Marshal(run.ParsedMetrics)
	if err != nil {
		return err
	}
	metrics, err := json.Marshal(run.Metrics)
	if err != nil {
		return err
	}
	artifacts, err := json.Marshal(run.Artifacts)
	if err != nil {
		return err
	}
	_, err = r.evaluationQueryer().Exec(ctx, `
		INSERT INTO harness_runs(
			id, tenant_id, candidate_id, harness_type, argv, working_dir,
			env_redacted, stdout_redacted, stderr_redacted, parsed_metrics,
			exit_code, metrics, artifacts, started_at, ended_at
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (id) DO NOTHING`,
		run.ID, run.TenantID, run.CandidateID, run.HarnessType, argv, run.WorkingDir,
		env, run.StdoutRedacted, run.StderrRedacted, parsed,
		run.ExitCode, metrics, artifacts, run.StartedAt, run.EndedAt)
	return err
}

func encodeOptimizationCandidate(candidate optimizer.OptimizationCandidate) (config, confidence, metrics, tokenUsage, artifacts, namespaces []byte, err error) {
	if config, err = json.Marshal(candidate.Config); err != nil {
		return
	}
	if confidence, err = json.Marshal(candidate.Confidence); err != nil {
		return
	}
	if metrics, err = json.Marshal(candidate.Metrics); err != nil {
		return
	}
	if tokenUsage, err = json.Marshal(candidate.TokenUsage); err != nil {
		return
	}
	if artifacts, err = json.Marshal(candidate.Artifacts); err != nil {
		return
	}
	namespaces, err = json.Marshal(candidate.TempNamespaces)
	return
}

type optimizationRunScanner interface {
	Scan(dest ...any) error
}

func scanOptimizationRun(row optimizationRunScanner) (optimizer.OptimizationRun, error) {
	var run optimizer.OptimizationRun
	var objective, searchSpace, runner, checkpoint, tokenUsage []byte
	err := row.Scan(&run.ID, &run.TenantID, &run.ProjectID, &run.DatasetID, &run.KnowledgeBaseID, &objective, &searchSpace, &runner,
		&run.Status, &run.StatusReason, &run.BestCandidateID, &run.HoldoutCandidateID, &run.SamplingStrategy,
		&run.SearchSpaceSize, &run.SampledCandidateCount, &run.CompletedCandidateCount,
		&checkpoint, &tokenUsage, &run.CostUSD, &run.CostBudgetUSD, &run.CancelRequestedAt,
		&run.CurrentTaskID, &run.ExecutionGeneration, &run.CreatedAt, &run.UpdatedAt)
	if err != nil {
		return optimizer.OptimizationRun{}, err
	}
	_ = json.Unmarshal(objective, &run.Objective)
	_ = json.Unmarshal(searchSpace, &run.SearchSpace)
	if err := json.Unmarshal(runner, &run.Runner); err != nil {
		return optimizer.OptimizationRun{}, err
	}
	if err := run.LoadConfigFromRunner(); err != nil {
		return optimizer.OptimizationRun{}, err
	}
	_ = json.Unmarshal(checkpoint, &run.Checkpoint)
	_ = json.Unmarshal(tokenUsage, &run.TokenUsage)
	return run, nil
}

func scanOptimizationCandidate(row optimizationRunScanner) (optimizer.OptimizationCandidate, error) {
	var candidate optimizer.OptimizationCandidate
	var config, confidence, metrics, tokenUsage, artifacts, namespaces []byte
	err := row.Scan(&candidate.ID, &candidate.OptimizationRunID, &config, &candidate.Status, &candidate.EvaluationRunID, &candidate.JudgeRunID,
		&candidate.ObjectiveScore, &candidate.HoldoutScore, &confidence, &metrics, &tokenUsage,
		&candidate.CostUSD, &artifacts, &namespaces, &candidate.CleanupStatus, &candidate.ExpiresAt,
		&candidate.Error, &candidate.CreatedAt, &candidate.UpdatedAt)
	if err != nil {
		return optimizer.OptimizationCandidate{}, err
	}
	_ = json.Unmarshal(config, &candidate.Config)
	_ = json.Unmarshal(confidence, &candidate.Confidence)
	_ = json.Unmarshal(metrics, &candidate.Metrics)
	_ = json.Unmarshal(tokenUsage, &candidate.TokenUsage)
	_ = json.Unmarshal(artifacts, &candidate.Artifacts)
	_ = json.Unmarshal(namespaces, &candidate.TempNamespaces)
	return candidate, nil
}
