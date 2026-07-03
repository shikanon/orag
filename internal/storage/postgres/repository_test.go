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
	"github.com/shikanon/orag/internal/dataset"
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

func TestRepositoryPutKnowledgeBaseReturnsExecError(t *testing.T) {
	want := errors.New("exec failed")
	queryer := &fakeKnowledgeBaseQueryer{execErr: want}
	repo := &Repository{kbQueryer: queryer}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := repo.PutKnowledgeBase(ctx, kb.KnowledgeBase{
		ID:        "kb_1",
		TenantID:  "tenant_1",
		Name:      "Docs",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})

	if !errors.Is(err, want) {
		t.Fatalf("PutKnowledgeBase() error = %v, want %v", err, want)
	}
	if queryer.execCalls != 1 {
		t.Fatalf("Exec calls = %d, want 1", queryer.execCalls)
	}
	if queryer.execCtx != ctx {
		t.Fatal("PutKnowledgeBase() did not pass caller context to Exec")
	}
}

func TestRepositoryListKnowledgeBasesReturnsRowsAndKeepsOrderingSQL(t *testing.T) {
	createdAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	queryer := &fakeKnowledgeBaseQueryer{queryRows: &fakeTraceRows{rows: [][]any{
		knowledgeBaseRow("kb_1", createdAt),
		knowledgeBaseRow("kb_2", createdAt.Add(time.Hour)),
	}}}
	repo := &Repository{kbQueryer: queryer}

	got, err := repo.ListKnowledgeBases("tenant_1")
	if err != nil {
		t.Fatalf("ListKnowledgeBases() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "kb_1" || got[1].ID != "kb_2" {
		t.Fatalf("ListKnowledgeBases() = %#v", got)
	}
	if got[0].Metadata["source"] != "test" {
		t.Fatalf("metadata = %#v", got[0].Metadata)
	}
	if !strings.Contains(queryer.querySQL, "ORDER BY created_at") {
		t.Fatalf("list query does not preserve created_at ordering: %s", queryer.querySQL)
	}
}

func TestRepositoryListKnowledgeBasesReturnsQueryError(t *testing.T) {
	want := errors.New("query failed")
	repo := &Repository{kbQueryer: &fakeKnowledgeBaseQueryer{queryErr: want}}

	got, err := repo.ListKnowledgeBases("tenant_1")
	if !errors.Is(err, want) {
		t.Fatalf("ListKnowledgeBases() error = %v, want %v", err, want)
	}
	if got != nil {
		t.Fatalf("ListKnowledgeBases() rows = %#v, want nil", got)
	}
}

func TestRepositoryListKnowledgeBasesReturnsScanError(t *testing.T) {
	want := errors.New("scan failed")
	createdAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	repo := &Repository{kbQueryer: &fakeKnowledgeBaseQueryer{queryRows: &fakeTraceRows{
		rows:    [][]any{knowledgeBaseRow("kb_1", createdAt)},
		scanErr: want,
	}}}

	_, err := repo.ListKnowledgeBases("tenant_1")
	if !errors.Is(err, want) {
		t.Fatalf("ListKnowledgeBases() error = %v, want %v", err, want)
	}
}

func TestRepositoryListKnowledgeBasesReturnsRowsError(t *testing.T) {
	want := errors.New("rows failed")
	repo := &Repository{kbQueryer: &fakeKnowledgeBaseQueryer{queryRows: &fakeTraceRows{err: want}}}

	_, err := repo.ListKnowledgeBases("tenant_1")
	if !errors.Is(err, want) {
		t.Fatalf("ListKnowledgeBases() error = %v, want %v", err, want)
	}
}

func TestRepositoryGetKnowledgeBaseNotFound(t *testing.T) {
	repo := &Repository{kbQueryer: &fakeKnowledgeBaseQueryer{row: fakeTraceRow{err: pgx.ErrNoRows}}}

	got, found, err := repo.GetKnowledgeBase("tenant_1", "kb_missing")
	if err != nil {
		t.Fatalf("GetKnowledgeBase() error = %v", err)
	}
	if found {
		t.Fatalf("GetKnowledgeBase() found = true, item = %#v", got)
	}
}

func TestRepositoryGetKnowledgeBaseReturnsScanError(t *testing.T) {
	want := errors.New("scan failed")
	repo := &Repository{kbQueryer: &fakeKnowledgeBaseQueryer{row: fakeTraceRow{err: want}}}

	_, found, err := repo.GetKnowledgeBase("tenant_1", "kb_1")
	if !errors.Is(err, want) {
		t.Fatalf("GetKnowledgeBase() error = %v, want %v", err, want)
	}
	if found {
		t.Fatal("GetKnowledgeBase() found = true, want false")
	}
}

func TestRepositoryDeleteKnowledgeBaseLocksAndDeletesChildrenInTransaction(t *testing.T) {
	tx := &fakeKnowledgeBaseTx{
		row: fakeTraceRow{values: []any{"kb_1"}},
		execTags: []pgconn.CommandTag{
			pgconn.NewCommandTag("DELETE 1"),
			pgconn.NewCommandTag("DELETE 1"),
			pgconn.NewCommandTag("DELETE 1"),
			pgconn.NewCommandTag("DELETE 1"),
		},
	}
	beginner := &fakeKnowledgeBaseTxBeginner{tx: tx}
	repo := &Repository{kbTxBeginner: beginner}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	deleted, err := repo.DeleteKnowledgeBase(ctx, "tenant_1", "kb_1")
	if err != nil {
		t.Fatalf("DeleteKnowledgeBase() error = %v", err)
	}
	if !deleted {
		t.Fatal("DeleteKnowledgeBase() deleted = false, want true")
	}
	if beginner.calls != 1 || beginner.beginCtx != ctx {
		t.Fatal("DeleteKnowledgeBase() did not begin a transaction with caller context")
	}
	for _, want := range []string{"FROM knowledge_bases", "tenant_id=$1 AND id=$2", "FOR UPDATE"} {
		if !strings.Contains(tx.queryRowSQL, want) {
			t.Fatalf("lock query missing %q: %s", want, tx.queryRowSQL)
		}
	}
	wantTables := []string{"chunks", "documents", "ingestion_jobs", "knowledge_bases"}
	if len(tx.execSQLs) != len(wantTables) {
		t.Fatalf("Exec calls = %d, want %d: %#v", len(tx.execSQLs), len(wantTables), tx.execSQLs)
	}
	for i, table := range wantTables {
		if !strings.Contains(tx.execSQLs[i], "DELETE FROM "+table) {
			t.Fatalf("delete %d SQL = %s, want table %s", i, tx.execSQLs[i], table)
		}
		if table == "knowledge_bases" {
			if !strings.Contains(tx.execSQLs[i], "tenant_id=$1 AND id=$2") {
				t.Fatalf("knowledge base delete missing tenant guard: %s", tx.execSQLs[i])
			}
			continue
		}
		if !strings.Contains(tx.execSQLs[i], "tenant_id=$1 AND knowledge_base_id=$2") {
			t.Fatalf("%s delete missing tenant/kb guard: %s", table, tx.execSQLs[i])
		}
	}
	if tx.commitCalls != 1 {
		t.Fatalf("Commit calls = %d, want 1", tx.commitCalls)
	}
}

func TestRepositoryDeleteKnowledgeBaseMissingDoesNotDeleteChildren(t *testing.T) {
	tx := &fakeKnowledgeBaseTx{row: fakeTraceRow{err: pgx.ErrNoRows}}
	repo := &Repository{kbTxBeginner: &fakeKnowledgeBaseTxBeginner{tx: tx}}

	deleted, err := repo.DeleteKnowledgeBase(context.Background(), "tenant_1", "kb_missing")
	if err != nil {
		t.Fatalf("DeleteKnowledgeBase() error = %v", err)
	}
	if deleted {
		t.Fatal("DeleteKnowledgeBase() deleted = true, want false")
	}
	if len(tx.execSQLs) != 0 {
		t.Fatalf("missing knowledge base deleted children: %#v", tx.execSQLs)
	}
	if tx.commitCalls != 0 {
		t.Fatalf("Commit calls = %d, want 0", tx.commitCalls)
	}
	if tx.rollbackCalls != 1 {
		t.Fatalf("Rollback calls = %d, want 1", tx.rollbackCalls)
	}
}

func TestRepositoryDeleteKnowledgeBaseRollsBackOnChildDeleteError(t *testing.T) {
	want := errors.New("delete chunks failed")
	tx := &fakeKnowledgeBaseTx{
		row:      fakeTraceRow{values: []any{"kb_1"}},
		execErrs: []error{want},
	}
	repo := &Repository{kbTxBeginner: &fakeKnowledgeBaseTxBeginner{tx: tx}}

	deleted, err := repo.DeleteKnowledgeBase(context.Background(), "tenant_1", "kb_1")
	if !errors.Is(err, want) {
		t.Fatalf("DeleteKnowledgeBase() error = %v, want %v", err, want)
	}
	if deleted {
		t.Fatal("DeleteKnowledgeBase() deleted = true, want false")
	}
	if tx.commitCalls != 0 {
		t.Fatalf("Commit calls = %d, want 0", tx.commitCalls)
	}
	if tx.rollbackCalls != 1 {
		t.Fatalf("Rollback calls = %d, want 1", tx.rollbackCalls)
	}
}

func TestRepositoryBootstrapDefaultsReturnsKnowledgeBaseError(t *testing.T) {
	want := errors.New("knowledge base insert failed")
	queryer := &fakeKnowledgeBaseQueryer{execErrs: []error{nil, want}}
	repo := &Repository{kbQueryer: queryer}

	err := repo.BootstrapDefaults(context.Background(), "tenant_1", "kb_default")
	if !errors.Is(err, want) {
		t.Fatalf("BootstrapDefaults() error = %v, want %v", err, want)
	}
	if queryer.execCalls != 2 {
		t.Fatalf("Exec calls = %d, want 2", queryer.execCalls)
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

func TestChunkSearchableMigration(t *testing.T) {
	initBody, err := os.ReadFile("../../../migrations/000001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(initBody), "searchable BOOLEAN NOT NULL DEFAULT TRUE") {
		t.Fatalf("initial schema does not create searchable chunks: %s", initBody)
	}
	body, err := os.ReadFile("../../../migrations/000004_chunk_searchable.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{"ADD COLUMN IF NOT EXISTS searchable", "DEFAULT TRUE", "WHERE searchable"} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q: %s", required, text)
		}
	}
}

func TestFTSRetrieverFiltersSearchableChunks(t *testing.T) {
	queryer := &fakeKnowledgeBaseQueryer{queryRows: &fakeTraceRows{}}
	retriever := FTSRetriever{queryer: queryer}

	_, err := retriever.Retrieve(context.Background(), kb.SearchRequest{
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		Query:           "partial ingest",
		TopK:            8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.querySQL, "AND searchable") {
		t.Fatalf("FTS query does not filter searchable chunks: %s", queryer.querySQL)
	}
}

func TestRepositoryAddDatasetItemRequiresTenantDataset(t *testing.T) {
	queryer := &fakeKnowledgeBaseQueryer{execTag: pgconn.NewCommandTag("INSERT 0 0")}
	repo := &Repository{kbQueryer: queryer, datasetRunner: queryer}

	_, err := repo.AddDatasetItem(context.Background(), "tenant_other", dataset.Item{
		ID:          "dsi_1",
		DatasetID:   "ds_1",
		Query:       "q",
		GroundTruth: "a",
	})

	if !errors.Is(err, dataset.ErrDatasetNotFound) {
		t.Fatalf("AddDatasetItem() error = %v, want dataset not found", err)
	}
	for _, want := range []string{"WHERE EXISTS", "tenant_id=$6", "id=$2"} {
		if !strings.Contains(queryer.execSQL, want) {
			t.Fatalf("dataset item insert missing %q tenant guard: %s", want, queryer.execSQL)
		}
	}
}

func TestRepositoryDatasetItemsFiltersTenant(t *testing.T) {
	createdAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	queryer := &fakeKnowledgeBaseQueryer{
		row:       fakeTraceRow{values: datasetRow("ds_1", createdAt)},
		queryRows: &fakeTraceRows{},
	}
	repo := &Repository{kbQueryer: queryer, datasetRunner: queryer}

	_, err := repo.DatasetItems(context.Background(), "tenant_1", "ds_1")
	if err != nil {
		t.Fatalf("DatasetItems() error = %v", err)
	}
	for _, want := range []string{"JOIN datasets d ON d.id=i.dataset_id", "d.tenant_id=$1", "i.dataset_id=$2"} {
		if !strings.Contains(queryer.querySQL, want) {
			t.Fatalf("dataset items query missing %q tenant guard: %s", want, queryer.querySQL)
		}
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

type fakeTraceReader struct {
	row          fakeTraceRow
	rows         *fakeTraceRows
	rowsErr      error
	rowsSQL      string
	queriedSpans bool
}

type fakeKnowledgeBaseQueryer struct {
	execErr   error
	execErrs  []error
	execTag   pgconn.CommandTag
	execCtx   context.Context
	execSQL   string
	execCalls int
	queryRows pgx.Rows
	queryErr  error
	querySQL  string
	row       pgx.Row
}

type fakeKnowledgeBaseTxBeginner struct {
	tx       *fakeKnowledgeBaseTx
	err      error
	beginCtx context.Context
	calls    int
}

func (f *fakeKnowledgeBaseTxBeginner) BeginKnowledgeBaseTx(ctx context.Context) (knowledgeBaseTx, error) {
	f.beginCtx = ctx
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.tx, nil
}

type fakeKnowledgeBaseTx struct {
	row           pgx.Row
	queryRowSQL   string
	queryRowArg   []any
	execErrs      []error
	execTags      []pgconn.CommandTag
	execSQLs      []string
	execArgs      [][]any
	commitErr     error
	commitCalls   int
	rollbackErr   error
	rollbackCalls int
}

func (f *fakeKnowledgeBaseTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	f.queryRowSQL = sql
	f.queryRowArg = args
	if f.row == nil {
		return fakeTraceRow{err: pgx.ErrNoRows}
	}
	return f.row
}

func (f *fakeKnowledgeBaseTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	call := len(f.execSQLs)
	f.execSQLs = append(f.execSQLs, sql)
	f.execArgs = append(f.execArgs, args)
	if call < len(f.execErrs) && f.execErrs[call] != nil {
		return pgconn.CommandTag{}, f.execErrs[call]
	}
	if call < len(f.execTags) {
		return f.execTags[call], nil
	}
	return pgconn.NewCommandTag("DELETE 0"), nil
}

func (f *fakeKnowledgeBaseTx) Commit(context.Context) error {
	f.commitCalls++
	return f.commitErr
}

func (f *fakeKnowledgeBaseTx) Rollback(context.Context) error {
	f.rollbackCalls++
	return f.rollbackErr
}

func (f *fakeKnowledgeBaseQueryer) Exec(ctx context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	f.execCtx = ctx
	f.execSQL = sql
	err := f.execErr
	if f.execCalls < len(f.execErrs) {
		err = f.execErrs[f.execCalls]
	}
	f.execCalls++
	return f.execTag, err
}

func (f *fakeKnowledgeBaseQueryer) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	f.querySQL = sql
	return f.queryRows, f.queryErr
}

func (f *fakeKnowledgeBaseQueryer) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	if f.row == nil {
		return fakeTraceRow{err: pgx.ErrNoRows}
	}
	return f.row
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
	rows    [][]any
	idx     int
	err     error
	scanErr error
}

func (r *fakeTraceRows) Close() {}

func (r *fakeTraceRows) Err() error {
	return r.err
}

func (r *fakeTraceRows) Next() bool {
	return r.idx < len(r.rows)
}

func (r *fakeTraceRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	assignScanValues(dest, r.rows[r.idx])
	r.idx++
	return nil
}

func (r *fakeTraceRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (r *fakeTraceRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (r *fakeTraceRows) Values() ([]any, error) {
	if r.idx == 0 || r.idx > len(r.rows) {
		return nil, nil
	}
	return r.rows[r.idx-1], nil
}

func (r *fakeTraceRows) RawValues() [][]byte {
	return nil
}

func (r *fakeTraceRows) Conn() *pgx.Conn {
	return nil
}

func knowledgeBaseRow(id string, createdAt time.Time) []any {
	return []any{
		id,
		"tenant_1",
		"Docs",
		"Description",
		[]byte(`{"source":"test"}`),
		createdAt,
		createdAt.Add(time.Minute),
	}
}

func datasetRow(id string, createdAt time.Time) []any {
	return []any{
		id,
		"tenant_1",
		"Golden",
		"golden",
		"20260629100000",
		createdAt,
	}
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
