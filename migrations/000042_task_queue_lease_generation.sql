-- +goose Up
ALTER TABLE task_queue
    ADD COLUMN lease_generation BIGINT NOT NULL DEFAULT 0
    CHECK (lease_generation >= 0);

-- +goose Down
ALTER TABLE task_queue
    DROP COLUMN IF EXISTS lease_generation;
