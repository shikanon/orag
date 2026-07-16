package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shikanon/orag/internal/tutorial"
)

var _ tutorial.ExperimentRunRepository = (*TutorialCloneRepository)(nil)

const tutorialExperimentRunColumns = `id, tenant_id, project_id, experiment_id, variant,
	COALESCE(baseline_run_id, ''), comparison_fingerprint, definition_fingerprint,
	knowledge_base_id, dataset_id, profile, top_k, parser_method,
	chunk_size_tokens, chunk_overlap_tokens, contextual_retrieval_enabled, retrieval_strategy, reused_baseline_index, indexed_chunk_count, average_chunk_tokens,
	contextualized_chunk_count, average_context_tokens,
	stage, status, evaluation_run_id, failure_code, created_at, updated_at`

func (r *TutorialCloneRepository) CreateOrGetRun(ctx context.Context, run tutorial.ExperimentRun, idempotencyKey string) (tutorial.ExperimentRun, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tutorial.ExperimentRun{}, false, err
	}
	defer tx.Rollback(ctx)
	created, err := scanTutorialExperimentRun(tx.QueryRow(ctx, `
		INSERT INTO tutorial_experiment_runs(
			id, tenant_id, project_id, experiment_id, variant, baseline_run_id, comparison_fingerprint, definition_fingerprint,
			knowledge_base_id, dataset_id, profile, top_k, parser_method, chunk_size_tokens, chunk_overlap_tokens, contextual_retrieval_enabled, retrieval_strategy, reused_baseline_index,
			indexed_chunk_count, average_chunk_tokens, contextualized_chunk_count, average_context_tokens,
			idempotency_key, stage, status, evaluation_run_id, failure_code, created_at, updated_at
		) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)
		ON CONFLICT (tenant_id, project_id, variant, idempotency_key) DO NOTHING
		RETURNING `+tutorialExperimentRunColumns,
		run.ID, run.TenantID, run.ProjectID, run.ExperimentID, run.Variant, run.BaselineRunID, run.ComparisonFingerprint, run.DefinitionFingerprint,
		run.KnowledgeBaseID, run.DatasetID, run.Profile, run.TopK, run.ParserMethod, run.ChunkSizeTokens, run.ChunkOverlapTokens, run.ContextualRetrievalEnabled, run.RetrievalStrategy, run.ReusedBaselineIndex,
		run.IndexedChunkCount, run.AverageChunkTokens, run.ContextualizedChunkCount, run.AverageContextTokens,
		idempotencyKey, run.Stage, run.Status, run.EvaluationRunID, run.FailureCode, run.CreatedAt, run.UpdatedAt,
	))
	if err == nil {
		if err := insertTutorialExperimentRunEvents(ctx, tx, created.ID, run.Events); err != nil {
			return tutorial.ExperimentRun{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return tutorial.ExperimentRun{}, false, err
		}
		return cloneTutorialExperimentRunWithEvents(created, run.Events), false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return tutorial.ExperimentRun{}, false, err
	}
	existing, err := scanTutorialExperimentRun(tx.QueryRow(ctx, `
		SELECT `+tutorialExperimentRunColumns+`
		FROM tutorial_experiment_runs
		WHERE tenant_id=$1 AND project_id=$2 AND variant=$3 AND idempotency_key=$4`, run.TenantID, run.ProjectID, run.Variant, idempotencyKey))
	if err != nil {
		return tutorial.ExperimentRun{}, false, err
	}
	events, err := tutorialExperimentRunEvents(ctx, tx, existing.ID)
	if err != nil {
		return tutorial.ExperimentRun{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return tutorial.ExperimentRun{}, false, err
	}
	return cloneTutorialExperimentRunWithEvents(existing, events), true, nil
}

func (r *TutorialCloneRepository) GetExperimentRun(ctx context.Context, tenantID, runID string) (tutorial.ExperimentRun, bool, error) {
	run, err := scanTutorialExperimentRun(r.pool.QueryRow(ctx, `
		SELECT `+tutorialExperimentRunColumns+`
		FROM tutorial_experiment_runs WHERE tenant_id=$1 AND id=$2`, tenantID, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return tutorial.ExperimentRun{}, false, nil
	}
	if err != nil {
		return tutorial.ExperimentRun{}, false, err
	}
	events, err := tutorialExperimentRunEvents(ctx, r.pool, run.ID)
	if err != nil {
		return tutorial.ExperimentRun{}, false, err
	}
	return cloneTutorialExperimentRunWithEvents(run, events), true, nil
}

func (r *TutorialCloneRepository) FindCompletedBaseline(ctx context.Context, tenantID, projectID, experimentID, comparisonFingerprint string) (tutorial.ExperimentRun, bool, error) {
	run, err := scanTutorialExperimentRun(r.pool.QueryRow(ctx, `
		SELECT `+tutorialExperimentRunColumns+`
		FROM tutorial_experiment_runs
		WHERE tenant_id=$1 AND project_id=$2 AND experiment_id=$3 AND variant='baseline'
		  AND comparison_fingerprint=$4 AND status='completed'
		ORDER BY updated_at DESC, id DESC LIMIT 1`, tenantID, projectID, experimentID, comparisonFingerprint))
	if errors.Is(err, pgx.ErrNoRows) {
		return tutorial.ExperimentRun{}, false, nil
	}
	if err != nil {
		return tutorial.ExperimentRun{}, false, err
	}
	events, err := tutorialExperimentRunEvents(ctx, r.pool, run.ID)
	if err != nil {
		return tutorial.ExperimentRun{}, false, err
	}
	return cloneTutorialExperimentRunWithEvents(run, events), true, nil
}

func (r *TutorialCloneRepository) AcquireExperimentRun(ctx context.Context, tenantID, runID string, now time.Time) (tutorial.ExperimentRun, bool, error) {
	return r.transitionExperimentRun(ctx, tenantID, runID, `
		UPDATE tutorial_experiment_runs SET status='running', updated_at=$3
		WHERE tenant_id=$1 AND id=$2 AND status='queued'
		RETURNING `+tutorialExperimentRunColumns, now, "started", "")
}

func (r *TutorialCloneRepository) AdvanceExperimentRun(ctx context.Context, tenantID, runID string, expected, next tutorial.ExperimentRunStage, now time.Time) (tutorial.ExperimentRun, bool, error) {
	return r.transitionExperimentRun(ctx, tenantID, runID, `
		UPDATE tutorial_experiment_runs SET stage=$3, status='queued', updated_at=$4
		WHERE tenant_id=$1 AND id=$2 AND stage=$5 AND status='running'
		RETURNING `+tutorialExperimentRunColumns, next, now, expected, "completed", "")
}

func (r *TutorialCloneRepository) RecordExperimentRunIndexStats(ctx context.Context, tenantID, runID string, stats tutorial.ExperimentRunIndexStats, now time.Time) (tutorial.ExperimentRun, bool, error) {
	return r.transitionExperimentRun(ctx, tenantID, runID, `
		UPDATE tutorial_experiment_runs
		SET indexed_chunk_count=$3, average_chunk_tokens=$4, contextualized_chunk_count=$5, average_context_tokens=$6, updated_at=$7
		WHERE tenant_id=$1 AND id=$2 AND stage='index_private_pack' AND status='running'
		RETURNING `+tutorialExperimentRunColumns, stats.ChunkCount, stats.AverageChunkTokens, stats.ContextualizedChunkCount, stats.AverageContextTokens, now, "indexed", "")
}

func (r *TutorialCloneRepository) CompleteExperimentRun(ctx context.Context, tenantID, runID, evaluationID string, now time.Time) (tutorial.ExperimentRun, bool, error) {
	return r.transitionExperimentRun(ctx, tenantID, runID, `
		UPDATE tutorial_experiment_runs SET stage='completed', status='completed', evaluation_run_id=$3, updated_at=$4
		WHERE tenant_id=$1 AND id=$2 AND stage='run_evaluation' AND status='running'
		RETURNING `+tutorialExperimentRunColumns, evaluationID, now, "completed", "")
}

func (r *TutorialCloneRepository) FailExperimentRun(ctx context.Context, tenantID, runID string, stage tutorial.ExperimentRunStage, code string, now time.Time) (tutorial.ExperimentRun, bool, error) {
	return r.transitionExperimentRun(ctx, tenantID, runID, `
		UPDATE tutorial_experiment_runs SET status='failed', failure_code=$3, updated_at=$4
		WHERE tenant_id=$1 AND id=$2 AND stage=$5 AND status='running'
		RETURNING `+tutorialExperimentRunColumns, code, now, stage, "failed", code)
}

func (r *TutorialCloneRepository) CancelExperimentRun(ctx context.Context, tenantID, runID string, now time.Time) (tutorial.ExperimentRun, bool, error) {
	run, changed, err := r.transitionExperimentRun(ctx, tenantID, runID, `
		UPDATE tutorial_experiment_runs SET status='cancelled', updated_at=$3
		WHERE tenant_id=$1 AND id=$2 AND status='queued'
		RETURNING `+tutorialExperimentRunColumns, now, "cancelled", "")
	if err != nil || changed {
		return run, changed, err
	}
	return r.transitionExperimentRun(ctx, tenantID, runID, `
		UPDATE tutorial_experiment_runs SET status='cancel_requested', updated_at=$3
		WHERE tenant_id=$1 AND id=$2 AND status='running'
		RETURNING `+tutorialExperimentRunColumns, now, "cancel_requested", "")
}

func (r *TutorialCloneRepository) MarkExperimentRunCancelled(ctx context.Context, tenantID, runID string, now time.Time) (tutorial.ExperimentRun, bool, error) {
	return r.transitionExperimentRun(ctx, tenantID, runID, `
		UPDATE tutorial_experiment_runs SET status='cancelled', updated_at=$3
		WHERE tenant_id=$1 AND id=$2 AND status='cancel_requested'
		RETURNING `+tutorialExperimentRunColumns, now, "cancelled", "")
}

func (r *TutorialCloneRepository) transitionExperimentRun(ctx context.Context, tenantID, runID, statement string, args ...any) (tutorial.ExperimentRun, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tutorial.ExperimentRun{}, false, err
	}
	defer tx.Rollback(ctx)
	arguments := append([]any{tenantID, runID}, args[:len(args)-2]...)
	outcome, detail := args[len(args)-2].(string), args[len(args)-1].(string)
	run, err := scanTutorialExperimentRun(tx.QueryRow(ctx, statement, arguments...))
	if errors.Is(err, pgx.ErrNoRows) {
		return tutorial.ExperimentRun{}, false, nil
	}
	if err != nil {
		return tutorial.ExperimentRun{}, false, err
	}
	event := tutorial.ExperimentRunEvent{Stage: run.Stage, Outcome: outcome, DetailCode: detail, OccurredAt: run.UpdatedAt}
	if err := insertTutorialExperimentRunEvents(ctx, tx, run.ID, []tutorial.ExperimentRunEvent{event}); err != nil {
		return tutorial.ExperimentRun{}, false, err
	}
	events, err := tutorialExperimentRunEvents(ctx, tx, run.ID)
	if err != nil {
		return tutorial.ExperimentRun{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return tutorial.ExperimentRun{}, false, err
	}
	return cloneTutorialExperimentRunWithEvents(run, events), true, nil
}

func (r *TutorialCloneRepository) RecoverExperimentRuns(ctx context.Context, now time.Time) ([]tutorial.ExperimentRun, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	for _, recovery := range []struct{ from, to, outcome string }{{"running", "queued", "recovered"}, {"cancel_requested", "cancelled", "cancelled"}} {
		rows, err := tx.Query(ctx, `
			UPDATE tutorial_experiment_runs SET status=$2, updated_at=$1
			WHERE status=$3
		RETURNING `+tutorialExperimentRunColumns, now, recovery.to, recovery.from)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			run, err := scanTutorialExperimentRun(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			if err := insertTutorialExperimentRunEvents(ctx, tx, run.ID, []tutorial.ExperimentRunEvent{{Stage: run.Stage, Outcome: recovery.outcome, OccurredAt: now}}); err != nil {
				rows.Close()
				return nil, err
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	rows, err := tx.Query(ctx, `
		SELECT `+tutorialExperimentRunColumns+`
		FROM tutorial_experiment_runs WHERE status='queued' ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	items := make([]tutorial.ExperimentRun, 0)
	for rows.Next() {
		run, err := scanTutorialExperimentRun(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		events, err := tutorialExperimentRunEvents(ctx, tx, run.ID)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, cloneTutorialExperimentRunWithEvents(run, events))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

func insertTutorialExperimentRunEvents(ctx context.Context, tx pgx.Tx, runID string, events []tutorial.ExperimentRunEvent) error {
	for _, event := range events {
		if _, err := tx.Exec(ctx, `
			INSERT INTO tutorial_experiment_run_events(run_id, stage, outcome, detail_code, occurred_at)
			VALUES($1,$2,$3,$4,$5)`, runID, event.Stage, event.Outcome, event.DetailCode, event.OccurredAt); err != nil {
			return err
		}
	}
	return nil
}

func tutorialExperimentRunEvents(ctx context.Context, queryer tutorialCloneEventQuerier, runID string) ([]tutorial.ExperimentRunEvent, error) {
	rows, err := queryer.Query(ctx, `
		SELECT stage, outcome, detail_code, occurred_at
		FROM tutorial_experiment_run_events WHERE run_id=$1 ORDER BY occurred_at ASC, id ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]tutorial.ExperimentRunEvent, 0)
	for rows.Next() {
		var event tutorial.ExperimentRunEvent
		if err := rows.Scan(&event.Stage, &event.Outcome, &event.DetailCode, &event.OccurredAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

type tutorialExperimentRunScanner interface{ Scan(...any) error }

func scanTutorialExperimentRun(row tutorialExperimentRunScanner) (tutorial.ExperimentRun, error) {
	var run tutorial.ExperimentRun
	err := row.Scan(
		&run.ID, &run.TenantID, &run.ProjectID, &run.ExperimentID, &run.Variant,
		&run.BaselineRunID, &run.ComparisonFingerprint, &run.DefinitionFingerprint,
		&run.KnowledgeBaseID, &run.DatasetID, &run.Profile, &run.TopK, &run.ParserMethod,
		&run.ChunkSizeTokens, &run.ChunkOverlapTokens, &run.ContextualRetrievalEnabled, &run.RetrievalStrategy, &run.ReusedBaselineIndex, &run.IndexedChunkCount, &run.AverageChunkTokens,
		&run.ContextualizedChunkCount, &run.AverageContextTokens,
		&run.Stage, &run.Status, &run.EvaluationRunID, &run.FailureCode, &run.CreatedAt, &run.UpdatedAt,
	)
	return run, err
}

func cloneTutorialExperimentRunWithEvents(run tutorial.ExperimentRun, events []tutorial.ExperimentRunEvent) tutorial.ExperimentRun {
	run.Events = append([]tutorial.ExperimentRunEvent(nil), events...)
	return run
}
