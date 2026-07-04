package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/shikanon/orag/internal/kb"
)

func TestFTSRetrieverUsesContextualSearchVector(t *testing.T) {
	queryer := &fakeKnowledgeBaseQueryer{queryRows: &fakeTraceRows{rows: [][]any{{
		"chunk_1",
		"tenant_1",
		"kb_1",
		"doc_1",
		"it reduced latency by 30 percent",
		"Qdrant rollout performance benchmark",
		"memory://doc.md",
		0,
		"",
		0,
		[]byte(`{"source":"test"}`),
		float64(0.42),
	}}}}
	retriever := FTSRetriever{queryer: queryer}

	results, err := retriever.Retrieve(context.Background(), kb.SearchRequest{
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		Query:           "qdrant benchmark",
		TopK:            5,
	})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if !strings.Contains(queryer.querySQL, "search_text_tsvector") {
		t.Fatalf("FTS SQL should use contextual search vector: %s", queryer.querySQL)
	}
	if len(results) != 1 || results[0].Chunk.ContextualText != "Qdrant rollout performance benchmark" {
		t.Fatalf("results = %#v", results)
	}
	if results[0].Chunk.Content != "it reduced latency by 30 percent" {
		t.Fatalf("content should remain original chunk text: %#v", results[0].Chunk)
	}
}
