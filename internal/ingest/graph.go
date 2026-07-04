package ingest

import (
	"context"

	"github.com/shikanon/orag/internal/kb"
)

type GraphBuildRequest struct {
	Document kb.Document
	Chunks   []kb.Chunk
}

type LightweightGraphBuilder struct {
	MaxEntitiesPerChunk int
}

func (b LightweightGraphBuilder) Build(_ context.Context, req GraphBuildRequest) ([]kb.GraphRelation, []string, error) {
	limit := b.MaxEntitiesPerChunk
	if limit <= 0 {
		limit = 6
	}
	var relations []kb.GraphRelation
	for _, chunk := range req.Chunks {
		if chunk.Metadata["kind"] == "raptor_summary" {
			continue
		}
		entities := kb.ExtractGraphEntities(chunk.SearchText(), limit)
		if len(entities) < 2 {
			continue
		}
		for i := 0; i < len(entities); i++ {
			for j := i + 1; j < len(entities); j++ {
				relations = append(relations, kb.GraphRelation{
					TenantID:        req.Document.TenantID,
					KnowledgeBaseID: req.Document.KnowledgeBaseID,
					DocumentID:      req.Document.ID,
					SourceChunkID:   chunk.ID,
					TargetChunkID:   chunk.ID,
					Subject:         entities[i],
					Predicate:       "co_occurs_with",
					Object:          entities[j],
					Weight:          1,
				})
			}
		}
	}
	return relations, nil, nil
}
