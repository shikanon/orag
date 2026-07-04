-- +goose Up
ALTER TABLE chunks
    ADD COLUMN IF NOT EXISTS contextual_text TEXT NOT NULL DEFAULT '';

ALTER TABLE chunks
    ADD COLUMN IF NOT EXISTS search_text_tsvector TSVECTOR
    GENERATED ALWAYS AS (to_tsvector('simple', contextual_text || ' ' || content)) STORED;

CREATE INDEX IF NOT EXISTS chunks_search_text_tsv_idx
    ON chunks USING GIN (search_text_tsvector);

-- +goose Down
DROP INDEX IF EXISTS chunks_search_text_tsv_idx;

ALTER TABLE chunks
    DROP COLUMN IF EXISTS search_text_tsvector;

ALTER TABLE chunks
    DROP COLUMN IF EXISTS contextual_text;
