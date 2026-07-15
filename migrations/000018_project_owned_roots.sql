-- +goose Up
ALTER TABLE knowledge_bases ADD COLUMN project_id TEXT;
ALTER TABLE datasets ADD COLUMN project_id TEXT;

INSERT INTO projects(id, tenant_id, name, description, created_at, updated_at)
SELECT 'prj_default_' || id, id, 'Legacy Default',
       'Compatibility project for resources created before project ownership.', now(), now()
FROM tenants ON CONFLICT (id) DO NOTHING;

INSERT INTO project_environments(id, project_id, kind, created_at, updated_at)
SELECT 'env_default_development_' || id, 'prj_default_' || id, 'development', now(), now()
FROM tenants ON CONFLICT (project_id, kind) DO NOTHING;
INSERT INTO project_environments(id, project_id, kind, created_at, updated_at)
SELECT 'env_default_staging_' || id, 'prj_default_' || id, 'staging', now(), now()
FROM tenants ON CONFLICT (project_id, kind) DO NOTHING;
INSERT INTO project_environments(id, project_id, kind, created_at, updated_at)
SELECT 'env_default_production_' || id, 'prj_default_' || id, 'production', now(), now()
FROM tenants ON CONFLICT (project_id, kind) DO NOTHING;

UPDATE knowledge_bases SET project_id='prj_default_' || tenant_id WHERE project_id IS NULL;
UPDATE datasets SET project_id='prj_default_' || tenant_id WHERE project_id IS NULL;

ALTER TABLE knowledge_bases ADD CONSTRAINT knowledge_bases_tenant_project_fk
    FOREIGN KEY (tenant_id, project_id) REFERENCES projects(tenant_id, id) ON DELETE CASCADE;
ALTER TABLE datasets ADD CONSTRAINT datasets_tenant_project_fk
    FOREIGN KEY (tenant_id, project_id) REFERENCES projects(tenant_id, id) ON DELETE CASCADE;

CREATE INDEX knowledge_bases_tenant_project_created_idx ON knowledge_bases (tenant_id, project_id, created_at);
CREATE INDEX datasets_tenant_project_created_idx ON datasets (tenant_id, project_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS datasets_tenant_project_created_idx;
DROP INDEX IF EXISTS knowledge_bases_tenant_project_created_idx;
ALTER TABLE datasets DROP CONSTRAINT IF EXISTS datasets_tenant_project_fk;
ALTER TABLE knowledge_bases DROP CONSTRAINT IF EXISTS knowledge_bases_tenant_project_fk;
ALTER TABLE datasets DROP COLUMN IF EXISTS project_id;
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS project_id;
