package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shikanon/orag/internal/offlineknowledge"
)

var _ offlineknowledge.Repository = (*Repository)(nil)

func TestOfflineKnowledgeMigrationContainsRequiredSchema(t *testing.T) {
	body, err := os.ReadFile("../../../migrations/000015_offline_knowledge_optimization.sql")
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
	requiredUp := []string{
		"CREATE TABLE IF NOT EXISTS offline_knowledge_runs",
		"tenant_id TEXT NOT NULL",
		"CONSTRAINT offline_knowledge_runs_window_unique UNIQUE (tenant_id, kb_id, window_start, window_end, config_hash)",
		"CREATE TABLE IF NOT EXISTS offline_question_clusters",
		"CONSTRAINT offline_question_clusters_question_unique UNIQUE (tenant_id, kb_id, question_hash)",
		"long_tail BOOLEAN NOT NULL DEFAULT false",
		"CREATE TABLE IF NOT EXISTS optimization_items",
		"source_fingerprints_json JSONB NOT NULL DEFAULT '[]'::jsonb",
		"CREATE TABLE IF NOT EXISTS optimization_item_events",
		"CREATE TABLE IF NOT EXISTS offline_negative_feedback",
		"offline_negative_feedback_scope_created_idx",
		"offline_negative_feedback_trace_created_idx",
		"CREATE TABLE IF NOT EXISTS shadow_retrieval_events",
		"PRIMARY KEY (id, created_at)",
		") PARTITION BY RANGE (created_at)",
		"CREATE TABLE IF NOT EXISTS shadow_retrieval_events_default",
		"PARTITION OF shadow_retrieval_events DEFAULT",
		"CREATE TABLE IF NOT EXISTS offline_codex_tool_audit",
		"offline_codex_tool_audit_tenant_kb_started_idx",
		"offline_knowledge_runs_tenant_status_idx",
		"optimization_items_tenant_type_idx",
		"shadow_retrieval_events_trace_created_idx",
	}
	for _, want := range requiredUp {
		if !strings.Contains(up, want) {
			t.Fatalf("up migration missing %q: %s", want, up)
		}
	}
	for _, want := range []string{
		"DROP TABLE IF EXISTS shadow_retrieval_events_default",
		"DROP TABLE IF EXISTS shadow_retrieval_events",
		"DROP TABLE IF EXISTS offline_negative_feedback",
		"DROP TABLE IF EXISTS offline_codex_tool_audit",
		"DROP TABLE IF EXISTS optimization_item_events",
		"DROP TABLE IF EXISTS optimization_items",
		"DROP TABLE IF EXISTS offline_question_clusters",
		"DROP TABLE IF EXISTS offline_knowledge_runs",
	} {
		if !strings.Contains(down, want) {
			t.Fatalf("down migration missing %q: %s", want, down)
		}
	}
}

func TestOfflineKnowledgeRepositoryTenantIsolation(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	queryer := &offlineKnowledgeRecordingQueryer{
		row: fakeTraceRow{err: pgx.ErrNoRows},
		rows: &fakeTraceRows{rows: [][]any{
			offlineKnowledgeItemRow("item_1", "tenant_a", now),
		}},
	}
	repo := &Repository{evalQueryer: queryer}

	if _, found, err := repo.GetRun(context.Background(), "tenant_b", "run_1"); err != nil || found {
		t.Fatalf("GetRun() found=%v err=%v, want tenant-isolated miss", found, err)
	}
	if queryer.rowArgs[0] != "tenant_b" || !strings.Contains(queryer.rowSQL, "WHERE tenant_id=$1 AND id=$2") {
		t.Fatalf("GetRun tenant filter SQL=%s args=%#v", queryer.rowSQL, queryer.rowArgs)
	}

	got, err := repo.ListOptimizationItems(context.Background(), offlineknowledge.OptimizationItemFilter{
		TenantID: "tenant_a",
		KBID:     "kb_1",
		Status:   offlineknowledge.ItemStatusVerified,
		Limit:    10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TenantID != "tenant_a" {
		t.Fatalf("ListOptimizationItems() = %#v", got)
	}
	if queryer.queryArgs[0] != "tenant_a" || !strings.Contains(queryer.querySQL, "WHERE tenant_id=$1") {
		t.Fatalf("ListOptimizationItems tenant filter SQL=%s args=%#v", queryer.querySQL, queryer.queryArgs)
	}
	if !strings.Contains(queryer.querySQL, "AND kb_id=$2") || !strings.Contains(queryer.querySQL, "AND status=$3") {
		t.Fatalf("ListOptimizationItems missing scoped filters: %s", queryer.querySQL)
	}
}

func TestOfflineKnowledgeNegativeFeedbackRepositoryPersistsAndFilters(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 15, 0, 0, time.UTC)
	queryer := &offlineKnowledgeRecordingQueryer{}
	repo := &Repository{evalQueryer: queryer}
	item := offlineknowledge.NegativeFeedback{
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		TraceID:   "trace_1",
		Query:     "What is ORAG?",
		Reason:    "answer missed citation",
		CreatedAt: now,
	}
	if err := repo.AddNegativeFeedback(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "INSERT INTO offline_negative_feedback") {
		t.Fatalf("AddNegativeFeedback SQL = %s", queryer.execSQL)
	}
	if queryer.execArgs[1] != "tenant_1" || queryer.execArgs[2] != "kb_1" || queryer.execArgs[3] != "trace_1" {
		t.Fatalf("AddNegativeFeedback args = %#v, want tenant/kb/trace", queryer.execArgs)
	}
	if id, ok := queryer.execArgs[0].(string); !ok || !strings.HasPrefix(id, "negfb_") {
		t.Fatalf("AddNegativeFeedback generated id = %#v", queryer.execArgs[0])
	}

	queryer.rows = &fakeTraceRows{rows: [][]any{
		offlineKnowledgeNegativeFeedbackRow("feedback_1", "tenant_1", "kb_1", "trace_1", now),
	}}
	got, err := repo.ListNegativeFeedback(context.Background(), offlineknowledge.NegativeFeedbackFilter{
		TenantID: "tenant_1",
		KBID:     "kb_1",
		Since:    now.Add(-time.Hour),
		Until:    now.Add(time.Hour),
		TraceIDs: []string{"trace_1", "trace_2"},
		Limit:    5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "feedback_1" || got[0].Reason != "answer missed citation" {
		t.Fatalf("ListNegativeFeedback() = %#v", got)
	}
	for _, want := range []string{"WHERE tenant_id=$1", "AND kb_id=$2", "AND created_at >= $3", "AND created_at < $4", "AND (trace_id = ANY($5) OR trace_id = '')", "LIMIT $6"} {
		if !strings.Contains(queryer.querySQL, want) {
			t.Fatalf("ListNegativeFeedback SQL missing %q: %s", want, queryer.querySQL)
		}
	}
	if queryer.queryArgs[0] != "tenant_1" || queryer.queryArgs[1] != "kb_1" {
		t.Fatalf("ListNegativeFeedback args = %#v, want scoped tenant/kb", queryer.queryArgs)
	}
}

func TestOfflineKnowledgeCreateRunReturnsConflictForDuplicateWindow(t *testing.T) {
	duplicate := &pgconn.PgError{Code: "23505", ConstraintName: "offline_knowledge_runs_window_unique"}
	queryer := &offlineKnowledgeRecordingQueryer{execErr: duplicate}
	repo := &Repository{evalQueryer: queryer}
	windowStart := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)

	err := repo.CreateRun(context.Background(), offlineknowledge.OfflineKnowledgeRun{
		ID:          "run_1",
		TenantID:    "tenant_1",
		KBID:        "kb_1",
		Status:      offlineknowledge.RunStatusPending,
		WindowStart: windowStart,
		WindowEnd:   windowStart.Add(24 * time.Hour),
		ConfigHash:  "config_hash",
		ConfigJSON:  map[string]any{"lookback_days": float64(7), "mode": "nightly"},
		StartedAt:   windowStart.Add(25 * time.Hour),
	})
	if !errors.Is(err, ErrOfflineKnowledgeRunConflict) {
		t.Fatalf("CreateRun() error = %v, want ErrOfflineKnowledgeRunConflict", err)
	}
	if !errors.Is(err, offlineknowledge.ErrRunConflict) {
		t.Fatalf("CreateRun() error = %v, want offlineknowledge.ErrRunConflict", err)
	}
	if queryer.execArgs[1] != "tenant_1" || queryer.execArgs[2] != "kb_1" {
		t.Fatalf("CreateRun args = %#v, want tenant and kb scoped insert", queryer.execArgs)
	}
	if config := string(queryer.execArgs[7].([]byte)); !strings.Contains(config, "lookback_days") || !strings.Contains(config, "nightly") {
		t.Fatalf("CreateRun config_json arg = %s", config)
	}
}

func TestOfflineKnowledgeRunConfigJSONRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 30, 0, 0, time.UTC)
	queryer := &offlineKnowledgeRecordingQueryer{
		row: fakeTraceRow{values: offlineKnowledgeRunRow("run_1", "tenant_1", now)},
		rows: &fakeTraceRows{rows: [][]any{
			offlineKnowledgeRunRow("run_1", "tenant_1", now),
		}},
	}
	repo := &Repository{evalQueryer: queryer}

	got, found, err := repo.GetRun(context.Background(), "tenant_1", "run_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("GetRun() found = false, want true")
	}
	if got.ConfigJSON["lookback_days"] != float64(7) || got.ConfigJSON["mode"] != "nightly" {
		t.Fatalf("GetRun ConfigJSON = %#v", got.ConfigJSON)
	}
	if !strings.Contains(queryer.rowSQL, "config_json") {
		t.Fatalf("GetRun SQL does not select config_json: %s", queryer.rowSQL)
	}

	list, err := repo.ListRuns(context.Background(), offlineknowledge.RunFilter{TenantID: "tenant_1", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ConfigJSON["mode"] != "nightly" {
		t.Fatalf("ListRuns() = %#v", list)
	}
	if !strings.Contains(queryer.querySQL, "config_json") {
		t.Fatalf("ListRuns SQL does not select config_json: %s", queryer.querySQL)
	}

	queryer.execTag = pgconn.NewCommandTag("UPDATE 1")
	updated, err := repo.UpdateRun(context.Background(), offlineknowledge.OfflineKnowledgeRun{
		ID:          "run_1",
		TenantID:    "tenant_1",
		KBID:        "kb_1",
		Status:      offlineknowledge.RunStatusCompleted,
		WindowStart: now.Add(-24 * time.Hour),
		WindowEnd:   now,
		ConfigHash:  "config_hash",
		ConfigJSON:  map[string]any{"lookback_days": float64(7), "mode": "nightly"},
		StartedAt:   now.Add(-time.Hour),
		FinishedAt:  now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated || !strings.Contains(queryer.execSQL, "UPDATE offline_knowledge_runs") {
		t.Fatalf("UpdateRun updated=%v SQL=%s", updated, queryer.execSQL)
	}
}

func TestOfflineKnowledgeItemCRUDStatusAndEvents(t *testing.T) {
	now := time.Date(2026, 7, 8, 11, 0, 0, 0, time.UTC)
	queryer := &offlineKnowledgeRecordingQueryer{execTag: pgconn.NewCommandTag("UPDATE 1")}
	repo := &Repository{evalQueryer: queryer}
	item := offlineknowledge.OptimizationItem{
		ID:                "item_1",
		TenantID:          "tenant_1",
		RunID:             "run_1",
		KBID:              "kb_1",
		QuestionClusterID: "cluster_1",
		ItemType:          offlineknowledge.ItemTypeAnswer,
		Status:            offlineknowledge.ItemStatusCandidate,
		CanonicalQuestion: "What is ORAG?",
		FinalAnswer:       "A retrieval framework.",
		RecallQuality:     offlineknowledge.RecallQualityMiss,
		FailureType:       offlineknowledge.FailureTypeSemanticGap,
		Confidence:        0.88,
		SourceFingerprints: []offlineknowledge.SourceFingerprint{
			{DocID: "doc_1", DocVersion: "v1", ChunkID: "chunk_1", ChunkContentHash: "sha256:a"},
			{DocID: "doc_1", DocVersion: "v1", ChunkID: "chunk_2", ChunkContentHash: "sha256:b"},
		},
		Evidence:        []offlineknowledge.Evidence{{ChunkID: "chunk_1", DocID: "doc_1", Quote: "ORAG", Supports: "definition"}},
		DeepSearchSteps: []offlineknowledge.DeepSearchStep{{Step: 1, Tool: "text_search", Query: "ORAG", Observation: "hit", Decision: "keep"}},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := repo.CreateOptimizationItem(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "INSERT INTO optimization_items") {
		t.Fatalf("CreateOptimizationItem SQL = %s", queryer.execSQL)
	}
	var chunkIDs []string
	if err := json.Unmarshal(queryer.execArgs[12].([]byte), &chunkIDs); err != nil {
		t.Fatal(err)
	}
	if len(chunkIDs) != 2 || chunkIDs[0] != "chunk_1" || chunkIDs[1] != "chunk_2" {
		t.Fatalf("source chunk ids = %#v", chunkIDs)
	}
	var fingerprints []offlineknowledge.SourceFingerprint
	if err := json.Unmarshal(queryer.execArgs[14].([]byte), &fingerprints); err != nil {
		t.Fatal(err)
	}
	if len(fingerprints) != 2 || fingerprints[0].ChunkContentHash != "sha256:a" {
		t.Fatalf("source fingerprints = %#v", fingerprints)
	}

	queryer.row = fakeTraceRow{values: offlineKnowledgeItemRow("item_1", "tenant_1", now)}
	got, found, err := repo.GetOptimizationItem(context.Background(), "tenant_1", "item_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || got.SourceFingerprints[0].DocID != "doc_1" || got.Evidence[0].Quote != "quote" {
		t.Fatalf("GetOptimizationItem() = %#v found=%v", got, found)
	}

	updated, err := repo.UpdateOptimizationItemStatus(context.Background(), "tenant_1", "item_1", offlineknowledge.ItemStatusVerified, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !updated || queryer.execArgs[0] != "tenant_1" || queryer.execArgs[2] != offlineknowledge.ItemStatusVerified {
		t.Fatalf("UpdateOptimizationItemStatus updated=%v args=%#v", updated, queryer.execArgs)
	}

	updatedItem, err := repo.UpdateOptimizationItem(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if !updatedItem || !strings.Contains(queryer.execSQL, "UPDATE optimization_items") {
		t.Fatalf("UpdateOptimizationItem updated=%v SQL=%s", updatedItem, queryer.execSQL)
	}

	if err := repo.AppendItemEvent(context.Background(), offlineknowledge.OptimizationItemEvent{
		ID:        "event_1",
		TenantID:  "tenant_1",
		ItemID:    "item_1",
		EventType: "status_changed",
		Operator:  "system",
		Payload:   map[string]any{"status": string(offlineknowledge.ItemStatusVerified)},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "INSERT INTO optimization_item_events") || queryer.execArgs[1] != "tenant_1" {
		t.Fatalf("AppendItemEvent SQL=%s args=%#v", queryer.execSQL, queryer.execArgs)
	}
	payload := string(queryer.execArgs[5].([]byte))
	if !strings.Contains(payload, "verified") {
		t.Fatalf("AppendItemEvent payload = %s", payload)
	}

	queryer.rows = &fakeTraceRows{rows: [][]any{
		offlineKnowledgeItemEventRow("event_1", "tenant_1", now),
	}}
	events, err := repo.ListItemEvents(context.Background(), offlineknowledge.OptimizationItemEventFilter{
		TenantID:  "tenant_1",
		ItemID:    "item_1",
		EventType: "status_changed",
		Since:     now.Add(-time.Hour),
		Until:     now.Add(time.Hour),
		Limit:     5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Payload["status"] != "verified" {
		t.Fatalf("ListItemEvents() = %#v", events)
	}
	for _, want := range []string{"WHERE tenant_id=$1", "AND item_id=$2", "AND event_type=$3", "AND created_at >= $4", "AND created_at < $5", "LIMIT $6"} {
		if !strings.Contains(queryer.querySQL, want) {
			t.Fatalf("ListItemEvents SQL missing %q: %s", want, queryer.querySQL)
		}
	}
}

func TestOfflineKnowledgeItemEvalReportJSONRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 8, 11, 30, 0, 0, time.UTC)
	evalReport := []byte(`{"passed":true,"reasons":["ok"],"full_dataset_used":true}`)
	updatedReport := []byte(`{"passed":false,"reasons":["latency_delta_above_threshold"],"full_dataset_used":true}`)
	queryer := &offlineKnowledgeRecordingQueryer{execTag: pgconn.NewCommandTag("UPDATE 1")}
	repo := &Repository{evalQueryer: queryer}
	item := offlineKnowledgeEvalReportItem("item_eval_report", now, evalReport)

	if err := repo.CreateOptimizationItem(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if got := compactJSONArg(t, queryer.execArgs[19]); got != compactJSON(t, evalReport) {
		t.Fatalf("CreateOptimizationItem eval_report_json = %s, want %s", got, compactJSON(t, evalReport))
	}

	queryer.row = fakeTraceRow{values: offlineKnowledgeItemRowWithEval("item_eval_report", "tenant_1", now, evalReport)}
	got, found, err := repo.GetOptimizationItem(context.Background(), "tenant_1", "item_eval_report")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("GetOptimizationItem() found = false, want true")
	}
	if string(got.EvalReportJSON) != string(evalReport) {
		t.Fatalf("GetOptimizationItem EvalReportJSON = %s, want %s", got.EvalReportJSON, evalReport)
	}
	if !strings.Contains(queryer.rowSQL, "eval_report_json") {
		t.Fatalf("GetOptimizationItem SQL does not select eval_report_json: %s", queryer.rowSQL)
	}

	queryer.rows = &fakeTraceRows{rows: [][]any{
		offlineKnowledgeItemRowWithEval("item_eval_report", "tenant_1", now, evalReport),
	}}
	list, err := repo.ListOptimizationItems(context.Background(), offlineknowledge.OptimizationItemFilter{
		TenantID: "tenant_1",
		KBID:     "kb_1",
		Limit:    5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || string(list[0].EvalReportJSON) != string(evalReport) {
		t.Fatalf("ListOptimizationItems() = %#v, want eval report round-trip", list)
	}
	if !strings.Contains(queryer.querySQL, "eval_report_json") {
		t.Fatalf("ListOptimizationItems SQL does not select eval_report_json: %s", queryer.querySQL)
	}

	item.EvalReportJSON = updatedReport
	updated, err := repo.UpdateOptimizationItem(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("UpdateOptimizationItem() updated = false, want true")
	}
	if !strings.Contains(queryer.execSQL, "eval_report_json=$18") {
		t.Fatalf("UpdateOptimizationItem SQL does not update eval_report_json: %s", queryer.execSQL)
	}
	if got := compactJSONArg(t, queryer.execArgs[17]); got != compactJSON(t, updatedReport) {
		t.Fatalf("UpdateOptimizationItem eval_report_json = %s, want %s", got, compactJSON(t, updatedReport))
	}
}

func TestOfflineKnowledgeItemEmptyEvalReportJSONDefaultsToObject(t *testing.T) {
	now := time.Date(2026, 7, 8, 11, 45, 0, 0, time.UTC)
	queryer := &offlineKnowledgeRecordingQueryer{execTag: pgconn.NewCommandTag("UPDATE 1")}
	repo := &Repository{evalQueryer: queryer}
	item := offlineKnowledgeEvalReportItem("item_empty_eval_report", now, nil)

	if err := repo.CreateOptimizationItem(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if got := compactJSONArg(t, queryer.execArgs[19]); got != "{}" {
		t.Fatalf("CreateOptimizationItem empty eval_report_json = %s, want {}", got)
	}

	updated, err := repo.UpdateOptimizationItem(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("UpdateOptimizationItem() updated = false, want true")
	}
	if got := compactJSONArg(t, queryer.execArgs[17]); got != "{}" {
		t.Fatalf("UpdateOptimizationItem empty eval_report_json = %s, want {}", got)
	}
}

func TestOfflineKnowledgeQuestionClusterAndShadowEvent(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	queryer := &offlineKnowledgeRecordingQueryer{}
	repo := &Repository{evalQueryer: queryer}

	if err := repo.UpsertQuestionCluster(context.Background(), offlineknowledge.QuestionCluster{
		ID:                 "cluster_1",
		TenantID:           "tenant_1",
		RunID:              "run_1",
		KBID:               "kb_1",
		CanonicalQuestion:  "How does ORAG optimize retrieval?",
		NormalizedQuestion: "how does orag optimize retrieval",
		QuestionHash:       "qhash",
		EmbeddingRef:       "embedding_1",
		EmbeddingJSON:      []float64{0.1, 0.2},
		OccurrenceCount:    3,
		SampleQuestions:    []string{"How does ORAG optimize retrieval?"},
		TraceIDs:           []string{"trace_1"},
		LongTail:           true,
		CreatedAt:          now,
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "ON CONFLICT (tenant_id, kb_id, question_hash)") || !strings.Contains(queryer.execSQL, "long_tail") {
		t.Fatalf("UpsertQuestionCluster SQL = %s", queryer.execSQL)
	}
	if queryer.execArgs[1] != "tenant_1" || queryer.execArgs[3] != "kb_1" {
		t.Fatalf("UpsertQuestionCluster args = %#v", queryer.execArgs)
	}

	queryer.row = fakeTraceRow{values: offlineKnowledgeQuestionClusterRow("cluster_1", "tenant_1", now)}
	cluster, found, err := repo.GetQuestionCluster(context.Background(), "tenant_1", "cluster_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || cluster.SampleQuestions[0] != "How does ORAG optimize retrieval?" || cluster.TraceIDs[0] != "trace_1" || !cluster.LongTail {
		t.Fatalf("GetQuestionCluster() = %#v found=%v", cluster, found)
	}
	queryer.rows = &fakeTraceRows{rows: [][]any{
		offlineKnowledgeQuestionClusterRow("cluster_1", "tenant_1", now),
	}}
	clusters, err := repo.ListQuestionClusters(context.Background(), offlineknowledge.QuestionClusterFilter{
		TenantID:     "tenant_1",
		KBID:         "kb_1",
		RunID:        "run_1",
		QuestionHash: "qhash",
		Limit:        5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 || clusters[0].QuestionHash != "qhash" {
		t.Fatalf("ListQuestionClusters() = %#v", clusters)
	}
	for _, want := range []string{"WHERE tenant_id=$1", "AND kb_id=$2", "AND run_id=$3", "AND question_hash=$4", "LIMIT $5"} {
		if !strings.Contains(queryer.querySQL, want) {
			t.Fatalf("ListQuestionClusters SQL missing %q: %s", want, queryer.querySQL)
		}
	}

	if err := repo.RecordShadowEvent(context.Background(), offlineknowledge.ShadowRetrievalEvent{
		ID:                "shadow_1",
		TenantID:          "tenant_1",
		KBID:              "kb_1",
		ItemID:            "item_1",
		TraceID:           "trace_1",
		Query:             "query",
		Matched:           true,
		Injected:          true,
		Rank:              1,
		Score:             0.91,
		RecallLift:        0.2,
		AnswerLift:        0.3,
		HallucinationRisk: 0.01,
		CreatedAt:         now,
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "INSERT INTO shadow_retrieval_events") || queryer.execArgs[1] != "tenant_1" {
		t.Fatalf("RecordShadowEvent SQL=%s args=%#v", queryer.execSQL, queryer.execArgs)
	}

	queryer.rows = &fakeTraceRows{rows: [][]any{
		{
			"shadow_1", "tenant_1", "kb_1", "item_1", "trace_1", "query",
			true, true, 1, 0.91, 0.2, 0.3, 0.01, now,
		},
	}}
	got, err := repo.ListShadowEvents(context.Background(), offlineknowledge.ShadowRetrievalEventFilter{
		TenantID: "tenant_1",
		KBID:     "kb_1",
		ItemID:   "item_1",
		TraceID:  "trace_1",
		Since:    now.Add(-time.Hour),
		Until:    now.Add(time.Hour),
		Limit:    5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Matched || got[0].ItemID != "item_1" {
		t.Fatalf("ListShadowEvents() = %#v", got)
	}
	for _, want := range []string{"WHERE tenant_id=$1", "AND kb_id=$2", "AND trace_id=$3", "AND optimization_item_id=$4", "AND created_at >= $5", "AND created_at < $6", "LIMIT $7"} {
		if !strings.Contains(queryer.querySQL, want) {
			t.Fatalf("ListShadowEvents SQL missing %q: %s", want, queryer.querySQL)
		}
	}
}

func TestOfflineKnowledgeCodexToolAuditRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 30, 0, 0, time.UTC)
	queryer := &offlineKnowledgeRecordingQueryer{}
	repo := &Repository{evalQueryer: queryer}

	err := repo.RecordCodexToolAudit(context.Background(), offlineknowledge.CodexToolAuditEvent{
		ID:         "audit_1",
		SessionID:  "session_1",
		TenantID:   "tenant_1",
		KBID:       "kb_1",
		Tool:       offlineknowledge.ReadOnlyToolSearchChunksByText,
		Rows:       3,
		Steps:      2,
		Allowed:    true,
		StartedAt:  now,
		FinishedAt: now.Add(10 * time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "INSERT INTO offline_codex_tool_audit") || queryer.execArgs[1] != "tenant_1" || queryer.execArgs[4] != offlineknowledge.ReadOnlyToolSearchChunksByText {
		t.Fatalf("RecordCodexToolAudit SQL=%s args=%#v", queryer.execSQL, queryer.execArgs)
	}

	queryer.rows = &fakeTraceRows{rows: [][]any{
		{
			"audit_1", "tenant_1", "kb_1", "session_1", offlineknowledge.ReadOnlyToolSearchChunksByText,
			3, 2, true, "", now, now.Add(10 * time.Millisecond),
		},
	}}
	got, err := repo.ListCodexToolAuditEvents(context.Background(), offlineknowledge.CodexToolAuditFilter{
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		SessionID: "session_1",
		Tool:      offlineknowledge.ReadOnlyToolSearchChunksByText,
		Since:     now.Add(-time.Hour),
		Until:     now.Add(time.Hour),
		Limit:     5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "audit_1" || !got[0].Allowed || got[0].Rows != 3 || got[0].Steps != 2 {
		t.Fatalf("ListCodexToolAuditEvents() = %#v", got)
	}
	for _, want := range []string{"WHERE tenant_id=$1", "AND kb_id=$2", "AND session_id=$3", "AND tool=$4", "AND started_at >= $5", "AND started_at < $6", "LIMIT $7"} {
		if !strings.Contains(queryer.querySQL, want) {
			t.Fatalf("ListCodexToolAuditEvents SQL missing %q: %s", want, queryer.querySQL)
		}
	}
}

type offlineKnowledgeRecordingQueryer struct {
	execTag  pgconn.CommandTag
	execErr  error
	execSQL  string
	execArgs []any

	rows      pgx.Rows
	queryErr  error
	querySQL  string
	queryArgs []any

	row     pgx.Row
	rowSQL  string
	rowArgs []any
}

func (q *offlineKnowledgeRecordingQueryer) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	q.execSQL = sql
	q.execArgs = append([]any(nil), args...)
	return q.execTag, q.execErr
}

func (q *offlineKnowledgeRecordingQueryer) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	q.querySQL = sql
	q.queryArgs = append([]any(nil), args...)
	return q.rows, q.queryErr
}

func (q *offlineKnowledgeRecordingQueryer) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	q.rowSQL = sql
	q.rowArgs = append([]any(nil), args...)
	if q.row == nil {
		return fakeTraceRow{err: pgx.ErrNoRows}
	}
	return q.row
}

func offlineKnowledgeItemRow(id, tenantID string, now time.Time) []any {
	return offlineKnowledgeItemRowWithEval(id, tenantID, now, []byte(`{"passed":true,"reasons":["ok"]}`))
}

func offlineKnowledgeItemRowWithEval(id, tenantID string, now time.Time, evalReport []byte) []any {
	return []any{
		id,
		tenantID,
		"run_1",
		"kb_1",
		"cluster_1",
		offlineknowledge.ItemTypeAnswer,
		offlineknowledge.ItemStatusVerified,
		"What is ORAG?",
		"answer",
		offlineknowledge.RecallQualityMiss,
		offlineknowledge.FailureTypeSemanticGap,
		0.91,
		[]byte(`[{"doc_id":"doc_1","doc_version":"v1","chunk_id":"chunk_1","chunk_content_hash":"sha256:a"}]`),
		[]byte(`[{"chunk_id":"chunk_1","doc_id":"doc_1","quote":"quote","supports":"claim"}]`),
		[]byte(`[{"step":1,"tool":"text_search","query":"orag","observation":"hit","decision":"keep"}]`),
		evalReport,
		now,
		now,
		sql.NullTime{},
	}
}

func offlineKnowledgeEvalReportItem(id string, now time.Time, evalReport []byte) offlineknowledge.OptimizationItem {
	return offlineknowledge.OptimizationItem{
		ID:                id,
		TenantID:          "tenant_1",
		RunID:             "run_1",
		KBID:              "kb_1",
		QuestionClusterID: "cluster_1",
		ItemType:          offlineknowledge.ItemTypeAnswer,
		Status:            offlineknowledge.ItemStatusRegressionPassed,
		CanonicalQuestion: "What is ORAG?",
		FinalAnswer:       "A retrieval framework.",
		RecallQuality:     offlineknowledge.RecallQualityMiss,
		FailureType:       offlineknowledge.FailureTypeSemanticGap,
		Confidence:        0.88,
		SourceFingerprints: []offlineknowledge.SourceFingerprint{
			{DocID: "doc_1", DocVersion: "v1", ChunkID: "chunk_1", ChunkContentHash: "sha256:a"},
		},
		Evidence:        []offlineknowledge.Evidence{{ChunkID: "chunk_1", DocID: "doc_1", Quote: "ORAG", Supports: "definition"}},
		DeepSearchSteps: []offlineknowledge.DeepSearchStep{{Step: 1, Tool: "text_search", Query: "ORAG", Observation: "hit", Decision: "keep"}},
		EvalReportJSON:  evalReport,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func compactJSONArg(t *testing.T, value any) string {
	t.Helper()
	data, ok := value.([]byte)
	if !ok {
		t.Fatalf("JSON arg type = %T, want []byte", value)
	}
	return compactJSON(t, data)
}

func compactJSON(t *testing.T, data []byte) string {
	t.Helper()
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal JSON %s: %v", data, err)
	}
	out, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("marshal compact JSON: %v", err)
	}
	return string(out)
}

func offlineKnowledgeQuestionClusterRow(id, tenantID string, now time.Time) []any {
	return []any{
		id,
		tenantID,
		"run_1",
		"kb_1",
		"How does ORAG optimize retrieval?",
		"how does orag optimize retrieval",
		"qhash",
		"embedding_1",
		[]byte(`[0.1,0.2]`),
		3,
		[]byte(`["How does ORAG optimize retrieval?"]`),
		[]byte(`["trace_1"]`),
		true,
		now,
	}
}

func offlineKnowledgeItemEventRow(id, tenantID string, now time.Time) []any {
	return []any{
		id,
		tenantID,
		"item_1",
		"status_changed",
		"system",
		[]byte(`{"status":"verified"}`),
		now,
	}
}

func offlineKnowledgeNegativeFeedbackRow(id, tenantID, kbID, traceID string, now time.Time) []any {
	return []any{
		id,
		tenantID,
		kbID,
		traceID,
		"What is ORAG?",
		"answer missed citation",
		now,
	}
}

func offlineKnowledgeRunRow(id, tenantID string, now time.Time) []any {
	return []any{
		id,
		tenantID,
		"kb_1",
		offlineknowledge.RunStatusPending,
		now.Add(-24 * time.Hour),
		now,
		"config_hash",
		[]byte(`{"lookback_days":7,"mode":"nightly"}`),
		"",
		now,
		sql.NullTime{},
	}
}
