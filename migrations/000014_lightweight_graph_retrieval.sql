-- +goose Up
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

-- +goose Down
DROP TABLE IF EXISTS graph_relations;
