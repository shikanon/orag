package postgres

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func TestDeleteKnowledgeBaseRowsCleansRelationalTables(t *testing.T) {
	execer := &fakeDeleteExecer{tags: []pgconn.CommandTag{
		pgconn.NewCommandTag("DELETE 2"),
		pgconn.NewCommandTag("DELETE 1"),
		pgconn.NewCommandTag("DELETE 1"),
		pgconn.NewCommandTag("DELETE 1"),
	}}

	deleted, err := deleteKnowledgeBaseRows(context.Background(), execer, "tenant_1", "kb_1")
	if err != nil {
		t.Fatalf("deleteKnowledgeBaseRows() error = %v", err)
	}
	if !deleted {
		t.Fatal("deleteKnowledgeBaseRows() deleted = false, want true")
	}

	wantDeletes := []string{
		"DELETE FROM chunks",
		"DELETE FROM documents",
		"DELETE FROM ingestion_jobs",
		"DELETE FROM knowledge_bases",
	}
	if len(execer.calls) != len(wantDeletes) {
		t.Fatalf("Exec calls = %d, want %d", len(execer.calls), len(wantDeletes))
	}
	for i, call := range execer.calls {
		sql := strings.Join(strings.Fields(call.sql), " ")
		if !strings.Contains(sql, wantDeletes[i]) {
			t.Fatalf("call %d SQL = %s, want %q", i, sql, wantDeletes[i])
		}
		if !reflect.DeepEqual(call.args, []any{"tenant_1", "kb_1"}) {
			t.Fatalf("call %d args = %#v", i, call.args)
		}
		if !strings.Contains(sql, "tenant_id=$1") || !strings.Contains(sql, "id=$2") {
			t.Fatalf("call %d is not tenant-scoped: %s", i, sql)
		}
	}
}

func TestDeleteKnowledgeBaseRowsReportsMissingKnowledgeBase(t *testing.T) {
	execer := &fakeDeleteExecer{tags: []pgconn.CommandTag{
		pgconn.NewCommandTag("DELETE 0"),
		pgconn.NewCommandTag("DELETE 0"),
		pgconn.NewCommandTag("DELETE 0"),
		pgconn.NewCommandTag("DELETE 0"),
	}}

	deleted, err := deleteKnowledgeBaseRows(context.Background(), execer, "tenant_1", "missing_kb")
	if err != nil {
		t.Fatalf("deleteKnowledgeBaseRows() error = %v", err)
	}
	if deleted {
		t.Fatal("deleteKnowledgeBaseRows() deleted = true, want false")
	}
}

func TestDeleteKnowledgeBaseRowsStopsOnCleanupError(t *testing.T) {
	cleanupErr := errors.New("delete chunks failed")
	execer := &fakeDeleteExecer{errAt: 0, err: cleanupErr}

	deleted, err := deleteKnowledgeBaseRows(context.Background(), execer, "tenant_1", "kb_1")
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("deleteKnowledgeBaseRows() error = %v, want %v", err, cleanupErr)
	}
	if deleted {
		t.Fatal("deleteKnowledgeBaseRows() deleted = true, want false")
	}
	if len(execer.calls) != 1 {
		t.Fatalf("Exec calls = %d, want 1", len(execer.calls))
	}
}

func TestRepositoryGetTraceFound(t *testing.T) {
	createdAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	reader := &fakeTraceReader{
		row: fakeTraceRow{values: []any{"trace_1", "tenant_1", "realtime", int64(123), createdAt}},
		rows: &fakeTraceRows{rows: [][]any{
			{"span_1", "retrieve", int64(12), "", createdAt.Add(time.Millisecond)},
			{"span_2", "generate", int64(111), "llm timeout", createdAt.Add(2 * time.Millisecond)},
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
	if !strings.Contains(reader.rowsSQL, "ORDER BY created_at, id") {
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

type deleteExecCall struct {
	sql  string
	args []any
}

type fakeDeleteExecer struct {
	calls []deleteExecCall
	tags  []pgconn.CommandTag
	errAt int
	err   error
}

func (f *fakeDeleteExecer) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	idx := len(f.calls)
	f.calls = append(f.calls, deleteExecCall{sql: sql, args: append([]any(nil), args...)})
	if f.err != nil && idx == f.errAt {
		return pgconn.CommandTag{}, f.err
	}
	if idx < len(f.tags) {
		return f.tags[idx], nil
	}
	return pgconn.NewCommandTag("DELETE 0"), nil
}

type fakeTraceReader struct {
	row          fakeTraceRow
	rows         *fakeTraceRows
	rowsErr      error
	rowsSQL      string
	queriedSpans bool
}

func (f *fakeTraceReader) QueryRow(_ context.Context, sql string, _ ...any) traceRow {
	return f.row
}

func (f *fakeTraceReader) Query(_ context.Context, sql string, _ ...any) (traceRows, error) {
	f.queriedSpans = true
	f.rowsSQL = sql
	return f.rows, f.rowsErr
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
