package rag

import (
	"context"
	"testing"
)

func TestInMemorySemanticCacheIsolatesProfileAndTopK(t *testing.T) {
	ctx := context.Background()
	cache := NewSemanticCache(10)
	entry := SemanticCacheEntry{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
		Profile:         ProfileRealtime,
		TopK:            8,
		Response: QueryResponse{
			Answer:  "cached realtime answer",
			Profile: ProfileRealtime,
			Citations: []Citation{{
				ChunkID:    "chk_1",
				DocumentID: "doc_1",
				SourceURI:  "memory://doc",
			}},
		},
	}
	if err := cache.Store(ctx, entry); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	tests := []struct {
		name    string
		profile Profile
		topK    int
		wantHit bool
	}{
		{name: "same profile and top_k", profile: ProfileRealtime, topK: 8, wantHit: true},
		{name: "different profile", profile: ProfileHighPrecision, topK: 8, wantHit: false},
		{name: "different top_k", profile: ProfileRealtime, topK: 16, wantHit: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok, err := cache.Lookup(ctx, SemanticCacheLookupRequest{
				TenantID:        entry.TenantID,
				KnowledgeBaseID: entry.KnowledgeBaseID,
				Query:           entry.Query,
				Profile:         tt.profile,
				TopK:            tt.topK,
			})
			if err != nil {
				t.Fatalf("Lookup() error = %v", err)
			}
			if ok != tt.wantHit {
				t.Fatalf("Lookup() hit = %v, want %v", ok, tt.wantHit)
			}
		})
	}
}

func TestCacheKeyIncludesProfileAndTopK(t *testing.T) {
	base := QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "Qdrant   Vector Search",
		Profile:         ProfileRealtime,
		TopK:            8,
	}
	profileVariant := base
	profileVariant.Profile = ProfileHighPrecision
	topKVariant := base
	topKVariant.TopK = 16
	queryWhitespaceVariant := base
	queryWhitespaceVariant.Query = " qdrant vector   search "

	if CacheKey(base) == CacheKey(profileVariant) {
		t.Fatalf("CacheKey() should differ by profile")
	}
	if CacheKey(base) == CacheKey(topKVariant) {
		t.Fatalf("CacheKey() should differ by top_k")
	}
	if CacheKey(base) != CacheKey(queryWhitespaceVariant) {
		t.Fatalf("CacheKey() should normalize query case and whitespace")
	}
}
