package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/shikanon/orag/internal/release"
)

var _ release.Repository = (*Repository)(nil)

func (r *Repository) Environment(ctx context.Context, projectID string, kind release.EnvironmentKind) (release.Environment, error) {
	var item release.Environment
	var bound bool
	err := r.Pool.QueryRow(ctx, `
		SELECT e.id, e.project_id, e.kind, COALESCE(e.active_version_id,''), e.revision,
		       EXISTS (SELECT 1 FROM project_environment_bindings b WHERE b.project_id=e.project_id AND b.environment_kind=e.kind)
		FROM project_environments e WHERE e.project_id=$1 AND e.kind=$2`, projectID, kind).
		Scan(&item.ID, &item.ProjectID, &item.Kind, &item.ActiveVersionID, &item.Revision, &bound)
	if errors.Is(err, pgx.ErrNoRows) {
		return release.Environment{}, release.ErrNotFound
	}
	if err != nil {
		return release.Environment{}, err
	}
	item.Bound = bound
	return item, nil
}

func (r *Repository) Version(ctx context.Context, projectID, versionID string) (release.Version, error) {
	var item release.Version
	err := r.Pool.QueryRow(ctx, `SELECT id, project_id, content_hash FROM pipeline_versions WHERE project_id=$1 AND id=$2`, projectID, versionID).
		Scan(&item.ID, &item.ProjectID, &item.ContentHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return release.Version{}, release.ErrNotFound
	}
	if err != nil {
		return release.Version{}, err
	}
	return item, nil
}

func (r *Repository) Evidence(ctx context.Context, projectID, versionID string, environment release.EnvironmentKind) (release.Evidence, error) {
	var item release.Evidence
	err := r.Pool.QueryRow(ctx, `SELECT version_id, environment_kind, passed, content_hash FROM project_release_validations WHERE project_id=$1 AND version_id=$2 AND environment_kind=$3`, projectID, versionID, environment).
		Scan(&item.VersionID, &item.EnvironmentID, &item.Passed, &item.ContentHash)
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

func (r *Repository) Commit(ctx context.Context, environment release.Environment, record release.Release) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `
		UPDATE project_environments SET active_version_id=$1, revision=$2, updated_at=$3
		WHERE project_id=$4 AND kind=$5 AND revision=$6`,
		environment.ActiveVersionID, environment.Revision, record.CreatedAt, environment.ProjectID, environment.Kind, environment.Revision-1)
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
