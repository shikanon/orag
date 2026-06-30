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

func TestSemanticCacheSearchFilterIncludesProfileAndTopK(t *testing.T) {
	filter := semanticCacheSearchFilter(rag.SemanticCacheLookupRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Profile:         rag.ProfileHighPrecision,
		TopK:            16,
	})
	conditions := filter.GetMust()
	if got := filterKeyword(conditions, "tenant_id"); got != "tenant_default" {
		t.Fatalf("tenant_id filter = %q", got)
	}
	if got := filterKeyword(conditions, "knowledge_base_id"); got != "kb_default" {
		t.Fatalf("knowledge_base_id filter = %q", got)
	}
	if got := filterKeyword(conditions, "cache_key_version"); got != semanticCachePayloadVersion {
		t.Fatalf("cache_key_version filter = %q", got)
	}
	if got := filterKeyword(conditions, "profile"); got != string(rag.ProfileHighPrecision) {
		t.Fatalf("profile filter = %q", got)
	}
	if got := filterInteger(conditions, "top_k"); got != 16 {
		t.Fatalf("top_k filter = %d", got)
	}
}

func TestSemanticCachePointIDIncludesProfileAndTopK(t *testing.T) {
	base := rag.SemanticCacheEntry{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant cache",
		Profile:         rag.ProfileRealtime,
		TopK:            8,
	}
	profileVariant := base
	profileVariant.Profile = rag.ProfileHighPrecision
	topKVariant := base
	topKVariant.TopK = 16

	if semanticCachePointID(base).GetNum() == semanticCachePointID(profileVariant).GetNum() {
		t.Fatalf("semantic cache point id should differ by profile")
	}
	if semanticCachePointID(base).GetNum() == semanticCachePointID(topKVariant).GetNum() {
		t.Fatalf("semantic cache point id should differ by top_k")
	}
}

func filterKeyword(conditions []*qdrant.Condition, key string) string {
	for _, condition := range conditions {
		field := condition.GetField()
		if field.GetKey() == key {
			return field.GetMatch().GetKeyword()
		}
	}
	return ""
}

func filterInteger(conditions []*qdrant.Condition, key string) int64 {
	for _, condition := range conditions {
		field := condition.GetField()
		if field.GetKey() == key {
			return field.GetMatch().GetInteger()
		}
	}
	return 0
}
