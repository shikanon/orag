package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/shikanon/orag/internal/ingest"
)

func (r *Repository) CreateJob(ctx context.Context, job ingest.Job) (ingest.Job, error) {
	_, err := r.Pool.Exec(ctx, `
		INSERT INTO ingestion_jobs(id, tenant_id, knowledge_base_id, status, source_uri, error, document_id, chunk_count, created_at, updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		job.ID, job.TenantID, job.KnowledgeBaseID, string(job.Status), job.SourceURI, job.Error, job.DocumentID, job.ChunkCount, job.CreatedAt, job.UpdatedAt)
	return job, err
}

func (r *Repository) UpdateJob(ctx context.Context, job ingest.Job) error {
	_, err := r.Pool.Exec(ctx, `
		UPDATE ingestion_jobs
		SET status=$1, error=$2, document_id=$3, chunk_count=$4, updated_at=$5
		WHERE tenant_id=$6 AND id=$7`,
		string(job.Status), job.Error, job.DocumentID, job.ChunkCount, job.UpdatedAt, job.TenantID, job.ID)
	return err
}

func (r *Repository) GetJob(ctx context.Context, tenantID, id string) (ingest.Job, bool, error) {
	row := r.Pool.QueryRow(ctx, `
		SELECT id, tenant_id, knowledge_base_id, status, source_uri, error, document_id, chunk_count, created_at, updated_at
		FROM ingestion_jobs
		WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	var job ingest.Job
	var status string
	if err := row.Scan(&job.ID, &job.TenantID, &job.KnowledgeBaseID, &status, &job.SourceURI, &job.Error, &job.DocumentID, &job.ChunkCount, &job.CreatedAt, &job.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return ingest.Job{}, false, nil
		}
		return ingest.Job{}, false, err
	}
	job.Status = ingest.JobStatus(status)
	return job, true, nil
}
