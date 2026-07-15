package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shikanon/orag/internal/tutorial"
)

var _ tutorial.CloneRepository = (*TutorialCloneRepository)(nil)

// TutorialCloneRepository owns only tutorial workflow state. Project creation
// stays in the project service so ordinary and tutorial-created projects share
// the same environment initialization rules.
type TutorialCloneRepository struct {
	pool *pgxpool.Pool
}

func NewTutorialCloneRepository(pool *pgxpool.Pool) *TutorialCloneRepository {
	return &TutorialCloneRepository{pool: pool}
}

func (r *TutorialCloneRepository) CreateOrGet(ctx context.Context, job tutorial.CloneJob) (tutorial.CloneJob, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	defer tx.Rollback(ctx)

	created, err := scanTutorialCloneJob(tx.QueryRow(ctx, `
		INSERT INTO tutorial_clone_jobs(
			id, tenant_id, subject_id, project_id, project_name, project_description,
			template_id, template_version, pack_tier, idempotency_key,
			stage, status, attempt, last_error_code, created_at, updated_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (tenant_id, subject_id, template_id, template_version, idempotency_key) DO NOTHING
		RETURNING id, tenant_id, subject_id, project_id, project_name, project_description,
		          template_id, template_version, pack_tier, idempotency_key,
		          stage, status, attempt, last_error_code, created_at, updated_at`,
		job.ID, job.TenantID, job.SubjectID, job.ProjectID, job.ProjectName, job.ProjectDescription,
		job.TemplateID, job.TemplateVersion, job.Tier, job.IdempotencyKey,
		job.Stage, job.Status, job.Attempt, job.LastErrorCode, job.CreatedAt, job.UpdatedAt,
	))
	if err == nil {
		for _, event := range job.Events {
			if _, err := tx.Exec(ctx, `
				INSERT INTO tutorial_clone_stage_events(job_id, stage, outcome, detail_code, occurred_at)
				VALUES($1,$2,$3,$4,$5)`, created.ID, event.Stage, event.Outcome, event.DetailCode, event.OccurredAt); err != nil {
				return tutorial.CloneJob{}, false, err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return tutorial.CloneJob{}, false, err
		}
		return cloneJobWithEvents(created, job.Events), false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return tutorial.CloneJob{}, false, err
	}
	existing, err := scanTutorialCloneJob(tx.QueryRow(ctx, `
		SELECT id, tenant_id, subject_id, project_id, project_name, project_description,
		       template_id, template_version, pack_tier, idempotency_key,
		       stage, status, attempt, last_error_code, created_at, updated_at
		FROM tutorial_clone_jobs
		WHERE tenant_id=$1 AND subject_id=$2 AND template_id=$3 AND template_version=$4 AND idempotency_key=$5`,
		job.TenantID, job.SubjectID, job.TemplateID, job.TemplateVersion, job.IdempotencyKey,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tutorial.CloneJob{}, false, fmt.Errorf("tutorial clone insert conflicted without matching idempotency job")
		}
		return tutorial.CloneJob{}, false, err
	}
	events, err := tutorialCloneEvents(ctx, tx, existing.ID)
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return tutorial.CloneJob{}, false, err
	}
	return cloneJobWithEvents(existing, events), true, nil
}

func (r *TutorialCloneRepository) GetJob(ctx context.Context, tenantID, jobID string) (tutorial.CloneJob, bool, error) {
	job, err := scanTutorialCloneJob(r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, subject_id, project_id, project_name, project_description,
		       template_id, template_version, pack_tier, idempotency_key,
		       stage, status, attempt, last_error_code, created_at, updated_at
		FROM tutorial_clone_jobs WHERE tenant_id=$1 AND id=$2`, tenantID, jobID))
	if errors.Is(err, pgx.ErrNoRows) {
		return tutorial.CloneJob{}, false, nil
	}
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	events, err := tutorialCloneEvents(ctx, r.pool, job.ID)
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	return cloneJobWithEvents(job, events), true, nil
}

func (r *TutorialCloneRepository) Retry(ctx context.Context, tenantID, jobID string, stage tutorial.CloneStage, now time.Time) (tutorial.CloneJob, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	defer tx.Rollback(ctx)
	job, err := scanTutorialCloneJob(tx.QueryRow(ctx, `
		UPDATE tutorial_clone_jobs
		SET stage=$3, status='queued', attempt=attempt+1, last_error_code='', updated_at=$4
		WHERE tenant_id=$1 AND id=$2 AND status='failed'
		RETURNING id, tenant_id, subject_id, project_id, project_name, project_description,
		          template_id, template_version, pack_tier, idempotency_key,
		          stage, status, attempt, last_error_code, created_at, updated_at`, tenantID, jobID, stage, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return tutorial.CloneJob{}, false, nil
	}
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	event := tutorial.StageEvent{Stage: stage, Outcome: "retry_queued", OccurredAt: now}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tutorial_clone_stage_events(job_id, stage, outcome, detail_code, occurred_at)
		VALUES($1,$2,$3,$4,$5)`, job.ID, event.Stage, event.Outcome, event.DetailCode, event.OccurredAt); err != nil {
		return tutorial.CloneJob{}, false, err
	}
	events, err := tutorialCloneEvents(ctx, tx, job.ID)
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return tutorial.CloneJob{}, false, err
	}
	return cloneJobWithEvents(job, events), true, nil
}

func (r *TutorialCloneRepository) GetExperiment(ctx context.Context, tenantID, projectID string) (tutorial.Experiment, bool, error) {
	var item tutorial.Experiment
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, project_id, template_id, template_version, pack_tier, pack_status, created_at, updated_at
		FROM tutorial_experiments WHERE tenant_id=$1 AND project_id=$2`, tenantID, projectID).
		Scan(&item.ID, &item.TenantID, &item.ProjectID, &item.TemplateID, &item.TemplateVersion, &item.Tier, &item.PackStatus, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return tutorial.Experiment{}, false, nil
	}
	if err != nil {
		return tutorial.Experiment{}, false, err
	}
	return item, true, nil
}

func (r *TutorialCloneRepository) Acquire(ctx context.Context, tenantID, jobID string, now time.Time) (tutorial.CloneJob, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	defer tx.Rollback(ctx)
	job, err := scanTutorialCloneJob(tx.QueryRow(ctx, `
		UPDATE tutorial_clone_jobs SET status='running', updated_at=$3
		WHERE tenant_id=$1 AND id=$2 AND status='queued'
		RETURNING id, tenant_id, subject_id, project_id, project_name, project_description,
		          template_id, template_version, pack_tier, idempotency_key,
		          stage, status, attempt, last_error_code, created_at, updated_at`, tenantID, jobID, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return tutorial.CloneJob{}, false, nil
	}
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	event := tutorial.StageEvent{Stage: job.Stage, Outcome: "started", OccurredAt: now}
	if _, err := tx.Exec(ctx, `INSERT INTO tutorial_clone_stage_events(job_id, stage, outcome, detail_code, occurred_at) VALUES($1,$2,$3,$4,$5)`, job.ID, event.Stage, event.Outcome, event.DetailCode, event.OccurredAt); err != nil {
		return tutorial.CloneJob{}, false, err
	}
	events, err := tutorialCloneEvents(ctx, tx, job.ID)
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return tutorial.CloneJob{}, false, err
	}
	return cloneJobWithEvents(job, events), true, nil
}

func (r *TutorialCloneRepository) Advance(ctx context.Context, tenantID, jobID string, expected, next tutorial.CloneStage, status tutorial.CloneStatus, now time.Time) (tutorial.CloneJob, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	defer tx.Rollback(ctx)
	job, err := scanTutorialCloneJob(tx.QueryRow(ctx, `
		UPDATE tutorial_clone_jobs SET stage=$4, status=$5, updated_at=$6
		WHERE tenant_id=$1 AND id=$2 AND stage=$3 AND status='running'
		RETURNING id, tenant_id, subject_id, project_id, project_name, project_description,
		          template_id, template_version, pack_tier, idempotency_key,
		          stage, status, attempt, last_error_code, created_at, updated_at`, tenantID, jobID, expected, next, status, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return tutorial.CloneJob{}, false, nil
	}
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	event := tutorial.StageEvent{Stage: next, Outcome: "completed", OccurredAt: now}
	if _, err := tx.Exec(ctx, `INSERT INTO tutorial_clone_stage_events(job_id, stage, outcome, detail_code, occurred_at) VALUES($1,$2,$3,$4,$5)`, job.ID, event.Stage, event.Outcome, event.DetailCode, event.OccurredAt); err != nil {
		return tutorial.CloneJob{}, false, err
	}
	events, err := tutorialCloneEvents(ctx, tx, job.ID)
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return tutorial.CloneJob{}, false, err
	}
	return cloneJobWithEvents(job, events), true, nil
}

func (r *TutorialCloneRepository) Fail(ctx context.Context, tenantID, jobID string, stage tutorial.CloneStage, code string, now time.Time) (tutorial.CloneJob, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	defer tx.Rollback(ctx)
	job, err := scanTutorialCloneJob(tx.QueryRow(ctx, `
		UPDATE tutorial_clone_jobs SET status='failed', last_error_code=$4, updated_at=$5
		WHERE tenant_id=$1 AND id=$2 AND stage=$3 AND status='running'
		RETURNING id, tenant_id, subject_id, project_id, project_name, project_description,
		          template_id, template_version, pack_tier, idempotency_key,
		          stage, status, attempt, last_error_code, created_at, updated_at`, tenantID, jobID, stage, code, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return tutorial.CloneJob{}, false, nil
	}
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	event := tutorial.StageEvent{Stage: stage, Outcome: "failed", DetailCode: code, OccurredAt: now}
	if _, err := tx.Exec(ctx, `INSERT INTO tutorial_clone_stage_events(job_id, stage, outcome, detail_code, occurred_at) VALUES($1,$2,$3,$4,$5)`, job.ID, event.Stage, event.Outcome, event.DetailCode, event.OccurredAt); err != nil {
		return tutorial.CloneJob{}, false, err
	}
	events, err := tutorialCloneEvents(ctx, tx, job.ID)
	if err != nil {
		return tutorial.CloneJob{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return tutorial.CloneJob{}, false, err
	}
	return cloneJobWithEvents(job, events), true, nil
}

func (r *TutorialCloneRepository) EnsureExperiment(ctx context.Context, item tutorial.Experiment) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO tutorial_experiments(id, tenant_id, project_id, template_id, template_version, pack_tier, pack_status, created_at, updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (project_id) DO NOTHING`,
		item.ID, item.TenantID, item.ProjectID, item.TemplateID, item.TemplateVersion, item.Tier, item.PackStatus, item.CreatedAt, item.UpdatedAt,
	)
	return err
}

func (r *TutorialCloneRepository) SetExperimentStatus(ctx context.Context, tenantID, projectID string, status tutorial.PackStatus, now time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE tutorial_experiments SET pack_status=$3, updated_at=$4
		WHERE tenant_id=$1 AND project_id=$2`, tenantID, projectID, status, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return tutorial.ErrCloneExperimentAbsent
	}
	return nil
}

type tutorialCloneEventQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func tutorialCloneEvents(ctx context.Context, queryer tutorialCloneEventQuerier, jobID string) ([]tutorial.StageEvent, error) {
	rows, err := queryer.Query(ctx, `
		SELECT stage, outcome, detail_code, occurred_at
		FROM tutorial_clone_stage_events WHERE job_id=$1 ORDER BY occurred_at ASC, id ASC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]tutorial.StageEvent, 0)
	for rows.Next() {
		var event tutorial.StageEvent
		if err := rows.Scan(&event.Stage, &event.Outcome, &event.DetailCode, &event.OccurredAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func scanTutorialCloneJob(row pgx.Row) (tutorial.CloneJob, error) {
	var job tutorial.CloneJob
	err := row.Scan(
		&job.ID, &job.TenantID, &job.SubjectID, &job.ProjectID, &job.ProjectName, &job.ProjectDescription,
		&job.TemplateID, &job.TemplateVersion, &job.Tier, &job.IdempotencyKey,
		&job.Stage, &job.Status, &job.Attempt, &job.LastErrorCode, &job.CreatedAt, &job.UpdatedAt,
	)
	return job, err
}

func cloneJobWithEvents(job tutorial.CloneJob, events []tutorial.StageEvent) tutorial.CloneJob {
	job.Events = append([]tutorial.StageEvent(nil), events...)
	return job
}
