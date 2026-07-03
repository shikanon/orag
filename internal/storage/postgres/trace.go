package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	raggraph "github.com/shikanon/orag/internal/graph"
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
	Sequence  int       `json:"sequence"`
	LatencyMS int64     `json:"latency_ms"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	CreatedAt time.Time `json:"created_at"`
}

const (
	defaultTraceListLimit = 50
	maxTraceListLimit     = 500
)

type TraceListFilter struct {
	TenantID string
	Profile  rag.Profile
	Since    time.Time
	Until    time.Time
	HasError *bool
	SlowMS   int64
	Limit    int
}

type TraceNodeStat struct {
	NodeName     string  `json:"node_name"`
	Count        int64   `json:"count"`
	AvgLatencyMS float64 `json:"avg_latency_ms"`
	P95LatencyMS float64 `json:"p95_latency_ms"`
	P99LatencyMS float64 `json:"p99_latency_ms"`
	ErrorCount   int64   `json:"error_count"`
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

type traceBeginner interface {
	Begin(ctx context.Context) (traceTx, error)
}

type traceTx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type pgxTraceWriter struct {
	pool *pgxpool.Pool
}

func (w pgxTraceWriter) Begin(ctx context.Context) (traceTx, error) {
	return w.pool.Begin(ctx)
}

func (r *Repository) StoreTrace(ctx context.Context, tenantID, traceID, query string, profile rag.Profile, latencyMS int64, spans []raggraph.NodeSpan) error {
	tx, err := r.traceBeginner().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO rag_traces(id, tenant_id, query, profile, latency_ms)
		VALUES($1,$2,$3,$4,$5)
		ON CONFLICT (id) DO NOTHING`,
		traceID, tenantID, query, string(profile), latencyMS); err != nil {
		return err
	}
	for i, span := range spans {
		seq := span.Sequence
		if seq <= 0 {
			seq = i + 1
		}
		startedAt, endedAt := normalizedSpanTimes(span)
		if _, err := tx.Exec(ctx, `
			INSERT INTO rag_node_spans(id, trace_id, node_name, sequence, latency_ms, error, started_at, ended_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (trace_id, sequence) DO UPDATE SET
				node_name=EXCLUDED.node_name,
				latency_ms=EXCLUDED.latency_ms,
				error=EXCLUDED.error,
				started_at=EXCLUDED.started_at,
				ended_at=EXCLUDED.ended_at`,
			stableSpanID(traceID, seq, span.NodeName), traceID, span.NodeName, seq, span.LatencyMS, span.Error, startedAt, endedAt); err != nil {
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
		SELECT id, node_name, sequence, latency_ms, error, started_at, ended_at, created_at
		FROM rag_node_spans
		WHERE trace_id=$1
		ORDER BY sequence, created_at, id`, traceID)
	if err != nil {
		return TraceRecord{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var span TraceNodeSpan
		if err := rows.Scan(&span.ID, &span.NodeName, &span.Sequence, &span.LatencyMS, &span.Error, &span.StartedAt, &span.EndedAt, &span.CreatedAt); err != nil {
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

func (r *Repository) ListTraces(ctx context.Context, filter TraceListFilter) ([]TraceRecord, error) {
	queryer := r.traceQueryer()
	args := make([]any, 0, 7)
	var query strings.Builder
	query.WriteString(`
		SELECT
			t.id,
			t.tenant_id,
			t.profile,
			t.latency_ms,
			t.created_at,
			EXISTS (
				SELECT 1 FROM rag_node_spans s
				WHERE s.trace_id=t.id AND s.error <> ''
			) AS has_error,
			COALESCE((
				SELECT count(*) FROM rag_node_spans s
				WHERE s.trace_id=t.id AND s.error <> ''
			), 0) AS error_count
		FROM rag_traces t
		WHERE TRUE`)
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	if filter.TenantID != "" {
		query.WriteString(" AND t.tenant_id=")
		query.WriteString(addArg(filter.TenantID))
	}
	if filter.Profile != "" {
		query.WriteString(" AND t.profile=")
		query.WriteString(addArg(string(filter.Profile)))
	}
	if !filter.Since.IsZero() {
		query.WriteString(" AND t.created_at >= ")
		query.WriteString(addArg(filter.Since))
	}
	if !filter.Until.IsZero() {
		query.WriteString(" AND t.created_at <= ")
		query.WriteString(addArg(filter.Until))
	}
	if filter.HasError != nil {
		if *filter.HasError {
			query.WriteString(" AND EXISTS (SELECT 1 FROM rag_node_spans s WHERE s.trace_id=t.id AND s.error <> '')")
		} else {
			query.WriteString(" AND NOT EXISTS (SELECT 1 FROM rag_node_spans s WHERE s.trace_id=t.id AND s.error <> '')")
		}
	}
	if filter.SlowMS > 0 {
		query.WriteString(" AND t.latency_ms >= ")
		query.WriteString(addArg(filter.SlowMS))
	}
	limit := normalizedTraceListLimit(filter.Limit)
	query.WriteString(" ORDER BY t.created_at DESC, t.id DESC LIMIT ")
	query.WriteString(addArg(limit))

	rows, err := queryer.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var traces []TraceRecord
	for rows.Next() {
		var record TraceRecord
		var profile string
		var errorCount int64
		if err := rows.Scan(&record.ID, &record.TenantID, &profile, &record.LatencyMS, &record.CreatedAt, &record.HasError, &errorCount); err != nil {
			return nil, err
		}
		record.Profile = rag.Profile(profile)
		record.ErrorCount = int(errorCount)
		traces = append(traces, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return traces, nil
}

func (r *Repository) TraceNodeStats(ctx context.Context, filter TraceListFilter) ([]TraceNodeStat, error) {
	queryer := r.traceQueryer()
	args := make([]any, 0, 6)
	var query strings.Builder
	query.WriteString(`
		SELECT
			s.node_name,
			count(*)::bigint AS span_count,
			COALESCE(avg(s.latency_ms), 0)::double precision AS avg_latency_ms,
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY s.latency_ms::double precision), 0)::double precision AS p95_latency_ms,
			COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY s.latency_ms::double precision), 0)::double precision AS p99_latency_ms,
			COALESCE(sum(CASE WHEN s.error <> '' THEN 1 ELSE 0 END), 0)::bigint AS error_count
		FROM rag_node_spans s
		JOIN rag_traces t ON t.id=s.trace_id
		WHERE TRUE`)
	appendTraceListFilters(&query, &args, filter)
	query.WriteString(`
		GROUP BY s.node_name
		ORDER BY avg_latency_ms DESC, s.node_name`)

	rows, err := queryer.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stats []TraceNodeStat
	for rows.Next() {
		var stat TraceNodeStat
		if err := rows.Scan(&stat.NodeName, &stat.Count, &stat.AvgLatencyMS, &stat.P95LatencyMS, &stat.P99LatencyMS, &stat.ErrorCount); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stats, nil
}

func appendTraceListFilters(query *strings.Builder, args *[]any, filter TraceListFilter) {
	addArg := func(value any) string {
		*args = append(*args, value)
		return fmt.Sprintf("$%d", len(*args))
	}
	if filter.TenantID != "" {
		query.WriteString(" AND t.tenant_id=")
		query.WriteString(addArg(filter.TenantID))
	}
	if filter.Profile != "" {
		query.WriteString(" AND t.profile=")
		query.WriteString(addArg(string(filter.Profile)))
	}
	if !filter.Since.IsZero() {
		query.WriteString(" AND t.created_at >= ")
		query.WriteString(addArg(filter.Since))
	}
	if !filter.Until.IsZero() {
		query.WriteString(" AND t.created_at <= ")
		query.WriteString(addArg(filter.Until))
	}
	if filter.HasError != nil {
		if *filter.HasError {
			query.WriteString(" AND EXISTS (SELECT 1 FROM rag_node_spans s2 WHERE s2.trace_id=t.id AND s2.error <> '')")
		} else {
			query.WriteString(" AND NOT EXISTS (SELECT 1 FROM rag_node_spans s2 WHERE s2.trace_id=t.id AND s2.error <> '')")
		}
	}
	if filter.SlowMS > 0 {
		query.WriteString(" AND t.latency_ms >= ")
		query.WriteString(addArg(filter.SlowMS))
	}
}

func normalizedTraceListLimit(limit int) int {
	if limit <= 0 {
		return defaultTraceListLimit
	}
	if limit > maxTraceListLimit {
		return maxTraceListLimit
	}
	return limit
}

func stableSpanID(traceID string, sequence int, nodeName string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%s", traceID, sequence, nodeName)))
	return "span_" + hex.EncodeToString(sum[:])[:24]
}

func normalizedSpanTimes(span raggraph.NodeSpan) (time.Time, time.Time) {
	startedAt := span.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	endedAt := span.EndedAt
	if endedAt.IsZero() {
		endedAt = startedAt
		if span.LatencyMS > 0 {
			endedAt = startedAt.Add(time.Duration(span.LatencyMS) * time.Millisecond)
		}
	}
	return startedAt, endedAt
}

func (r *Repository) traceQueryer() traceQueryer {
	if r.traceReader != nil {
		return r.traceReader
	}
	return pgxTraceQueryer{pool: r.Pool}
}

func (r *Repository) traceBeginner() traceBeginner {
	if r.traceWriter != nil {
		return r.traceWriter
	}
	return pgxTraceWriter{pool: r.Pool}
}
