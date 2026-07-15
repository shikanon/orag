-- +goose Up
ALTER TABLE projects
    ADD CONSTRAINT projects_tenant_id_id_unique UNIQUE (tenant_id, id);

ALTER TABLE api_keys
    ADD COLUMN project_id TEXT,
    ADD COLUMN prefix TEXT,
    ADD COLUMN role TEXT,
    ADD COLUMN created_by TEXT,
    ADD COLUMN expires_at TIMESTAMPTZ,
    ADD COLUMN revoked_at TIMESTAMPTZ,
    ADD COLUMN last_used_at TIMESTAMPTZ;

UPDATE api_keys
SET prefix = 'legacy:' || id,
    role = 'tenant_admin',
    created_by = 'legacy:migration'
WHERE prefix IS NULL OR role IS NULL OR created_by IS NULL;

ALTER TABLE api_keys
    ALTER COLUMN prefix SET NOT NULL,
    ALTER COLUMN role SET NOT NULL,
    ALTER COLUMN created_by SET NOT NULL,
    ADD CONSTRAINT api_keys_role_check
        CHECK (role IN ('tenant_admin', 'project_editor', 'project_viewer')),
    ADD CONSTRAINT api_keys_project_role_check
        CHECK (role = 'tenant_admin' OR project_id IS NOT NULL),
    ADD CONSTRAINT api_keys_tenant_project_fk
        FOREIGN KEY (tenant_id, project_id)
        REFERENCES projects(tenant_id, id) ON DELETE CASCADE,
    ADD CONSTRAINT api_keys_expiry_check
        CHECK (expires_at IS NULL OR expires_at > created_at),
    ADD CONSTRAINT api_keys_prefix_unique UNIQUE (prefix),
    ADD CONSTRAINT api_keys_hash_unique UNIQUE (key_hash);

CREATE INDEX api_keys_tenant_created_idx
    ON api_keys (tenant_id, created_at DESC);

CREATE INDEX api_keys_active_lookup_idx
    ON api_keys (id)
    WHERE revoked_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS api_keys_active_lookup_idx;
DROP INDEX IF EXISTS api_keys_tenant_created_idx;

ALTER TABLE api_keys
    DROP CONSTRAINT IF EXISTS api_keys_hash_unique,
    DROP CONSTRAINT IF EXISTS api_keys_prefix_unique,
    DROP CONSTRAINT IF EXISTS api_keys_expiry_check,
    DROP CONSTRAINT IF EXISTS api_keys_project_role_check,
    DROP CONSTRAINT IF EXISTS api_keys_tenant_project_fk,
    DROP CONSTRAINT IF EXISTS api_keys_role_check,
    DROP COLUMN IF EXISTS last_used_at,
    DROP COLUMN IF EXISTS revoked_at,
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS role,
    DROP COLUMN IF EXISTS prefix,
    DROP COLUMN IF EXISTS project_id;

ALTER TABLE projects
    DROP CONSTRAINT IF EXISTS projects_tenant_id_id_unique;
