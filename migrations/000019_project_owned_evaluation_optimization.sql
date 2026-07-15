-- +goose Up
ALTER TABLE evaluation_runs ADD COLUMN project_id TEXT;
ALTER TABLE optimization_runs ADD COLUMN project_id TEXT;

UPDATE evaluation_runs AS run
SET project_id = dataset.project_id
FROM datasets AS dataset
WHERE run.tenant_id = dataset.tenant_id
  AND run.dataset_id = dataset.id
  AND run.project_id IS NULL;

UPDATE optimization_runs AS run
SET project_id = dataset.project_id
FROM datasets AS dataset
WHERE run.tenant_id = dataset.tenant_id
  AND run.dataset_id = dataset.id
  AND run.project_id IS NULL;

UPDATE evaluation_runs
SET project_id = 'prj_default_' || tenant_id
WHERE project_id IS NULL;

UPDATE optimization_runs
SET project_id = 'prj_default_' || tenant_id
WHERE project_id IS NULL;

ALTER TABLE evaluation_runs ADD CONSTRAINT evaluation_runs_tenant_project_fk
    FOREIGN KEY (tenant_id, project_id) REFERENCES projects(tenant_id, id) ON DELETE CASCADE;
ALTER TABLE optimization_runs ADD CONSTRAINT optimization_runs_tenant_project_fk
    FOREIGN KEY (tenant_id, project_id) REFERENCES projects(tenant_id, id) ON DELETE CASCADE;

CREATE INDEX evaluation_runs_tenant_project_created_idx
    ON evaluation_runs (tenant_id, project_id, created_at DESC);
CREATE INDEX optimization_runs_tenant_project_status_idx
    ON optimization_runs (tenant_id, project_id, status, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS optimization_runs_tenant_project_status_idx;
DROP INDEX IF EXISTS evaluation_runs_tenant_project_created_idx;
ALTER TABLE optimization_runs DROP CONSTRAINT IF EXISTS optimization_runs_tenant_project_fk;
ALTER TABLE evaluation_runs DROP CONSTRAINT IF EXISTS evaluation_runs_tenant_project_fk;
ALTER TABLE optimization_runs DROP COLUMN IF EXISTS project_id;
ALTER TABLE evaluation_runs DROP COLUMN IF EXISTS project_id;
