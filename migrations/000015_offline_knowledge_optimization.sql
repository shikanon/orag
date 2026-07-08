-- +goose Up
CREATE TABLE IF NOT EXISTS offline_knowledge_runs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    kb_id TEXT NOT NULL,
    status TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    config_hash TEXT NOT NULL,
    config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    total_questions INT NOT NULL DEFAULT 0,
    total_clusters INT NOT NULL DEFAULT 0,
    processed_clusters INT NOT NULL DEFAULT 0,
    created_items INT NOT NULL DEFAULT 0,
    verified_items INT NOT NULL DEFAULT 0,
    rejected_items INT NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    CONSTRAINT offline_knowledge_runs_window_unique UNIQUE (tenant_id, kb_id, window_start, window_end, config_hash)
);

CREATE TABLE IF NOT EXISTS offline_question_clusters (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    run_id TEXT NOT NULL REFERENCES offline_knowledge_runs(id) ON DELETE CASCADE,
    kb_id TEXT NOT NULL,
    canonical_question TEXT NOT NULL,
    normalized_question TEXT NOT NULL,
    question_hash TEXT NOT NULL,
    embedding_ref TEXT NOT NULL DEFAULT '',
    embedding_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    occurrence_count INT NOT NULL DEFAULT 0,
    sample_questions_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    trace_ids_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    long_tail BOOLEAN NOT NULL DEFAULT false,
    baseline_recall_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT offline_question_clusters_question_unique UNIQUE (tenant_id, kb_id, question_hash)
);

CREATE TABLE IF NOT EXISTS optimization_items (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    run_id TEXT NOT NULL REFERENCES offline_knowledge_runs(id) ON DELETE CASCADE,
    kb_id TEXT NOT NULL,
    question_cluster_id TEXT NOT NULL REFERENCES offline_question_clusters(id) ON DELETE CASCADE,
    item_type TEXT NOT NULL,
    status TEXT NOT NULL,
    canonical_question TEXT NOT NULL,
    final_answer TEXT NOT NULL DEFAULT '',
    recall_quality TEXT NOT NULL,
    failure_type TEXT NOT NULL DEFAULT '',
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    source_chunk_ids_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_doc_ids_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_fingerprints_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    evidence_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    deep_search_steps_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    analyzer_report_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    validation_report_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    eval_report_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS optimization_item_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    item_id TEXT NOT NULL REFERENCES optimization_items(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    operator TEXT NOT NULL DEFAULT '',
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS offline_negative_feedback (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    kb_id TEXT NOT NULL,
    trace_id TEXT NOT NULL DEFAULT '',
    query TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS shadow_retrieval_events (
    id TEXT NOT NULL,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    kb_id TEXT NOT NULL,
    optimization_item_id TEXT NOT NULL REFERENCES optimization_items(id) ON DELETE CASCADE,
    trace_id TEXT NOT NULL,
    query TEXT NOT NULL,
    matched BOOLEAN NOT NULL DEFAULT false,
    injected BOOLEAN NOT NULL DEFAULT false,
    rank INT NOT NULL DEFAULT 0,
    score DOUBLE PRECISION NOT NULL DEFAULT 0,
    recall_lift DOUBLE PRECISION NOT NULL DEFAULT 0,
    answer_lift DOUBLE PRECISION NOT NULL DEFAULT 0,
    hallucination_risk DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE IF NOT EXISTS shadow_retrieval_events_default
    PARTITION OF shadow_retrieval_events DEFAULT;

CREATE TABLE IF NOT EXISTS offline_codex_tool_audit (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    kb_id TEXT NOT NULL,
    session_id TEXT NOT NULL DEFAULT '',
    tool TEXT NOT NULL,
    rows INT NOT NULL DEFAULT 0,
    steps INT NOT NULL DEFAULT 0,
    allowed BOOLEAN NOT NULL DEFAULT false,
    error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS offline_knowledge_runs_tenant_status_idx
    ON offline_knowledge_runs (tenant_id, status, started_at DESC);
CREATE INDEX IF NOT EXISTS offline_knowledge_runs_tenant_kb_window_idx
    ON offline_knowledge_runs (tenant_id, kb_id, window_start DESC, window_end DESC);

CREATE INDEX IF NOT EXISTS offline_question_clusters_tenant_kb_hash_idx
    ON offline_question_clusters (tenant_id, kb_id, question_hash);
CREATE INDEX IF NOT EXISTS offline_question_clusters_run_idx
    ON offline_question_clusters (tenant_id, run_id, created_at DESC);

CREATE INDEX IF NOT EXISTS optimization_items_tenant_status_idx
    ON optimization_items (tenant_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS optimization_items_tenant_kb_idx
    ON optimization_items (tenant_id, kb_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS optimization_items_tenant_type_idx
    ON optimization_items (tenant_id, item_type, updated_at DESC);
CREATE INDEX IF NOT EXISTS optimization_items_question_cluster_idx
    ON optimization_items (tenant_id, question_cluster_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS optimization_item_events_item_created_idx
    ON optimization_item_events (tenant_id, item_id, created_at DESC);
CREATE INDEX IF NOT EXISTS optimization_item_events_type_created_idx
    ON optimization_item_events (tenant_id, event_type, created_at DESC);

CREATE INDEX IF NOT EXISTS offline_negative_feedback_scope_created_idx
    ON offline_negative_feedback (tenant_id, kb_id, created_at DESC);
CREATE INDEX IF NOT EXISTS offline_negative_feedback_trace_created_idx
    ON offline_negative_feedback (tenant_id, trace_id, created_at DESC);

CREATE INDEX IF NOT EXISTS shadow_retrieval_events_trace_created_idx
    ON shadow_retrieval_events (tenant_id, trace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS shadow_retrieval_events_item_created_idx
    ON shadow_retrieval_events (tenant_id, optimization_item_id, created_at DESC);
CREATE INDEX IF NOT EXISTS shadow_retrieval_events_created_idx
    ON shadow_retrieval_events (created_at DESC);

CREATE INDEX IF NOT EXISTS offline_codex_tool_audit_tenant_kb_started_idx
    ON offline_codex_tool_audit (tenant_id, kb_id, started_at DESC);
CREATE INDEX IF NOT EXISTS offline_codex_tool_audit_session_started_idx
    ON offline_codex_tool_audit (tenant_id, session_id, started_at DESC);
CREATE INDEX IF NOT EXISTS offline_codex_tool_audit_tool_started_idx
    ON offline_codex_tool_audit (tenant_id, tool, started_at DESC);

-- +goose Down
DROP INDEX IF EXISTS offline_codex_tool_audit_tool_started_idx;
DROP INDEX IF EXISTS offline_codex_tool_audit_session_started_idx;
DROP INDEX IF EXISTS offline_codex_tool_audit_tenant_kb_started_idx;
DROP INDEX IF EXISTS shadow_retrieval_events_created_idx;
DROP INDEX IF EXISTS shadow_retrieval_events_item_created_idx;
DROP INDEX IF EXISTS shadow_retrieval_events_trace_created_idx;
DROP INDEX IF EXISTS offline_negative_feedback_trace_created_idx;
DROP INDEX IF EXISTS offline_negative_feedback_scope_created_idx;
DROP INDEX IF EXISTS optimization_item_events_type_created_idx;
DROP INDEX IF EXISTS optimization_item_events_item_created_idx;
DROP INDEX IF EXISTS optimization_items_question_cluster_idx;
DROP INDEX IF EXISTS optimization_items_tenant_type_idx;
DROP INDEX IF EXISTS optimization_items_tenant_kb_idx;
DROP INDEX IF EXISTS optimization_items_tenant_status_idx;
DROP INDEX IF EXISTS offline_question_clusters_run_idx;
DROP INDEX IF EXISTS offline_question_clusters_tenant_kb_hash_idx;
DROP INDEX IF EXISTS offline_knowledge_runs_tenant_kb_window_idx;
DROP INDEX IF EXISTS offline_knowledge_runs_tenant_status_idx;

DROP TABLE IF EXISTS offline_codex_tool_audit;
DROP TABLE IF EXISTS shadow_retrieval_events_default;
DROP TABLE IF EXISTS shadow_retrieval_events;
DROP TABLE IF EXISTS offline_negative_feedback;
DROP TABLE IF EXISTS optimization_item_events;
DROP TABLE IF EXISTS optimization_items;
DROP TABLE IF EXISTS offline_question_clusters;
DROP TABLE IF EXISTS offline_knowledge_runs;
