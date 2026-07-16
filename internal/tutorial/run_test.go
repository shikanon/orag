package tutorial

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/ingest/chunker"
	"github.com/shikanon/orag/internal/kb"
)

func TestLiveRunIndexesPrivatePackAndDelegatesEvaluation(t *testing.T) {
	now := time.Date(2026, 7, 16, 13, 0, 0, 0, time.UTC)
	content := []byte("tutorial baseline corpus")
	hash := sha256.Sum256(content)
	object := PackObject{Path: "corpus/data.txt", SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(content)), ContentType: "text/plain"}
	store := installPrivateObject(t, "tenant_a", "prj_1", "tclj_1", object, content)
	repo := NewMemoryCloneRepository()
	experiment := Experiment{
		ID: "texp_1", TenantID: "tenant_a", ProjectID: "prj_1", CloneJobID: "tclj_1",
		TemplateID: "text-rag", TemplateVersion: "1.0.0", Tier: "quick", PackStatus: PackStatusInstalled,
		RuntimeStatus: "ready", KnowledgeBaseID: "tkb_1", DatasetID: "tds_1", BaselineProfile: "realtime", BaselineTopK: 5,
		PackManifest: Manifest{Objects: []PackObject{object}, Runtime: &RuntimeManifest{
			Baseline: RuntimeBaseline{Profile: "realtime", TopK: 5}, Documents: []RuntimeDocument{{ObjectPath: object.Path, Name: "教程语料"}},
			Dataset: RuntimeDataset{Name: "评测", Items: []RuntimeDatasetItem{{Query: "问题", GroundTruth: "答案"}}},
		}},
	}
	if err := repo.EnsureExperiment(context.Background(), experiment); err != nil {
		t.Fatal(err)
	}
	ingestor := &recordingRuntimeIngestor{}
	evaluator := &recordingRuntimeEvaluator{}
	service := NewLiveRunService(repo, repo, func() time.Time { return now })
	service.Configure(ingestor, evaluator, store)
	subject := Subject{TenantID: "tenant_a", ID: "user_a"}
	run, replayed, err := service.Start(context.Background(), subject, experiment.ProjectID, "run_1")
	if err != nil || replayed || run.Status != ExperimentRunQueued || run.Stage != ExperimentRunStageIndex {
		t.Fatalf("run=%#v replayed=%v err=%v", run, replayed, err)
	}
	duplicate, replayed, err := service.Start(context.Background(), subject, experiment.ProjectID, "run_1")
	if err != nil || !replayed || duplicate.ID != run.ID {
		t.Fatalf("duplicate=%#v replayed=%v err=%v", duplicate, replayed, err)
	}
	if err := service.Execute(context.Background(), subject.TenantID, run.ID); err != nil {
		t.Fatal(err)
	}
	completed, err := service.Get(context.Background(), subject, run.ID)
	if err != nil || completed.Status != ExperimentRunCompleted || completed.Stage != ExperimentRunStageComplete || completed.EvaluationRunID != "eval_1" {
		t.Fatalf("completed=%#v err=%v", completed, err)
	}
	if len(ingestor.requests) != 1 || string(ingestor.requests[0].Content) != string(content) || ingestor.requests[0].KnowledgeBaseID != experiment.KnowledgeBaseID {
		t.Fatalf("ingest requests=%#v", ingestor.requests)
	}
	if evaluator.request.DatasetID != experiment.DatasetID || evaluator.request.KnowledgeBaseID != experiment.KnowledgeBaseID || evaluator.request.TopK != 5 {
		t.Fatalf("evaluation request=%#v", evaluator.request)
	}
}

func TestLiveRunRequiresCompatibleBaselineAndUsesIndependentCandidateIndexes(t *testing.T) {
	now := time.Date(2026, 7, 16, 14, 0, 0, 0, time.UTC)
	content := []byte(`{"service":{"port":8080,"name":"ORAG"}}`)
	hash := sha256.Sum256(content)
	object := PackObject{Path: "corpus/service.json", SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(content)), ContentType: "application/json"}
	store := installPrivateObject(t, "tenant_a", "prj_1", "tclj_1", object, content)
	repo := NewMemoryCloneRepository()
	experiment := Experiment{
		ID: "texp_1", TenantID: "tenant_a", ProjectID: "prj_1", CloneJobID: "tclj_1",
		TemplateID: "text-rag", TemplateVersion: "1.0.0", Tier: "quick", PackStatus: PackStatusInstalled,
		RuntimeStatus: "ready", KnowledgeBaseID: "tkb_baseline", DatasetID: "tds_1", BaselineProfile: "realtime", BaselineTopK: 5,
		PackManifest: Manifest{Objects: []PackObject{object}, Runtime: &RuntimeManifest{
			Baseline: RuntimeBaseline{Profile: "realtime", TopK: 5}, Documents: []RuntimeDocument{{ObjectPath: object.Path, Name: "服务配置"}},
			Dataset: RuntimeDataset{Name: "评测", Items: []RuntimeDatasetItem{{Query: "端口", GroundTruth: "8080"}}},
			Candidates: []RuntimeCandidate{
				{ID: TutorialP1StructuredJSONCandidateID, Chapter: TutorialP1DocumentParserChapter, ParserMethod: TutorialStructuredJSONParserMethod},
				{ID: TutorialP2RecursiveChunkCandidateID, Chapter: TutorialP2ChunkingChapter, ParserMethod: "basic", ChunkSizeTokens: TutorialP2ChunkSizeTokens, ChunkOverlapTokens: TutorialP2ChunkOverlapTokens},
				{ID: TutorialP3ContextualCandidateID, Chapter: TutorialP3ContextualChapter, ParserMethod: "basic", ChunkSizeTokens: TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens, ContextualRetrieval: true},
			},
		}},
	}
	if err := repo.EnsureExperiment(context.Background(), experiment); err != nil {
		t.Fatal(err)
	}
	baselineIngestor := &recordingRuntimeIngestor{}
	p1Ingestor := &recordingRuntimeIngestor{}
	p2Ingestor := &recordingRuntimeIngestor{}
	p3Ingestor := &recordingRuntimeIngestor{chunks: []kb.Chunk{{Content: "service port configuration", ContextualText: "The document describes the ORAG service port."}}}
	evaluator := &recordingRuntimeEvaluator{}
	service := NewLiveRunService(repo, repo, func() time.Time { return now })
	service.Configure(baselineIngestor, evaluator, store)
	service.ConfigureCandidateIngestors(RuntimeEnvironment{ChatModel: "chat", EmbeddingModel: "embed", RerankModel: "rerank", MultimodalModel: "vision", PromptCacheMode: "auto", EvaluatorVersion: "standard_eval_v1"}, map[string]RuntimeIngestor{
		TutorialP1StructuredJSONCandidateID: p1Ingestor,
		TutorialP2RecursiveChunkCandidateID: p2Ingestor,
		TutorialP3ContextualCandidateID:     p3Ingestor,
	})
	subject := Subject{TenantID: "tenant_a", ID: "user_a"}
	if _, _, err := service.StartVariant(context.Background(), subject, experiment.ProjectID, TutorialP1StructuredJSONCandidateID, "p1-before-p0"); err != ErrBaselineRequired {
		t.Fatalf("P1 before P0 error=%v", err)
	}
	if _, _, err := service.StartVariant(context.Background(), subject, experiment.ProjectID, TutorialP2RecursiveChunkCandidateID, "p2-before-p0"); err != ErrBaselineRequired {
		t.Fatalf("P2 before P0 error=%v", err)
	}
	if _, _, err := service.StartVariant(context.Background(), subject, experiment.ProjectID, TutorialP3ContextualCandidateID, "p3-before-p0"); err != ErrBaselineRequired {
		t.Fatalf("P3 before P0 error=%v", err)
	}
	baseline, _, err := service.StartVariant(context.Background(), subject, experiment.ProjectID, "baseline", "p0")
	if err != nil {
		t.Fatal(err)
	}
	if baseline.ParserMethod != "basic" || baseline.ChunkSizeTokens != TutorialBaselineChunkSizeTokens || baseline.ChunkOverlapTokens != TutorialBaselineChunkOverlapTokens {
		t.Fatalf("baseline audit fields=%#v", baseline)
	}
	if err := service.Execute(context.Background(), subject.TenantID, baseline.ID); err != nil {
		t.Fatal(err)
	}
	candidate, replayed, err := service.StartVariant(context.Background(), subject, experiment.ProjectID, TutorialP1StructuredJSONCandidateID, "p1")
	if err != nil || replayed {
		t.Fatalf("candidate=%#v replayed=%v err=%v", candidate, replayed, err)
	}
	if candidate.BaselineRunID != baseline.ID || candidate.KnowledgeBaseID == baseline.KnowledgeBaseID || candidate.ParserMethod != TutorialStructuredJSONParserMethod || candidate.ChunkSizeTokens != TutorialBaselineChunkSizeTokens || candidate.ChunkOverlapTokens != TutorialBaselineChunkOverlapTokens || candidate.ComparisonFingerprint == "" || candidate.DefinitionFingerprint == "" {
		t.Fatalf("candidate audit fields=%#v", candidate)
	}
	if err := service.Execute(context.Background(), subject.TenantID, candidate.ID); err != nil {
		t.Fatal(err)
	}
	if len(baselineIngestor.requests) != 1 || baselineIngestor.requests[0].KnowledgeBaseID != baseline.KnowledgeBaseID {
		t.Fatalf("baseline ingest=%#v", baselineIngestor.requests)
	}
	if len(p1Ingestor.requests) != 1 || p1Ingestor.requests[0].KnowledgeBaseID != candidate.KnowledgeBaseID {
		t.Fatalf("P1 ingest=%#v", p1Ingestor.requests)
	}
	p2, replayed, err := service.StartVariant(context.Background(), subject, experiment.ProjectID, TutorialP2RecursiveChunkCandidateID, "p2")
	if err != nil || replayed {
		t.Fatalf("P2=%#v replayed=%v err=%v", p2, replayed, err)
	}
	if p2.BaselineRunID != baseline.ID || p2.KnowledgeBaseID == baseline.KnowledgeBaseID || p2.KnowledgeBaseID == candidate.KnowledgeBaseID || p2.ParserMethod != "basic" || p2.ChunkSizeTokens != TutorialP2ChunkSizeTokens || p2.ChunkOverlapTokens != TutorialP2ChunkOverlapTokens || p2.ComparisonFingerprint != baseline.ComparisonFingerprint {
		t.Fatalf("P2 audit fields=%#v", p2)
	}
	if err := service.Execute(context.Background(), subject.TenantID, p2.ID); err != nil {
		t.Fatal(err)
	}
	if len(p2Ingestor.requests) != 1 || p2Ingestor.requests[0].KnowledgeBaseID != p2.KnowledgeBaseID {
		t.Fatalf("P2 ingest=%#v", p2Ingestor.requests)
	}
	completedP2, err := service.Get(context.Background(), subject, p2.ID)
	if err != nil || completedP2.IndexedChunkCount != 1 || completedP2.AverageChunkTokens <= 0 {
		t.Fatalf("P2 index stats=%#v err=%v", completedP2, err)
	}
	p3, replayed, err := service.StartVariant(context.Background(), subject, experiment.ProjectID, TutorialP3ContextualCandidateID, "p3")
	if err != nil || replayed || !p3.ContextualRetrievalEnabled || p3.BaselineRunID != baseline.ID || p3.KnowledgeBaseID == baseline.KnowledgeBaseID || p3.ParserMethod != "basic" || p3.ChunkSizeTokens != TutorialBaselineChunkSizeTokens || p3.ChunkOverlapTokens != TutorialBaselineChunkOverlapTokens {
		t.Fatalf("P3=%#v replayed=%v err=%v", p3, replayed, err)
	}
	if err := service.Execute(context.Background(), subject.TenantID, p3.ID); err != nil {
		t.Fatal(err)
	}
	completedP3, err := service.Get(context.Background(), subject, p3.ID)
	if err != nil || completedP3.ContextualizedChunkCount != 1 || completedP3.AverageContextTokens != float64(chunker.TokenCount("The document describes the ORAG service port.")) {
		t.Fatalf("P3 contextual stats=%#v err=%v", completedP3, err)
	}
}

func TestLiveRunRejectsComparisonFingerprintMismatch(t *testing.T) {
	now := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	content := []byte(`{"service":{"port":8080}}`)
	hash := sha256.Sum256(content)
	object := PackObject{Path: "corpus/service.json", SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(content)), ContentType: "application/json"}
	repo := NewMemoryCloneRepository()
	experiment := Experiment{
		ID: "texp_1", TenantID: "tenant_a", ProjectID: "prj_1", CloneJobID: "tclj_1", TemplateID: "text-rag", TemplateVersion: "1.0.0", Tier: "quick",
		PackStatus: PackStatusInstalled, RuntimeStatus: "ready", KnowledgeBaseID: "tkb_baseline", DatasetID: "tds_1", BaselineProfile: "realtime", BaselineTopK: 5,
		PackManifest: Manifest{Objects: []PackObject{object}, Runtime: &RuntimeManifest{
			Baseline: RuntimeBaseline{Profile: "realtime", TopK: 5}, Documents: []RuntimeDocument{{ObjectPath: object.Path, Name: "服务配置"}},
			Dataset:    RuntimeDataset{Name: "评测", Items: []RuntimeDatasetItem{{Query: "端口", GroundTruth: "8080"}}},
			Candidates: []RuntimeCandidate{{ID: TutorialP1StructuredJSONCandidateID, Chapter: TutorialP1DocumentParserChapter, ParserMethod: TutorialStructuredJSONParserMethod}},
		}},
	}
	if err := repo.EnsureExperiment(context.Background(), experiment); err != nil {
		t.Fatal(err)
	}
	store := installPrivateObject(t, "tenant_a", "prj_1", "tclj_1", object, content)
	service := NewLiveRunService(repo, repo, func() time.Time { return now })
	service.Configure(&recordingRuntimeIngestor{}, &recordingRuntimeEvaluator{}, store)
	subject := Subject{TenantID: "tenant_a", ID: "user_a"}
	service.ConfigureCandidateIngestors(RuntimeEnvironment{ChatModel: "chat-a", EvaluatorVersion: "standard_eval_v1"}, map[string]RuntimeIngestor{TutorialP1StructuredJSONCandidateID: &recordingRuntimeIngestor{}})
	baseline, _, err := service.StartVariant(context.Background(), subject, experiment.ProjectID, "baseline", "p0")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Execute(context.Background(), subject.TenantID, baseline.ID); err != nil {
		t.Fatal(err)
	}
	service.ConfigureCandidateIngestors(RuntimeEnvironment{ChatModel: "chat-b", EvaluatorVersion: "standard_eval_v1"}, map[string]RuntimeIngestor{TutorialP1StructuredJSONCandidateID: &recordingRuntimeIngestor{}})
	if _, _, err := service.StartVariant(context.Background(), subject, experiment.ProjectID, TutorialP1StructuredJSONCandidateID, "p1"); err != ErrBaselineRequired {
		t.Fatalf("mismatched P1 start error=%v", err)
	}
}

func TestLiveRunComparisonUsesPersistedStandardMetrics(t *testing.T) {
	now := time.Date(2026, 7, 16, 16, 0, 0, 0, time.UTC)
	repo := NewMemoryCloneRepository()
	baseline := ExperimentRun{
		ID: "terun_p0", TenantID: "tenant_a", ProjectID: "prj_1", ExperimentID: "texp_1", Variant: "baseline",
		ComparisonFingerprint: "same", DefinitionFingerprint: "p0", KnowledgeBaseID: "tkb_p0", DatasetID: "tds_1", Profile: "realtime", TopK: 5, ParserMethod: "basic", ChunkSizeTokens: TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens, IndexedChunkCount: 2, AverageChunkTokens: 12,
		Stage: ExperimentRunStageComplete, Status: ExperimentRunCompleted, EvaluationRunID: "eval_p0", CreatedAt: now, UpdatedAt: now,
	}
	candidate := ExperimentRun{
		ID: "terun_p1", TenantID: "tenant_a", ProjectID: "prj_1", ExperimentID: "texp_1", Variant: TutorialP1StructuredJSONCandidateID, BaselineRunID: baseline.ID,
		ComparisonFingerprint: "same", DefinitionFingerprint: "p1", KnowledgeBaseID: "tkb_p1", DatasetID: "tds_1", Profile: "realtime", TopK: 5, ParserMethod: TutorialStructuredJSONParserMethod, ChunkSizeTokens: TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens, IndexedChunkCount: 3, AverageChunkTokens: 10,
		Stage: ExperimentRunStageComplete, Status: ExperimentRunCompleted, EvaluationRunID: "eval_p1", CreatedAt: now, UpdatedAt: now.Add(time.Second),
	}
	if _, _, err := repo.CreateOrGetRun(context.Background(), baseline, "p0"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.CreateOrGetRun(context.Background(), candidate, "p1"); err != nil {
		t.Fatal(err)
	}
	evaluator := &comparisonRuntimeEvaluator{runs: map[string]eval.RunResult{
		"eval_p0": {ID: "eval_p0", ProjectID: "prj_1", Metrics: map[string]float64{"accuracy": 0.5, "total_tokens": 20}},
		"eval_p1": {ID: "eval_p1", ProjectID: "prj_1", Metrics: map[string]float64{"accuracy": 0.75, "total_tokens": 24}},
	}}
	service := NewLiveRunService(repo, repo, time.Now)
	service.Configure(nil, evaluator, nil)
	comparison, err := service.Compare(context.Background(), Subject{TenantID: "tenant_a", ID: "user_a"}, "prj_1", "texp_1", candidate.ID)
	if err != nil || !comparison.Comparable || len(comparison.Metrics) != 2 || len(comparison.IndexMetrics) != 2 {
		t.Fatalf("comparison=%#v err=%v", comparison, err)
	}
	if got := comparison.Metrics[0]; got.Name != "accuracy" || got.AbsoluteDelta != 0.25 || got.RelativeDelta == nil || *got.RelativeDelta != 0.5 {
		t.Fatalf("accuracy delta=%#v", got)
	}
}

func TestLiveRunComparisonAllowsP2AndReportsIndexMetrics(t *testing.T) {
	now := time.Date(2026, 7, 16, 16, 30, 0, 0, time.UTC)
	repo := NewMemoryCloneRepository()
	baseline := ExperimentRun{
		ID: "terun_p0", TenantID: "tenant_a", ProjectID: "prj_1", ExperimentID: "texp_1", Variant: "baseline",
		ComparisonFingerprint: "same", DefinitionFingerprint: "p0", KnowledgeBaseID: "tkb_p0", DatasetID: "tds_1", Profile: "realtime", TopK: 5, ParserMethod: "basic", ChunkSizeTokens: TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens, IndexedChunkCount: 2, AverageChunkTokens: 600,
		Stage: ExperimentRunStageComplete, Status: ExperimentRunCompleted, EvaluationRunID: "eval_p0", CreatedAt: now, UpdatedAt: now,
	}
	candidate := ExperimentRun{
		ID: "terun_p2", TenantID: "tenant_a", ProjectID: "prj_1", ExperimentID: "texp_1", Variant: TutorialP2RecursiveChunkCandidateID, BaselineRunID: baseline.ID,
		ComparisonFingerprint: "same", DefinitionFingerprint: "p2", KnowledgeBaseID: "tkb_p2", DatasetID: "tds_1", Profile: "realtime", TopK: 5, ParserMethod: "basic", ChunkSizeTokens: TutorialP2ChunkSizeTokens, ChunkOverlapTokens: TutorialP2ChunkOverlapTokens, IndexedChunkCount: 4, AverageChunkTokens: 320,
		Stage: ExperimentRunStageComplete, Status: ExperimentRunCompleted, EvaluationRunID: "eval_p2", CreatedAt: now, UpdatedAt: now.Add(time.Second),
	}
	if _, _, err := repo.CreateOrGetRun(context.Background(), baseline, "p0"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.CreateOrGetRun(context.Background(), candidate, "p2"); err != nil {
		t.Fatal(err)
	}
	service := NewLiveRunService(repo, repo, time.Now)
	service.Configure(nil, &comparisonRuntimeEvaluator{runs: map[string]eval.RunResult{
		"eval_p0": {ID: "eval_p0", ProjectID: "prj_1", Metrics: map[string]float64{"accuracy": 0.5}},
		"eval_p2": {ID: "eval_p2", ProjectID: "prj_1", Metrics: map[string]float64{"accuracy": 0.75}},
	}}, nil)
	comparison, err := service.Compare(context.Background(), Subject{TenantID: "tenant_a", ID: "user_a"}, "prj_1", "texp_1", candidate.ID)
	if err != nil || !comparison.Comparable || len(comparison.IndexMetrics) != 2 {
		t.Fatalf("comparison=%#v err=%v", comparison, err)
	}
	if got := comparison.IndexMetrics[1]; got.Name != "chunk_count" || got.Baseline != 2 || got.Candidate != 4 || got.AbsoluteDelta != 2 {
		t.Fatalf("chunk count delta=%#v", got)
	}
}

func TestLiveRunComparisonAllowsP3OnlyWithMeasuredContext(t *testing.T) {
	now := time.Date(2026, 7, 16, 16, 45, 0, 0, time.UTC)
	repo := NewMemoryCloneRepository()
	baseline := ExperimentRun{
		ID: "terun_p0", TenantID: "tenant_a", ProjectID: "prj_1", ExperimentID: "texp_1", Variant: "baseline",
		ComparisonFingerprint: "same", DefinitionFingerprint: "p0", KnowledgeBaseID: "tkb_p0", DatasetID: "tds_1", Profile: "realtime", TopK: 5, ParserMethod: "basic", ChunkSizeTokens: TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens, IndexedChunkCount: 2, AverageChunkTokens: 600,
		Stage: ExperimentRunStageComplete, Status: ExperimentRunCompleted, EvaluationRunID: "eval_p0", CreatedAt: now, UpdatedAt: now,
	}
	candidate := ExperimentRun{
		ID: "terun_p3", TenantID: "tenant_a", ProjectID: "prj_1", ExperimentID: "texp_1", Variant: TutorialP3ContextualCandidateID, BaselineRunID: baseline.ID,
		ComparisonFingerprint: "same", DefinitionFingerprint: "p3", KnowledgeBaseID: "tkb_p3", DatasetID: "tds_1", Profile: "realtime", TopK: 5, ParserMethod: "basic", ChunkSizeTokens: TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens, ContextualRetrievalEnabled: true, IndexedChunkCount: 2, AverageChunkTokens: 600, ContextualizedChunkCount: 2, AverageContextTokens: 14,
		Stage: ExperimentRunStageComplete, Status: ExperimentRunCompleted, EvaluationRunID: "eval_p3", CreatedAt: now, UpdatedAt: now.Add(time.Second),
	}
	if _, _, err := repo.CreateOrGetRun(context.Background(), baseline, "p0"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.CreateOrGetRun(context.Background(), candidate, "p3"); err != nil {
		t.Fatal(err)
	}
	service := NewLiveRunService(repo, repo, time.Now)
	service.Configure(nil, &comparisonRuntimeEvaluator{runs: map[string]eval.RunResult{
		"eval_p0": {ID: "eval_p0", ProjectID: "prj_1", Metrics: map[string]float64{"accuracy": 0.5}},
		"eval_p3": {ID: "eval_p3", ProjectID: "prj_1", Metrics: map[string]float64{"accuracy": 0.75}},
	}}, nil)
	comparison, err := service.Compare(context.Background(), Subject{TenantID: "tenant_a", ID: "user_a"}, "prj_1", "texp_1", candidate.ID)
	if err != nil || !comparison.Comparable || len(comparison.IndexMetrics) != 4 {
		t.Fatalf("comparison=%#v err=%v", comparison, err)
	}
	if got := comparison.IndexMetrics[2]; got.Name != "contextualized_chunk_count" || got.Baseline != 0 || got.Candidate != 2 || got.AbsoluteDelta != 2 {
		t.Fatalf("context count delta=%#v", got)
	}
	candidate.ContextualizedChunkCount = 0
	if runsComparable(baseline, candidate) {
		t.Fatalf("comparison accepted P3 without contextual facts: %#v", candidate)
	}
}

func TestLiveRunRejectsUnavailableRuntimeAndCancelsQueuedRun(t *testing.T) {
	repo := NewMemoryCloneRepository()
	if err := repo.EnsureExperiment(context.Background(), Experiment{ID: "texp_missing", TenantID: "tenant_a", ProjectID: "prj_missing", TemplateID: "text-rag", Tier: "quick", PackStatus: PackStatusInstalled, RuntimeStatus: "runtime_unavailable"}); err != nil {
		t.Fatal(err)
	}
	service := NewLiveRunService(repo, repo, time.Now)
	subject := Subject{TenantID: "tenant_a", ID: "user_a"}
	if _, _, err := service.Start(context.Background(), subject, "prj_missing", "key"); err != ErrRuntimeUnavailable {
		t.Fatalf("unavailable start error=%v", err)
	}

	experiment := Experiment{ID: "texp_1", TenantID: "tenant_a", ProjectID: "prj_1", CloneJobID: "tclj_1", TemplateID: "text-rag", Tier: "quick", PackStatus: PackStatusInstalled, RuntimeStatus: "ready", KnowledgeBaseID: "kb_1", DatasetID: "ds_1", BaselineProfile: "realtime", BaselineTopK: 1, PackManifest: Manifest{Runtime: &RuntimeManifest{}}}
	if err := repo.EnsureExperiment(context.Background(), experiment); err != nil {
		t.Fatal(err)
	}
	run, _, err := service.Start(context.Background(), subject, "prj_1", "cancel")
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := service.Cancel(context.Background(), subject, run.ID)
	if err != nil || cancelled.Status != ExperimentRunCancelled {
		t.Fatalf("cancelled=%#v err=%v", cancelled, err)
	}
	unsupported := experiment
	unsupported.ID, unsupported.ProjectID, unsupported.Tier = "texp_benchmark", "prj_benchmark", "benchmark"
	if err := repo.EnsureExperiment(context.Background(), unsupported); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Start(context.Background(), subject, unsupported.ProjectID, "benchmark"); err != ErrRuntimeUnavailable {
		t.Fatalf("unsupported tier start error=%v", err)
	}
}

func installPrivateObject(t *testing.T, tenantID, projectID, jobID string, object PackObject, content []byte) *LocalPrivateStore {
	t.Helper()
	root := t.TempDir()
	store, err := NewLocalPrivateStore(filepath.Join(root, "private"), "tutorial-experiments")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "input")
	if err := os.WriteFile(input, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.PutVerified(context.Background(), PrivateObject{TenantID: tenantID, ProjectID: projectID, JobID: jobID, Object: VerifiedObject{PackObject: object, TempPath: input}}); err != nil {
		t.Fatal(err)
	}
	return store
}

type recordingRuntimeIngestor struct {
	requests []ingest.Request
	chunks   []kb.Chunk
}

func (r *recordingRuntimeIngestor) Ingest(_ context.Context, request ingest.Request) (ingest.Result, error) {
	r.requests = append(r.requests, request)
	if r.chunks != nil {
		return ingest.Result{Chunks: append([]kb.Chunk(nil), r.chunks...)}, nil
	}
	return ingest.Result{Chunks: []kb.Chunk{{Content: string(request.Content)}}}, nil
}

type recordingRuntimeEvaluator struct {
	request  eval.RunRequest
	requests []eval.RunRequest
}

func (r *recordingRuntimeEvaluator) Run(_ context.Context, request eval.RunRequest) (eval.RunResult, error) {
	r.request = request
	r.requests = append(r.requests, request)
	return eval.RunResult{ID: "eval_" + fmt.Sprint(len(r.requests))}, nil
}

type comparisonRuntimeEvaluator struct{ runs map[string]eval.RunResult }

func (r *comparisonRuntimeEvaluator) Run(_ context.Context, _ eval.RunRequest) (eval.RunResult, error) {
	return eval.RunResult{}, nil
}

func (r *comparisonRuntimeEvaluator) GetInProject(_ context.Context, _ string, projectID, runID string) (eval.RunResult, bool, error) {
	run, found := r.runs[runID]
	return run, found && run.ProjectID == projectID, nil
}
