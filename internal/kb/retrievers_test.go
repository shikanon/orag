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
