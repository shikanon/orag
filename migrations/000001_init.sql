-- +goose Up
CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, username)
);

CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS knowledge_bases (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS documents (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    knowledge_base_id TEXT NOT NULL REFERENCES knowledge_bases(id),
    source_uri TEXT NOT NULL,
    title TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, knowledge_base_id, content_hash)
);

CREATE TABLE IF NOT EXISTS chunks (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    knowledge_base_id TEXT NOT NULL REFERENCES knowledge_bases(id),
    document_id TEXT NOT NULL REFERENCES documents(id),
    content TEXT NOT NULL,
    contextual_text TEXT NOT NULL DEFAULT '',
    content_tsvector TSVECTOR GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED,
    search_text_tsvector TSVECTOR GENERATED ALWAYS AS (to_tsvector('simple', contextual_text || ' ' || content)) STORED,
    source_uri TEXT NOT NULL,
    page INT NOT NULL DEFAULT 0,
    section TEXT NOT NULL DEFAULT '',
    offset_start INT NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    searchable BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS chunks_tenant_kb_idx ON chunks (tenant_id, knowledge_base_id);
CREATE INDEX IF NOT EXISTS chunks_content_tsv_idx ON chunks USING GIN (content_tsvector);
CREATE INDEX IF NOT EXISTS chunks_search_text_tsv_idx ON chunks USING GIN (search_text_tsvector);

CREATE TABLE IF NOT EXISTS graph_relations (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    knowledge_base_id TEXT NOT NULL REFERENCES knowledge_bases(id),
    document_id TEXT NOT NULL REFERENCES documents(id),
    source_chunk_id TEXT NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    target_chunk_id TEXT NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    subject TEXT NOT NULL,
    predicate TEXT NOT NULL,
    object TEXT NOT NULL,
    weight DOUBLE PRECISION NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS graph_relations_subject_idx ON graph_relations (tenant_id, knowledge_base_id, (lower(subject)));
CREATE INDEX IF NOT EXISTS graph_relations_object_idx ON graph_relations (tenant_id, knowledge_base_id, (lower(object)));
CREATE INDEX IF NOT EXISTS graph_relations_document_idx ON graph_relations (tenant_id, knowledge_base_id, document_id);

CREATE TABLE IF NOT EXISTS ingestion_jobs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    knowledge_base_id TEXT NOT NULL REFERENCES knowledge_bases(id),
    status TEXT NOT NULL,
    source_uri TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS conversations (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id),
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS semantic_cache_entries (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    query TEXT NOT NULL,
    answer TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS datasets (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    version TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS dataset_items (
    id TEXT PRIMARY KEY,
    dataset_id TEXT NOT NULL REFERENCES datasets(id),
    query TEXT NOT NULL,
    ground_truth TEXT NOT NULL,
    relevant_doc_ids JSONB NOT NULL DEFAULT '[]'::jsonb
);

CREATE TABLE IF NOT EXISTS evaluation_runs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    dataset_id TEXT NOT NULL REFERENCES datasets(id),
    profile TEXT NOT NULL,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS evaluation_results (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES evaluation_runs(id),
    dataset_item_id TEXT NOT NULL REFERENCES dataset_items(id),
    answer TEXT NOT NULL,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS rag_profiles (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rag_traces (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    query TEXT NOT NULL,
    profile TEXT NOT NULL,
    latency_ms BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rag_node_spans (
    id TEXT PRIMARY KEY,
    trace_id TEXT NOT NULL REFERENCES rag_traces(id),
    node_name TEXT NOT NULL,
    sequence INT NOT NULL DEFAULT 0,
    latency_ms BIGINT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS rag_node_spans_trace_sequence_uidx ON rag_node_spans (trace_id, sequence);

-- +goose Down
DROP TABLE IF EXISTS rag_node_spans;
DROP TABLE IF EXISTS rag_traces;
DROP TABLE IF EXISTS rag_profiles;
DROP TABLE IF EXISTS evaluation_results;
DROP TABLE IF EXISTS evaluation_runs;
DROP TABLE IF EXISTS dataset_items;
DROP TABLE IF EXISTS datasets;
DROP TABLE IF EXISTS semantic_cache_entries;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS ingestion_jobs;
DROP TABLE IF EXISTS graph_relations;
DROP TABLE IF EXISTS chunks;
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS knowledge_bases;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;
