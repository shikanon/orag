package qdrantstore

import (
	"context"
	"errors"

	qdrant "github.com/qdrant/go-client/qdrant"
	"github.com/shikanon/orag/internal/kb"
)

var ErrVisibilityFilterRequired = errors.New("qdrant vector retrieval requires a searchable chunk filter")

type VectorStore struct {
	Client     *Client
	Collection string
	Visibility kb.SearchableChunkFilter
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

func (s VectorStore) DeleteKnowledgeBaseVectors(ctx context.Context, tenantID, kbID string) error {
	wait := true
	_, err := s.Client.Points.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.Collection,
		Wait:           &wait,
		Points: &qdrant.PointsSelector{PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
			Filter: knowledgeBaseFilter(tenantID, kbID),
		}},
	})
	return err
}

func (s VectorStore) DeleteDocumentSource(ctx context.Context, tenantID, kbID, sourceURI string) error {
	wait := true
	_, err := s.Client.Points.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.Collection,
		Wait:           &wait,
		Points: &qdrant.PointsSelector{PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
			Filter: documentSourceFilter(tenantID, kbID, sourceURI),
		}},
	})
	return err
}

func (s VectorStore) PrepareActivation(ctx context.Context, doc kb.Document, _ []kb.Chunk) error {
	return s.setDocumentSearchable(ctx, doc, true)
}

func (s VectorStore) CommitActivation(context.Context, kb.Document, []kb.Chunk) error {
	return nil
}

func (s VectorStore) AbortActivation(ctx context.Context, doc kb.Document, _ []kb.Chunk) error {
	return s.setDocumentSearchable(ctx, doc, false)
}

func (s VectorStore) FinalizeActivation(ctx context.Context, doc kb.Document, chunks []kb.Chunk) error {
	if len(chunks) == 0 || doc.SourceURI == "" {
		return nil
	}
	wait := true
	_, err := s.Client.Points.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.Collection,
		Wait:           &wait,
		Points: &qdrant.PointsSelector{PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
			Filter: documentSourceExceptDocumentFilter(doc.TenantID, doc.KnowledgeBaseID, doc.SourceURI, doc.ID),
		}},
	})
	return err
}

func (s VectorStore) setDocumentSearchable(ctx context.Context, doc kb.Document, searchable bool) error {
	wait := true
	_, err := s.Client.Points.SetPayload(ctx, &qdrant.SetPayloadPoints{
		CollectionName: s.Collection,
		Wait:           &wait,
		Payload:        map[string]*qdrant.Value{"searchable": boolValue(searchable)},
		PointsSelector: &qdrant.PointsSelector{PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
			Filter: documentFilter(doc.TenantID, doc.KnowledgeBaseID, doc.ID),
		}},
	})
	return err
}

func (s VectorStore) Retrieve(ctx context.Context, req kb.SearchRequest) ([]kb.SearchResult, error) {
	if s.Visibility == nil {
		return nil, ErrVisibilityFilterRequired
	}
	limit := req.TopK
	if req.DenseTopK > 0 {
		limit = req.DenseTopK
	}
	if limit <= 0 {
		limit = 50
	}
	pageSize := max(limit*2, 32)
	scanCap := max(limit*8, 256)
	results := make([]kb.SearchResult, 0, limit)
	var offset uint64
	for scanned := 0; scanned < scanCap && len(results) < limit; {
		requestLimit := min(pageSize, scanCap-scanned)
		requestOffset := offset
		resp, err := s.Client.Points.Search(ctx, &qdrant.SearchPoints{
			CollectionName: s.Collection,
			Vector:         float32Vector(req.Vector),
			Limit:          uint64(requestLimit),
			Offset:         &requestOffset,
			Filter:         knowledgeBaseFilter(req.TenantID, req.KnowledgeBaseID),
			WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
		})
		if err != nil {
			return nil, err
		}
		points := resp.GetResult()
		if len(points) == 0 {
			break
		}

		chunkIDs := make([]string, 0, len(points))
		for _, point := range points {
			chunkIDs = append(chunkIDs, payloadString(point.GetPayload(), "chunk_id"))
		}
		active, err := s.Visibility.FilterSearchableChunkIDs(ctx, req.TenantID, req.KnowledgeBaseID, chunkIDs)
		if err != nil {
			return nil, err
		}
		for idx, point := range points {
			if _, ok := active[chunkIDs[idx]]; !ok {
				continue
			}
			results = append(results, kb.SearchResult{
				Chunk: chunkFromPayload(point.GetPayload()),
				Score: float64(point.GetScore()),
				Rank:  len(results) + 1,
				From:  "qdrant_dense",
			})
			if len(results) == limit {
				break
			}
		}

		scanned += len(points)
		offset += uint64(len(points))
		if len(points) < requestLimit {
			break
		}
	}
	return results, nil
}

func knowledgeBaseFilter(tenantID, kbID string) *qdrant.Filter {
	return &qdrant.Filter{Must: []*qdrant.Condition{
		matchKeyword("tenant_id", tenantID),
		matchKeyword("knowledge_base_id", kbID),
	}}
}

func documentSourceFilter(tenantID, kbID, sourceURI string) *qdrant.Filter {
	return &qdrant.Filter{Must: []*qdrant.Condition{
		matchKeyword("tenant_id", tenantID),
		matchKeyword("knowledge_base_id", kbID),
		matchKeyword("source_uri", sourceURI),
	}}
}

func documentFilter(tenantID, kbID, documentID string) *qdrant.Filter {
	return &qdrant.Filter{Must: []*qdrant.Condition{
		matchKeyword("tenant_id", tenantID),
		matchKeyword("knowledge_base_id", kbID),
		matchKeyword("document_id", documentID),
	}}
}

func documentSourceExceptDocumentFilter(tenantID, kbID, sourceURI, documentID string) *qdrant.Filter {
	filter := documentSourceFilter(tenantID, kbID, sourceURI)
	filter.MustNot = []*qdrant.Condition{matchKeyword("document_id", documentID)}
	return filter
}

func matchKeyword(key, value string) *qdrant.Condition {
	return &qdrant.Condition{ConditionOneOf: &qdrant.Condition_Field{Field: &qdrant.FieldCondition{
		Key: key,
		Match: &qdrant.Match{MatchValue: &qdrant.Match_Keyword{
			Keyword: value,
		}},
	}}}
}
