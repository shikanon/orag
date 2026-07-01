package qdrantstore

import (
	"context"
	"encoding/json"
	"time"

	qdrant "github.com/qdrant/go-client/qdrant"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
)

type SemanticCache struct {
	Client     *Client
	Collection string
	Threshold  float64
}

const semanticCachePayloadVersion = "v2"

func (s SemanticCache) Lookup(ctx context.Context, req rag.SemanticCacheLookupRequest) (rag.QueryResponse, bool, error) {
	if len(req.Vector) == 0 {
		return rag.QueryResponse{}, false, nil
	}
	threshold := req.Threshold
	if threshold <= 0 {
		threshold = s.Threshold
	}
	if threshold <= 0 {
		threshold = 0.92
	}
	resp, err := s.Client.Points.Search(ctx, &qdrant.SearchPoints{
		CollectionName: s.Collection,
		Vector:         float32Vector(req.Vector),
		Limit:          1,
		Filter: &qdrant.Filter{Must: append(knowledgeBaseFilter(req.TenantID, req.KnowledgeBaseID).Must,
			matchKeyword("cache_key_version", semanticCachePayloadVersion),
			matchKeyword("profile", string(semanticCacheProfile(req.Profile))),
			matchInteger("top_k", int64(req.TopK)),
		)},
		WithPayload: &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
	})
	if err != nil {
		return rag.QueryResponse{}, false, err
	}
	if len(resp.GetResult()) == 0 {
		return rag.QueryResponse{}, false, nil
	}
	point := resp.GetResult()[0]
	if float64(point.GetScore()) < threshold {
		return rag.QueryResponse{}, false, nil
	}
	return semanticCacheResponseFromPayload(point.GetPayload()), true, nil
}

func (s SemanticCache) Store(ctx context.Context, entry rag.SemanticCacheEntry) error {
	if len(entry.Vector) == 0 {
		return nil
	}
	wait := true
	profile := semanticCacheEntryProfile(entry)
	_, err := s.Client.Points.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: s.Collection,
		Wait:           &wait,
		Points: []*qdrant.PointStruct{{
			Id: pointID(rag.CacheKey(rag.QueryRequest{
				TenantID:        entry.TenantID,
				KnowledgeBaseID: entry.KnowledgeBaseID,
				Query:           entry.Query,
				Profile:         profile,
				TopK:            entry.TopK,
			})),
			Vectors: &qdrant.Vectors{VectorsOptions: &qdrant.Vectors_Vector{Vector: &qdrant.Vector{
				Data: float32Vector(entry.Vector),
			}}},
			Payload: semanticCachePayload(entry),
		}},
	})
	return err
}

func (s SemanticCache) DeleteKnowledgeBase(ctx context.Context, tenantID, kbID string) error {
	return deleteKnowledgeBasePoints(ctx, s.Client, s.Collection, tenantID, kbID)
}

func semanticCachePayload(entry rag.SemanticCacheEntry) map[string]*qdrant.Value {
	createdAt := entry.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	resp := entry.Response
	profile := semanticCacheEntryProfile(entry)
	return map[string]*qdrant.Value{
		"tenant_id":         stringValue(entry.TenantID),
		"knowledge_base_id": stringValue(entry.KnowledgeBaseID),
		"query":             stringValue(entry.Query),
		"cache_key_version": stringValue(semanticCachePayloadVersion),
		"profile":           stringValue(string(profile)),
		"top_k":             integerValue(int64(entry.TopK)),
		"answer":            stringValue(resp.Answer),
		"citations_json":    stringValue(mustMarshalString(resp.Citations)),
		"retrieved_json":    stringValue(mustMarshalString(resp.RetrievedChunks)),
		"created_at":        stringValue(createdAt.UTC().Format(time.RFC3339Nano)),
	}
}

func semanticCacheResponseFromPayload(payload map[string]*qdrant.Value) rag.QueryResponse {
	var citations []rag.Citation
	_ = json.Unmarshal([]byte(payloadString(payload, "citations_json")), &citations)
	var retrieved []kb.SearchResult
	_ = json.Unmarshal([]byte(payloadString(payload, "retrieved_json")), &retrieved)
	createdAt, _ := time.Parse(time.RFC3339Nano, payloadString(payload, "created_at"))
	return rag.QueryResponse{
		Answer:          payloadString(payload, "answer"),
		Citations:       citations,
		RetrievedChunks: retrieved,
		Profile:         rag.Profile(payloadString(payload, "profile")),
		CreatedAt:       createdAt,
	}
}

func mustMarshalString(v any) string {
	body, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(body)
}

func semanticCacheProfile(profile rag.Profile) rag.Profile {
	if profile == "" {
		return rag.ProfileRealtime
	}
	return profile
}

func semanticCacheEntryProfile(entry rag.SemanticCacheEntry) rag.Profile {
	if entry.Profile != "" {
		return entry.Profile
	}
	return semanticCacheProfile(entry.Response.Profile)
}

func matchInteger(key string, value int64) *qdrant.Condition {
	return &qdrant.Condition{ConditionOneOf: &qdrant.Condition_Field{Field: &qdrant.FieldCondition{
		Key: key,
		Match: &qdrant.Match{MatchValue: &qdrant.Match_Integer{
			Integer: value,
		}},
	}}}
}
