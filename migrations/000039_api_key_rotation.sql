-- +goose Up
ALTER TABLE api_keys
    ADD COLUMN rotated_from_key_id TEXT REFERENCES api_keys(id) ON DELETE RESTRICT;

CREATE UNIQUE INDEX api_keys_rotation_source_unique
    ON api_keys(rotated_from_key_id)
    WHERE rotated_from_key_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS api_keys_rotation_source_unique;

ALTER TABLE api_keys
    DROP COLUMN IF EXISTS rotated_from_key_id;
