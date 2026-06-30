-- +goose Up
ALTER TABLE chunks
    ADD COLUMN IF NOT EXISTS searchable BOOLEAN NOT NULL DEFAULT TRUE;

CREATE INDEX IF NOT EXISTS chunks_searchable_tenant_kb_idx
    ON chunks (tenant_id, knowledge_base_id)
    WHERE searchable;

-- +goose Down
DROP INDEX IF EXISTS chunks_searchable_tenant_kb_idx;

ALTER TABLE chunks
    DROP COLUMN IF EXISTS searchable;
