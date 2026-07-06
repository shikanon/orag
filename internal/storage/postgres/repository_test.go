package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shikanon/orag/internal/dataset"
	evalpkg "github.com/shikanon/orag/internal/eval"
	raggraph "github.com/shikanon/orag/internal/graph"
	"github.com/shikanon/orag/internal/kb"
	optimizerpkg "github.com/shikanon/orag/internal/optimizer"
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

func TestExtractGooseDown(t *testing.T) {
	got, err := extractGooseDown(`-- +goose Up
CREATE TABLE example(id TEXT);
-- +goose Down
DROP TABLE example;`)
	if err != nil {
		t.Fatalf("extractGooseDown() error = %v", err)
	}
	if strings.Contains(got, "CREATE TABLE") {
		t.Fatalf("down migration contains up section: %q", got)
	}
	if !strings.Contains(got, "DROP TABLE example") {
		t.Fatalf("down migration missing drop statement: %q", got)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got, err := repo.ListKnowledgeBases(ctx, "tenant_1")
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
	if queryer.queryCtx != ctx {
		t.Fatal("ListKnowledgeBases() did not pass caller context to Query")
	}
}

func TestRepositoryListKnowledgeBasesReturnsQueryError(t *testing.T) {
	want := errors.New("query failed")
	repo := &Repository{kbQueryer: &fakeKnowledgeBaseQueryer{queryErr: want}}

	got, err := repo.ListKnowledgeBases(context.Background(), "tenant_1")
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

	_, err := repo.ListKnowledgeBases(context.Background(), "tenant_1")
	if !errors.Is(err, want) {
		t.Fatalf("ListKnowledgeBases() error = %v, want %v", err, want)
	}
}

func TestRepositoryListKnowledgeBasesReturnsRowsError(t *testing.T) {
	want := errors.New("rows failed")
	repo := &Repository{kbQueryer: &fakeKnowledgeBaseQueryer{queryRows: &fakeTraceRows{err: want}}}

	_, err := repo.ListKnowledgeBases(context.Background(), "tenant_1")
	if !errors.Is(err, want) {
		t.Fatalf("ListKnowledgeBases() error = %v, want %v", err, want)
	}
}

func TestRepositoryGetKnowledgeBaseNotFound(t *testing.T) {
	queryer := &fakeKnowledgeBaseQueryer{row: fakeTraceRow{err: pgx.ErrNoRows}}
	repo := &Repository{kbQueryer: queryer}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got, found, err := repo.GetKnowledgeBase(ctx, "tenant_1", "kb_missing")
	if err != nil {
		t.Fatalf("GetKnowledgeBase() error = %v", err)
	}
	if found {
		t.Fatalf("GetKnowledgeBase() found = true, item = %#v", got)
	}
	if queryer.rowCtx != ctx {
		t.Fatal("GetKnowledgeBase() did not pass caller context to QueryRow")
	}
}

func TestRepositoryGetKnowledgeBaseReturnsScanError(t *testing.T) {
	want := errors.New("scan failed")
	repo := &Repository{kbQueryer: &fakeKnowledgeBaseQueryer{row: fakeTraceRow{err: want}}}

	_, found, err := repo.GetKnowledgeBase(context.Background(), "tenant_1", "kb_1")
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
	wantTables := []string{"harness_runs", "optimization_candidates", "optimization_runs", "chunks", "documents", "ingestion_jobs", "knowledge_bases"}
	if len(tx.execSQLs) != len(wantTables) {
		t.Fatalf("Exec calls = %d, want %d: %#v", len(tx.execSQLs), len(wantTables), tx.execSQLs)
	}
	for i, table := range wantTables {
		if !strings.Contains(tx.execSQLs[i], "DELETE FROM "+table) {
			t.Fatalf("delete %d SQL = %s, want table %s", i, tx.execSQLs[i], table)
		}
		switch table {
		case "knowledge_bases":
			if !strings.Contains(tx.execSQLs[i], "tenant_id=$1 AND id=$2") {
				t.Fatalf("knowledge base delete missing tenant guard: %s", tx.execSQLs[i])
			}
			continue
		case "harness_runs":
			for _, want := range []string{"tenant_id=$1", "candidate_id IN", "r.tenant_id=$1", "r.knowledge_base_id=$2"} {
				if !strings.Contains(tx.execSQLs[i], want) {
					t.Fatalf("harness delete missing %q: %s", want, tx.execSQLs[i])
				}
			}
			continue
		case "optimization_candidates":
			for _, want := range []string{"USING optimization_runs", "r.tenant_id=$1", "r.knowledge_base_id=$2"} {
				if !strings.Contains(tx.execSQLs[i], want) {
					t.Fatalf("optimization candidate delete missing %q: %s", want, tx.execSQLs[i])
				}
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

func TestRepositoryStoreStagedChunksDoesNotDeleteExistingSource(t *testing.T) {
	tx := &fakeKnowledgeBaseTx{}
	repo := &Repository{
		StageChunks:  true,
		kbTxBeginner: &fakeKnowledgeBaseTxBeginner{tx: tx},
	}

	err := repo.Store(context.Background(), kb.Document{
		ID:              "doc_new",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		SourceURI:       "memory://replace.md",
		Title:           "replace.md",
		ContentHash:     "hash_new",
	}, []kb.Chunk{{
		ID:              "chk_new",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      "doc_new",
		SourceURI:       "memory://replace.md",
		Content:         "new content",
	}})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if len(tx.execSQLs) != 2 {
		t.Fatalf("Store() exec calls = %d, want document and chunk inserts: %#v", len(tx.execSQLs), tx.execSQLs)
	}
	for _, sql := range tx.execSQLs {
		if strings.Contains(sql, "DELETE FROM") {
			t.Fatalf("staged Store deleted existing source before activation: %s", sql)
		}
	}
	if got := tx.execArgs[1][11]; got != false {
		t.Fatalf("staged chunk searchable arg = %#v, want false", got)
	}
	if tx.commitCalls != 1 {
		t.Fatalf("Commit calls = %d, want 1", tx.commitCalls)
	}
}

func TestRepositoryActivateDeletesOldSourceAndMarksNewChunksSearchable(t *testing.T) {
	tx := &fakeKnowledgeBaseTx{}
	repo := &Repository{kbTxBeginner: &fakeKnowledgeBaseTxBeginner{tx: tx}}

	err := repo.Activate(context.Background(), kb.Document{
		ID:              "doc_new",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		SourceURI:       "memory://replace.md",
		ContentHash:     "hash_new",
	}, []kb.Chunk{{ID: "chk_new"}})
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if len(tx.execSQLs) != 3 {
		t.Fatalf("Activate() exec calls = %d, want old chunk delete, old doc delete, activation update: %#v", len(tx.execSQLs), tx.execSQLs)
	}
	for i := 0; i < 2; i++ {
		if !strings.Contains(tx.execSQLs[i], "content_hash<>$4") {
			t.Fatalf("old source delete %d missing current hash guard: %s", i, tx.execSQLs[i])
		}
		if got := tx.execArgs[i][3]; got != "hash_new" {
			t.Fatalf("old source delete %d keep hash arg = %#v, want hash_new", i, got)
		}
	}
	if !strings.Contains(tx.execSQLs[2], "SET searchable=TRUE") {
		t.Fatalf("activation update missing searchable flag: %s", tx.execSQLs[2])
	}
	if got := tx.execArgs[2][2]; got != "doc_new" {
		t.Fatalf("activation document arg = %#v, want doc_new", got)
	}
	if tx.commitCalls != 1 {
		t.Fatalf("Commit calls = %d, want 1", tx.commitCalls)
	}
}

func TestRepositoryBootstrapDefaultsReturnsKnowledgeBaseWriteError(t *testing.T) {
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

func TestTask1MetadataAndPersistenceMigrationsAreReversible(t *testing.T) {
	tests := []struct {
		file         string
		upRequired   []string
		downRequired []string
	}{
		{
			file: "000007_dataset_eval_metadata.sql",
			upRequired: []string{
				"ADD COLUMN IF NOT EXISTS split TEXT NOT NULL DEFAULT 'eval'",
				"ADD COLUMN IF NOT EXISTS weight DOUBLE PRECISION NOT NULL DEFAULT 1",
				"expected_evidence JSONB NOT NULL DEFAULT '[]'::jsonb",
				"human_scores JSONB NOT NULL DEFAULT '{}'::jsonb",
			},
			downRequired: []string{
				"DROP COLUMN IF EXISTS human_scores",
				"DROP COLUMN IF EXISTS expected_evidence",
				"DROP COLUMN IF EXISTS weight",
				"DROP COLUMN IF EXISTS split",
			},
		},
		{
			file: "000008_judge_results.sql",
			upRequired: []string{
				"CREATE TABLE IF NOT EXISTS judge_runs",
				"CREATE TABLE IF NOT EXISTS judge_results",
				"CREATE TABLE IF NOT EXISTS pairwise_judge_results",
				"CREATE TABLE IF NOT EXISTS judge_calibration_runs",
				"CREATE INDEX IF NOT EXISTS judge_runs_eval_idx",
			},
			downRequired: []string{
				"DROP INDEX IF EXISTS judge_runs_eval_idx",
				"DROP TABLE IF EXISTS judge_calibration_runs",
				"DROP TABLE IF EXISTS pairwise_judge_results",
				"DROP TABLE IF EXISTS judge_results",
				"DROP TABLE IF EXISTS judge_runs",
			},
		},
		{
			file: "000009_optimizer_runs.sql",
			upRequired: []string{
				"CREATE TABLE IF NOT EXISTS optimization_runs",
				"CREATE TABLE IF NOT EXISTS optimization_candidates",
				"CREATE INDEX IF NOT EXISTS optimization_runs_tenant_status_idx",
				"temp_namespaces JSONB NOT NULL DEFAULT '[]'::jsonb",
			},
			downRequired: []string{
				"DROP INDEX IF EXISTS optimization_runs_tenant_status_idx",
				"DROP TABLE IF EXISTS optimization_candidates",
				"DROP TABLE IF EXISTS optimization_runs",
			},
		},
		{
			file: "000010_harness_runs.sql",
			upRequired: []string{
				"CREATE TABLE IF NOT EXISTS harness_runs",
				"argv JSONB NOT NULL DEFAULT '[]'::jsonb",
				"env_redacted JSONB NOT NULL DEFAULT '{}'::jsonb",
				"CREATE INDEX IF NOT EXISTS harness_runs_candidate_idx",
			},
			downRequired: []string{
				"DROP INDEX IF EXISTS harness_runs_candidate_idx",
				"DROP TABLE IF EXISTS harness_runs",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			body, err := os.ReadFile("../../../migrations/" + tt.file)
			if err != nil {
				t.Fatal(err)
			}
			up, err := extractGooseUp(string(body))
			if err != nil {
				t.Fatal(err)
			}
			down, err := extractGooseDown(string(body))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.upRequired {
				if !strings.Contains(up, want) {
					t.Fatalf("up migration missing %q: %s", want, up)
				}
			}
			for _, want := range tt.downRequired {
				if !strings.Contains(down, want) {
					t.Fatalf("down migration missing %q: %s", want, down)
				}
			}
		})
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

func TestEvaluationRunMetricsRejectsUnknownMetric(t *testing.T) {
	_, err := encodeEvaluationRunMetrics(evalpkg.RunResult{
		Total:    1,
		HitRate:  1,
		Accuracy: 1,
		Metrics: map[string]float64{
			"answer_accuracy": 1,
			"harness_custom":  0.5,
		},
	})
	if err == nil {
		t.Fatal("encodeEvaluationRunMetrics() error = nil, want validation")
	}
	if !strings.Contains(err.Error(), "harness_custom") {
		t.Fatalf("encodeEvaluationRunMetrics() error = %v, want metric name", err)
	}
}

func TestRepositoryStoresJudgeRunAndResults(t *testing.T) {
	queryer := &fakeKnowledgeBaseQueryer{}
	repo := &Repository{evalQueryer: queryer}
	now := time.Date(2026, 7, 4, 8, 0, 0, 0, time.UTC)

	err := repo.StoreJudgeRun(context.Background(), "tenant_1", evalpkg.JudgeRunRecord{
		ID:              "judge_1",
		EvaluationRunID: "eval_1",
		Provider:        "test-provider",
		Model:           "judge-model",
		PromptVersion:   "prompt-v1",
		RubricHash:      "rubric_hash",
		PromptHash:      "prompt_hash",
		ConfigHash:      "config_hash",
		Mode:            "llm_judge",
		ComparisonMode:  "absolute",
		CreatedAt:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "INSERT INTO judge_runs") {
		t.Fatalf("StoreJudgeRun SQL = %s, want judge_runs insert", queryer.execSQL)
	}
	if len(queryer.execArgs) != 15 || queryer.execArgs[0] != "judge_1" || queryer.execArgs[1] != "tenant_1" {
		t.Fatalf("StoreJudgeRun args = %#v", queryer.execArgs)
	}

	err = repo.StoreJudgeResult(context.Background(), evalpkg.JudgeResultRecord{
		ID:            "judger_1",
		JudgeRunID:    "judge_1",
		DatasetItemID: "dsi_1",
		Scores:        map[string]float64{"faithfulness": 0.9},
		Pass:          true,
		Rationale:     "supported",
		Findings:      []evalpkg.JudgeFinding{{Metric: "faithfulness", Label: "good"}},
		RawResponse:   `{"scores":{"faithfulness":0.9}}`,
		ParsedJSON:    map[string]any{"scores": map[string]any{"faithfulness": 0.9}},
		TokenUsage:    evalpkg.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		CostUSD:       0.02,
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "INSERT INTO judge_results") {
		t.Fatalf("StoreJudgeResult SQL = %s, want judge_results insert", queryer.execSQL)
	}
	rawResponse, _ := queryer.execArgs[8].(string)
	if rawResponse == "" || !strings.Contains(rawResponse, "faithfulness") {
		t.Fatalf("raw response arg = %#v, want unparsed raw response", queryer.execArgs[8])
	}
	tokenUsage, _ := queryer.execArgs[11].([]byte)
	if !strings.Contains(string(tokenUsage), "total_tokens") {
		t.Fatalf("token usage arg = %s", string(tokenUsage))
	}
	if queryer.execArgs[12] != 0.02 {
		t.Fatalf("cost arg = %#v, want 0.02", queryer.execArgs[12])
	}
}

func TestRepositoryStoresPairwiseAndCalibrationJudgeDetails(t *testing.T) {
	queryer := &fakeKnowledgeBaseQueryer{}
	repo := &Repository{evalQueryer: queryer}
	now := time.Date(2026, 7, 4, 8, 0, 0, 0, time.UTC)

	err := repo.StorePairwiseJudgeResult(context.Background(), evalpkg.PairwiseJudgeResultRecord{
		ID:            "pair_1",
		JudgeRunID:    "judge_1",
		DatasetItemID: "dsi_1",
		CandidateAID:  "candidate_a",
		CandidateBID:  "candidate_b",
		Winner:        "A",
		Preference:    "A_better",
		Reasons:       []evalpkg.JudgeFinding{{Metric: "pairwise", Label: "A"}},
		RawResponse:   `{"winner":"A"}`,
		ParsedJSON:    map[string]any{"winner": "A"},
		TokenUsage:    evalpkg.TokenUsage{TotalTokens: 3},
		CostUSD:       0.01,
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "INSERT INTO pairwise_judge_results") {
		t.Fatalf("StorePairwiseJudgeResult SQL = %s", queryer.execSQL)
	}

	err = repo.StoreJudgeCalibrationRun(context.Background(), "tenant_1", evalpkg.JudgeCalibrationRunRecord{
		ID:                "cal_1",
		DatasetID:         "ds_1",
		JudgeConfigHash:   "config_hash",
		HumanScoreVersion: "gold-v1",
		Spearman:          0.9,
		CohenKappa:        0.8,
		SampleCount:       10,
		Metrics:           map[string]float64{"faithfulness": 0.9},
		CreatedAt:         now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "INSERT INTO judge_calibration_runs") {
		t.Fatalf("StoreJudgeCalibrationRun SQL = %s", queryer.execSQL)
	}
	if queryer.execArgs[1] != "tenant_1" || queryer.execArgs[7] != 10 {
		t.Fatalf("calibration args = %#v", queryer.execArgs)
	}
}

func TestRepositoryStoresOptimizationRunAndCandidate(t *testing.T) {
	queryer := &fakeKnowledgeBaseQueryer{}
	repo := &Repository{evalQueryer: queryer}
	now := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)
	costBudget := 2.0

	run := optimizerpkg.OptimizationRun{
		ID:              "opt_1",
		TenantID:        "tenant_1",
		DatasetID:       "ds_1",
		KnowledgeBaseID: "kb_1",
		Objective:       optimizerpkg.ObjectiveSpec{Maximize: "pairwise_accuracy"},
		SearchSpace: optimizerpkg.SearchSpace{Retrieval: optimizerpkg.RetrievalSpace{
			DenseTopK: []int{4, 8},
		}},
		Config: optimizerpkg.RunConfig{
			DatasetID:       "ds_1",
			KnowledgeBaseID: "kb_1",
			Objective:       optimizerpkg.ObjectiveSpec{Maximize: "pairwise_accuracy"},
			SearchSpace: optimizerpkg.SearchSpace{Retrieval: optimizerpkg.RetrievalSpace{
				DenseTopK: []int{4, 8},
			}},
			Search:         optimizerpkg.SearchSpec{Strategy: optimizerpkg.SearchStrategyGrid, MaxCandidates: 2},
			Budget:         optimizerpkg.Budget{MaxWallTimeSeconds: 30},
			Profile:        rag.ProfileHighPrecision,
			TopK:           8,
			SelectionSplit: "eval",
			HoldoutSplit:   "holdout",
		},
		Runner:                map[string]any{"type": "internal_rag", "profile": string(rag.ProfileHighPrecision), "top_k": 8},
		Status:                optimizerpkg.RunStatusQueued,
		SamplingStrategy:      optimizerpkg.SearchStrategyGrid,
		SearchSpaceSize:       2,
		SampledCandidateCount: 2,
		Checkpoint: optimizerpkg.Checkpoint{
			Stage:                 "submitted",
			CompletedCandidateIDs: []string{"cand_done"},
			CostUSD:               0.5,
		},
		CostUSD:       0.5,
		CostBudgetUSD: &costBudget,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := repo.CreateOptimizationRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "INSERT INTO optimization_runs") {
		t.Fatalf("CreateOptimizationRun SQL = %s", queryer.execSQL)
	}
	checkpoint, _ := queryer.execArgs[15].([]byte)
	if !strings.Contains(string(checkpoint), "completed_candidate_ids") || queryer.execArgs[18] != &costBudget {
		t.Fatalf("run args = %#v checkpoint=%s", queryer.execArgs, string(checkpoint))
	}
	runner, _ := queryer.execArgs[6].([]byte)
	for _, want := range []string{`"run_config"`, `"holdout_split":"holdout"`, `"profile":"high_precision"`, `"top_k":8`} {
		if !strings.Contains(string(runner), want) {
			t.Fatalf("runner config JSON missing %s: %s", want, string(runner))
		}
	}

	expiresAt := now.Add(time.Hour)
	candidate := optimizerpkg.OptimizationCandidate{
		ID:                "cand_1",
		OptimizationRunID: "opt_1",
		Config:            optimizerpkg.CandidateConfig{Retrieval: optimizerpkg.RetrievalCandidate{DenseTopK: 8}},
		Status:            optimizerpkg.CandidateStatusScored,
		EvaluationRunID:   "eval_1",
		ObjectiveScore:    0.9,
		Confidence:        map[string]float64{"pairwise_win_rate": 0.75},
		Metrics:           map[string]float64{"pairwise_accuracy": 0.9},
		CostUSD:           0.1,
		Artifacts:         map[string]any{"path": "/tmp/out.json"},
		TempNamespaces: []optimizerpkg.TempNamespace{{
			Name:      "tmp_ns",
			OwnerID:   "cand_1",
			Kind:      "index",
			Status:    optimizerpkg.CleanupPending,
			ExpiresAt: expiresAt,
		}},
		CleanupStatus: optimizerpkg.CleanupPending,
		ExpiresAt:     &expiresAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := repo.CreateOptimizationCandidate(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "INSERT INTO optimization_candidates") {
		t.Fatalf("CreateOptimizationCandidate SQL = %s", queryer.execSQL)
	}
	metrics, _ := queryer.execArgs[9].([]byte)
	namespaces, _ := queryer.execArgs[13].([]byte)
	if !strings.Contains(string(metrics), "pairwise_accuracy") || !strings.Contains(string(namespaces), "tmp_ns") {
		t.Fatalf("candidate metrics/namespaces = %s / %s", string(metrics), string(namespaces))
	}
}

func TestRepositoryUpdateOptimizationRunIncludesReadbackFields(t *testing.T) {
	queryer := &fakeKnowledgeBaseQueryer{}
	repo := &Repository{evalQueryer: queryer}
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

	run := optimizerpkg.OptimizationRun{
		ID:              "opt_1",
		TenantID:        "tenant_1",
		DatasetID:       "ds_replacement",
		KnowledgeBaseID: "kb_replacement",
		Objective:       optimizerpkg.ObjectiveSpec{Maximize: "faithfulness"},
		SearchSpace: optimizerpkg.SearchSpace{Retrieval: optimizerpkg.RetrievalSpace{
			DenseTopK: []int{6},
		}},
		Status:    optimizerpkg.RunStatusRunning,
		UpdatedAt: now,
	}
	if err := repo.UpdateOptimizationRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"dataset_id=$3", "knowledge_base_id=$4", "status=$8"} {
		if !strings.Contains(queryer.execSQL, want) {
			t.Fatalf("UpdateOptimizationRun SQL missing %q: %s", want, queryer.execSQL)
		}
	}
	if len(queryer.execArgs) < 8 {
		t.Fatalf("UpdateOptimizationRun args = %#v, want at least 8 args", queryer.execArgs)
	}
	if queryer.execArgs[2] != "ds_replacement" {
		t.Fatalf("dataset arg = %#v, want replacement dataset", queryer.execArgs[2])
	}
	if queryer.execArgs[3] != "kb_replacement" {
		t.Fatalf("knowledge base arg = %#v, want replacement knowledge base", queryer.execArgs[3])
	}
	if queryer.execArgs[7] != optimizerpkg.RunStatusRunning {
		t.Fatalf("status arg = %#v, want %q", queryer.execArgs[7], optimizerpkg.RunStatusRunning)
	}
}

func TestRepositoryListsOptimizationCandidatesWithTenantGuard(t *testing.T) {
	queryer := &fakeKnowledgeBaseQueryer{queryRows: &fakeTraceRows{}}
	repo := &Repository{evalQueryer: queryer}
	if _, err := repo.ListOptimizationCandidates(context.Background(), "tenant_1", "opt_1"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"JOIN optimization_runs r", "r.tenant_id=$1", "c.optimization_run_id=$2"} {
		if !strings.Contains(queryer.querySQL, want) {
			t.Fatalf("ListOptimizationCandidates query missing %q: %s", want, queryer.querySQL)
		}
	}
}

func TestRepositoryStoresHarnessRun(t *testing.T) {
	queryer := &fakeKnowledgeBaseQueryer{}
	repo := &Repository{evalQueryer: queryer}
	now := time.Date(2026, 7, 4, 9, 30, 0, 0, time.UTC)
	ended := now.Add(time.Second)

	err := repo.StoreHarnessRun(context.Background(), optimizerpkg.HarnessRunRecord{
		ID:             "harness_1",
		TenantID:       "tenant_1",
		CandidateID:    "cand_1",
		HarnessType:    "codex-cli",
		Argv:           []string{"codex", "eval"},
		WorkingDir:     "/tmp/harness",
		EnvRedacted:    map[string]string{"TOKEN": "[REDACTED]"},
		StdoutRedacted: `{"metrics":{"faithfulness":0.9}}`,
		StderrRedacted: "ok",
		ParsedMetrics:  map[string]float64{"faithfulness": 0.9},
		ExitCode:       0,
		Metrics:        map[string]float64{"faithfulness": 0.9},
		Artifacts:      map[string]any{"path": "/tmp/out.json"},
		StartedAt:      now,
		EndedAt:        &ended,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "INSERT INTO harness_runs") {
		t.Fatalf("StoreHarnessRun SQL = %s", queryer.execSQL)
	}
	argv, _ := queryer.execArgs[4].([]byte)
	env, _ := queryer.execArgs[6].([]byte)
	if !strings.Contains(string(argv), "codex") || !strings.Contains(string(env), "[REDACTED]") {
		t.Fatalf("harness argv/env = %s / %s", string(argv), string(env))
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

func TestRepositoryStoreTraceReplacesSpansForRepeatedTraceID(t *testing.T) {
	db := newFakeTraceDB(time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC))
	repo := &Repository{traceReader: db, traceTxBeginner: db}
	ctx := context.Background()

	err := repo.StoreTrace(ctx, "tenant_1", "trace_reused", "first query", rag.ProfileRealtime, 111, []raggraph.NodeSpan{
		{NodeName: "retrieve_first", LatencyMS: 12},
		{NodeName: "generate_first", LatencyMS: 99, Error: "first failure"},
	})
	if err != nil {
		t.Fatalf("first StoreTrace() error = %v", err)
	}
	err = repo.StoreTrace(ctx, "tenant_1", "trace_reused", "second query", rag.ProfileHighPrecision, 222, []raggraph.NodeSpan{
		{NodeName: "retrieve_second", LatencyMS: 21},
		{NodeName: "generate_second", LatencyMS: 201},
	})
	if err != nil {
		t.Fatalf("second StoreTrace() error = %v", err)
	}

	got, found, err := repo.GetTrace(ctx, "trace_reused")
	if err != nil {
		t.Fatalf("GetTrace() error = %v", err)
	}
	if !found {
		t.Fatal("GetTrace() found = false, want true")
	}
	if got.Profile != rag.ProfileHighPrecision || got.LatencyMS != 222 {
		t.Fatalf("GetTrace() metadata = %#v, want second trace metadata", got)
	}
	if got.HasError || got.ErrorCount != 0 {
		t.Fatalf("GetTrace() mixed first error span: has_error=%v error_count=%d", got.HasError, got.ErrorCount)
	}
	if len(got.NodeSpans) != 2 {
		t.Fatalf("GetTrace() spans = %#v, want only second trace spans", got.NodeSpans)
	}
	for _, span := range got.NodeSpans {
		if strings.Contains(span.NodeName, "first") || span.Error == "first failure" {
			t.Fatalf("GetTrace() mixed first trace span after repeated trace_id: %#v", got.NodeSpans)
		}
	}
	if got.NodeSpans[0].NodeName != "retrieve_second" || got.NodeSpans[1].NodeName != "generate_second" {
		t.Fatalf("GetTrace() spans = %#v, want second trace order", got.NodeSpans)
	}
}

func TestRepositoryStoreTraceFailureSpanReadsHasError(t *testing.T) {
	db := newFakeTraceDB(time.Date(2026, 7, 3, 11, 0, 0, 0, time.UTC))
	repo := &Repository{traceReader: db, traceTxBeginner: db}
	ctx := context.Background()

	err := repo.StoreTrace(ctx, "tenant_1", "trace_failed", "query", rag.ProfileHighPrecision, 47, []raggraph.NodeSpan{
		{NodeName: "init", LatencyMS: 1},
		{NodeName: "hybrid_retrieve", LatencyMS: 46, Error: "retrieval unavailable"},
	})
	if err != nil {
		t.Fatalf("StoreTrace() error = %v", err)
	}

	got, found, err := repo.GetTrace(ctx, "trace_failed")
	if err != nil {
		t.Fatalf("GetTrace() error = %v", err)
	}
	if !found {
		t.Fatal("GetTrace() found = false, want true")
	}
	if !got.HasError || got.ErrorCount != 1 {
		t.Fatalf("GetTrace() error status = has_error:%v error_count:%d", got.HasError, got.ErrorCount)
	}
	if len(got.NodeSpans) != 2 || got.NodeSpans[1].NodeName != "hybrid_retrieve" || got.NodeSpans[1].Error != "retrieval unavailable" {
		t.Fatalf("GetTrace() spans = %#v", got.NodeSpans)
	}
}

func TestRepositoryGetTraceFound(t *testing.T) {
	createdAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	reader := &fakeTraceReader{
		row: fakeTraceRow{values: []any{"trace_1", "tenant_1", "realtime", int64(123), createdAt}},
		rows: &fakeTraceRows{rows: [][]any{
			{"span_1", "retrieve", 1, int64(12), "", createdAt, createdAt.Add(12 * time.Millisecond), createdAt.Add(time.Millisecond)},
			{"span_2", "generate", 2, int64(111), "llm timeout", createdAt, createdAt.Add(111 * time.Millisecond), createdAt.Add(2 * time.Millisecond)},
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

type fakeTraceReader struct {
	row          fakeTraceRow
	rows         *fakeTraceRows
	rowsErr      error
	rowsSQL      string
	queriedSpans bool
}

type fakeTraceDB struct {
	baseTime time.Time
	seq      int
	records  map[string]fakeStoredTrace
	spans    map[string][]TraceNodeSpan
}

type fakeStoredTrace struct {
	id        string
	tenantID  string
	profile   string
	latencyMS int64
	createdAt time.Time
}

type fakeTraceTx struct {
	db      *fakeTraceDB
	seq     int
	records map[string]fakeStoredTrace
	spans   map[string][]TraceNodeSpan
}

type fakeKnowledgeBaseQueryer struct {
	execErr   error
	execErrs  []error
	execTag   pgconn.CommandTag
	execCtx   context.Context
	execSQL   string
	execArgs  []any
	execCalls int
	queryRows pgx.Rows
	queryErr  error
	queryCtx  context.Context
	querySQL  string
	rowCtx    context.Context
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

func newFakeTraceDB(baseTime time.Time) *fakeTraceDB {
	return &fakeTraceDB{
		baseTime: baseTime,
		records:  make(map[string]fakeStoredTrace),
		spans:    make(map[string][]TraceNodeSpan),
	}
}

func (db *fakeTraceDB) BeginTraceTx(context.Context) (traceTx, error) {
	return &fakeTraceTx{
		db:      db,
		seq:     db.seq,
		records: cloneFakeTraceRecords(db.records),
		spans:   cloneFakeTraceSpans(db.spans),
	}, nil
}

func (db *fakeTraceDB) QueryRow(_ context.Context, sql string, args ...any) traceRow {
	if !strings.Contains(sql, "FROM rag_traces") {
		return fakeTraceRow{err: errors.New("unexpected trace row query")}
	}
	traceID, _ := args[0].(string)
	record, ok := db.records[traceID]
	if !ok {
		return fakeTraceRow{err: pgx.ErrNoRows}
	}
	return fakeTraceRow{values: []any{record.id, record.tenantID, record.profile, record.latencyMS, record.createdAt}}
}

func (db *fakeTraceDB) Query(_ context.Context, sql string, args ...any) (traceRows, error) {
	if !strings.Contains(sql, "FROM rag_node_spans") {
		return nil, errors.New("unexpected trace spans query")
	}
	traceID, _ := args[0].(string)
	rows := make([][]any, 0, len(db.spans[traceID]))
	for _, span := range db.spans[traceID] {
		rows = append(rows, []any{span.ID, span.NodeName, span.Sequence, span.LatencyMS, span.Error, span.StartedAt, span.EndedAt, span.CreatedAt})
	}
	return &fakeTraceRows{rows: rows}, nil
}

func (tx *fakeTraceTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	switch {
	case strings.Contains(sql, "INSERT INTO rag_traces"):
		traceID, _ := args[0].(string)
		tenantID, _ := args[1].(string)
		profile, _ := args[3].(string)
		latencyMS, _ := args[4].(int64)
		tx.records[traceID] = fakeStoredTrace{
			id:        traceID,
			tenantID:  tenantID,
			profile:   profile,
			latencyMS: latencyMS,
			createdAt: tx.nextTime(),
		}
		return pgconn.NewCommandTag("INSERT 1"), nil
	case strings.Contains(sql, "DELETE FROM rag_node_spans"):
		traceID, _ := args[0].(string)
		delete(tx.spans, traceID)
		return pgconn.NewCommandTag("DELETE 1"), nil
	case strings.Contains(sql, "INSERT INTO rag_node_spans"):
		traceID, _ := args[1].(string)
		spanID, _ := args[0].(string)
		nodeName, _ := args[2].(string)
		sequence, _ := args[3].(int)
		latencyMS, _ := args[4].(int64)
		spanErr, _ := args[5].(string)
		startedAt, _ := args[6].(time.Time)
		endedAt, _ := args[7].(time.Time)
		tx.spans[traceID] = append(tx.spans[traceID], TraceNodeSpan{
			ID:        spanID,
			NodeName:  nodeName,
			Sequence:  sequence,
			LatencyMS: latencyMS,
			Error:     spanErr,
			StartedAt: startedAt,
			EndedAt:   endedAt,
			CreatedAt: tx.nextTime(),
		})
		return pgconn.NewCommandTag("INSERT 1"), nil
	default:
		return pgconn.CommandTag{}, errors.New("unexpected trace exec")
	}
}

func (tx *fakeTraceTx) Commit(context.Context) error {
	tx.db.seq = tx.seq
	tx.db.records = tx.records
	tx.db.spans = tx.spans
	return nil
}

func (tx *fakeTraceTx) Rollback(context.Context) error {
	return nil
}

func (tx *fakeTraceTx) nextTime() time.Time {
	tx.seq++
	return tx.db.baseTime.Add(time.Duration(tx.seq) * time.Millisecond)
}

func cloneFakeTraceRecords(in map[string]fakeStoredTrace) map[string]fakeStoredTrace {
	out := make(map[string]fakeStoredTrace, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneFakeTraceSpans(in map[string][]TraceNodeSpan) map[string][]TraceNodeSpan {
	out := make(map[string][]TraceNodeSpan, len(in))
	for key, value := range in {
		out[key] = append([]TraceNodeSpan(nil), value...)
	}
	return out
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

func (f *fakeKnowledgeBaseQueryer) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.execCtx = ctx
	f.execSQL = sql
	f.execArgs = append([]any(nil), args...)
	err := f.execErr
	if f.execCalls < len(f.execErrs) {
		err = f.execErrs[f.execCalls]
	}
	f.execCalls++
	return f.execTag, err
}

func (f *fakeKnowledgeBaseQueryer) Query(ctx context.Context, sql string, _ ...any) (pgx.Rows, error) {
	f.queryCtx = ctx
	f.querySQL = sql
	return f.queryRows, f.queryErr
}

func (f *fakeKnowledgeBaseQueryer) QueryRow(ctx context.Context, _ string, _ ...any) pgx.Row {
	f.rowCtx = ctx
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
