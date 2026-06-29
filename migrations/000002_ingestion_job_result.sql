-- +goose Up
ALTER TABLE ingestion_jobs
    ADD COLUMN IF NOT EXISTS document_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS chunk_count INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE ingestion_jobs
    DROP COLUMN IF EXISTS chunk_count,
    DROP COLUMN IF EXISTS document_id;
