package rag

import (
	"context"
	"testing"
	"time"
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
		name            string
		tenantID        string
		knowledgeBaseID string
		profile         Profile
		topK            int
		wantHit         bool
	}{
		{name: "same tenant knowledge base profile and top_k", tenantID: entry.TenantID, knowledgeBaseID: entry.KnowledgeBaseID, profile: ProfileRealtime, topK: 8, wantHit: true},
		{name: "different tenant", tenantID: "tenant_other", knowledgeBaseID: entry.KnowledgeBaseID, profile: ProfileRealtime, topK: 8, wantHit: false},
		{name: "different knowledge base", tenantID: entry.TenantID, knowledgeBaseID: "kb_other", profile: ProfileRealtime, topK: 8, wantHit: false},
		{name: "different profile", tenantID: entry.TenantID, knowledgeBaseID: entry.KnowledgeBaseID, profile: ProfileHighPrecision, topK: 8, wantHit: false},
		{name: "different top_k", tenantID: entry.TenantID, knowledgeBaseID: entry.KnowledgeBaseID, profile: ProfileRealtime, topK: 16, wantHit: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok, err := cache.Lookup(ctx, SemanticCacheLookupRequest{
				TenantID:        tt.tenantID,
				KnowledgeBaseID: tt.knowledgeBaseID,
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

func TestInMemorySemanticCacheIsolatesNamespace(t *testing.T) {
	ctx := context.Background()
	cache := NewSemanticCache(10)
	entry := SemanticCacheEntry{
		Namespace:       "optimizer_candidate:cand_a",
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
		Profile:         ProfileRealtime,
		TopK:            8,
		Response: QueryResponse{
			Answer:  "candidate a answer",
			Profile: ProfileRealtime,
			Citations: []Citation{{
				ChunkID:    "chk_a",
				DocumentID: "doc_a",
				SourceURI:  "memory://candidate-a",
			}},
		},
	}
	if err := cache.Store(ctx, entry); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	tests := []struct {
		name      string
		namespace string
		wantHit   bool
	}{
		{name: "same namespace", namespace: entry.Namespace, wantHit: true},
		{name: "different namespace", namespace: "optimizer_candidate:cand_b", wantHit: false},
		{name: "empty namespace", namespace: "", wantHit: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, ok, err := cache.Lookup(ctx, SemanticCacheLookupRequest{
				Namespace:       tt.namespace,
				TenantID:        entry.TenantID,
				KnowledgeBaseID: entry.KnowledgeBaseID,
				Query:           entry.Query,
				Profile:         entry.Profile,
				TopK:            entry.TopK,
			})
			if err != nil {
				t.Fatalf("Lookup() error = %v", err)
			}
			if ok != tt.wantHit {
				t.Fatalf("Lookup() hit = %v, want %v", ok, tt.wantHit)
			}
			if ok {
				if len(resp.Citations) != 1 || resp.Citations[0].ChunkID != "chk_a" {
					t.Fatalf("Lookup() response = %#v, want candidate a chunk", resp)
				}
			}
		})
	}

	legacyEntry := entry
	legacyEntry.Namespace = ""
	legacyEntry.Response.Answer = "legacy answer"
	legacyEntry.Response.Citations = []Citation{{
		ChunkID:    "chk_legacy",
		DocumentID: "doc_legacy",
		SourceURI:  "memory://legacy",
	}}
	if err := cache.Store(ctx, legacyEntry); err != nil {
		t.Fatalf("Store() legacy error = %v", err)
	}
	resp, ok, err := cache.Lookup(ctx, SemanticCacheLookupRequest{
		TenantID:        legacyEntry.TenantID,
		KnowledgeBaseID: legacyEntry.KnowledgeBaseID,
		Query:           legacyEntry.Query,
		Profile:         legacyEntry.Profile,
		TopK:            legacyEntry.TopK,
	})
	if err != nil {
		t.Fatalf("Lookup() legacy error = %v", err)
	}
	if !ok || len(resp.Citations) != 1 || resp.Citations[0].ChunkID != "chk_legacy" {
		t.Fatalf("legacy lookup = %#v, hit %v; want legacy chunk hit", resp, ok)
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
	tenantVariant := base
	tenantVariant.TenantID = "tenant_other"
	knowledgeBaseVariant := base
	knowledgeBaseVariant.KnowledgeBaseID = "kb_other"
	queryWhitespaceVariant := base
	queryWhitespaceVariant.Query = " qdrant vector   search "

	if CacheKey(base) == CacheKey(tenantVariant) {
		t.Fatalf("CacheKey() should differ by tenant")
	}
	if CacheKey(base) == CacheKey(knowledgeBaseVariant) {
		t.Fatalf("CacheKey() should differ by knowledge base")
	}
	if CacheKey(base) == CacheKey(profileVariant) {
		t.Fatalf("CacheKey() should differ by profile")
	}
	if CacheKey(base) == CacheKey(topKVariant) {
		t.Fatalf("CacheKey() should differ by top_k")
	}
	if CacheKey(base) != CacheKey(queryWhitespaceVariant) {
		t.Fatalf("CacheKey() should normalize query case and whitespace")
	}
	if NamespacedCacheKey("", base) != CacheKey(base) {
		t.Fatalf("empty namespace should preserve existing CacheKey() behavior")
	}
	if NamespacedCacheKey("optimizer_candidate:cand_a", base) == CacheKey(base) {
		t.Fatalf("namespaced key should differ from the existing unnamespaced key")
	}
	if NamespacedCacheKey("optimizer_candidate:cand_a", base) == NamespacedCacheKey("optimizer_candidate:cand_b", base) {
		t.Fatalf("namespaced key should differ by namespace")
	}
}

func TestSemanticCacheStoreUsesRequestProfile(t *testing.T) {
	ctx := context.Background()
	cache := &recordingSemanticCache{}
	service := Service{Cache: cache}
	req := QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
	}
	resp := QueryResponse{
		Answer:  "cached answer",
		Profile: ProfileHighPrecision,
		Citations: []Citation{{
			ChunkID:    "chk_1",
			DocumentID: "doc_1",
			SourceURI:  "memory://doc",
		}},
	}

	if warning := service.StoreSemanticCache(ctx, req, []float64{0.1}, ProfileRealtime, 8, resp); warning != "" {
		t.Fatalf("StoreSemanticCache() warning = %q", warning)
	}
	if cache.entry.Profile != ProfileRealtime {
		t.Fatalf("stored profile = %q, want %q", cache.entry.Profile, ProfileRealtime)
	}
	if cache.entry.Response.Profile != ProfileRealtime {
		t.Fatalf("stored response profile = %q, want %q", cache.entry.Response.Profile, ProfileRealtime)
	}

	resp.Profile = ""
	if warning := service.StoreSemanticCache(ctx, req, []float64{0.1}, ProfileRealtime, 8, resp); warning != "" {
		t.Fatalf("StoreSemanticCache() warning = %q", warning)
	}
	if cache.entry.Profile != ProfileRealtime {
		t.Fatalf("fallback profile = %q, want %q", cache.entry.Profile, ProfileRealtime)
	}
	if cache.entry.Response.Profile != ProfileRealtime {
		t.Fatalf("fallback response profile = %q, want %q", cache.entry.Response.Profile, ProfileRealtime)
	}
}

type recordingSemanticCache struct {
	entry SemanticCacheEntry
}

func (c *recordingSemanticCache) Lookup(context.Context, SemanticCacheLookupRequest) (QueryResponse, bool, error) {
	return QueryResponse{}, false, nil
}

func (c *recordingSemanticCache) Store(_ context.Context, entry SemanticCacheEntry) error {
	c.entry = entry
	return nil
}

func TestLookupSemanticCacheRejectsMismatchedStoredProfile(t *testing.T) {
	service := Service{Cache: staticSemanticCacheStore{
		resp: QueryResponse{
			Answer:  "cached realtime answer",
			Profile: ProfileRealtime,
		},
		ok: true,
	}}
	_, ok, warning := service.LookupSemanticCache(context.Background(), QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
	}, []float64{0.1, 0.2}, "trace_1", ProfileHighPrecision, 8, time.Now())
	if warning != "" {
		t.Fatalf("warning = %q", warning)
	}
	if ok {
		t.Fatalf("LookupSemanticCache() hit = true, want false for mismatched profile")
	}
}

type staticSemanticCacheStore struct {
	resp QueryResponse
	ok   bool
}

func (s staticSemanticCacheStore) Lookup(context.Context, SemanticCacheLookupRequest) (QueryResponse, bool, error) {
	return s.resp, s.ok, nil
}

func (s staticSemanticCacheStore) Store(context.Context, SemanticCacheEntry) error {
	return nil
}
