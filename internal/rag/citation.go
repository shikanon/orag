package rag

import "github.com/shikanon/orag/internal/kb"

func ValidateCitations(citations []Citation, results []kb.SearchResult) ([]Citation, []string) {
	valid := map[string]bool{}
	for _, result := range results {
		valid[result.Chunk.ID] = true
	}
	out := make([]Citation, 0, len(citations))
	var warnings []string
	for _, citation := range citations {
		if valid[citation.ChunkID] {
			out = append(out, citation)
			continue
		}
		warnings = append(warnings, "citation "+citation.ChunkID+" not found in retrieved context")
	}
	return out, warnings
}
