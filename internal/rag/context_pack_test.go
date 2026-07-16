package rag

import (
	"fmt"
	"testing"

	"github.com/shikanon/orag/internal/kb"
)

func TestContextPackerP8ReducesEvidencePackWithoutChangingOrder(t *testing.T) {
	results := make([]kb.SearchResult, 0, 5)
	for i := 1; i <= 5; i++ {
		results = append(results, kb.SearchResult{Chunk: kb.Chunk{
			ID: fmt.Sprintf("chk_%d", i), DocumentID: "doc_1", SourceURI: "memory://service", Section: fmt.Sprintf("section-%d", i), Content: fmt.Sprintf("evidence %d", i),
		}})
	}
	_, baseline := ContextPacker{TopN: 5, MaxTokens: 6000}.Pack(results)
	_, p8 := ContextPacker{TopN: 3, MaxTokens: 6000}.Pack(results)
	if len(baseline) != 5 || len(p8) != 3 {
		t.Fatalf("citations baseline=%#v p8=%#v", baseline, p8)
	}
	for index := range p8 {
		if p8[index].ChunkID != baseline[index].ChunkID || p8[index].SourceURI != baseline[index].SourceURI {
			t.Fatalf("citation %d baseline=%#v p8=%#v", index, baseline[index], p8[index])
		}
	}
}
