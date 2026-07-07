package qdrantstore

import (
	"context"
	"encoding/json"
	"strings"
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

const (
	semanticCachePayloadVersion           = "v2"
	semanticCacheNamespacedPayloadVersion = "v3"
)

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
		Filter:         semanticCacheLookupFilter(req),
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
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
	cached, ok := semanticCacheLookupResponseFromPayload(req, point.GetPayload())
	return cached, ok, nil
}

func (s SemanticCache) Store(ctx context.Context, entry rag.SemanticCacheEntry) error {
	if len(entry.Vector) == 0 {
		return nil
	}
	wait := true
	_, err := s.Client.Points.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: s.Collection,
		Wait:           &wait,
		Points: []*qdrant.PointStruct{{
			Id: semanticCachePointID(entry),
			Vectors: &qdrant.Vectors{VectorsOptions: &qdrant.Vectors_Vector{Vector: &qdrant.Vector{
				Data: float32Vector(entry.Vector),
			}}},
			Payload: semanticCachePayload(entry),
		}},
	})
	return err
}

func (s SemanticCache) DeleteKnowledgeBaseSemanticCache(ctx context.Context, tenantID, kbID string) error {
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

func semanticCacheLookupFilter(req rag.SemanticCacheLookupRequest) *qdrant.Filter {
	version := semanticCachePayloadVersion
	if semanticCacheNamespace(req.Namespace) != "" {
		version = semanticCacheNamespacedPayloadVersion
	}
	must := []*qdrant.Condition{
		matchKeyword("tenant_id", req.TenantID),
		matchKeyword("knowledge_base_id", req.KnowledgeBaseID),
		matchKeyword("cache_key_version", version),
		matchKeyword("profile", string(semanticCacheProfile(req.Profile))),
		matchInteger("top_k", int64(req.TopK)),
	}
	if namespace := semanticCacheNamespace(req.Namespace); namespace != "" {
		must = append(must, matchKeyword("namespace", namespace))
	}
	return &qdrant.Filter{Must: must}
}

func semanticCachePointID(entry rag.SemanticCacheEntry) *qdrant.PointId {
	return pointID(semanticCachePointKey(entry))
}

func semanticCachePointKey(entry rag.SemanticCacheEntry) string {
	return rag.NamespacedCacheKey(semanticCacheNamespace(entry.Namespace), rag.QueryRequest{
		TenantID:        entry.TenantID,
		KnowledgeBaseID: entry.KnowledgeBaseID,
		Query:           entry.Query,
		Profile:         semanticCacheEntryProfile(entry),
		TopK:            entry.TopK,
	})
}

func semanticCachePayload(entry rag.SemanticCacheEntry) map[string]*qdrant.Value {
	createdAt := entry.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	resp := entry.Response
	profile := semanticCacheEntryProfile(entry)
	payload := map[string]*qdrant.Value{
		"tenant_id":         stringValue(entry.TenantID),
		"knowledge_base_id": stringValue(entry.KnowledgeBaseID),
		"cache_key_version": stringValue(semanticCachePayloadVersion),
		"query":             stringValue(entry.Query),
		"profile":           stringValue(string(profile)),
		"top_k":             integerValue(int64(entry.TopK)),
		"answer":            stringValue(resp.Answer),
		"citations_json":    stringValue(mustMarshalString(resp.Citations)),
		"retrieved_json":    stringValue(mustMarshalString(resp.RetrievedChunks)),
		"created_at":        stringValue(createdAt.UTC().Format(time.RFC3339Nano)),
	}
	if namespace := semanticCacheNamespace(entry.Namespace); namespace != "" {
		payload["cache_key_version"] = stringValue(semanticCacheNamespacedPayloadVersion)
		payload["namespace"] = stringValue(namespace)
	}
	return payload
}

func semanticCacheLookupResponseFromPayload(req rag.SemanticCacheLookupRequest, payload map[string]*qdrant.Value) (rag.QueryResponse, bool) {
	profile := payloadString(payload, "profile")
	if profile == "" || rag.Profile(profile) != semanticCacheProfile(req.Profile) {
		return rag.QueryResponse{}, false
	}
	payloadNamespace := payloadString(payload, "namespace")
	requestNamespace := semanticCacheNamespace(req.Namespace)
	if requestNamespace != "" {
		if payloadNamespace != requestNamespace {
			return rag.QueryResponse{}, false
		}
	} else if payloadNamespace != "" {
		return rag.QueryResponse{}, false
	}
	return semanticCacheResponseFromPayload(payload), true
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

func semanticCacheNamespace(namespace string) string {
	return strings.TrimSpace(namespace)
}

func matchInteger(key string, value int64) *qdrant.Condition {
	return &qdrant.Condition{ConditionOneOf: &qdrant.Condition_Field{Field: &qdrant.FieldCondition{
		Key: key,
		Match: &qdrant.Match{MatchValue: &qdrant.Match_Integer{
			Integer: value,
		}},
	}}}
}
