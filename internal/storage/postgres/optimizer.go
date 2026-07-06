package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/shikanon/orag/internal/optimizer"
)

func (r *Repository) CreateOptimizationRun(ctx context.Context, run optimizer.OptimizationRun) error {
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
	_, err = r.evaluationQueryer().Exec(ctx, `
		INSERT INTO optimization_runs(
			id, tenant_id, dataset_id, knowledge_base_id, objective, search_space, runner,
			status, status_reason, best_candidate_id, holdout_candidate_id, sampling_strategy,
			search_space_size, sampled_candidate_count, completed_candidate_count,
			checkpoint, token_usage, cost_usd, cost_budget_usd, cancel_requested_at,
			created_at, updated_at
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`,
		run.ID, run.TenantID, run.DatasetID, run.KnowledgeBaseID, objective, searchSpace, runner,
		run.Status, run.StatusReason, run.BestCandidateID, run.HoldoutCandidateID, run.SamplingStrategy,
		run.SearchSpaceSize, run.SampledCandidateCount, run.CompletedCandidateCount,
		checkpoint, tokenUsage, run.CostUSD, run.CostBudgetUSD, run.CancelRequestedAt,
		run.CreatedAt, run.UpdatedAt)
	return err
}

func (r *Repository) GetOptimizationRun(ctx context.Context, tenantID, runID string) (optimizer.OptimizationRun, bool, error) {
	row := r.evaluationQueryer().QueryRow(ctx, `
		SELECT id, tenant_id, dataset_id, knowledge_base_id, objective, search_space, runner,
			status, status_reason, best_candidate_id, holdout_candidate_id, sampling_strategy,
			search_space_size, sampled_candidate_count, completed_candidate_count,
			checkpoint, token_usage, cost_usd, cost_budget_usd, cancel_requested_at,
			created_at, updated_at
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
	_, err = r.evaluationQueryer().Exec(ctx, `
		UPDATE optimization_runs
		SET dataset_id=$3, knowledge_base_id=$4, objective=$5, search_space=$6, runner=$7,
			status=$8, status_reason=$9, best_candidate_id=$10, holdout_candidate_id=$11,
			sampling_strategy=$12, search_space_size=$13, sampled_candidate_count=$14,
			completed_candidate_count=$15, checkpoint=$16, token_usage=$17, cost_usd=$18,
			cost_budget_usd=$19, cancel_requested_at=$20, updated_at=$21
		WHERE tenant_id=$1 AND id=$2`,
		run.TenantID, run.ID, run.DatasetID, run.KnowledgeBaseID, objective, searchSpace, runner, run.Status, run.StatusReason,
		run.BestCandidateID, run.HoldoutCandidateID, run.SamplingStrategy,
		run.SearchSpaceSize, run.SampledCandidateCount, run.CompletedCandidateCount,
		checkpoint, tokenUsage, run.CostUSD, run.CostBudgetUSD, run.CancelRequestedAt, run.UpdatedAt)
	return err
}

func (r *Repository) CreateOptimizationCandidate(ctx context.Context, candidate optimizer.OptimizationCandidate) error {
	config, confidence, metrics, tokenUsage, artifacts, namespaces, err := encodeOptimizationCandidate(candidate)
	if err != nil {
		return err
	}
	_, err = r.evaluationQueryer().Exec(ctx, `
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
	_, err = r.evaluationQueryer().Exec(ctx, `
		UPDATE optimization_candidates
		SET config=$3, status=$4, evaluation_run_id=$5, judge_run_id=$6,
			objective_score=$7, holdout_score=$8, confidence=$9, metrics=$10,
			token_usage=$11, cost_usd=$12, artifacts=$13, temp_namespaces=$14,
			cleanup_status=$15, expires_at=$16, error=$17, updated_at=$18
		WHERE optimization_run_id=$1 AND id=$2`,
		candidate.OptimizationRunID, candidate.ID, config, candidate.Status, candidate.EvaluationRunID, candidate.JudgeRunID,
		candidate.ObjectiveScore, candidate.HoldoutScore, confidence, metrics, tokenUsage, candidate.CostUSD,
		artifacts, namespaces, candidate.CleanupStatus, candidate.ExpiresAt, candidate.Error, candidate.UpdatedAt)
	return err
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
	err := row.Scan(&run.ID, &run.TenantID, &run.DatasetID, &run.KnowledgeBaseID, &objective, &searchSpace, &runner,
		&run.Status, &run.StatusReason, &run.BestCandidateID, &run.HoldoutCandidateID, &run.SamplingStrategy,
		&run.SearchSpaceSize, &run.SampledCandidateCount, &run.CompletedCandidateCount,
		&checkpoint, &tokenUsage, &run.CostUSD, &run.CostBudgetUSD, &run.CancelRequestedAt,
		&run.CreatedAt, &run.UpdatedAt)
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
