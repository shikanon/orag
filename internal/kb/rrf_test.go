package kb

import "testing"

func TestRRFCombinesRanks(t *testing.T) {
	a := Chunk{ID: "a"}
	b := Chunk{ID: "b"}
	c := Chunk{ID: "c"}
	out := RRF([][]SearchResult{
		{{Chunk: a, Rank: 1}, {Chunk: b, Rank: 2}},
		{{Chunk: b, Rank: 1}, {Chunk: c, Rank: 2}},
	}, 60, 0)
	if len(out) != 3 {
		t.Fatalf("len(out) = %d", len(out))
	}
	if out[0].Chunk.ID != "b" {
		t.Fatalf("expected b first, got %s", out[0].Chunk.ID)
	}
}

func TestHybridRetriever(t *testing.T) {
	store := NewMemoryStore()
	doc := Document{ID: "doc", TenantID: "t", KnowledgeBaseID: "kb"}
	_ = store.Store(nil, doc, []Chunk{
		{ID: "c1", TenantID: "t", KnowledgeBaseID: "kb", DocumentID: "doc", Content: "hello rag framework", Vector: []float64{1, 0}},
		{ID: "c2", TenantID: "t", KnowledgeBaseID: "kb", DocumentID: "doc", Content: "other text", Vector: []float64{0, 1}},
	})
	retriever := HybridRetriever{
		Dense:  DenseRetriever{Store: store},
		Sparse: SparseRetriever{Store: store},
		RRFK:   60,
		TopN:   5,
	}
	out, err := retriever.Retrieve(nil, SearchRequest{
		TenantID:        "t",
		KnowledgeBaseID: "kb",
		Query:           "rag",
		Vector:          []float64{1, 0},
		TopK:            5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 || out[0].Chunk.ID != "c1" {
		t.Fatalf("unexpected results: %#v", out)
	}
}
