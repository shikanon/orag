package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shikanon/orag/internal/audit"
)

var _ audit.AuditRepository = (*AuditRepository)(nil)

type AuditRepository struct {
	pool *pgxpool.Pool
}

func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{pool: pool}
}

const auditEventSelect = `
	SELECT id, tenant_id, COALESCE(project_id,''), actor_type, actor_id, action,
	       COALESCE(resource_type,''), COALESCE(resource_id,''), outcome,
	       COALESCE(trace_id,''), COALESCE(task_id,''), COALESCE(idempotency_key,''),
	       COALESCE(metadata,'{}'::jsonb), COALESCE(error_code,''),
	       COALESCE(error_message,''), created_at
	FROM audit_events`

type auditEventScanner interface {
	Scan(dest ...any) error
}

func scanAuditEvent(row auditEventScanner) (audit.AuditEvent, error) {
	var e audit.AuditEvent
	var metadata []byte
	err := row.Scan(
		&e.ID, &e.TenantID, &e.ProjectID, &e.ActorType, &e.ActorID, &e.Action,
		&e.ResourceType, &e.ResourceID, &e.Outcome,
		&e.TraceID, &e.TaskID, &e.IdempotencyKey,
		&metadata, &e.ErrorCode, &e.ErrorMessage, &e.CreatedAt,
	)
	if err != nil {
		return e, err
	}
	e.Metadata = stringMap(metadata)
	return e, nil
}

// Record inserts a new audit event into the database.
func (r *AuditRepository) Record(ctx context.Context, event audit.AuditEvent) error {
	metadata := mustJSON(event.Metadata)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_events(
			id, tenant_id, project_id, actor_type, actor_id, action,
			resource_type, resource_id, outcome, trace_id, task_id,
			idempotency_key, metadata, error_code, error_message, created_at
		) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,
			NULLIF($7,''),NULLIF($8,''),$9,
			NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),
			$13,NULLIF($14,''),NULLIF($15,''),$16)`,
		event.ID, event.TenantID, event.ProjectID, event.ActorType, event.ActorID, event.Action,
		event.ResourceType, event.ResourceID, event.Outcome,
		event.TraceID, event.TaskID, event.IdempotencyKey,
		metadata, event.ErrorCode, event.ErrorMessage, event.CreatedAt,
	)
	return err
}

// List returns audit events matching the given filter with cursor-based pagination.
// The cursor is opaque and should be passed back from a previous List call.
func (r *AuditRepository) List(ctx context.Context, filter audit.AuditFilter) ([]audit.AuditEvent, string, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var conditions []string
	var args []any
	argIdx := 1

	if filter.TenantID != "" {
		conditions = append(conditions, fmt.Sprintf("tenant_id=$%d", argIdx))
		args = append(args, filter.TenantID)
		argIdx++
	}
	if filter.ProjectID != "" {
		conditions = append(conditions, fmt.Sprintf("project_id=$%d", argIdx))
		args = append(args, filter.ProjectID)
		argIdx++
	}
	if filter.ResourceType != "" {
		conditions = append(conditions, fmt.Sprintf("resource_type=$%d", argIdx))
		args = append(args, filter.ResourceType)
		argIdx++
	}
	if filter.ResourceID != "" {
		conditions = append(conditions, fmt.Sprintf("resource_id=$%d", argIdx))
		args = append(args, filter.ResourceID)
		argIdx++
	}
	if filter.Action != "" {
		conditions = append(conditions, fmt.Sprintf("action=$%d", argIdx))
		args = append(args, filter.Action)
		argIdx++
	}
	if filter.ActorID != "" {
		conditions = append(conditions, fmt.Sprintf("actor_id=$%d", argIdx))
		args = append(args, filter.ActorID)
		argIdx++
	}
	if filter.Outcome != "" {
		conditions = append(conditions, fmt.Sprintf("outcome=$%d", argIdx))
		args = append(args, filter.Outcome)
		argIdx++
	}

	var cursorCreatedAt time.Time
	var cursorID string
	if filter.Cursor != "" {
		parts := strings.SplitN(filter.Cursor, ":", 2)
		if len(parts) == 2 {
			if timeBytes, err := base64.URLEncoding.DecodeString(parts[0]); err == nil {
				if t, err := time.Parse(time.RFC3339Nano, string(timeBytes)); err == nil {
					cursorCreatedAt = t
					cursorID = parts[1]
				}
			}
		}
	}

	if !cursorCreatedAt.IsZero() {
		conditions = append(conditions, fmt.Sprintf(
			"(created_at < $%d OR (created_at = $%d AND id < $%d))",
			argIdx, argIdx, argIdx+1,
		))
		args = append(args, cursorCreatedAt, cursorID)
		argIdx += 2
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	fetchLimit := limit + 1
	query := auditEventSelect + whereClause + fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", argIdx)
	args = append(args, fetchLimit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var events []audit.AuditEvent
	for rows.Next() {
		e, err := scanAuditEvent(rows)
		if err != nil {
			return nil, "", err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(events) > limit {
		events = events[:limit]
		last := events[len(events)-1]
		timeStr := last.CreatedAt.Format(time.RFC3339Nano)
		nextCursor = base64.URLEncoding.EncodeToString([]byte(timeStr)) + ":" + last.ID
	}

	return events, nextCursor, nil
}

// Get retrieves an audit event by its ID.
func (r *AuditRepository) Get(ctx context.Context, eventID string) (audit.AuditEvent, bool, error) {
	row := r.pool.QueryRow(ctx, auditEventSelect+` WHERE id=$1`, eventID)
	e, err := scanAuditEvent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return audit.AuditEvent{}, false, nil
		}
		return audit.AuditEvent{}, false, err
	}
	return e, true, nil
}
