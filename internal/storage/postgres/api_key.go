package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shikanon/orag/internal/auth"
)

var _ auth.APIKeyRepository = (*APIKeyRepository)(nil)

type APIKeyRepository struct {
	pool *pgxpool.Pool
}

func NewAPIKeyRepository(pool *pgxpool.Pool) *APIKeyRepository {
	return &APIKeyRepository{pool: pool}
}

func (r *APIKeyRepository) CreateAPIKey(ctx context.Context, item auth.APIKey) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO api_keys(
			id, tenant_id, project_id, name, prefix, key_hash, role, created_by,
			created_at, expires_at, revoked_at, last_used_at
		) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		item.ID, item.TenantID, item.ProjectID, item.Name, item.Prefix, item.KeyHash,
		item.Role, item.CreatedBy, item.CreatedAt, item.ExpiresAt, item.RevokedAt, item.LastUsedAt)
	return apiKeyPersistenceError(err)
}

func (r *APIKeyRepository) ListAPIKeys(ctx context.Context, tenantID string) ([]auth.APIKey, error) {
	rows, err := r.pool.Query(ctx, apiKeySelect+` WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]auth.APIKey, 0)
	for rows.Next() {
		item, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *APIKeyRepository) GetAPIKeyByID(ctx context.Context, id string) (auth.APIKey, bool, error) {
	item, err := scanAPIKey(r.pool.QueryRow(ctx, apiKeySelect+` WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.APIKey{}, false, nil
	}
	return item, err == nil, err
}

func (r *APIKeyRepository) RevokeAPIKey(ctx context.Context, tenantID, id string, revokedAt time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE api_keys SET revoked_at=COALESCE(revoked_at,$3)
		WHERE tenant_id=$1 AND id=$2`, tenantID, id, revokedAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

const apiKeySelect = `
	SELECT id, tenant_id, COALESCE(project_id,''), name, prefix, key_hash, role,
		created_by, created_at, expires_at, revoked_at, last_used_at
	FROM api_keys`

func scanAPIKey(row interface{ Scan(...any) error }) (auth.APIKey, error) {
	var item auth.APIKey
	err := row.Scan(&item.ID, &item.TenantID, &item.ProjectID, &item.Name, &item.Prefix,
		&item.KeyHash, &item.Role, &item.CreatedBy, &item.CreatedAt, &item.ExpiresAt,
		&item.RevokedAt, &item.LastUsedAt)
	return item, err
}

func apiKeyPersistenceError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23503" || pgErr.Code == "23505" || pgErr.Code == "23514") {
		return errors.Join(auth.ErrAPIKeyInvalid, err)
	}
	return err
}
