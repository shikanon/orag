-- +goose Up
ALTER TABLE task_queue
    ADD COLUMN lease_generation BIGINT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE task_queue
    DROP COLUMN lease_generation;
