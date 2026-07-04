package postgres

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	evalpkg "github.com/shikanon/orag/internal/eval"
	raggraph "github.com/shikanon/orag/internal/graph"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
)

func TestExtractGooseUp(t *testing.T) {
	got, err := extractGooseUp(`-- +goose Up
CREATE TABLE example(id TEXT);
-- +goose Down
DROP TABLE example;`)
	if err != nil {
		t.Fatalf("extractGooseUp() error = %v", err)
	}
	if strings.Contains(got, "DROP TABLE") {
		t.Fatalf("up migration contains down section: %q", got)
	}
	if !strings.Contains(got, "CREATE TABLE example") {
		t.Fatalf("up migration missing create statement: %q", got)
	}
}

func TestStringMapRoundTrip(t *testing.T) {
	body := mustJSON(map[string]string{"source": "test"})
	got := stringMap(body)
	if got["source"] != "test" {
		t.Fatalf("stringMap() = %#v", got)
	}
}

func TestIngestionJobResultMigration(t *testing.T) {
	body, err := os.ReadFile("../../../migrations/000002_ingestion_job_result.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{"document_id", "chunk_count", "ADD COLUMN IF NOT EXISTS"} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q: %s", required, text)
		}
	}
}

func TestTraceSpanOrderingMigration(t *testing.T) {
	body, err := os.ReadFile("../../../migrations/000004_trace_span_ordering.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS sequence",
		"ADD COLUMN IF NOT EXISTS started_at",
		"ADD COLUMN IF NOT EXISTS ended_at",
		"row_number() OVER (PARTITION BY trace_id ORDER BY created_at, id)",
		"rag_node_spans_trace_sequence_uidx",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q: %s", required, text)
		}
	}
}

func TestTraceReadIndexesMigration(t *testing.T) {
	body, err := os.ReadFile("../../../migrations/000005_trace_read_indexes.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{
		"rag_traces_tenant_created_idx",
		"rag_traces_profile_created_idx",
		"rag_traces_latency_created_idx",
		"rag_node_spans_trace_error_idx",
		"rag_node_spans_trace_order_idx",
		"WHERE error <> ''",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q: %s", required, text)
		}
	}
}

func TestDatasetItemDiversityAnnotationsMigration(t *testing.T) {
	body, err := os.ReadFile("../../../migrations/000006_dataset_item_diversity_annotations.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS diversity_annotations",
		"JSONB NOT NULL DEFAULT '[]'::jsonb",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q: %s", required, text)
		}
	}
}

func TestEvaluationRunMetricsJSONBRoundTrip(t *testing.T) {
	body, err := encodeEvaluationRunMetrics(evalpkg.RunResult{
		Total:    2,
		HitRate:  0.5,
		Accuracy: 0.5,
		Metrics: map[string]float64{
			"ndcg_at_k":              0.75,
			"recall_at_k":            0.5,
			"redundancy_rate":        0.25,
			"alpha_ndcg":             0.8,
			"retrieval_failure_rate": 0.5,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]float64
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"total", "hit_rate", "accuracy", "ndcg_at_k", "recall_at_k", "redundancy_rate", "alpha_ndcg", "retrieval_failure_rate"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("encoded metric %q missing from %s", key, string(body))
		}
	}

	var decoded evalpkg.RunResult
	decodeEvaluationRunMetrics(body, &decoded)
	if decoded.Total != 2 || decoded.HitRate != 0.5 || decoded.Accuracy != 0.5 {
		t.Fatalf("decoded summary = %#v", decoded)
	}
	if decoded.Metrics["ndcg_at_k"] != 0.75 || decoded.Metrics["alpha_ndcg"] != 0.8 {
		t.Fatalf("decoded metrics = %#v", decoded.Metrics)
	}
}

func TestEvaluationRunMetricsDecodeOldJSONB(t *testing.T) {
	var decoded evalpkg.RunResult
	decodeEvaluationRunMetrics([]byte(`{"total":3,"hit_rate":0.6666666667,"accuracy":0.5}`), &decoded)
	if decoded.Total != 3 || decoded.HitRate != 0.6666666667 || decoded.Accuracy != 0.5 {
		t.Fatalf("decoded old summary = %#v", decoded)
	}
	if decoded.Metrics["total"] != 3 || decoded.Metrics["accuracy"] != 0.5 {
		t.Fatalf("decoded old metrics = %#v", decoded.Metrics)
	}
}

func TestChunksWithDocumentIDRemapsChunkIDs(t *testing.T) {
	chunks := []kb.Chunk{
		{ID: "chk_old_0", DocumentID: "doc_old", Content: "first"},
		{ID: "chk_old_1", DocumentID: "doc_old", Content: "second"},
	}

	got := chunksWithDocumentID(chunks, "doc_existing")

	if got[0].DocumentID != "doc_existing" || got[1].DocumentID != "doc_existing" {
		t.Fatalf("document ids = %q, %q", got[0].DocumentID, got[1].DocumentID)
	}
	if got[0].ID != chunkID("doc_existing", 0) || got[1].ID != chunkID("doc_existing", 1) {
		t.Fatalf("chunk ids not remapped: %#v", got)
	}
	if chunks[0].DocumentID != "doc_old" || chunks[0].ID != "chk_old_0" {
		t.Fatalf("input chunks were mutated: %#v", chunks)
	}
}

func TestRepositoryStoreTraceUsesIdempotentSpanUpsert(t *testing.T) {
	startedAt := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	writer := &fakeTraceWriter{}
	repo := &Repository{traceWriter: writer}
	spans := []raggraph.NodeSpan{
		{
			NodeName:  "retrieve",
			Sequence:  1,
			LatencyMS: 12,
			StartedAt: startedAt,
			EndedAt:   startedAt.Add(12 * time.Millisecond),
		},
		{
			NodeName:  "generate",
			LatencyMS: 42,
			Error:     "llm timeout",
		},
	}

	for i := 0; i < 2; i++ {
		if err := repo.StoreTrace(context.Background(), "tenant_1", "trace_1", "query", rag.ProfileRealtime, 54, spans); err != nil {
			t.Fatalf("StoreTrace() call %d error = %v", i+1, err)
		}
	}

	if len(writer.txs) != 2 {
		t.Fatalf("transactions = %d, want 2", len(writer.txs))
	}
	firstCall := writer.txs[0]
	secondCall := writer.txs[1]
	if !firstCall.committed || !secondCall.committed {
		t.Fatalf("expected both StoreTrace calls to commit")
	}
	if len(firstCall.execs) != 3 || len(secondCall.execs) != 3 {
		t.Fatalf("exec counts = %d, %d; want 3, 3", len(firstCall.execs), len(secondCall.execs))
	}
	firstSpan := firstCall.execs[1]
	repeatedFirstSpan := secondCall.execs[1]
	if !strings.Contains(firstSpan.sql, "ON CONFLICT (trace_id, sequence) DO UPDATE") {
		t.Fatalf("span insert is not idempotent upsert: %s", firstSpan.sql)
	}
	if firstSpan.args[0] != repeatedFirstSpan.args[0] {
		t.Fatalf("stable span id changed across repeated writes: %v vs %v", firstSpan.args[0], repeatedFirstSpan.args[0])
	}
	if firstSpan.args[3] != 1 || firstCall.execs[2].args[3] != 2 {
		t.Fatalf("span sequences = %v, %v; want 1, 2", firstSpan.args[3], firstCall.execs[2].args[3])
	}
}

func TestRepositoryGetTraceFound(t *testing.T) {
	createdAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	startedAt := createdAt.Add(time.Millisecond)
	reader := &fakeTraceReader{
		row: fakeTraceRow{values: []any{"trace_1", "tenant_1", "realtime", int64(123), createdAt}},
		rows: &fakeTraceRows{rows: [][]any{
			{"span_1", "retrieve", 1, int64(12), "", startedAt, startedAt.Add(12 * time.Millisecond), createdAt.Add(time.Millisecond)},
			{"span_2", "generate", 2, int64(111), "llm timeout", startedAt.Add(20 * time.Millisecond), startedAt.Add(131 * time.Millisecond), createdAt.Add(2 * time.Millisecond)},
		}},
	}
	repo := &Repository{traceReader: reader}

	got, found, err := repo.GetTrace(context.Background(), "trace_1")
	if err != nil {
		t.Fatalf("GetTrace() error = %v", err)
	}
	if !found {
		t.Fatal("GetTrace() found = false, want true")
	}
	if got.ID != "trace_1" || got.TenantID != "tenant_1" || got.Profile != rag.Profile("realtime") || got.LatencyMS != 123 {
		t.Fatalf("GetTrace() metadata = %#v", got)
	}
	if !got.HasError || got.ErrorCount != 1 {
		t.Fatalf("GetTrace() error status = has_error:%v error_count:%d", got.HasError, got.ErrorCount)
	}
	if len(got.NodeSpans) != 2 || got.NodeSpans[0].NodeName != "retrieve" || got.NodeSpans[1].Error != "llm timeout" {
		t.Fatalf("GetTrace() spans = %#v", got.NodeSpans)
	}
	if got.NodeSpans[0].Sequence != 1 || !got.NodeSpans[0].StartedAt.Equal(startedAt) {
		t.Fatalf("GetTrace() span ordering fields = %#v", got.NodeSpans[0])
	}
	if !strings.Contains(reader.rowsSQL, "ORDER BY sequence, created_at, id") {
		t.Fatalf("span query is not time ordered: %s", reader.rowsSQL)
	}
}

func TestRepositoryGetTraceNotFound(t *testing.T) {
	reader := &fakeTraceReader{row: fakeTraceRow{err: pgx.ErrNoRows}}
	repo := &Repository{traceReader: reader}

	got, found, err := repo.GetTrace(context.Background(), "missing_trace")
	if err != nil {
		t.Fatalf("GetTrace() error = %v", err)
	}
	if found {
		t.Fatalf("GetTrace() found = true, trace = %#v", got)
	}
	if reader.queriedSpans {
		t.Fatal("GetTrace() queried spans for missing trace")
	}
}

func TestRepositoryListTracesFiltersAndScans(t *testing.T) {
	since := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	until := since.Add(time.Hour)
	hasError := true
	createdAt := since.Add(10 * time.Minute)
	reader := &fakeTraceReader{
		rows: &fakeTraceRows{rows: [][]any{
			{"trace_2", "tenant_1", "high_precision", int64(250), createdAt, true, int64(2)},
		}},
	}
	repo := &Repository{traceReader: reader}

	got, err := repo.ListTraces(context.Background(), TraceListFilter{
		TenantID: "tenant_1",
		Profile:  rag.ProfileHighPrecision,
		Since:    since,
		Until:    until,
		HasError: &hasError,
		SlowMS:   200,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("ListTraces() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListTraces() len = %d, want 1", len(got))
	}
	if got[0].ID != "trace_2" || got[0].TenantID != "tenant_1" || got[0].Profile != rag.ProfileHighPrecision || got[0].LatencyMS != 250 {
		t.Fatalf("ListTraces() trace = %#v", got[0])
	}
	if !got[0].HasError || got[0].ErrorCount != 2 || len(got[0].NodeSpans) != 0 {
		t.Fatalf("ListTraces() error summary/spans = %#v", got[0])
	}
	for _, required := range []string{
		"FROM rag_traces t",
		"t.tenant_id=$1",
		"t.profile=$2",
		"t.created_at >= $3",
		"t.created_at <= $4",
		"EXISTS (SELECT 1 FROM rag_node_spans s WHERE s.trace_id=t.id AND s.error <> '')",
		"t.latency_ms >= $5",
		"ORDER BY t.created_at DESC, t.id DESC LIMIT $6",
	} {
		if !strings.Contains(reader.rowsSQL, required) {
			t.Fatalf("ListTraces() SQL missing %q: %s", required, reader.rowsSQL)
		}
	}
	wantArgs := []any{"tenant_1", "high_precision", since, until, int64(200), 10}
	if !reflect.DeepEqual(reader.rowsArgs, wantArgs) {
		t.Fatalf("ListTraces() args = %#v, want %#v", reader.rowsArgs, wantArgs)
	}
}

func TestRepositoryListTracesDefaultLimitAndHasErrorFalse(t *testing.T) {
	hasError := false
	reader := &fakeTraceReader{rows: &fakeTraceRows{}}
	repo := &Repository{traceReader: reader}

	got, err := repo.ListTraces(context.Background(), TraceListFilter{HasError: &hasError})
	if err != nil {
		t.Fatalf("ListTraces() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListTraces() len = %d, want 0", len(got))
	}
	if !strings.Contains(reader.rowsSQL, "NOT EXISTS (SELECT 1 FROM rag_node_spans s WHERE s.trace_id=t.id AND s.error <> '')") {
		t.Fatalf("ListTraces() missing has_error=false filter: %s", reader.rowsSQL)
	}
	if len(reader.rowsArgs) != 1 || reader.rowsArgs[0] != defaultTraceListLimit {
		t.Fatalf("ListTraces() default limit args = %#v, want [%d]", reader.rowsArgs, defaultTraceListLimit)
	}
}

func TestRepositoryListTracesCapsLimit(t *testing.T) {
	reader := &fakeTraceReader{rows: &fakeTraceRows{}}
	repo := &Repository{traceReader: reader}

	if _, err := repo.ListTraces(context.Background(), TraceListFilter{Limit: maxTraceListLimit + 1}); err != nil {
		t.Fatalf("ListTraces() error = %v", err)
	}
	if len(reader.rowsArgs) != 1 || reader.rowsArgs[0] != maxTraceListLimit {
		t.Fatalf("ListTraces() capped limit args = %#v, want [%d]", reader.rowsArgs, maxTraceListLimit)
	}
}

func TestTraceNodeStats(t *testing.T) {
	since := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	until := since.Add(time.Hour)
	hasError := true
	reader := &fakeTraceReader{
		rows: &fakeTraceRows{rows: [][]any{
			{"generate", int64(3), float64(150), float64(195), float64(199), int64(1)},
			{"retrieve", int64(2), float64(45), float64(58), float64(59), int64(1)},
		}},
	}
	repo := &Repository{traceReader: reader}

	got, err := repo.TraceNodeStats(context.Background(), TraceListFilter{
		TenantID: "tenant_1",
		Profile:  rag.ProfileRealtime,
		Since:    since,
		Until:    until,
		HasError: &hasError,
		SlowMS:   100,
	})
	if err != nil {
		t.Fatalf("TraceNodeStats() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("TraceNodeStats() len = %d, want 2", len(got))
	}
	if got[0].NodeName != "generate" || got[0].Count != 3 || got[0].AvgLatencyMS != 150 || got[0].P95LatencyMS != 195 || got[0].P99LatencyMS != 199 || got[0].ErrorCount != 1 {
		t.Fatalf("TraceNodeStats() first stat = %#v", got[0])
	}
	if got[1].NodeName != "retrieve" || got[1].Count != 2 || got[1].AvgLatencyMS != 45 || got[1].P95LatencyMS != 58 || got[1].P99LatencyMS != 59 || got[1].ErrorCount != 1 {
		t.Fatalf("TraceNodeStats() second stat = %#v", got[1])
	}
	for _, required := range []string{
		"FROM rag_node_spans s",
		"JOIN rag_traces t ON t.id=s.trace_id",
		"count(*)::bigint AS span_count",
		"avg(s.latency_ms)",
		"percentile_cont(0.95) WITHIN GROUP (ORDER BY s.latency_ms::double precision)",
		"percentile_cont(0.99) WITHIN GROUP (ORDER BY s.latency_ms::double precision)",
		"sum(CASE WHEN s.error <> '' THEN 1 ELSE 0 END)",
		"t.tenant_id=$1",
		"t.profile=$2",
		"t.created_at >= $3",
		"t.created_at <= $4",
		"EXISTS (SELECT 1 FROM rag_node_spans s2 WHERE s2.trace_id=t.id AND s2.error <> '')",
		"t.latency_ms >= $5",
		"GROUP BY s.node_name",
		"ORDER BY avg_latency_ms DESC, s.node_name",
	} {
		if !strings.Contains(reader.rowsSQL, required) {
			t.Fatalf("TraceNodeStats() SQL missing %q: %s", required, reader.rowsSQL)
		}
	}
	wantArgs := []any{"tenant_1", "realtime", since, until, int64(100)}
	if !reflect.DeepEqual(reader.rowsArgs, wantArgs) {
		t.Fatalf("TraceNodeStats() args = %#v, want %#v", reader.rowsArgs, wantArgs)
	}
}

type fakeTraceReader struct {
	row          fakeTraceRow
	rows         *fakeTraceRows
	rowsErr      error
	rowsSQL      string
	rowsArgs     []any
	queriedSpans bool
}

func (f *fakeTraceReader) QueryRow(_ context.Context, sql string, _ ...any) traceRow {
	return f.row
}

func (f *fakeTraceReader) Query(_ context.Context, sql string, args ...any) (traceRows, error) {
	f.queriedSpans = true
	f.rowsSQL = sql
	f.rowsArgs = append([]any(nil), args...)
	return f.rows, f.rowsErr
}

type fakeTraceWriter struct {
	txs []*fakeTraceTx
}

func (w *fakeTraceWriter) Begin(context.Context) (traceTx, error) {
	tx := &fakeTraceTx{}
	w.txs = append(w.txs, tx)
	return tx, nil
}

type fakeTraceTx struct {
	execs      []fakeExec
	committed  bool
	rolledBack bool
}

type fakeExec struct {
	sql  string
	args []any
}

func (tx *fakeTraceTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	copied := append([]any(nil), args...)
	tx.execs = append(tx.execs, fakeExec{sql: sql, args: copied})
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (tx *fakeTraceTx) Commit(context.Context) error {
	tx.committed = true
	return nil
}

func (tx *fakeTraceTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}

type fakeTraceRow struct {
	values []any
	err    error
}

func (r fakeTraceRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	assignScanValues(dest, r.values)
	return nil
}

type fakeTraceRows struct {
	rows [][]any
	idx  int
	err  error
}

func (r *fakeTraceRows) Close() {}

func (r *fakeTraceRows) Err() error {
	return r.err
}

func (r *fakeTraceRows) Next() bool {
	return r.idx < len(r.rows)
}

func (r *fakeTraceRows) Scan(dest ...any) error {
	assignScanValues(dest, r.rows[r.idx])
	r.idx++
	return nil
}

func assignScanValues(dest []any, values []any) {
	for i := range dest {
		target := reflect.ValueOf(dest[i]).Elem()
		value := reflect.ValueOf(values[i])
		if value.Type().ConvertibleTo(target.Type()) {
			value = value.Convert(target.Type())
		}
		target.Set(value)
	}
}
