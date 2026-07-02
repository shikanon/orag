package qdrantstore

import (
	"context"
	"testing"
	"time"

	qdrant "github.com/qdrant/go-client/qdrant"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
	"google.golang.org/grpc"
)

func TestSemanticCachePayloadRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	entry := rag.SemanticCacheEntry{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant cache",
		Profile:         rag.ProfileRealtime,
		TopK:            8,
		CreatedAt:       now,
		Response: rag.QueryResponse{
			Answer:  "cached answer",
			Profile: rag.ProfileRealtime,
			Citations: []rag.Citation{{
				ChunkID:    "chk_1",
				DocumentID: "doc_1",
				SourceURI:  "memory://doc",
			}},
			RetrievedChunks: []kb.SearchResult{{
				Chunk: kb.Chunk{
					ID:              "chk_1",
					TenantID:        "tenant_default",
					KnowledgeBaseID: "kb_default",
					DocumentID:      "doc_1",
					Content:         "Qdrant semantic cache content",
				},
				Score: 0.99,
				Rank:  1,
				From:  "qdrant_dense",
			}},
		},
	}

	payload := semanticCachePayload(entry)
	if got := payloadString(payload, "cache_key_version"); got != semanticCachePayloadVersion {
		t.Fatalf("cache_key_version = %q", got)
	}
	if got := payloadString(payload, "profile"); got != string(entry.Profile) {
		t.Fatalf("payload profile = %q", got)
	}
	if got := payload["top_k"].GetIntegerValue(); got != int64(entry.TopK) {
		t.Fatalf("payload top_k = %d", got)
	}
	resp := semanticCacheResponseFromPayload(payload)
	if resp.Answer != entry.Response.Answer {
		t.Fatalf("answer = %q", resp.Answer)
	}
	if resp.Profile != rag.ProfileRealtime {
		t.Fatalf("profile = %q", resp.Profile)
	}
	if len(resp.Citations) != 1 || resp.Citations[0].ChunkID != "chk_1" {
		t.Fatalf("citations roundtrip failed: %#v", resp.Citations)
	}
	if len(resp.RetrievedChunks) != 1 || resp.RetrievedChunks[0].Chunk.ID != "chk_1" {
		t.Fatalf("retrieved chunks roundtrip failed: %#v", resp.RetrievedChunks)
	}
	if !resp.CreatedAt.Equal(now) {
		t.Fatalf("created_at = %s", resp.CreatedAt)
	}
}

func TestSemanticCacheLookupIsolatesProfile(t *testing.T) {
	ctx := context.Background()
	points := &semanticCacheFakePointsClient{}
	cache := SemanticCache{
		Client:     &Client{Points: points},
		Collection: "semantic_cache",
		Threshold:  0.92,
	}
	entry := rag.SemanticCacheEntry{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
		Vector:          []float64{0.1, 0.2, 0.3},
		Profile:         rag.ProfileRealtime,
		TopK:            8,
		Response: rag.QueryResponse{
			Answer:  "cached realtime answer",
			Profile: rag.ProfileRealtime,
			Citations: []rag.Citation{{
				ChunkID:    "chk_1",
				DocumentID: "doc_1",
				SourceURI:  "memory://doc",
			}},
		},
	}
	if err := cache.Store(ctx, entry); err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if len(points.points) != 1 {
		t.Fatalf("stored points = %d, want 1", len(points.points))
	}
	realtimePointID := pointID(rag.CacheKey(rag.QueryRequest{
		TenantID:        entry.TenantID,
		KnowledgeBaseID: entry.KnowledgeBaseID,
		Query:           entry.Query,
		Profile:         rag.ProfileRealtime,
		TopK:            entry.TopK,
	})).GetNum()
	highPrecisionPointID := pointID(rag.CacheKey(rag.QueryRequest{
		TenantID:        entry.TenantID,
		KnowledgeBaseID: entry.KnowledgeBaseID,
		Query:           entry.Query,
		Profile:         rag.ProfileHighPrecision,
		TopK:            entry.TopK,
	})).GetNum()
	if realtimePointID == highPrecisionPointID {
		t.Fatalf("point id should differ by profile")
	}
	if got := points.points[0].GetId().GetNum(); got != realtimePointID {
		t.Fatalf("stored point id = %d, want %d", got, realtimePointID)
	}

	_, ok, err := cache.Lookup(ctx, rag.SemanticCacheLookupRequest{
		TenantID:        entry.TenantID,
		KnowledgeBaseID: entry.KnowledgeBaseID,
		Query:           entry.Query,
		Vector:          entry.Vector,
		Threshold:       0.92,
		Profile:         rag.ProfileHighPrecision,
		TopK:            entry.TopK,
	})
	if err != nil {
		t.Fatalf("Lookup(high_precision) error = %v", err)
	}
	if ok {
		t.Fatalf("Lookup(high_precision) hit = true, want false")
	}

	resp, ok, err := cache.Lookup(ctx, rag.SemanticCacheLookupRequest{
		TenantID:        entry.TenantID,
		KnowledgeBaseID: entry.KnowledgeBaseID,
		Query:           entry.Query,
		Vector:          entry.Vector,
		Threshold:       0.92,
		Profile:         rag.ProfileRealtime,
		TopK:            entry.TopK,
	})
	if err != nil {
		t.Fatalf("Lookup(realtime) error = %v", err)
	}
	if !ok {
		t.Fatalf("Lookup(realtime) hit = false, want true")
	}
	if resp.Profile != rag.ProfileRealtime || resp.Answer != entry.Response.Answer {
		t.Fatalf("Lookup(realtime) response = %#v", resp)
	}
}

type semanticCacheFakePointsClient struct {
	points []*qdrant.PointStruct
}

func (f *semanticCacheFakePointsClient) Upsert(_ context.Context, in *qdrant.UpsertPoints, _ ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
	for _, point := range in.GetPoints() {
		replaced := false
		for i, existing := range f.points {
			if existing.GetId().GetNum() == point.GetId().GetNum() {
				f.points[i] = point
				replaced = true
				break
			}
		}
		if !replaced {
			f.points = append(f.points, point)
		}
	}
	return &qdrant.PointsOperationResponse{}, nil
}

func (f *semanticCacheFakePointsClient) Search(_ context.Context, in *qdrant.SearchPoints, _ ...grpc.CallOption) (*qdrant.SearchResponse, error) {
	for _, point := range f.points {
		if semanticCacheFakeMatchesFilter(point.GetPayload(), in.GetFilter()) {
			return &qdrant.SearchResponse{Result: []*qdrant.ScoredPoint{{
				Id:      point.GetId(),
				Payload: point.GetPayload(),
				Score:   1,
			}}}, nil
		}
	}
	return &qdrant.SearchResponse{}, nil
}

func semanticCacheFakeMatchesFilter(payload map[string]*qdrant.Value, filter *qdrant.Filter) bool {
	if filter == nil {
		return true
	}
	for _, condition := range filter.GetMust() {
		field := condition.GetField()
		if field == nil || field.GetMatch() == nil {
			return false
		}
		switch field.GetMatch().GetMatchValue().(type) {
		case *qdrant.Match_Keyword:
			if payloadString(payload, field.GetKey()) != field.GetMatch().GetKeyword() {
				return false
			}
		case *qdrant.Match_Integer:
			value := payload[field.GetKey()]
			if value == nil || value.GetIntegerValue() != field.GetMatch().GetInteger() {
				return false
			}
		default:
			return false
		}
	}
	return true
}
