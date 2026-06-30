package rag

import (
	"context"
	"testing"
	"time"
)

func TestLookupSemanticCachePreservesCachedProfile(t *testing.T) {
	cache := &semanticCacheStub{
		resp: QueryResponse{
			Answer:  "cached realtime answer",
			Profile: ProfileRealtime,
		},
		hit: true,
	}
	service := Service{
		Cache:                  cache,
		SemanticCacheThreshold: 0.92,
	}

	resp, ok, warning := service.LookupSemanticCache(context.Background(), QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
	}, []float64{0.1, 0.2}, "trace_high_precision", ProfileHighPrecision, 16, time.Now())
	if warning != "" {
		t.Fatalf("LookupSemanticCache() warning = %q", warning)
	}
	if !ok {
		t.Fatalf("LookupSemanticCache() hit = false, want true")
	}
	if cache.lookupReq.Profile != ProfileHighPrecision {
		t.Fatalf("lookup profile = %q, want %q", cache.lookupReq.Profile, ProfileHighPrecision)
	}
	if cache.lookupReq.TopK != 16 {
		t.Fatalf("lookup top_k = %d, want 16", cache.lookupReq.TopK)
	}
	if resp.Profile != ProfileRealtime {
		t.Fatalf("response profile = %q, want cached profile %q", resp.Profile, ProfileRealtime)
	}
	if resp.CacheStatus != "hit" {
		t.Fatalf("cache_status = %q, want hit", resp.CacheStatus)
	}
	if resp.TraceID != "trace_high_precision" {
		t.Fatalf("trace_id = %q, want trace_high_precision", resp.TraceID)
	}
}

type semanticCacheStub struct {
	lookupReq SemanticCacheLookupRequest
	resp      QueryResponse
	hit       bool
	err       error
}

func (s *semanticCacheStub) Lookup(_ context.Context, req SemanticCacheLookupRequest) (QueryResponse, bool, error) {
	s.lookupReq = req
	return s.resp, s.hit, s.err
}

func (s *semanticCacheStub) Store(context.Context, SemanticCacheEntry) error {
	return nil
}
