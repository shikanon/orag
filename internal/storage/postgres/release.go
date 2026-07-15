package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shikanon/orag/internal/release"
)

var _ release.Repository = (*Repository)(nil)

func (r *Repository) Environments(ctx context.Context, projectID string) ([]release.Environment, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, project_id, kind, COALESCE(active_version_id,''), COALESCE(active_release_id,''), revision, EXISTS (SELECT 1 FROM project_environment_bindings b WHERE b.project_id=e.project_id AND b.environment_kind=e.kind) FROM project_environments e WHERE project_id=$1 ORDER BY CASE kind WHEN 'development' THEN 1 WHEN 'staging' THEN 2 WHEN 'production' THEN 3 ELSE 4 END`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]release.Environment, 0, 3)
	for rows.Next() {
		var item release.Environment
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.Kind, &item.ActiveVersionID, &item.ActiveReleaseID, &item.Revision, &item.Bound); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Environment(ctx context.Context, projectID string, kind release.EnvironmentKind) (release.Environment, error) {
	var item release.Environment
	var bound bool
	err := r.Pool.QueryRow(ctx, `
		SELECT e.id, e.project_id, e.kind, COALESCE(e.active_version_id,''), COALESCE(e.active_release_id,''), e.revision,
		       EXISTS (SELECT 1 FROM project_environment_bindings b WHERE b.project_id=e.project_id AND b.environment_kind=e.kind)
		FROM project_environments e WHERE e.project_id=$1 AND e.kind=$2`, projectID, kind).
		Scan(&item.ID, &item.ProjectID, &item.Kind, &item.ActiveVersionID, &item.ActiveReleaseID, &item.Revision, &bound)
	if errors.Is(err, pgx.ErrNoRows) {
		return release.Environment{}, release.ErrNotFound
	}
	if err != nil {
		return release.Environment{}, err
	}
	item.Bound = bound
	return item, nil
}

func (r *Repository) Releases(ctx context.Context, projectID string) ([]release.Release, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, project_id, source_version_id, target_version_id, source_environment, target_environment, action, actor, reason, created_at FROM project_releases WHERE project_id=$1 ORDER BY created_at DESC, id DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]release.Release, 0)
	for rows.Next() {
		var item release.Release
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.SourceVersionID, &item.TargetVersionID, &item.SourceEnvironment, &item.TargetEnvironment, &item.Action, &item.Actor, &item.Reason, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Versions(ctx context.Context, projectID string) ([]release.Version, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, project_id, COALESCE(pipeline_id,''), content_hash, definition, created_at FROM pipeline_versions WHERE project_id=$1 ORDER BY created_at DESC, id DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]release.Version, 0)
	for rows.Next() {
		var item release.Version
		var definition []byte
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.PipelineID, &item.ContentHash, &definition, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Definition = append(json.RawMessage(nil), definition...)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateVersion(ctx context.Context, version release.Version) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO pipeline_versions(id, project_id, pipeline_id, content_hash, definition, created_at) VALUES($1,$2,NULLIF($3,''),$4,$5,$6)`, version.ID, version.ProjectID, version.PipelineID, version.ContentHash, version.Definition, version.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return release.ErrConflict
		}
		return err
	}
	return nil
}

func (r *Repository) Version(ctx context.Context, projectID, versionID string) (release.Version, error) {
	var item release.Version
	var definition []byte
	err := r.Pool.QueryRow(ctx, `SELECT id, project_id, COALESCE(pipeline_id,''), content_hash, definition, created_at FROM pipeline_versions WHERE project_id=$1 AND id=$2`, projectID, versionID).
		Scan(&item.ID, &item.ProjectID, &item.PipelineID, &item.ContentHash, &definition, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return release.Version{}, release.ErrNotFound
	}
	if err != nil {
		return release.Version{}, err
	}
	item.Definition = append(json.RawMessage(nil), definition...)
	return item, nil
}

func (r *Repository) SaveEvidence(ctx context.Context, evidence release.Evidence) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO project_release_validations(project_id, version_id, environment_kind, passed, content_hash, dataset_id, evaluation_run_id, validated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (project_id, version_id, environment_kind) DO UPDATE SET passed=EXCLUDED.passed, content_hash=EXCLUDED.content_hash, dataset_id=EXCLUDED.dataset_id, evaluation_run_id=EXCLUDED.evaluation_run_id, validated_at=EXCLUDED.validated_at`, evidence.ProjectID, evidence.VersionID, evidence.EnvironmentID, evidence.Passed, evidence.ContentHash, evidence.DatasetID, evidence.EvaluationRunID, time.Now().UTC())
	return err
}

func (r *Repository) Evidence(ctx context.Context, projectID, versionID string, environment release.EnvironmentKind) (release.Evidence, error) {
	var item release.Evidence
	err := r.Pool.QueryRow(ctx, `SELECT version_id, environment_kind, passed, content_hash, dataset_id, evaluation_run_id FROM project_release_validations WHERE project_id=$1 AND version_id=$2 AND environment_kind=$3`, projectID, versionID, environment).
		Scan(&item.VersionID, &item.EnvironmentID, &item.Passed, &item.ContentHash, &item.DatasetID, &item.EvaluationRunID)
	if errors.Is(err, pgx.ErrNoRows) {
		return release.Evidence{}, nil
	}
	if err != nil {
		return release.Evidence{}, err
	}
	return item, nil
}

func (r *Repository) PreviouslyValidated(ctx context.Context, projectID, versionID string, environment release.EnvironmentKind) (bool, error) {
	var passed bool
	err := r.Pool.QueryRow(ctx, `SELECT passed FROM project_release_validations WHERE project_id=$1 AND version_id=$2 AND environment_kind=$3`, projectID, versionID, environment).Scan(&passed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return passed, err
}

func (r *Repository) Bind(ctx context.Context, projectID string, environment release.EnvironmentKind, bindingRef string) error {
	_, err := r.Pool.Exec(ctx, `
		INSERT INTO project_environment_bindings(project_id, environment_kind, binding_ref)
		VALUES($1,$2,$3)
		ON CONFLICT (project_id, environment_kind)
		DO UPDATE SET binding_ref=EXCLUDED.binding_ref, created_at=now()`, projectID, environment, bindingRef)
	return err
}

func (r *Repository) Commit(ctx context.Context, environment release.Environment, record release.Release) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `
		UPDATE project_environments SET active_version_id=$1, active_release_id=$2, revision=$3, updated_at=$4
		WHERE project_id=$5 AND kind=$6 AND revision=$7`,
		environment.ActiveVersionID, environment.ActiveReleaseID, environment.Revision, record.CreatedAt, environment.ProjectID, environment.Kind, environment.Revision-1)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return release.ErrConflict
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO project_releases(id, project_id, source_version_id, target_version_id, source_environment, target_environment, action, actor, reason, created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, record.ID, record.ProjectID, record.SourceVersionID, record.TargetVersionID, record.SourceEnvironment, record.TargetEnvironment, record.Action, record.Actor, record.Reason, record.CreatedAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
