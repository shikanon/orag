package qdrantstore

import (
	"testing"
	"time"

	qdrant "github.com/qdrant/go-client/qdrant"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
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
