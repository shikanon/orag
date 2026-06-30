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
