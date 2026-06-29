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
	item := dataset.Item{RelevantDocIDs: []string{"doc_1"}, GroundTruth: "qdrant"}
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
	if _, ok, err := runner.Get(ctx, "tenant_default", result.ID); err != nil || !ok {
		t.Fatalf("Get() ok=%v err=%v", ok, err)
	}
}

func TestRunnerRejectsCrossTenantDataset(t *testing.T) {
	ctx := context.Background()
	dsSvc := dataset.NewService()
	ds, err := dsSvc.Create(ctx, "tenant_a", "regression", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dsSvc.AddItem(ctx, "tenant_a", ds.ID, dataset.Item{Query: "q", GroundTruth: "a"}); err != nil {
		t.Fatal(err)
	}

	called := false
	runner := Runner{
		RAG:      &rag.Service{Pipeline: recordingPipeline{called: &called}},
		Datasets: dsSvc,
	}
	_, err = runner.Run(ctx, RunRequest{
		TenantID:        "tenant_b",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Profile:         rag.ProfileRealtime,
	})
	if !apperrors.IsCode(err, apperrors.CodeNotFound) {
		t.Fatalf("Run() err = %v, want not_found", err)
	}
	if called {
		t.Fatal("Run() queried RAG after dataset tenant check failed")
	}
}

func TestOptimizerRejectsCrossTenantDataset(t *testing.T) {
	ctx := context.Background()
	dsSvc := dataset.NewService()
	ds, err := dsSvc.Create(ctx, "tenant_a", "regression", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dsSvc.AddItem(ctx, "tenant_a", ds.ID, dataset.Item{Query: "q", GroundTruth: "a"}); err != nil {
		t.Fatal(err)
	}

	called := false
	_, err = (Optimizer{Runner: Runner{
		RAG:      &rag.Service{Pipeline: recordingPipeline{called: &called}},
		Datasets: dsSvc,
	}}).Optimize(ctx, OptimizeRequest{
		TenantID:        "tenant_b",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Profiles:        []rag.Profile{rag.ProfileRealtime},
		TopKs:           []int{1},
	})
	if !apperrors.IsCode(err, apperrors.CodeNotFound) {
		t.Fatalf("Optimize() err = %v, want not_found", err)
	}
	if called {
		t.Fatal("Optimize() queried RAG after dataset tenant check failed")
	}
}

type recordingPipeline struct {
	called *bool
}

func (p recordingPipeline) Invoke(context.Context, rag.QueryRequest) (rag.QueryResponse, error) {
	*p.called = true
	return rag.QueryResponse{}, errors.New("query should not run")
}
