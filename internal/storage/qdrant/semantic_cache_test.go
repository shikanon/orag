package qdrantstore

import (
	"testing"
	"time"

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
