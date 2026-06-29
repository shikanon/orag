package rag

import (
	"strings"

	"github.com/shikanon/orag/internal/kb"
)

type ContextPacker struct {
	MaxTokens int
	TopN      int
}

func (p ContextPacker) Pack(results []kb.SearchResult) (string, []Citation) {
	maxTokens := p.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 6000
	}
	topN := p.TopN
	if topN <= 0 || topN > len(results) {
		topN = len(results)
	}
	var b strings.Builder
	var citations []Citation
	usedTokens := 0
	seen := map[string]bool{}
	for _, result := range results[:topN] {
		chunk := result.Chunk
		if seen[chunk.ID] {
			continue
		}
		seen[chunk.ID] = true
		tokens := len(strings.Fields(chunk.Content))
		if usedTokens+tokens > maxTokens && usedTokens > 0 {
			break
		}
		b.WriteString("[")
		b.WriteString(chunk.ID)
		b.WriteString("] ")
		b.WriteString(chunk.Content)
		b.WriteString("\n\n")
		citations = append(citations, Citation{
			ChunkID:    chunk.ID,
			DocumentID: chunk.DocumentID,
			SourceURI:  chunk.SourceURI,
			Section:    chunk.Section,
			Quote:      truncate(chunk.Content, 180),
		})
		usedTokens += tokens
	}
	return strings.TrimSpace(b.String()), citations
}

func truncate(s string, n int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= n {
		return string(runes)
	}
	return string(runes[:n])
}
