-- +goose Up
ALTER TABLE rag_node_spans
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- +goose Down
ALTER TABLE rag_node_spans
    DROP COLUMN IF EXISTS created_at;
