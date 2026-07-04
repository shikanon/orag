package ingest

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/llm/ark"
)

type RAPTORRequest struct {
	Document kb.Document
	Chunks   []kb.Chunk
}

type LLMRAPTORBuilder struct {
	Model           ChatModel
	BranchFactor    int
	MaxLevels       int
	MaxSummaryChars int
}

func (b LLMRAPTORBuilder) Build(ctx context.Context, req RAPTORRequest) ([]kb.Chunk, []string, error) {
	if len(req.Chunks) == 0 {
		return nil, nil, nil
	}
	if b.Model == nil {
		return nil, []string{"RAPTOR indexing skipped: model unavailable"}, nil
	}
	branchFactor := defaultInt(b.BranchFactor, 4)
	maxLevels := defaultInt(b.MaxLevels, 2)
	if branchFactor < 2 {
		branchFactor = 2
	}
	var summaries []kb.Chunk
	levelNodes := append([]kb.Chunk(nil), req.Chunks...)
	for level := 1; level <= maxLevels && len(levelNodes) > 1; level++ {
		groups := chunkGroups(levelNodes, branchFactor)
		next := make([]kb.Chunk, 0, len(groups))
		for groupIndex, group := range groups {
			if len(group) == 0 {
				continue
			}
			summary, err := b.summarize(ctx, req.Document.Title, group)
			if err != nil {
				return summaries, nil, err
			}
			node := kb.Chunk{
				ID:              raptorChunkID(req.Document.ID, level, groupIndex),
				TenantID:        req.Document.TenantID,
				KnowledgeBaseID: req.Document.KnowledgeBaseID,
				DocumentID:      req.Document.ID,
				Content:         cleanContextualText(summary, defaultInt(b.MaxSummaryChars, 1000)),
				SourceURI:       req.Document.SourceURI,
				Section:         fmt.Sprintf("raptor:level:%d", level),
				Metadata: map[string]string{
					"kind":            "raptor_summary",
					"level":           strconv.Itoa(level),
					"child_chunk_ids": strings.Join(chunkIDs(group), ","),
				},
			}
			if node.Content == "" {
				continue
			}
			summaries = append(summaries, node)
			next = append(next, node)
		}
		levelNodes = next
	}
	return summaries, nil, nil
}

func (b LLMRAPTORBuilder) summarize(ctx context.Context, documentTitle string, chunks []kb.Chunk) (string, error) {
	var body strings.Builder
	for _, chunk := range chunks {
		if body.Len() > 0 {
			body.WriteString("\n\n")
		}
		body.WriteString("- ")
		body.WriteString(chunk.SearchText())
	}
	return b.Model.Chat(ctx, []ark.ChatMessage{
		{Role: "system", Content: "你是 RAPTOR 分层索引摘要器。请把一组相邻或相似 chunk 压缩成可检索的中文摘要，只输出摘要。"},
		{Role: "user", Content: fmt.Sprintf("document_title: %s\n\nchunks:\n%s", documentTitle, body.String())},
	})
}

func chunkGroups(chunks []kb.Chunk, size int) [][]kb.Chunk {
	var groups [][]kb.Chunk
	for start := 0; start < len(chunks); start += size {
		end := start + size
		if end > len(chunks) {
			end = len(chunks)
		}
		groups = append(groups, chunks[start:end])
	}
	return groups
}

func chunkIDs(chunks []kb.Chunk) []string {
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, chunk.ID)
	}
	return out
}

func raptorChunkID(docID string, level, groupIndex int) string {
	return fmt.Sprintf("%s_raptor_l%d_%d", docID, level, groupIndex)
}
