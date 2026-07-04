package eval

import (
	"context"
	"errors"
	"testing"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/llm/ark"
	"github.com/shikanon/orag/internal/platform/apperrors"
	"github.com/shikanon/orag/internal/prompt"
	"github.com/shikanon/orag/internal/rag"
)

func TestScoreItem(t *testing.T) {
	item := dataset.Item{
		RelevantDocIDs: []string{"doc_1"},
		GroundTruth:    "qdrant",
		DiversityAnnotations: []dataset.DiversityAnnotation{
			{Aspect: "retrieval", DocumentID: "doc_1"},
		},
	}
	resp := rag.QueryResponse{
		Answer: "qdrant answer",
		Citations: []rag.Citation{{
			ChunkID:    "chk_1",
			DocumentID: "doc_1",
		}},
		RetrievedChunks: []kb.SearchResult{{
			Chunk: kb.Chunk{ID: "chk_1", DocumentID: "doc_1"},
		}},
	}
	metrics := ScoreItem(item, resp)
	if metrics["accuracy"] != 1 || metrics["context_recall"] != 1 || metrics["citation_precision"] != 1 {
		t.Fatalf("metrics = %#v", metrics)
	}
	for _, key := range []string{"ndcg_at_k", "recall_at_k", "mrr", "map", "coverage", "alpha_ndcg", "aspect_coverage"} {
		if metrics[key] != 1 {
			t.Fatalf("%s = %f, want 1; metrics=%#v", key, metrics[key], metrics)
		}
	}
	if metrics["retrieval_failure_rate"] != 0 {
		t.Fatalf("retrieval_failure_rate = %f, want 0", metrics["retrieval_failure_rate"])
	}
}

func TestScoreItemRedundancyMetrics(t *testing.T) {
	tests := []struct {
		name             string
		results          []kb.SearchResult
		redundancyRate   float64
		duplicateCount   float64
		dedupedTopKCount float64
	}{
		{
			name: "same chunk id is duplicate",
			results: []kb.SearchResult{
				{Chunk: kb.Chunk{ID: "chk_1", DocumentID: "doc_1", Content: "vector search"}},
				{Chunk: kb.Chunk{ID: "chk_1", DocumentID: "doc_1", Content: "vector search"}},
			},
			redundancyRate:   0.5,
			duplicateCount:   1,
			dedupedTopKCount: 1,
		},
		{
			name: "same document normalized text is duplicate",
			results: []kb.SearchResult{
				{Chunk: kb.Chunk{DocumentID: "doc_1", Content: "Qdrant, vector search!"}},
				{Chunk: kb.Chunk{DocumentID: "doc_1", Content: " qdrant vector   search "}},
			},
			redundancyRate:   0.5,
			duplicateCount:   1,
			dedupedTopKCount: 1,
		},
		{
			name: "different documents are not duplicate",
			results: []kb.SearchResult{
				{Chunk: kb.Chunk{DocumentID: "doc_1", Content: "qdrant vector search"}},
				{Chunk: kb.Chunk{DocumentID: "doc_2", Content: "qdrant vector search"}},
			},
			redundancyRate:   0,
			duplicateCount:   0,
			dedupedTopKCount: 2,
		},
		{
			name:             "empty results",
			results:          nil,
			redundancyRate:   0,
			duplicateCount:   0,
			dedupedTopKCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := ScoreItem(dataset.Item{}, rag.QueryResponse{RetrievedChunks: tt.results})
			if metrics["redundancy_rate"] != tt.redundancyRate ||
				metrics["duplicate_count"] != tt.duplicateCount ||
				metrics["deduped_top_k_count"] != tt.dedupedTopKCount {
				t.Fatalf("redundancy metrics = %#v", metrics)
			}
		})
	}
}

func TestScoreItemCitationOnlyDoesNotSetAccuracy(t *testing.T) {
	item := dataset.Item{RelevantDocIDs: []string{"doc_1"}, GroundTruth: "qdrant"}
	resp := rag.QueryResponse{
		Answer: "postgres answer",
		Citations: []rag.Citation{{
			ChunkID:    "chk_1",
			DocumentID: "doc_1",
		}},
	}
	metrics := ScoreItem(item, resp)
	assertMetric(t, metrics, "answer_accuracy", 0)
	assertMetric(t, metrics, "accuracy", 0)
	assertMetric(t, metrics, "citation_hit_rate", 1)
}

func TestRunnerPersistsRunInMemoryRepository(t *testing.T) {
	ctx := context.Background()
	dsRepo := dataset.NewMemoryRepository()
	dsSvc := dataset.NewService(dsRepo)
	ds, err := dsSvc.Create(ctx, "tenant_default", "regression", "golden")
	if err != nil {
		t.Fatal(err)
	}
	_, err = dsSvc.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{
		Query:          "qdrant vector",
		GroundTruth:    "qdrant",
		RelevantDocIDs: []string{"doc_1"},
		DiversityAnnotations: []dataset.DiversityAnnotation{
			{Aspect: "retrieval", DocumentID: "doc_1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	store := kb.NewMemoryStore()
	_ = store.Store(ctx, kb.Document{
		ID:              "doc_1",
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		SourceURI:       "memory://doc",
		Title:           "Doc",
	}, []kb.Chunk{{
		ID:              "chk_1",
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		DocumentID:      "doc_1",
		Content:         "qdrant vector search",
		SourceURI:       "memory://doc",
	}})
	ragSvc := &rag.Service{
		Retriever: kb.HybridRetriever{
			Dense:  kb.DenseRetriever{Store: store},
			Sparse: kb.SparseRetriever{Store: store},
			RRFK:   60,
			TopN:   8,
		},
		Model:           ark.NewClient(ark.Config{EmbeddingDimensions: 4}, nil),
		Cache:           rag.NewSemanticCache(10),
		Packer:          rag.ContextPacker{MaxTokens: 512, TopN: 4},
		PromptStrategy:  prompt.NewStrategy("auto"),
		DefaultProfile:  rag.ProfileRealtime,
		NoContextAnswer: "no context",
		TopK:            8,
	}
	evalRepo := NewMemoryRepository()
	runner := Runner{RAG: ragSvc, Datasets: dsSvc, Repository: evalRepo}
	result, err := runner.Run(ctx, RunRequest{
		TenantID:        "tenant_default",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Profile:         rag.ProfileRealtime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Accuracy == 0 {
		t.Fatalf("result = %#v", result)
	}
	for _, key := range []string{"ndcg_at_k", "recall_at_k", "mrr", "map", "coverage", "redundancy_rate", "deduped_top_k_count", "alpha_ndcg", "aspect_coverage"} {
		if _, ok := result.Metrics[key]; !ok {
			t.Fatalf("run metric %q missing from %#v", key, result.Metrics)
		}
	}
	storedItemMetrics := evalRepo.results[result.ID]
	if len(storedItemMetrics) != 1 {
		t.Fatalf("stored item metrics len = %d, want 1", len(storedItemMetrics))
	}
	for _, key := range []string{"ndcg_at_k", "recall_at_k", "deduped_top_k_count", "alpha_ndcg"} {
		if _, ok := storedItemMetrics[0][key]; !ok {
			t.Fatalf("stored item metric %q missing from %#v", key, storedItemMetrics[0])
		}
	}
	storedRun, ok, err := runner.Get(ctx, "tenant_default", result.ID)
	if err != nil || !ok {
		t.Fatalf("Get() ok=%v err=%v", ok, err)
	}
	if storedRun.Metrics["ndcg_at_k"] == 0 || storedRun.Metrics["alpha_ndcg"] == 0 {
		t.Fatalf("stored run metrics = %#v", storedRun.Metrics)
	}
	if _, ok, err := runner.Get(ctx, "tenant_other", result.ID); err != nil || ok {
		t.Fatalf("cross-tenant Get() ok=%v err=%v, want not found", ok, err)
	}
}

func TestMemoryRepositoryRejectsUnknownRunMetric(t *testing.T) {
	repo := NewMemoryRepository()

	err := repo.StoreEvaluationRun(context.Background(), "tenant_default", RunResult{
		ID:        "eval_unknown",
		DatasetID: "ds_default",
		Metrics: map[string]float64{
			"answer_accuracy": 1,
			"harness_custom":  0.5,
		},
	})
	if !apperrors.IsCode(err, apperrors.CodeValidation) {
		t.Fatalf("StoreEvaluationRun() error = %v, want validation", err)
	}
	if len(repo.runs) != 0 {
		t.Fatalf("persisted runs = %d, want 0", len(repo.runs))
	}
}

func TestMemoryRepositoryRejectsUnknownItemMetric(t *testing.T) {
	repo := NewMemoryRepository()

	err := repo.StoreEvaluationResult(context.Background(), "eval_default", "item_default", "answer", map[string]float64{
		"answer_accuracy": 1,
		"harness_custom":  0.5,
	})
	if !apperrors.IsCode(err, apperrors.CodeValidation) {
		t.Fatalf("StoreEvaluationResult() error = %v, want validation", err)
	}
	if len(repo.results) != 0 {
		t.Fatalf("persisted results = %d, want 0", len(repo.results))
	}
}

func TestRunnerRejectsMissingRequiredIDs(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		req  RunRequest
	}{
		{
			name: "dataset id",
			req: RunRequest{
				TenantID:        "tenant_default",
				KnowledgeBaseID: "kb_default",
				Profile:         rag.ProfileRealtime,
			},
		},
		{
			name: "knowledge base id",
			req: RunRequest{
				TenantID:  "tenant_default",
				DatasetID: "ds_default",
				Profile:   rag.ProfileRealtime,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := (Runner{}).Run(ctx, tt.req); !apperrors.IsCode(err, apperrors.CodeValidation) {
				t.Fatalf("Run() error = %v, want validation", err)
			}
		})
	}
}

func TestRunnerRejectsUnknownDataset(t *testing.T) {
	ctx := context.Background()
	dsSvc := dataset.NewService(dataset.NewMemoryRepository())
	runner := Runner{Datasets: dsSvc}

	_, err := runner.Run(ctx, RunRequest{
		TenantID:        "tenant_default",
		DatasetID:       "ds_missing",
		KnowledgeBaseID: "kb_default",
		Profile:         rag.ProfileRealtime,
	})
	if !errors.Is(err, dataset.ErrDatasetNotFound) {
		t.Fatalf("Run() error = %v, want dataset not found", err)
	}
	if !apperrors.IsCode(err, apperrors.CodeNotFound) {
		t.Fatalf("Run() error = %v, want not-found app error", err)
	}
}

func TestRunnerRejectsEmptyDataset(t *testing.T) {
	ctx := context.Background()
	dsSvc := dataset.NewService(dataset.NewMemoryRepository())
	ds, err := dsSvc.Create(ctx, "tenant_default", "empty", "golden")
	if err != nil {
		t.Fatal(err)
	}
	evalRepo := NewMemoryRepository()
	runner := Runner{Datasets: dsSvc, Repository: evalRepo}

	_, err = runner.Run(ctx, RunRequest{
		TenantID:        "tenant_default",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Profile:         rag.ProfileRealtime,
	})
	if !apperrors.IsCode(err, apperrors.CodeValidation) {
		t.Fatalf("Run() error = %v, want validation", err)
	}
	if len(evalRepo.runs) != 0 {
		t.Fatalf("persisted runs = %d, want 0", len(evalRepo.runs))
	}
}

func TestRunnerRequiresDatasetTenant(t *testing.T) {
	ctx := context.Background()
	dsRepo := dataset.NewMemoryRepository()
	dsSvc := dataset.NewService(dsRepo)
	ds, err := dsSvc.Create(ctx, "tenant_default", "regression", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dsSvc.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{Query: "q", GroundTruth: "a"}); err != nil {
		t.Fatal(err)
	}

	runner := Runner{Datasets: dsSvc}
	_, err = runner.Run(ctx, RunRequest{
		TenantID:        "tenant_other",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Profile:         rag.ProfileRealtime,
	})
	if !errors.Is(err, dataset.ErrDatasetNotFound) {
		t.Fatalf("Run() error = %v, want dataset not found", err)
	}
	if !apperrors.IsCode(err, apperrors.CodeNotFound) {
		t.Fatalf("Run() error = %v, want not-found app error", err)
	}
}

func TestRunnerDoesNotCountCitationOnlyAsAnswerAccuracy(t *testing.T) {
	ctx := context.Background()
	dsRepo := dataset.NewMemoryRepository()
	dsSvc := dataset.NewService(dsRepo)
	ds, err := dsSvc.Create(ctx, "tenant_default", "citation-only", "golden")
	if err != nil {
		t.Fatal(err)
	}
	_, err = dsSvc.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{
		Query:          "qdrant vector",
		GroundTruth:    "qdrant",
		RelevantDocIDs: []string{"doc_1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	ragSvc := &rag.Service{
		Pipeline: pipelineFunc(func(context.Context, rag.QueryRequest) (rag.QueryResponse, error) {
			return rag.QueryResponse{
				Answer: "postgres answer",
				Citations: []rag.Citation{{
					ChunkID:    "chk_1",
					DocumentID: "doc_1",
				}},
				RetrievedChunks: []kb.SearchResult{{
					Chunk: kb.Chunk{ID: "chk_1", DocumentID: "doc_1"},
				}},
				CacheStatus: "miss",
				LatencyMS:   7,
			}, nil
		}),
	}
	evalRepo := NewMemoryRepository()
	runner := Runner{RAG: ragSvc, Datasets: dsSvc, Repository: evalRepo}
	result, err := runner.Run(ctx, RunRequest{
		TenantID:        "tenant_default",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Profile:         rag.ProfileRealtime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Fatalf("total = %d, want 1", result.Total)
	}
	if result.Accuracy != 0 || result.HitRate != 0 {
		t.Fatalf("result accuracy=%v hit_rate=%v, want 0/0", result.Accuracy, result.HitRate)
	}
	assertMetric(t, result.Metrics, "answer_accuracy", 0)
	assertMetric(t, result.Metrics, "accuracy", 0)
	assertMetric(t, result.Metrics, "hit_rate", 0)
	assertMetric(t, result.Metrics, "citation_hit_rate", 1)

	evalRepo.mu.RLock()
	persisted := append([]map[string]float64(nil), evalRepo.results[result.ID]...)
	evalRepo.mu.RUnlock()
	if len(persisted) != 1 {
		t.Fatalf("persisted results = %d, want 1", len(persisted))
	}
	assertMetric(t, persisted[0], "answer_accuracy", 0)
	assertMetric(t, persisted[0], "accuracy", 0)
	assertMetric(t, persisted[0], "citation_hit_rate", 1)
}

func TestRunnerPersistsOptionalJudgeAndQAGDetails(t *testing.T) {
	ctx := context.Background()
	dsRepo := dataset.NewMemoryRepository()
	dsSvc := dataset.NewService(dsRepo)
	ds, err := dsSvc.Create(ctx, "tenant_default", "judge-qag", "golden")
	if err != nil {
		t.Fatal(err)
	}
	item, err := dsSvc.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{
		Query:            "What does ORAG use?",
		GroundTruth:      "qdrant",
		RelevantDocIDs:   []string{"doc_1"},
		ExpectedEvidence: []string{"qdrant evidence"},
	})
	if err != nil {
		t.Fatal(err)
	}

	ragSvc := &rag.Service{
		Pipeline: pipelineFunc(func(context.Context, rag.QueryRequest) (rag.QueryResponse, error) {
			return rag.QueryResponse{
				Answer: "ORAG uses qdrant evidence",
				Citations: []rag.Citation{{
					ChunkID:    "chk_1",
					DocumentID: "doc_1",
				}},
				RetrievedChunks: []kb.SearchResult{{
					Chunk: kb.Chunk{ID: "chk_1", DocumentID: "doc_1", Content: "qdrant evidence"},
				}},
				CacheStatus: "miss",
				LatencyMS:   11,
			}, nil
		}),
	}
	evalRepo := NewMemoryRepository()
	runner := Runner{
		RAG:        ragSvc,
		Datasets:   dsSvc,
		Repository: evalRepo,
		Judge:      fakeJudge{},
		QAG:        fakeQAGJudge{},
	}

	result, err := runner.Run(ctx, RunRequest{
		TenantID:        "tenant_default",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Profile:         rag.ProfileRealtime,
		Judge:           &JudgeConfig{Provider: "test", Model: "judge-model", PromptVersion: "pv1"},
		QAG:             &JudgeConfig{Provider: "test", Model: "qag-model", PromptVersion: "pv1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics["qag_score"] != 1 || result.Metrics["prompt_tokens"] != 17 || result.Metrics["total_tokens"] != 26 {
		t.Fatalf("run metrics = %#v, want qag and token totals", result.Metrics)
	}
	if result.Metrics["cost_usd"] != 0.03 {
		t.Fatalf("cost_usd = %v, want 0.03", result.Metrics["cost_usd"])
	}

	detail, ok, err := runner.GetDetail(ctx, "tenant_default", result.ID, EvaluationDetailOptions{
		IncludeItems: true,
		IncludeJudge: true,
	})
	if err != nil || !ok {
		t.Fatalf("GetDetail() ok=%v err=%v", ok, err)
	}
	if len(detail.Items) != 1 || detail.Items[0].DatasetItemID != item.ID || detail.Items[0].Metrics["qag_score"] != 1 {
		t.Fatalf("items = %#v, want persisted item metrics", detail.Items)
	}
	if len(detail.JudgeRuns) != 2 {
		t.Fatalf("judge runs = %d, want judge and qag runs", len(detail.JudgeRuns))
	}
	if len(detail.JudgeResults) != 2 {
		t.Fatalf("judge results = %#v, want judge and qag results", detail.JudgeResults)
	}
	seenRaw := false
	seenParsed := false
	seenTokenCost := false
	for _, result := range detail.JudgeResults {
		if result.RawResponse != "" {
			seenRaw = true
		}
		if result.ParsedJSON["source"] != "" || result.ParsedJSON["score"] != nil {
			seenParsed = true
		}
		if result.TokenUsage.TotalTokens > 0 && result.CostUSD > 0 {
			seenTokenCost = true
		}
	}
	if !seenRaw || !seenParsed || !seenTokenCost {
		t.Fatalf("judge results missing raw/parsed/token/cost separation: %#v", detail.JudgeResults)
	}
}

func TestRunnerRejectsRequestedJudgeWhenNotConfigured(t *testing.T) {
	ctx := context.Background()
	dsSvc := dataset.NewService(dataset.NewMemoryRepository())
	ds, err := dsSvc.Create(ctx, "tenant_default", "judge-missing", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dsSvc.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{Query: "q", GroundTruth: "a"}); err != nil {
		t.Fatal(err)
	}

	runner := Runner{
		RAG: &rag.Service{Pipeline: pipelineFunc(func(context.Context, rag.QueryRequest) (rag.QueryResponse, error) {
			return rag.QueryResponse{Answer: "a"}, nil
		})},
		Datasets: dsSvc,
	}
	_, err = runner.Run(ctx, RunRequest{
		TenantID:        "tenant_default",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Judge:           &JudgeConfig{},
	})
	if !apperrors.IsCode(err, apperrors.CodeValidation) {
		t.Fatalf("Run() error = %v, want validation for missing judge", err)
	}
}

func TestOptimizerCandidatesDoNotReuseSemanticCacheAcrossProfileOrTopK(t *testing.T) {
	ctx := context.Background()
	dsRepo := dataset.NewMemoryRepository()
	dsSvc := dataset.NewService(dsRepo)
	ds, err := dsSvc.Create(ctx, "tenant_default", "optimizer", "golden")
	if err != nil {
		t.Fatal(err)
	}
	_, err = dsSvc.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{
		Query:          "qdrant vector",
		GroundTruth:    "qdrant",
		RelevantDocIDs: []string{"doc_1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	store := kb.NewMemoryStore()
	_ = store.Store(ctx, kb.Document{
		ID:              "doc_1",
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		SourceURI:       "memory://doc",
		Title:           "Doc",
	}, []kb.Chunk{{
		ID:              "chk_1",
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		DocumentID:      "doc_1",
		Content:         "qdrant vector search",
		SourceURI:       "memory://doc",
	}})
	ragSvc := &rag.Service{
		Retriever: kb.HybridRetriever{
			Dense:  kb.DenseRetriever{Store: store},
			Sparse: kb.SparseRetriever{Store: store},
			RRFK:   60,
			TopN:   8,
		},
		Model:           ark.NewClient(ark.Config{EmbeddingDimensions: 4}, nil),
		Cache:           rag.NewSemanticCache(10),
		Packer:          rag.ContextPacker{MaxTokens: 512, TopN: 4},
		PromptStrategy:  prompt.NewStrategy("auto"),
		DefaultProfile:  rag.ProfileRealtime,
		NoContextAnswer: "no context",
		TopK:            8,
	}
	evalRepo := NewMemoryRepository()
	runner := Runner{RAG: ragSvc, Datasets: dsSvc, Repository: evalRepo}
	result, err := (Optimizer{Runner: runner}).Optimize(ctx, OptimizeRequest{
		TenantID:        "tenant_default",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Profiles:        []rag.Profile{rag.ProfileRealtime, rag.ProfileHighPrecision},
		TopKs:           []int{1, 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 4 {
		t.Fatalf("candidates = %d, want 4", len(result.Candidates))
	}
	for _, candidate := range result.Candidates {
		run, ok, err := runner.Get(ctx, "tenant_default", candidate.RunID)
		if err != nil || !ok {
			t.Fatalf("Get(%q) ok=%v err=%v", candidate.RunID, ok, err)
		}
		if got := run.Metrics["cache_hit_rate"]; got != 0 {
			t.Fatalf("candidate profile=%s top_k=%d cache_hit_rate = %v, want 0", candidate.Profile, candidate.TopK, got)
		}
	}
}

func TestOptimizerRejectsUnknownDataset(t *testing.T) {
	ctx := context.Background()
	dsSvc := dataset.NewService(dataset.NewMemoryRepository())
	runner := Runner{Datasets: dsSvc}

	result, err := (Optimizer{Runner: runner}).Optimize(ctx, OptimizeRequest{
		TenantID:        "tenant_default",
		DatasetID:       "ds_missing",
		KnowledgeBaseID: "kb_default",
		Profiles:        []rag.Profile{rag.ProfileRealtime},
		TopKs:           []int{1},
	})
	if !errors.Is(err, dataset.ErrDatasetNotFound) {
		t.Fatalf("Optimize() error = %v, want dataset not found", err)
	}
	if !apperrors.IsCode(err, apperrors.CodeNotFound) {
		t.Fatalf("Optimize() error = %v, want not-found app error", err)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %d, want 0", len(result.Candidates))
	}
}

func TestOptimizerTopKChangesHybridFinalCandidateCount(t *testing.T) {
	ctx := context.Background()
	dsRepo := dataset.NewMemoryRepository()
	dsSvc := dataset.NewService(dsRepo)
	ds, err := dsSvc.Create(ctx, "tenant_default", "optimizer-topk", "golden")
	if err != nil {
		t.Fatal(err)
	}
	_, err = dsSvc.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{
		Query:          "qdrant vector",
		GroundTruth:    "qdrant",
		RelevantDocIDs: []string{"doc_1", "doc_2", "doc_3"},
	})
	if err != nil {
		t.Fatal(err)
	}

	retrieved := []kb.SearchResult{
		{
			Chunk: kb.Chunk{ID: "chk_1", DocumentID: "doc_1", Content: "qdrant vector document one"},
			Score: 1,
			Rank:  1,
			From:  "test",
		},
		{
			Chunk: kb.Chunk{ID: "chk_2", DocumentID: "doc_2", Content: "qdrant vector document two"},
			Score: 0.9,
			Rank:  2,
			From:  "test",
		},
		{
			Chunk: kb.Chunk{ID: "chk_3", DocumentID: "doc_3", Content: "qdrant vector document three"},
			Score: 0.8,
			Rank:  3,
			From:  "test",
		},
	}
	ragSvc := &rag.Service{
		Retriever: kb.HybridRetriever{
			Dense:      fixedResultsRetriever{results: retrieved},
			Sparse:     fixedResultsRetriever{results: retrieved},
			RRFK:       60,
			TopN:       8,
			DenseTopK:  8,
			SparseTopK: 8,
		},
		Model:           ark.NewClient(ark.Config{EmbeddingDimensions: 4}, nil),
		Packer:          rag.ContextPacker{MaxTokens: 512, TopN: 10},
		PromptStrategy:  prompt.NewStrategy("auto"),
		DefaultProfile:  rag.ProfileRealtime,
		NoContextAnswer: "no context",
		TopK:            8,
	}
	evalRepo := NewMemoryRepository()
	runner := Runner{RAG: ragSvc, Datasets: dsSvc, Repository: evalRepo}
	result, err := (Optimizer{Runner: runner}).Optimize(ctx, OptimizeRequest{
		TenantID:        "tenant_default",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Profiles:        []rag.Profile{rag.ProfileRealtime},
		TopKs:           []int{1, 3},
	})
	if err != nil {
		t.Fatal(err)
	}

	recalls := map[int]float64{}
	for _, candidate := range result.Candidates {
		run, ok, err := runner.Get(ctx, "tenant_default", candidate.RunID)
		if err != nil || !ok {
			t.Fatalf("Get(%q) ok=%v err=%v", candidate.RunID, ok, err)
		}
		recalls[candidate.TopK] = run.Metrics["context_recall"]
	}
	if recalls[1] >= recalls[3] {
		t.Fatalf("context_recall by top_k = %#v, want top_k=3 to retrieve more relevant context than top_k=1", recalls)
	}
	if recalls[3] != 1 {
		t.Fatalf("top_k=3 context_recall = %v, want 1", recalls[3])
	}
}

type fixedResultsRetriever struct {
	results []kb.SearchResult
}

func (r fixedResultsRetriever) Retrieve(context.Context, kb.SearchRequest) ([]kb.SearchResult, error) {
	return append([]kb.SearchResult(nil), r.results...), nil
}

type pipelineFunc func(context.Context, rag.QueryRequest) (rag.QueryResponse, error)

func (f pipelineFunc) Invoke(ctx context.Context, req rag.QueryRequest) (rag.QueryResponse, error) {
	return f(ctx, req)
}

type fakeJudge struct{}

func (fakeJudge) Judge(_ context.Context, input JudgeInput) (JudgeOutput, error) {
	return JudgeOutput{
		Scores:      map[string]float64{"faithfulness": 0.9},
		Labels:      map[string]string{"faithfulness": "good"},
		Pass:        true,
		Rationale:   "supported",
		Findings:    []JudgeFinding{{Metric: "faithfulness", Label: "good", Message: input.Query}},
		RawResponse: `{"scores":{"faithfulness":0.9},"pass":true}`,
		ParsedJSON:  map[string]any{"source": "judge"},
		TokenUsage:  TokenUsage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
		CostUSD:     0.01,
	}, nil
}

type fakeQAGJudge struct{}

func (fakeQAGJudge) ScoreQAG(_ context.Context, input JudgeInput) (QAGOutput, error) {
	return QAGOutput{
		Score:       1,
		Metrics:     map[string]float64{"qag_score": 1, "qag_claim_coverage": 1, "qag_question_count": 1, "qag_unverifiable_rate": 0},
		Claims:      []QAGClaim{{Claim: input.Answer, Question: "supported?", Verdict: "supported", Evidence: "qdrant evidence"}},
		RawResponse: `{"score":1,"claims":[{"verdict":"supported"}]}`,
		ParsedJSON:  map[string]any{"score": float64(1)},
		TokenUsage:  TokenUsage{PromptTokens: 7, CompletionTokens: 5, TotalTokens: 12},
		CostUSD:     0.02,
	}, nil
}

func assertMetric(t *testing.T, metrics map[string]float64, key string, want float64) {
	t.Helper()
	if got, ok := metrics[key]; !ok || got != want {
		t.Fatalf("metrics[%q] = %v (present=%v), want %v; metrics=%#v", key, got, ok, want, metrics)
	}
}

func TestRunnerRejectsForeignTenantDatasetWithoutPersistingRun(t *testing.T) {
	ctx := context.Background()
	dsSvc := dataset.NewService()
	ds, err := dsSvc.Create(ctx, "tenant_a", "regression", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dsSvc.AddItem(ctx, "tenant_a", ds.ID, dataset.Item{
		Query:       "qdrant vector",
		GroundTruth: "qdrant",
	}); err != nil {
		t.Fatal(err)
	}

	evalRepo := NewMemoryRepository()
	runner := Runner{Datasets: dsSvc, Repository: evalRepo}
	_, err = runner.Run(ctx, RunRequest{
		TenantID:        "tenant_b",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Profile:         rag.ProfileRealtime,
	})
	if !apperrors.IsCode(err, apperrors.CodeNotFound) {
		t.Fatalf("Run() err = %v, want dataset not found", err)
	}
	if len(evalRepo.runs) != 0 || len(evalRepo.results) != 0 {
		t.Fatalf("foreign dataset run persisted runs=%d results=%d", len(evalRepo.runs), len(evalRepo.results))
	}
}

func TestOptimizerRejectsForeignTenantDatasetWithoutPersistingRun(t *testing.T) {
	ctx := context.Background()
	dsSvc := dataset.NewService()
	ds, err := dsSvc.Create(ctx, "tenant_a", "regression", "golden")
	if err != nil {
		t.Fatal(err)
	}

	evalRepo := NewMemoryRepository()
	_, err = (Optimizer{Runner: Runner{Datasets: dsSvc, Repository: evalRepo}}).Optimize(ctx, OptimizeRequest{
		TenantID:        "tenant_b",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Profiles:        []rag.Profile{rag.ProfileRealtime},
		TopKs:           []int{8},
	})
	if !apperrors.IsCode(err, apperrors.CodeNotFound) {
		t.Fatalf("Optimize() err = %v, want dataset not found", err)
	}
	if len(evalRepo.runs) != 0 || len(evalRepo.results) != 0 {
		t.Fatalf("foreign dataset optimization persisted runs=%d results=%d", len(evalRepo.runs), len(evalRepo.results))
	}
}
