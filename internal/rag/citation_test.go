package rag

import (
	"testing"

	"github.com/shikanon/orag/internal/kb"
)

func TestValidateCitations(t *testing.T) {
	valid, warnings := ValidateCitations([]Citation{{ChunkID: "a"}, {ChunkID: "b"}}, []kb.SearchResult{{Chunk: kb.Chunk{ID: "a"}}})
	if len(valid) != 1 || valid[0].ChunkID != "a" {
		t.Fatalf("unexpected valid citations: %#v", valid)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected warning, got %#v", warnings)
	}
}
