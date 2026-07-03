package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	raggraph "github.com/shikanon/orag/internal/graph"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/rag"
)

type TraceRecord struct {
	ID         string          `json:"trace_id"`
	TenantID   string          `json:"tenant_id"`
	Profile    rag.Profile     `json:"profile"`
	LatencyMS  int64           `json:"latency_ms"`
	CreatedAt  time.Time       `json:"created_at"`
	HasError   bool            `json:"has_error"`
	ErrorCount int             `json:"error_count"`
	NodeSpans  []TraceNodeSpan `json:"node_spans"`
}

type TraceNodeSpan struct {
	ID        string    `json:"id"`
	NodeName  string    `json:"node_name"`
	LatencyMS int64     `json:"latency_ms"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type traceQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) traceRow
	Query(ctx context.Context, sql string, args ...any) (traceRows, error)
}

type traceRow interface {
	Scan(dest ...any) error
}

type traceRows interface {
	Close()
	Err() error
	Next() bool
	Scan(dest ...any) error
}

func (r *Repository) StoreTrace(ctx context.Context, tenantID, traceID, query string, profile rag.Profile, latencyMS int64, spans []raggraph.NodeSpan) error {
	tx, err := r.traceTxStarter().BeginTraceTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO rag_traces(id, tenant_id, query, profile, latency_ms)
		VALUES($1,$2,$3,$4,$5)
		ON CONFLICT (id) DO UPDATE SET
			tenant_id=EXCLUDED.tenant_id,
			query=EXCLUDED.query,
			profile=EXCLUDED.profile,
			latency_ms=EXCLUDED.latency_ms,
			created_at=now()`,
		traceID, tenantID, query, string(profile), latencyMS); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM rag_node_spans
		WHERE trace_id=$1`,
		traceID); err != nil {
		return err
	}
	for _, span := range spans {
		if _, err := tx.Exec(ctx, `
			INSERT INTO rag_node_spans(id, trace_id, node_name, latency_ms, error)
			VALUES($1,$2,$3,$4,$5)`,
			id.New("span"), traceID, span.NodeName, span.LatencyMS, span.Error); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) GetTrace(ctx context.Context, traceID string) (TraceRecord, bool, error) {
	queryer := r.traceQueryer()
	var record TraceRecord
	var profile string
	err := queryer.QueryRow(ctx, `
		SELECT id, tenant_id, profile, latency_ms, created_at
		FROM rag_traces
		WHERE id=$1`, traceID).
		Scan(&record.ID, &record.TenantID, &profile, &record.LatencyMS, &record.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return TraceRecord{}, false, nil
		}
		return TraceRecord{}, false, err
	}
	record.Profile = rag.Profile(profile)

	rows, err := queryer.Query(ctx, `
		SELECT id, node_name, latency_ms, error, created_at
		FROM rag_node_spans
		WHERE trace_id=$1
		ORDER BY created_at, id`, traceID)
	if err != nil {
		return TraceRecord{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var span TraceNodeSpan
		if err := rows.Scan(&span.ID, &span.NodeName, &span.LatencyMS, &span.Error, &span.CreatedAt); err != nil {
			return TraceRecord{}, false, err
		}
		if span.Error != "" {
			record.HasError = true
			record.ErrorCount++
		}
		record.NodeSpans = append(record.NodeSpans, span)
	}
	if err := rows.Err(); err != nil {
		return TraceRecord{}, false, err
	}
	return record, true, nil
}

func (r *Repository) traceQueryer() traceQueryer {
	if r.traceReader != nil {
		return r.traceReader
	}
	return pgxTraceQueryer{pool: r.Pool}
}

func (r *Repository) traceTxStarter() traceTxBeginner {
	if r.traceTxBeginner != nil {
		return r.traceTxBeginner
	}
	return pgxTraceTxBeginner{pool: r.Pool}
}
