package eval

import (
	"context"
	"errors"
	"testing"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/platform/apperrors"
	"github.com/shikanon/orag/internal/rag"
)

func TestOptimizerUsesAnswerAccuracy(t *testing.T) {
	ctx := context.Background()
	dsRepo := dataset.NewMemoryRepository()
	dsSvc := dataset.NewService(dsRepo)
	ds, err := dsSvc.Create(ctx, "tenant_default", "optimizer-answer", "golden")
	if err != nil {
		t.Fatal(err)
	}
	_, err = dsSvc.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{
		Query:       "qdrant vector",
		GroundTruth: "qdrant",
	})
	if err != nil {
		t.Fatal(err)
	}

	ragSvc := &rag.Service{
		Pipeline: pipelineFunc(func(_ context.Context, req rag.QueryRequest) (rag.QueryResponse, error) {
			if req.Profile == rag.ProfileHighPrecision {
				return rag.QueryResponse{Answer: "qdrant answer", CacheStatus: "miss"}, nil
			}
			return rag.QueryResponse{
				Answer: "postgres answer",
				Citations: []rag.Citation{{
					ChunkID:    "chk_1",
					DocumentID: "doc_1",
				}},
				CacheStatus: "miss",
			}, nil
		}),
	}
	runner := Runner{RAG: ragSvc, Datasets: dsSvc, Repository: NewMemoryRepository()}
	result, err := (Optimizer{Runner: runner}).Optimize(ctx, OptimizeRequest{
		TenantID:        "tenant_default",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Profiles:        []rag.Profile{rag.ProfileRealtime, rag.ProfileHighPrecision},
		TopKs:           []int{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(result.Candidates))
	}
	if result.Candidates[0].Profile != rag.ProfileRealtime || result.Candidates[0].Score != 0 {
		t.Fatalf("citation-only candidate = %#v, want realtime score 0", result.Candidates[0])
	}
	if result.Candidates[1].Profile != rag.ProfileHighPrecision || result.Candidates[1].Score != 1 {
		t.Fatalf("answer-correct candidate = %#v, want high_precision score 1", result.Candidates[1])
	}
	if result.Best.Profile != rag.ProfileHighPrecision || result.Best.Score != 1 {
		t.Fatalf("best = %#v, want high_precision score 1", result.Best)
	}
}

func TestOptimizerRequiresDatasetTenant(t *testing.T) {
	ctx := context.Background()
	dsSvc := dataset.NewService(dataset.NewMemoryRepository())
	ds, err := dsSvc.Create(ctx, "tenant_a", "optimizer-tenant", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dsSvc.Create(ctx, "tenant_b", "tenant-b-regression", "golden"); err != nil {
		t.Fatal(err)
	}
	if _, err := dsSvc.AddItem(ctx, "tenant_a", ds.ID, dataset.Item{Query: "q", GroundTruth: "a"}); err != nil {
		t.Fatal(err)
	}

	runner := Runner{Datasets: dsSvc}
	result, err := (Optimizer{Runner: runner}).Optimize(ctx, OptimizeRequest{
		TenantID:        "tenant_b",
		DatasetID:       ds.ID,
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
