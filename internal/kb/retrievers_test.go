package kb

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubRetriever struct {
	results []SearchResult
	err     error
}

func (r stubRetriever) Retrieve(context.Context, SearchRequest) ([]SearchResult, error) {
	return r.results, r.err
}

type capturingRetriever struct {
	requests []SearchRequest
	results  []SearchResult
}

func (r *capturingRetriever) Retrieve(_ context.Context, req SearchRequest) ([]SearchResult, error) {
	r.requests = append(r.requests, req)
	limit := req.TopK
	if req.DenseTopK > 0 {
		limit = req.DenseTopK
	}
	if req.SparseTopK > 0 {
		limit = req.SparseTopK
	}
	return top(append([]SearchResult(nil), r.results...), limit), nil
}

func TestHybridRetrieverReturnsWarningsOnSingleSideFailure(t *testing.T) {
	hybrid := HybridRetriever{
		Dense: stubRetriever{err: errors.New("qdrant down")},
		Sparse: stubRetriever{results: []SearchResult{{
			Chunk: Chunk{ID: "chk_1", Content: "postgres fts"},
			Score: 1,
			Rank:  1,
			From:  "postgres_fts",
		}}},
		RRFK: 60,
		TopN: 8,
	}
	results, warnings, err := hybrid.RetrieveWithWarnings(context.Background(), SearchRequest{TopK: 8})
	if err != nil {
		t.Fatalf("RetrieveWithWarnings() error = %v", err)
	}
	if len(results) != 1 || results[0].Chunk.ID != "chk_1" {
		t.Fatalf("results = %#v", results)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "dense retrieval failed") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestSparseRetrieverSearchesContextualText(t *testing.T) {
	store := NewMemoryStore()
	if err := store.Store(context.Background(), Document{
		ID:              "doc_1",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		SourceURI:       "memory://doc.md",
		Title:           "doc.md",
		ContentHash:     "hash",
	}, []Chunk{{
		ID:              "chunk_1",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      "doc_1",
		Content:         "it reduced latency by 30 percent",
		ContextualText:  "Qdrant rollout performance benchmark",
		SourceURI:       "memory://doc.md",
	}}); err != nil {
		t.Fatal(err)
	}

	results, err := (SparseRetriever{Store: store}).Retrieve(context.Background(), SearchRequest{
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		Query:           "qdrant benchmark",
		TopK:            5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Chunk.ID != "chunk_1" {
		t.Fatalf("results = %#v", results)
	}
	if results[0].Chunk.Content != "it reduced latency by 30 percent" {
		t.Fatalf("retrieved content should remain original chunk content: %#v", results[0].Chunk)
	}
}

func TestHybridRetrieverFailsWhenBothSidesFail(t *testing.T) {
	hybrid := HybridRetriever{
		Dense:  stubRetriever{err: errors.New("qdrant down")},
		Sparse: stubRetriever{err: errors.New("postgres down")},
		RRFK:   60,
		TopN:   8,
	}
	_, warnings, err := hybrid.RetrieveWithWarnings(context.Background(), SearchRequest{TopK: 8})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestHybridRetrieverMergesAndRanksStable(t *testing.T) {
	hybrid := HybridRetriever{
		Dense: stubRetriever{results: []SearchResult{
			{Chunk: Chunk{ID: "chk_1"}, Rank: 1, Score: 0.9, From: "dense"},
			{Chunk: Chunk{ID: "chk_2"}, Rank: 2, Score: 0.8, From: "dense"},
		}},
		Sparse: stubRetriever{results: []SearchResult{
			{Chunk: Chunk{ID: "chk_2"}, Rank: 1, Score: 1.0, From: "sparse"},
			{Chunk: Chunk{ID: "chk_3"}, Rank: 2, Score: 0.7, From: "sparse"},
		}},
		RRFK: 60,
		TopN: 8,
	}
	results, warnings, err := hybrid.RetrieveWithWarnings(context.Background(), SearchRequest{TopK: 8})
	if err != nil {
		t.Fatalf("RetrieveWithWarnings() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d", len(results))
	}
	if results[0].Chunk.ID != "chk_2" {
		t.Fatalf("top result = %#v", results[0])
	}
}

func TestHybridRetrieverRequestTopKOverridesConfiguredCandidateTopK(t *testing.T) {
	dense := &capturingRetriever{results: rankedResults("dense", 20)}
	sparse := &capturingRetriever{results: rankedResults("sparse", 20)}
	hybrid := HybridRetriever{
		Dense:      dense,
		Sparse:     sparse,
		RRFK:       60,
		TopN:       50,
		DenseTopK:  50,
		SparseTopK: 50,
	}

	results, warnings, err := hybrid.RetrieveWithWarnings(context.Background(), SearchRequest{TopK: 5})
	if err != nil {
		t.Fatalf("RetrieveWithWarnings() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(dense.requests) != 1 {
		t.Fatalf("dense requests = %#v, want exactly one", dense.requests)
	}
	if got := dense.requests[0]; got.TopK != 5 || got.DenseTopK != 0 {
		t.Fatalf("dense request = %#v, want TopK 5 without configured DenseTopK override", got)
	}
	if len(sparse.requests) != 1 {
		t.Fatalf("sparse requests = %#v, want exactly one", sparse.requests)
	}
	if got := sparse.requests[0]; got.TopK != 5 || got.SparseTopK != 0 {
		t.Fatalf("sparse request = %#v, want TopK 5 without configured SparseTopK override", got)
	}
	if len(results) != 5 {
		t.Fatalf("len(results) = %d, want 5", len(results))
	}
}

func TestHybridRetrieverUsesConfiguredCandidateTopKWhenRequestTopKAbsent(t *testing.T) {
	dense := &capturingRetriever{results: rankedResults("dense", 60)}
	sparse := &capturingRetriever{results: rankedResults("sparse", 60)}
	hybrid := HybridRetriever{
		Dense:      dense,
		Sparse:     sparse,
		RRFK:       60,
		TopN:       7,
		DenseTopK:  50,
		SparseTopK: 40,
	}

	results, warnings, err := hybrid.RetrieveWithWarnings(context.Background(), SearchRequest{})
	if err != nil {
		t.Fatalf("RetrieveWithWarnings() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(dense.requests) != 1 {
		t.Fatalf("dense requests = %#v, want exactly one", dense.requests)
	}
	if got := dense.requests[0]; got.TopK != 50 || got.DenseTopK != 50 {
		t.Fatalf("dense request = %#v, want configured DenseTopK 50", got)
	}
	if len(sparse.requests) != 1 {
		t.Fatalf("sparse requests = %#v, want exactly one", sparse.requests)
	}
	if got := sparse.requests[0]; got.TopK != 40 || got.SparseTopK != 40 {
		t.Fatalf("sparse request = %#v, want configured SparseTopK 40", got)
	}
	if len(results) != 7 {
		t.Fatalf("len(results) = %d, want configured TopN 7", len(results))
	}
}

func TestHybridRetrieverRequestTopKCapsRRFWhenDefaultsConfigured(t *testing.T) {
	hybrid := HybridRetriever{
		Dense:      stubRetriever{results: rankedResults("dense", 8)},
		Sparse:     stubRetriever{results: rankedResults("sparse", 8)},
		RRFK:       60,
		TopN:       8,
		DenseTopK:  8,
		SparseTopK: 8,
	}
	results, warnings, err := hybrid.RetrieveWithWarnings(context.Background(), SearchRequest{TopK: 5})
	if err != nil {
		t.Fatalf("RetrieveWithWarnings() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(results) != 5 {
		t.Fatalf("len(results) = %d, want 5", len(results))
	}
}

func TestHybridRetrieverFallsBackToConfiguredTopNWhenRequestTopKAbsent(t *testing.T) {
	hybrid := HybridRetriever{
		Dense:  stubRetriever{results: rankedResults("dense", 6)},
		Sparse: stubRetriever{results: rankedResults("sparse", 6)},
		RRFK:   60,
		TopN:   4,
	}
	results, warnings, err := hybrid.RetrieveWithWarnings(context.Background(), SearchRequest{})
	if err != nil {
		t.Fatalf("RetrieveWithWarnings() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(results) != 4 {
		t.Fatalf("len(results) = %d, want 4", len(results))
	}
}

func rankedResults(prefix string, n int) []SearchResult {
	results := make([]SearchResult, 0, n)
	for i := 1; i <= n; i++ {
		id := prefix + "_" + string(rune('a'+i-1))
		results = append(results, SearchResult{
			Chunk: Chunk{ID: id, Content: id},
			Score: float64(n - i + 1),
			Rank:  i,
			From:  prefix,
		})
	}
	return results
}
