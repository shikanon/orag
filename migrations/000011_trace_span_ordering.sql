-- +goose Up
ALTER TABLE rag_node_spans
    ADD COLUMN IF NOT EXISTS sequence INT,
    ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS ended_at TIMESTAMPTZ;

WITH ordered AS (
    SELECT id, row_number() OVER (PARTITION BY trace_id ORDER BY created_at, id) AS seq
    FROM rag_node_spans
)
UPDATE rag_node_spans AS spans
SET sequence = ordered.seq
FROM ordered
WHERE spans.id = ordered.id
  AND spans.sequence IS NULL;

UPDATE rag_node_spans
SET started_at = COALESCE(started_at, created_at, now()),
    ended_at = COALESCE(ended_at, created_at, now());

ALTER TABLE rag_node_spans
    ALTER COLUMN sequence SET DEFAULT 0,
    ALTER COLUMN sequence SET NOT NULL,
    ALTER COLUMN started_at SET DEFAULT now(),
    ALTER COLUMN started_at SET NOT NULL,
    ALTER COLUMN ended_at SET DEFAULT now(),
    ALTER COLUMN ended_at SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS rag_node_spans_trace_sequence_uidx ON rag_node_spans (trace_id, sequence);

-- +goose Down
DROP INDEX IF EXISTS rag_node_spans_trace_sequence_uidx;

ALTER TABLE rag_node_spans
    DROP COLUMN IF EXISTS ended_at,
    DROP COLUMN IF EXISTS started_at,
    DROP COLUMN IF EXISTS sequence;
