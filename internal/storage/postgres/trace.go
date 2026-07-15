package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	raggraph "github.com/shikanon/orag/internal/graph"
	"github.com/shikanon/orag/internal/offlineknowledge"
	"github.com/shikanon/orag/internal/rag"
)

type TraceRecord struct {
	ID                string               `json:"trace_id"`
	TenantID          string               `json:"tenant_id"`
	KBID              string               `json:"kb_id,omitempty"`
	ProjectID         string               `json:"project_id,omitempty"`
	PipelineID        string               `json:"pipeline_id,omitempty"`
	PipelineVersionID string               `json:"pipeline_version_id,omitempty"`
	ReleaseID         string               `json:"release_id,omitempty"`
	Environment       string               `json:"environment,omitempty"`
	DatasetID         string               `json:"dataset_id,omitempty"`
	EvaluationRunID   string               `json:"evaluation_run_id,omitempty"`
	RetrievalParams   TraceRetrievalParams `json:"retrieval_params,omitempty"`
	Query             string               `json:"query"`
	Profile           rag.Profile          `json:"profile"`
	Answer            string               `json:"answer,omitempty"`
	RetrievedChunks   []string             `json:"retrieved_chunks,omitempty"`
	LatencyMS         int64                `json:"latency_ms"`
	CreatedAt         time.Time            `json:"created_at"`
	HasError          bool                 `json:"has_error"`
	ErrorCount        int                  `json:"error_count"`
	NodeSpans         []TraceNodeSpan      `json:"node_spans"`
}

// TraceRetrievalParams records client retrieval knobs alongside the immutable
// server-resolved release lineage for audit and replay.
type TraceRetrievalParams struct {
	TopK             int         `json:"top_k,omitempty"`
	RequestedProfile rag.Profile `json:"requested_profile,omitempty"`
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
	KBID     string
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

func (r *Repository) StoreTrace(ctx context.Context, input raggraph.TraceInput) error {
	tx, err := r.traceTxStarter().BeginTraceTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	retrievedChunksJSON, err := json.Marshal(input.RetrievedChunks)
	if err != nil {
		return err
	}
	retrievalParamsJSON, err := json.Marshal(TraceRetrievalParams{TopK: input.RequestedTopK, RequestedProfile: input.RequestedProfile})
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO rag_traces(id, tenant_id, knowledge_base_id, project_id, pipeline_id, pipeline_version_id, release_id, environment_kind, dataset_id, evaluation_run_id, retrieval_params, query, profile, answer, retrieved_chunks_json, latency_ms)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (id) DO UPDATE SET
			tenant_id=EXCLUDED.tenant_id,
			knowledge_base_id=EXCLUDED.knowledge_base_id,
			project_id=EXCLUDED.project_id,
			pipeline_id=EXCLUDED.pipeline_id,
			pipeline_version_id=EXCLUDED.pipeline_version_id,
			release_id=EXCLUDED.release_id,
			environment_kind=EXCLUDED.environment_kind,
			dataset_id=EXCLUDED.dataset_id,
			evaluation_run_id=EXCLUDED.evaluation_run_id,
			retrieval_params=EXCLUDED.retrieval_params,
			query=EXCLUDED.query, profile=EXCLUDED.profile, answer=EXCLUDED.answer,
			retrieved_chunks_json=EXCLUDED.retrieved_chunks_json, latency_ms=EXCLUDED.latency_ms,
			created_at=now()`,
		input.TraceID, input.TenantID, input.KnowledgeBaseID, input.ProjectID, input.PipelineID, input.PipelineVersionID, input.ReleaseID, input.Environment, input.DatasetID, input.EvaluationRunID, retrievalParamsJSON, sanitizeTraceQuery(input.Query), string(input.Profile), input.Answer, retrievedChunksJSON, input.LatencyMS); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM rag_node_spans
		WHERE trace_id=$1`,
		input.TraceID); err != nil {
		return err
	}
	for i, span := range input.Spans {
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
			stableSpanID(input.TraceID, seq, span.NodeName), input.TraceID, span.NodeName, seq, span.LatencyMS, span.Error, startedAt, endedAt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func sanitizeTraceQuery(query string) string {
	const maxBytes = 2048
	query = strings.TrimSpace(query)
	lower := strings.ToLower(query)
	for _, marker := range []string{"authorization: bearer ", "api_key=", "api-key=", "apikey=", "token="} {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		end := idx + len(marker)
		for end < len(query) && query[end] != ' ' && query[end] != '\n' && query[end] != '\t' {
			end++
		}
		query = query[:idx+len(marker)] + "[redacted]" + query[end:]
		lower = strings.ToLower(query)
	}
	if len(query) > maxBytes {
		return query[:maxBytes] + "...[truncated]"
	}
	return query
}

func (r *Repository) GetTrace(ctx context.Context, traceID string) (TraceRecord, bool, error) {
	return r.getTrace(ctx, traceID, "")
}

func (r *Repository) GetTraceForTenant(ctx context.Context, tenantID, traceID string) (TraceRecord, bool, error) {
	return r.getTrace(ctx, traceID, tenantID)
}

func (r *Repository) getTrace(ctx context.Context, traceID, tenantID string) (TraceRecord, bool, error) {
	queryer := r.traceQueryer()
	var record TraceRecord
	var profile string
	var retrievedChunksJSON []byte
	var retrievalParamsJSON []byte
	args := []any{traceID}
	query := `
		SELECT id, tenant_id, knowledge_base_id, project_id, pipeline_id, pipeline_version_id, release_id, environment_kind, dataset_id, evaluation_run_id, retrieval_params, query, profile, answer, retrieved_chunks_json, latency_ms, created_at
		FROM rag_traces
		WHERE id=$1`
	if tenantID != "" {
		query += " AND tenant_id=$2"
		args = append(args, tenantID)
	}
	err := queryer.QueryRow(ctx, query, args...).
		Scan(&record.ID, &record.TenantID, &record.KBID, &record.ProjectID, &record.PipelineID, &record.PipelineVersionID, &record.ReleaseID, &record.Environment, &record.DatasetID, &record.EvaluationRunID, &retrievalParamsJSON, &record.Query, &profile, &record.Answer, &retrievedChunksJSON, &record.LatencyMS, &record.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return TraceRecord{}, false, nil
		}
		return TraceRecord{}, false, err
	}
	record.Profile = rag.Profile(profile)
	if err := json.Unmarshal(retrievedChunksJSON, &record.RetrievedChunks); err != nil {
		return TraceRecord{}, false, err
	}
	if err := json.Unmarshal(retrievalParamsJSON, &record.RetrievalParams); err != nil {
		return TraceRecord{}, false, err
	}

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
			t.knowledge_base_id,
			t.project_id,
			t.pipeline_id,
			t.pipeline_version_id,
			t.release_id,
			t.environment_kind,
			t.dataset_id,
			t.evaluation_run_id,
			t.retrieval_params,
			t.query,
			t.profile,
			t.answer,
			t.retrieved_chunks_json,
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
	appendTraceListFilters(&query, &args, filter)
	limit := normalizedTraceListLimit(filter.Limit)
	query.WriteString(" ORDER BY t.created_at DESC, t.id DESC LIMIT ")
	query.WriteString(addTraceArg(&args, limit))

	rows, err := queryer.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var traces []TraceRecord
	for rows.Next() {
		var record TraceRecord
		var profile string
		var retrievedChunksJSON []byte
		var retrievalParamsJSON []byte
		var errorCount int64
		if err := rows.Scan(&record.ID, &record.TenantID, &record.KBID, &record.ProjectID, &record.PipelineID, &record.PipelineVersionID, &record.ReleaseID, &record.Environment, &record.DatasetID, &record.EvaluationRunID, &retrievalParamsJSON, &record.Query, &profile, &record.Answer, &retrievedChunksJSON, &record.LatencyMS, &record.CreatedAt, &record.HasError, &errorCount); err != nil {
			return nil, err
		}
		record.Profile = rag.Profile(profile)
		if err := json.Unmarshal(retrievedChunksJSON, &record.RetrievedChunks); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(retrievalParamsJSON, &record.RetrievalParams); err != nil {
			return nil, err
		}
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
	if filter.TenantID != "" {
		query.WriteString(" AND t.tenant_id=")
		query.WriteString(addTraceArg(args, filter.TenantID))
	}
	if filter.KBID != "" {
		query.WriteString(" AND t.knowledge_base_id=")
		query.WriteString(addTraceArg(args, filter.KBID))
	}
	if filter.Profile != "" {
		query.WriteString(" AND t.profile=")
		query.WriteString(addTraceArg(args, string(filter.Profile)))
	}
	if !filter.Since.IsZero() {
		query.WriteString(" AND t.created_at >= ")
		query.WriteString(addTraceArg(args, filter.Since))
	}
	if !filter.Until.IsZero() {
		query.WriteString(" AND t.created_at <= ")
		query.WriteString(addTraceArg(args, filter.Until))
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
		query.WriteString(addTraceArg(args, filter.SlowMS))
	}
}

func (r *Repository) ListHistoryTraces(ctx context.Context, filter offlineknowledge.HistoryTraceFilter) ([]offlineknowledge.HistoryTrace, error) {
	traces, err := r.ListTraces(ctx, TraceListFilter{
		TenantID: filter.TenantID,
		KBID:     filter.KBID,
		Since:    filter.Since,
		Until:    filter.Until,
		Limit:    filter.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]offlineknowledge.HistoryTrace, 0, len(traces))
	for _, trace := range traces {
		out = append(out, offlineknowledge.HistoryTrace{
			TenantID:        trace.TenantID,
			KBID:            trace.KBID,
			TraceID:         trace.ID,
			Query:           trace.Query,
			Answer:          trace.Answer,
			RetrievedChunks: append([]string(nil), trace.RetrievedChunks...),
			Latency:         time.Duration(trace.LatencyMS) * time.Millisecond,
			HasError:        trace.HasError,
			Error:           firstTraceError(trace.NodeSpans),
			CreatedAt:       trace.CreatedAt,
		})
	}
	return out, nil
}

func firstTraceError(spans []TraceNodeSpan) string {
	for _, span := range spans {
		if span.Error != "" {
			return span.Error
		}
	}
	return ""
}

func addTraceArg(args *[]any, value any) string {
	*args = append(*args, value)
	return fmt.Sprintf("$%d", len(*args))
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

func tracePercentileCont(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if len(values) == 1 {
		return values[0]
	}
	position := percentile * float64(len(values)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return values[lower]
	}
	weight := position - float64(lower)
	return values[lower] + (values[upper]-values[lower])*weight
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
