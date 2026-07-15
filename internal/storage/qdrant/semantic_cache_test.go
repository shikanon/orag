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
		Namespace:       "optimizer_candidate:cand_a",
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
	if got := payloadString(payload, "cache_key_version"); got != semanticCacheNamespacedPayloadVersion {
		t.Fatalf("cache_key_version = %q", got)
	}
	if got := payloadString(payload, "profile"); got != string(entry.Profile) {
		t.Fatalf("payload profile = %q", got)
	}
	if got := payloadString(payload, "namespace"); got != entry.Namespace {
		t.Fatalf("payload namespace = %q, want %q", got, entry.Namespace)
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

func TestSemanticCacheLookupFilterIncludesProfile(t *testing.T) {
	req := rag.SemanticCacheLookupRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Profile:         rag.ProfileHighPrecision,
		TopK:            12,
	}

	filter := semanticCacheLookupFilter(req)
	if got := filterKeyword(t, filter, "tenant_id"); got != req.TenantID {
		t.Fatalf("tenant filter = %q", got)
	}
	if got := filterKeyword(t, filter, "knowledge_base_id"); got != req.KnowledgeBaseID {
		t.Fatalf("knowledge base filter = %q", got)
	}
	if got := filterKeyword(t, filter, "cache_key_version"); got != semanticCachePayloadVersion {
		t.Fatalf("cache key version filter = %q", got)
	}
	if got := filterKeyword(t, filter, "profile"); got != string(req.Profile) {
		t.Fatalf("profile filter = %q, want %q", got, req.Profile)
	}
	if got := filterInteger(t, filter, "top_k"); got != int64(req.TopK) {
		t.Fatalf("top_k filter = %d", got)
	}

	req.Profile = ""
	if got := filterKeyword(t, semanticCacheLookupFilter(req), "profile"); got != string(rag.ProfileRealtime) {
		t.Fatalf("empty request profile filter = %q, want %q", got, rag.ProfileRealtime)
	}
}

func TestSemanticCacheLookupFilterIncludesNamespaceWhenPresent(t *testing.T) {
	req := rag.SemanticCacheLookupRequest{
		Namespace:       " optimizer_candidate:cand_a ",
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Profile:         rag.ProfileRealtime,
		TopK:            12,
	}

	filter := semanticCacheLookupFilter(req)
	if got := filterKeyword(t, filter, "namespace"); got != "optimizer_candidate:cand_a" {
		t.Fatalf("namespace filter = %q, want trimmed namespace", got)
	}

	req.Namespace = ""
	if filterHasField(semanticCacheLookupFilter(req), "namespace") {
		t.Fatalf("empty namespace should not add a namespace filter")
	}
}

func TestSemanticCachePointKeyUsesResolvedProfile(t *testing.T) {
	entry := rag.SemanticCacheEntry{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "  Qdrant   Cache  ",
		TopK:            8,
		Response: rag.QueryResponse{
			Profile: rag.ProfileHighPrecision,
		},
	}

	got := semanticCachePointKey(entry)
	want := rag.CacheKey(rag.QueryRequest{
		TenantID:        entry.TenantID,
		KnowledgeBaseID: entry.KnowledgeBaseID,
		Query:           entry.Query,
		Profile:         rag.ProfileHighPrecision,
		TopK:            entry.TopK,
	})
	if got != want {
		t.Fatalf("point key = %q, want %q", got, want)
	}
	if payloadProfile := payloadString(semanticCachePayload(entry), "profile"); payloadProfile != string(rag.ProfileHighPrecision) {
		t.Fatalf("payload profile = %q", payloadProfile)
	}

	profileVariant := entry
	profileVariant.Response.Profile = rag.ProfileRealtime
	if semanticCachePointKey(profileVariant) == got {
		t.Fatalf("point key should differ by resolved profile")
	}

	topKVariant := entry
	topKVariant.TopK = 99
	if semanticCachePointKey(topKVariant) == got {
		t.Fatalf("point key should differ by top_k")
	}
}

func TestSemanticCachePointKeyIncludesNamespace(t *testing.T) {
	entry := rag.SemanticCacheEntry{
		Namespace:       "optimizer_candidate:cand_a",
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant cache",
		Profile:         rag.ProfileRealtime,
		TopK:            8,
	}

	got := semanticCachePointKey(entry)
	want := rag.NamespacedCacheKey(entry.Namespace, rag.QueryRequest{
		TenantID:        entry.TenantID,
		KnowledgeBaseID: entry.KnowledgeBaseID,
		Query:           entry.Query,
		Profile:         entry.Profile,
		TopK:            entry.TopK,
	})
	if got != want {
		t.Fatalf("point key = %q, want %q", got, want)
	}

	namespaceVariant := entry
	namespaceVariant.Namespace = "optimizer_candidate:cand_b"
	if semanticCachePointKey(namespaceVariant) == got {
		t.Fatalf("point key should differ by namespace")
	}

	legacy := entry
	legacy.Namespace = ""
	if semanticCachePointKey(legacy) != rag.CacheKey(rag.QueryRequest{
		TenantID:        legacy.TenantID,
		KnowledgeBaseID: legacy.KnowledgeBaseID,
		Query:           legacy.Query,
		Profile:         legacy.Profile,
		TopK:            legacy.TopK,
	}) {
		t.Fatalf("empty namespace should preserve legacy point key")
	}
}

func TestSemanticCacheLookupPayloadRequiresMatchingProfile(t *testing.T) {
	payload := map[string]*qdrant.Value{
		"profile":        stringValue(string(rag.ProfileRealtime)),
		"answer":         stringValue("cached answer"),
		"citations_json": stringValue("[]"),
		"retrieved_json": stringValue("[]"),
	}

	resp, ok := semanticCacheLookupResponseFromPayload(rag.SemanticCacheLookupRequest{Profile: rag.ProfileRealtime}, payload)
	if !ok {
		t.Fatalf("same profile payload should hit")
	}
	if resp.Profile != rag.ProfileRealtime {
		t.Fatalf("response profile = %q", resp.Profile)
	}

	if _, ok := semanticCacheLookupResponseFromPayload(rag.SemanticCacheLookupRequest{Profile: rag.ProfileHighPrecision}, payload); ok {
		t.Fatalf("mismatched profile payload should miss")
	}

	delete(payload, "profile")
	if _, ok := semanticCacheLookupResponseFromPayload(rag.SemanticCacheLookupRequest{Profile: rag.ProfileRealtime}, payload); ok {
		t.Fatalf("empty profile payload should miss")
	}
}

func TestSemanticCacheLookupPayloadRequiresMatchingNamespace(t *testing.T) {
	payload := map[string]*qdrant.Value{
		"namespace":      stringValue("optimizer_candidate:cand_a"),
		"profile":        stringValue(string(rag.ProfileRealtime)),
		"answer":         stringValue("cached answer"),
		"citations_json": stringValue("[]"),
		"retrieved_json": stringValue("[]"),
	}

	resp, ok := semanticCacheLookupResponseFromPayload(rag.SemanticCacheLookupRequest{
		Namespace: "optimizer_candidate:cand_a",
		Profile:   rag.ProfileRealtime,
	}, payload)
	if !ok {
		t.Fatalf("same namespace payload should hit")
	}
	if resp.Answer != "cached answer" {
		t.Fatalf("response answer = %q", resp.Answer)
	}

	if _, ok := semanticCacheLookupResponseFromPayload(rag.SemanticCacheLookupRequest{
		Namespace: "optimizer_candidate:cand_b",
		Profile:   rag.ProfileRealtime,
	}, payload); ok {
		t.Fatalf("mismatched namespace payload should miss")
	}
	if _, ok := semanticCacheLookupResponseFromPayload(rag.SemanticCacheLookupRequest{
		Profile: rag.ProfileRealtime,
	}, payload); ok {
		t.Fatalf("unnamespaced request should not hit namespaced payload")
	}

	delete(payload, "namespace")
	if _, ok := semanticCacheLookupResponseFromPayload(rag.SemanticCacheLookupRequest{
		Namespace: "optimizer_candidate:cand_a",
		Profile:   rag.ProfileRealtime,
	}, payload); ok {
		t.Fatalf("namespaced request should not hit legacy payload")
	}
	if _, ok := semanticCacheLookupResponseFromPayload(rag.SemanticCacheLookupRequest{
		Profile: rag.ProfileRealtime,
	}, payload); !ok {
		t.Fatalf("unnamespaced request should still hit legacy payload")
	}
}

func TestSemanticCacheDeleteKnowledgeBaseUsesTenantScopedFilter(t *testing.T) {
	points := &recordingPointsClient{}
	cache := SemanticCache{
		Client:     &Client{Points: points},
		Collection: "orag_semantic_cache_test",
	}

	if err := cache.DeleteKnowledgeBaseSemanticCache(context.Background(), "tenant_1", "kb_1"); err != nil {
		t.Fatal(err)
	}
	if points.deleteReq == nil {
		t.Fatal("DeleteKnowledgeBaseSemanticCache did not call Qdrant delete")
	}
	if points.deleteReq.GetCollectionName() != cache.Collection {
		t.Fatalf("collection = %q, want %q", points.deleteReq.GetCollectionName(), cache.Collection)
	}
	if !points.deleteReq.GetWait() {
		t.Fatalf("wait = %v, want true", points.deleteReq.GetWait())
	}
	filter := points.deleteReq.GetPoints().GetFilter()
	if got := filterKeyword(t, filter, "tenant_id"); got != "tenant_1" {
		t.Fatalf("tenant filter = %q", got)
	}
	if got := filterKeyword(t, filter, "knowledge_base_id"); got != "kb_1" {
		t.Fatalf("knowledge base filter = %q", got)
	}
}

func filterKeyword(t *testing.T, filter *qdrant.Filter, key string) string {
	t.Helper()
	field := filterField(t, filter, key)
	return field.GetMatch().GetKeyword()
}

func filterInteger(t *testing.T, filter *qdrant.Filter, key string) int64 {
	t.Helper()
	field := filterField(t, filter, key)
	return field.GetMatch().GetInteger()
}

func filterField(t *testing.T, filter *qdrant.Filter, key string) *qdrant.FieldCondition {
	t.Helper()
	for _, cond := range filter.GetMust() {
		field := cond.GetField()
		if field.GetKey() == key {
			return field
		}
	}
	t.Fatalf("filter missing field %q", key)
	return nil
}

func filterHasField(filter *qdrant.Filter, key string) bool {
	for _, cond := range filter.GetMust() {
		field := cond.GetField()
		if field.GetKey() == key {
			return true
		}
	}
	return false
}

type recordingPointsClient struct {
	upsertReq     *qdrant.UpsertPoints
	setPayloadReq *qdrant.SetPayloadPoints
	deleteReq     *qdrant.DeletePoints
}

func (c *recordingPointsClient) Upsert(_ context.Context, req *qdrant.UpsertPoints, _ ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
	c.upsertReq = req
	return &qdrant.PointsOperationResponse{}, nil
}

func (c *recordingPointsClient) SetPayload(_ context.Context, req *qdrant.SetPayloadPoints, _ ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
	c.setPayloadReq = req
	return &qdrant.PointsOperationResponse{}, nil
}

func (c *recordingPointsClient) Search(context.Context, *qdrant.SearchPoints, ...grpc.CallOption) (*qdrant.SearchResponse, error) {
	return &qdrant.SearchResponse{}, nil
}

func (c *recordingPointsClient) Delete(_ context.Context, req *qdrant.DeletePoints, _ ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
	c.deleteReq = req
	return &qdrant.PointsOperationResponse{}, nil
}

func (c *recordingPointsClient) Count(context.Context, *qdrant.CountPoints, ...grpc.CallOption) (*qdrant.CountResponse, error) {
	return &qdrant.CountResponse{}, nil
}
