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
	_, err := insertAPIKey(ctx, r.pool, item)
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

// RotateAPIKey atomically inserts a replacement and revokes its active source.
// A row lock prevents two callers from successfully rotating the same key.
func (r *APIKeyRepository) RotateAPIKey(ctx context.Context, tenantID, sourceID string, replacement auth.APIKey, revokedAt time.Time) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	source, err := scanAPIKey(tx.QueryRow(ctx, apiKeySelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, sourceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if source.RevokedAt != nil || (source.ExpiresAt != nil && !source.ExpiresAt.After(revokedAt)) {
		return false, nil
	}
	if replacement.TenantID != source.TenantID || replacement.RotatedFromKeyID != source.ID {
		return false, auth.ErrAPIKeyInvalid
	}
	if _, err := insertAPIKey(ctx, tx, replacement); err != nil {
		return false, apiKeyPersistenceError(err)
	}
	tag, err := tx.Exec(ctx, `UPDATE api_keys SET revoked_at=$3 WHERE tenant_id=$1 AND id=$2 AND revoked_at IS NULL`, tenantID, sourceID, revokedAt)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *APIKeyRepository) TouchAPIKeyLastUsed(ctx context.Context, id string, usedAt, notAfter time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE api_keys SET last_used_at=$2
		WHERE id=$1 AND (last_used_at IS NULL OR last_used_at <= $3)`, id, usedAt, notAfter)
	return err
}

const apiKeySelect = `
	SELECT id, tenant_id, COALESCE(project_id,''), name, prefix, key_hash, role,
		created_by, created_at, expires_at, revoked_at, last_used_at, COALESCE(rotated_from_key_id,'')
	FROM api_keys`

type apiKeyExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertAPIKey(ctx context.Context, executor apiKeyExecutor, item auth.APIKey) (pgconn.CommandTag, error) {
	return executor.Exec(ctx, `
		INSERT INTO api_keys(
			id, tenant_id, project_id, name, prefix, key_hash, role, created_by,
			created_at, expires_at, revoked_at, last_used_at, rotated_from_key_id
		) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,NULLIF($13,''))`,
		item.ID, item.TenantID, item.ProjectID, item.Name, item.Prefix, item.KeyHash,
		item.Role, item.CreatedBy, item.CreatedAt, item.ExpiresAt, item.RevokedAt, item.LastUsedAt, item.RotatedFromKeyID)
}

func scanAPIKey(row interface{ Scan(...any) error }) (auth.APIKey, error) {
	var item auth.APIKey
	err := row.Scan(&item.ID, &item.TenantID, &item.ProjectID, &item.Name, &item.Prefix,
		&item.KeyHash, &item.Role, &item.CreatedBy, &item.CreatedAt, &item.ExpiresAt,
		&item.RevokedAt, &item.LastUsedAt, &item.RotatedFromKeyID)
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
