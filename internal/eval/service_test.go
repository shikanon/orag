package eval

import (
	"context"
	"testing"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/llm/ark"
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

func TestRunnerPersistsRunInMemoryRepository(t *testing.T) {
	ctx := context.Background()
	dsRepo := dataset.NewMemoryRepository()
	dsSvc := dataset.NewService(dsRepo)
	ds, err := dsSvc.Create(ctx, "tenant_default", "regression", "golden")
	if err != nil {
		t.Fatal(err)
	}
	_, err = dsSvc.AddItem(ctx, ds.ID, dataset.Item{
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
}
