package qdrantstore

import (
	"context"

	qdrant "github.com/qdrant/go-client/qdrant"
	"github.com/shikanon/orag/internal/kb"
)

type VectorStore struct {
	Client     *Client
	Collection string
}

func (s VectorStore) Store(ctx context.Context, _ kb.Document, chunks []kb.Chunk) error {
	points := make([]*qdrant.PointStruct, 0, len(chunks))
	for _, chunk := range chunks {
		if len(chunk.Vector) == 0 {
			continue
		}
		points = append(points, &qdrant.PointStruct{
			Id: pointID(chunk.ID),
			Vectors: &qdrant.Vectors{VectorsOptions: &qdrant.Vectors_Vector{Vector: &qdrant.Vector{
				Data: float32Vector(chunk.Vector),
			}}},
			Payload: chunkPayload(chunk),
		})
	}
	if len(points) == 0 {
		return nil
	}
	wait := true
	_, err := s.Client.Points.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: s.Collection,
		Wait:           &wait,
		Points:         points,
	})
	return err
}

func (s VectorStore) Retrieve(ctx context.Context, req kb.SearchRequest) ([]kb.SearchResult, error) {
	limit := req.TopK
	if req.DenseTopK > 0 {
		limit = req.DenseTopK
	}
	if limit <= 0 {
		limit = 50
	}
	resp, err := s.Client.Points.Search(ctx, &qdrant.SearchPoints{
		CollectionName: s.Collection,
		Vector:         float32Vector(req.Vector),
		Limit:          uint64(limit),
		Filter:         knowledgeBaseFilter(req.TenantID, req.KnowledgeBaseID),
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
	})
	if err != nil {
		return nil, err
	}
	results := make([]kb.SearchResult, 0, len(resp.GetResult()))
	for i, point := range resp.GetResult() {
		results = append(results, kb.SearchResult{
			Chunk: chunkFromPayload(point.GetPayload()),
			Score: float64(point.GetScore()),
			Rank:  i + 1,
			From:  "qdrant_dense",
		})
	}
	return results, nil
}

func (s VectorStore) DeleteKnowledgeBasePoints(ctx context.Context, tenantID, kbID string) error {
	_, err := s.Client.Points.Delete(ctx, deleteKnowledgeBasePointsRequest(s.Collection, tenantID, kbID))
	return err
}

func deleteKnowledgeBasePointsRequest(collection, tenantID, kbID string) *qdrant.DeletePoints {
	wait := true
	return &qdrant.DeletePoints{
		CollectionName: collection,
		Wait:           &wait,
		Points: &qdrant.PointsSelector{PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
			Filter: knowledgeBaseFilter(tenantID, kbID),
		}},
	}
}

func knowledgeBaseFilter(tenantID, kbID string) *qdrant.Filter {
	return &qdrant.Filter{Must: []*qdrant.Condition{
		matchKeyword("tenant_id", tenantID),
		matchKeyword("knowledge_base_id", kbID),
	}}
}

func matchKeyword(key, value string) *qdrant.Condition {
	return &qdrant.Condition{ConditionOneOf: &qdrant.Condition_Field{Field: &qdrant.FieldCondition{
		Key: key,
		Match: &qdrant.Match{MatchValue: &qdrant.Match_Keyword{
			Keyword: value,
		}},
	}}}
}
