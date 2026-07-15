package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shikanon/orag/internal/pipeline"
)

var _ pipeline.Repository = (*Repository)(nil)

func (r *Repository) CreatePipeline(ctx context.Context, item pipeline.Pipeline) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO pipelines(id, project_id, name, created_at, updated_at) VALUES($1,$2,$3,$4,$5)`, item.ID, item.ProjectID, item.Name, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return err
	}
	_, err = r.Pool.Exec(ctx, `INSERT INTO pipeline_drafts(pipeline_id, project_id, revision, schema_version, definition, updated_at) VALUES($1,$2,0,1,$3,$4)`, item.ID, item.ProjectID, []byte(`{"nodes":[],"edges":[]}`), item.UpdatedAt)
	return err
}

func (r *Repository) ListPipelines(ctx context.Context, projectID string) ([]pipeline.Pipeline, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, project_id, name, created_at, updated_at FROM pipelines WHERE project_id=$1 ORDER BY updated_at DESC, id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []pipeline.Pipeline{}
	for rows.Next() {
		var item pipeline.Pipeline
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.Name, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) GetDraft(ctx context.Context, projectID, pipelineID string) (pipeline.Draft, error) {
	var item pipeline.Draft
	var definition []byte
	err := r.Pool.QueryRow(ctx, `SELECT pipeline_id, project_id, revision, schema_version, definition, updated_at FROM pipeline_drafts WHERE project_id=$1 AND pipeline_id=$2`, projectID, pipelineID).Scan(&item.PipelineID, &item.ProjectID, &item.Revision, &item.SchemaVersion, &definition, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return pipeline.Draft{}, pipeline.ErrNotFound
	}
	if err != nil {
		return pipeline.Draft{}, err
	}
	if err := json.Unmarshal(definition, &item.Definition); err != nil {
		return pipeline.Draft{}, err
	}
	return item, nil
}

func (r *Repository) SaveDraft(ctx context.Context, projectID, pipelineID string, expected int64, draft pipeline.Draft) (pipeline.Draft, error) {
	body, err := json.Marshal(draft.Definition)
	if err != nil {
		return pipeline.Draft{}, err
	}
	updated := time.Now().UTC()
	result, err := r.Pool.Exec(ctx, `UPDATE pipeline_drafts SET revision=revision+1, schema_version=$4, definition=$5, updated_at=$6 WHERE project_id=$1 AND pipeline_id=$2 AND revision=$3`, projectID, pipelineID, expected, draft.SchemaVersion, body, updated)
	if err != nil {
		return pipeline.Draft{}, err
	}
	if result.RowsAffected() == 0 {
		current, getErr := r.GetDraft(ctx, projectID, pipelineID)
		if getErr != nil {
			return pipeline.Draft{}, getErr
		}
		return current, pipeline.ErrRevisionConflict
	}
	return r.GetDraft(ctx, projectID, pipelineID)
}
