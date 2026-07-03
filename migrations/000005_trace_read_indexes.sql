-- +goose Up
CREATE INDEX IF NOT EXISTS rag_traces_tenant_created_idx
    ON rag_traces (tenant_id, created_at DESC, id);

CREATE INDEX IF NOT EXISTS rag_traces_profile_created_idx
    ON rag_traces (profile, created_at DESC, id);

CREATE INDEX IF NOT EXISTS rag_traces_latency_created_idx
    ON rag_traces (latency_ms, created_at DESC, id);

CREATE INDEX IF NOT EXISTS rag_node_spans_trace_error_idx
    ON rag_node_spans (trace_id)
    WHERE error <> '';

CREATE INDEX IF NOT EXISTS rag_node_spans_trace_order_idx
    ON rag_node_spans (trace_id, sequence, created_at, id);

-- +goose Down
DROP INDEX IF EXISTS rag_node_spans_trace_order_idx;
DROP INDEX IF EXISTS rag_node_spans_trace_error_idx;
DROP INDEX IF EXISTS rag_traces_latency_created_idx;
DROP INDEX IF EXISTS rag_traces_profile_created_idx;
DROP INDEX IF EXISTS rag_traces_tenant_created_idx;
