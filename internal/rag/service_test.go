package rag

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/llm/ark"
	"github.com/shikanon/orag/internal/prompt"
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
		SemanticCacheNamespace: "optimizer_candidate:cand_a",
	}

	resp, ok, warning := service.LookupSemanticCache(context.Background(), QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
	}, []float64{0.1, 0.2}, "trace_realtime", ProfileRealtime, 16, time.Now())
	if warning != "" {
		t.Fatalf("LookupSemanticCache() warning = %q", warning)
	}
	if !ok {
		t.Fatalf("LookupSemanticCache() hit = false, want true")
	}
	if cache.lookupReq.Profile != ProfileRealtime {
		t.Fatalf("lookup profile = %q, want %q", cache.lookupReq.Profile, ProfileRealtime)
	}
	if cache.lookupReq.TopK != 16 {
		t.Fatalf("lookup top_k = %d, want 16", cache.lookupReq.TopK)
	}
	if cache.lookupReq.Namespace != "optimizer_candidate:cand_a" {
		t.Fatalf("lookup semantic cache namespace = %q", cache.lookupReq.Namespace)
	}
	if resp.Profile != ProfileRealtime {
		t.Fatalf("response profile = %q, want cached profile %q", resp.Profile, ProfileRealtime)
	}
	if resp.CacheStatus != "hit" {
		t.Fatalf("cache_status = %q, want hit", resp.CacheStatus)
	}
	if resp.TraceID != "trace_realtime" {
		t.Fatalf("trace_id = %q, want trace_realtime", resp.TraceID)
	}
}

func TestSemanticCacheNamespacePropagatesLookupAndStore(t *testing.T) {
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
		SemanticCacheNamespace: "optimizer_candidate:cand_a",
	}
	req := QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
	}

	_, ok, warning := service.LookupSemanticCache(context.Background(), req, []float64{0.1, 0.2}, "trace_realtime", ProfileRealtime, 16, time.Now())
	if warning != "" {
		t.Fatalf("LookupSemanticCache() warning = %q", warning)
	}
	if !ok {
		t.Fatalf("LookupSemanticCache() hit = false, want true")
	}
	if cache.lookupReq.Namespace != service.SemanticCacheNamespace {
		t.Fatalf("lookup namespace = %q, want %q", cache.lookupReq.Namespace, service.SemanticCacheNamespace)
	}

	resp := QueryResponse{
		Answer:  "candidate answer",
		Profile: ProfileRealtime,
		Citations: []Citation{{
			ChunkID:    "chk_a",
			DocumentID: "doc_a",
			SourceURI:  "memory://candidate-a",
		}},
	}
	if warning := service.StoreSemanticCache(context.Background(), req, []float64{0.1, 0.2}, ProfileRealtime, 16, resp); warning != "" {
		t.Fatalf("StoreSemanticCache() warning = %q", warning)
	}
	if cache.storeEntry.Namespace != service.SemanticCacheNamespace {
		t.Fatalf("store namespace = %q, want %q", cache.storeEntry.Namespace, service.SemanticCacheNamespace)
	}
}

func TestExecuteHighPrecisionMultiQueryAndHyDEUseExpandedRetrieval(t *testing.T) {
	ctx := context.Background()
	retriever := &recordingServiceRetriever{}
	model := &scriptedServiceModel{}
	service := Service{
		Retriever:           retriever,
		Model:               model,
		Packer:              ContextPacker{MaxTokens: 512, TopN: 4},
		PromptStrategy:      prompt.NewStrategy("auto"),
		DefaultProfile:      ProfileRealtime,
		NoContextAnswer:     "no context",
		TopK:                4,
		MultiQueryCount:     3,
		HyDEEnabled:         true,
		QueryRewriteEnabled: false,
	}

	resp, err := service.Execute(ctx, QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
		Profile:         ProfileHighPrecision,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(retriever.requests) != 4 {
		t.Fatalf("retrieval calls = %d, want 4: %#v", len(retriever.requests), retriever.requests)
	}
	if !model.sawSystemPrompt("多查询") {
		t.Fatalf("multi-query generation chat was not called: %#v", model.systemPrompts)
	}
	if !model.sawSystemPrompt("HyDE") {
		t.Fatalf("HyDE generation chat was not called: %#v", model.systemPrompts)
	}
	if resp.Profile != ProfileHighPrecision {
		t.Fatalf("profile = %q, want %q", resp.Profile, ProfileHighPrecision)
	}
	if len(resp.RetrievedChunks) == 0 {
		t.Fatalf("expected retrieved chunks in response")
	}
}

func TestBuildRetrievalQueriesAllowsServerOwnedRealtimeMultiQueryOnly(t *testing.T) {
	model := &scriptedServiceModel{}
	service := Service{
		Model:                 model,
		MultiQueryCount:       3,
		MultiQueryForRealtime: true,
		QueryRewriteEnabled:   false,
		HyDEEnabled:           false,
	}
	queries, warnings := service.BuildRetrievalQueries(context.Background(), QueryRequest{Query: "qdrant vector search"}, ProfileRealtime, "")
	if len(queries) != 3 {
		t.Fatalf("queries=%#v, want three retrieval queries", queries)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings=%#v", warnings)
	}
	if !model.sawSystemPrompt("多查询") {
		t.Fatalf("multi-query generation chat was not called: %#v", model.systemPrompts)
	}
	if model.sawSystemPrompt("HyDE") {
		t.Fatalf("HyDE must remain disabled for realtime multi-query: %#v", model.systemPrompts)
	}
}

func TestApplyRerankCanBeDisabledForTutorialIsolation(t *testing.T) {
	model := &scriptedServiceModel{}
	results := []kb.SearchResult{
		{Chunk: kb.Chunk{ID: "chk_first"}, Score: 0.9, Rank: 1, From: "hybrid"},
		{Chunk: kb.Chunk{ID: "chk_second"}, Score: 0.8, Rank: 2, From: "hybrid"},
	}
	service := Service{Model: model, DisableRerank: true, TopK: 2}
	got := service.ApplyRerank(context.Background(), "qdrant vector search", results, 2)
	if len(model.rerankTopNs) != 0 {
		t.Fatalf("rerank calls=%#v, want none", model.rerankTopNs)
	}
	if !reflect.DeepEqual(got, results) {
		t.Fatalf("results=%#v, want original=%#v", got, results)
	}
}

func TestRetrieveExpandedUsesConfiguredRRFK(t *testing.T) {
	ctx := context.Background()
	retriever := &rrfKServiceRetriever{}
	service := Service{
		Retriever: retriever,
		RRFK:      7,
		TopK:      5,
	}

	results, _, _, err := service.RetrieveExpanded(ctx, QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
	}, 5, []RetrievalQuery{
		{Query: "qdrant hybrid retrieval", EmbeddingText: "qdrant vector search"},
		{Query: "dense sparse fusion", EmbeddingText: "qdrant vector search"},
	}, []float64{1, 2})
	if err != nil {
		t.Fatalf("RetrieveExpanded() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1: %#v", len(results), results)
	}
	wantScore := 1.0/float64(7+1) + 1.0/float64(7+2)
	if math.Abs(results[0].Score-wantScore) > 1e-12 {
		t.Fatalf("RRF score = %.12f, want %.12f", results[0].Score, wantScore)
	}
}

func TestRetrieveExpandedUsesEffectiveTopKWithHybridRetrieverDefault(t *testing.T) {
	ctx := context.Background()
	dense := &topKServiceRetriever{prefix: "dense", max: 60}
	sparse := &topKServiceRetriever{prefix: "sparse", max: 60}
	service := Service{
		Retriever: kb.HybridRetriever{
			Dense:  dense,
			Sparse: sparse,
			RRFK:   60,
			TopN:   8,
		},
		TopK: 50,
	}

	results, _, warnings, err := service.RetrieveExpanded(ctx, QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
	}, 0, []RetrievalQuery{{Query: "qdrant vector search"}}, []float64{1, 2})
	if err != nil {
		t.Fatalf("RetrieveExpanded() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(results) != 50 {
		t.Fatalf("len(results) = %d, want 50", len(results))
	}
	seen := map[string]struct{}{}
	for _, result := range results {
		if _, ok := seen[result.Chunk.ID]; ok {
			t.Fatalf("duplicate result id %q in %#v", result.Chunk.ID, results)
		}
		seen[result.Chunk.ID] = struct{}{}
	}
	if got := len(seen); got != 50 {
		t.Fatalf("unique result ids = %d, want 50", got)
	}
	if len(dense.requests) != 1 || dense.requests[0].TopK != 50 || dense.requests[0].DenseTopK != 50 {
		t.Fatalf("dense request = %#v, want TopK and DenseTopK 50", dense.requests)
	}
	if len(sparse.requests) != 1 || sparse.requests[0].TopK != 50 || sparse.requests[0].SparseTopK != 50 {
		t.Fatalf("sparse request = %#v, want TopK and SparseTopK 50", sparse.requests)
	}
}

func TestExecuteExplicitTopKNotTruncatedByContextTopN(t *testing.T) {
	ctx := context.Background()
	model := &scriptedServiceModel{}
	service := Service{
		Retriever:       &topKServiceRetriever{prefix: "rerank", max: 20},
		Model:           model,
		Packer:          ContextPacker{MaxTokens: 512, TopN: 8},
		PromptStrategy:  prompt.NewStrategy("auto"),
		DefaultProfile:  ProfileRealtime,
		NoContextAnswer: "no context",
		TopK:            50,
	}

	resp, err := service.Execute(ctx, QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
		TopK:            16,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(model.rerankTopNs) != 1 || model.rerankTopNs[0] != 16 {
		t.Fatalf("rerank top_n calls = %#v, want [16]", model.rerankTopNs)
	}
	if len(resp.RetrievedChunks) != 16 {
		t.Fatalf("len(retrieved_chunks) = %d, want 16", len(resp.RetrievedChunks))
	}
	if len(resp.Citations) != 8 {
		t.Fatalf("len(citations) = %d, want context TopN 8", len(resp.Citations))
	}
	for _, result := range resp.RetrievedChunks {
		if result.From != "ark_rerank" {
			t.Fatalf("retrieved chunk source = %q, want ark_rerank", result.From)
		}
	}
}

func TestExecuteNoContextReturnsEmptyCollections(t *testing.T) {
	service := Service{
		Retriever:           emptyServiceRetriever{},
		Model:               &scriptedServiceModel{},
		Packer:              ContextPacker{MaxTokens: 512, TopN: 4},
		PromptStrategy:      prompt.NewStrategy("auto"),
		DefaultProfile:      ProfileRealtime,
		NoContextAnswer:     "no context",
		TopK:                4,
		QueryRewriteEnabled: false,
		HyDEEnabled:         false,
	}

	response, err := service.Query(context.Background(), QueryRequest{TenantID: "tenant_default", KnowledgeBaseID: "kb_default", Query: "missing context"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if response.Citations == nil || response.RetrievedChunks == nil {
		t.Fatalf("Query() returned null collections: %#v", response)
	}
	if len(response.Citations) != 0 || len(response.RetrievedChunks) != 0 {
		t.Fatalf("Query() returned non-empty no-context collections: %#v", response)
	}
}

func TestExecuteDirectRouteBypassesRetrieval(t *testing.T) {
	ctx := context.Background()
	retriever := &recordingServiceRetriever{}
	model := &scriptedServiceModel{}
	service := Service{
		Retriever:       retriever,
		Model:           model,
		Packer:          ContextPacker{MaxTokens: 512, TopN: 4},
		PromptStrategy:  prompt.NewStrategy("auto"),
		DefaultProfile:  ProfileRealtime,
		NoContextAnswer: "no context",
		TopK:            4,
		QueryRouter: fixedQueryRouter{
			decision: RouteDecision{Route: QueryRouteDirect, Reason: "small talk", Strategy: "test"},
		},
	}

	resp, err := service.Execute(ctx, QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "你好",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(retriever.requests) != 0 {
		t.Fatalf("retrieval calls = %d, want 0", len(retriever.requests))
	}
	if resp.Route == nil || resp.Route.Route != QueryRouteDirect {
		t.Fatalf("route = %#v, want direct route", resp.Route)
	}
	if resp.CacheStatus != "bypass" {
		t.Fatalf("cache_status = %q, want bypass", resp.CacheStatus)
	}
	if resp.Answer == "" || resp.Answer == service.NoContextAnswer {
		t.Fatalf("answer = %q, want generated direct answer", resp.Answer)
	}
}

func TestExecuteShadowRetrievalDefaultRecordsWithoutChangingAnswer(t *testing.T) {
	ctx := context.Background()
	shadow := &recordingShadowRetriever{
		matches: []ShadowMatch{shadowTestMatch("shadow_chunk")},
	}
	service := Service{
		Retriever:       &recordingServiceRetriever{},
		Model:           &scriptedServiceModel{},
		Packer:          ContextPacker{MaxTokens: 512, TopN: 4},
		PromptStrategy:  prompt.NewStrategy("auto"),
		DefaultProfile:  ProfileRealtime,
		NoContextAnswer: "no context",
		TopK:            4,
		Shadow:          ShadowOptions{Enabled: true, Inject: false, Limit: 3},
		ShadowRetriever: shadow,
		ShadowSourceReader: staticShadowSourceReader{
			chunks: map[string]ShadowSourceChunk{
				"shadow_chunk": {
					TenantID: "tenant_default",
					KBID:     "kb_default",
					DocID:    "shadow_doc",
					ChunkID:  "shadow_chunk",
					Text:     "shadow optimization source text",
				},
			},
		},
	}

	resp, err := service.Execute(ctx, QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
		TraceID:         "trace_shadow_default",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(shadow.requests) != 1 {
		t.Fatalf("shadow retrieve calls = %d, want 1", len(shadow.requests))
	}
	if shadow.requests[0].Inject {
		t.Fatalf("shadow inject flag = true, want false by default")
	}
	if shadow.requests[0].TraceID != "trace_shadow_default" {
		t.Fatalf("shadow trace id = %q, want trace_shadow_default", shadow.requests[0].TraceID)
	}
	if resp.Answer != "answer [chk_service_1]" {
		t.Fatalf("answer = %q, want online answer unchanged", resp.Answer)
	}
	if len(resp.Citations) != 1 || resp.Citations[0].ChunkID != "chk_service_1" {
		t.Fatalf("citations = %#v, want only online citation", resp.Citations)
	}
	if len(resp.RetrievedChunks) != 1 || resp.RetrievedChunks[0].Chunk.ID != "chk_service_1" {
		t.Fatalf("retrieved chunks = %#v, want only online chunk", resp.RetrievedChunks)
	}
}

func TestExecuteShadowRetrievalFailureDegradesToOnlineAnswer(t *testing.T) {
	ctx := context.Background()
	shadow := &recordingShadowRetriever{err: errors.New("shadow write failed")}
	service := Service{
		Retriever:       &recordingServiceRetriever{},
		Model:           &scriptedServiceModel{},
		Packer:          ContextPacker{MaxTokens: 512, TopN: 4},
		PromptStrategy:  prompt.NewStrategy("auto"),
		DefaultProfile:  ProfileRealtime,
		NoContextAnswer: "no context",
		TopK:            4,
		Shadow:          ShadowOptions{Enabled: true, Inject: false, Limit: 3},
		ShadowRetriever: shadow,
	}

	resp, err := service.Execute(ctx, QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want degradation to success", err)
	}
	if resp.Answer != "answer [chk_service_1]" {
		t.Fatalf("answer = %q, want online answer after shadow failure", resp.Answer)
	}
	if len(resp.Citations) != 1 || resp.Citations[0].ChunkID != "chk_service_1" {
		t.Fatalf("citations = %#v, want only online citation", resp.Citations)
	}
}

func TestExecuteShadowInjectUsesSourceChunkWithoutFinalAnswer(t *testing.T) {
	ctx := context.Background()
	model := &scriptedServiceModel{}
	service := Service{
		Retriever:       &recordingServiceRetriever{},
		Model:           model,
		Packer:          ContextPacker{MaxTokens: 512, TopN: 4},
		PromptStrategy:  prompt.NewStrategy("auto"),
		DefaultProfile:  ProfileRealtime,
		NoContextAnswer: "no context",
		TopK:            4,
		Shadow:          ShadowOptions{Enabled: true, Inject: true, Limit: 3},
		ShadowRetriever: &recordingShadowRetriever{
			matches: []ShadowMatch{{
				ItemID:   "item_shadow",
				ItemType: "answer_item",
				Source:   "optimization_library",
				Score:    1,
				Rank:     1,
				Metadata: map[string]any{
					"canonical_question": "What is ORAG?",
					"final_answer":       "SECRET FINAL ANSWER",
				},
				AnswerItem: &ShadowAnswerItem{
					Evidence: []ShadowEvidence{{ChunkID: "shadow_chunk", DocID: "shadow_doc", Quote: "real source"}},
					GuidanceMetadata: map[string]any{
						"canonical_question": "What is ORAG?",
						"final_answer":       "SECRET FINAL ANSWER",
					},
				},
			}},
		},
		ShadowSourceReader: staticShadowSourceReader{
			chunks: map[string]ShadowSourceChunk{
				"shadow_chunk": {
					TenantID:         "tenant_default",
					KBID:             "kb_default",
					DocID:            "shadow_doc",
					DocVersion:       "v1",
					ChunkID:          "shadow_chunk",
					ChunkContentHash: "sha256:shadow",
					Text:             "real shadow source chunk",
				},
			},
		},
	}

	resp, err := service.Execute(ctx, QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(resp.Citations) == 0 || resp.Citations[0].ChunkID != "shadow_chunk" {
		t.Fatalf("citations = %#v, want injected source chunk first", resp.Citations)
	}
	if !model.sawUserPrompt("real shadow source chunk") {
		t.Fatalf("shadow source chunk was not injected into prompt: %#v", model.userPrompts)
	}
	if !model.sawUserPrompt("canonical_question") {
		t.Fatalf("shadow guidance metadata was not injected into prompt: %#v", model.userPrompts)
	}
	if model.sawUserPrompt("SECRET FINAL ANSWER") {
		t.Fatalf("final_answer leaked into prompt: %#v", model.userPrompts)
	}
	if strings.Contains(resp.Answer, "SECRET FINAL ANSWER") {
		t.Fatalf("final_answer leaked into response answer: %q", resp.Answer)
	}
}

func TestExecuteShadowRetrievalPassesScopedItemID(t *testing.T) {
	ctx := context.Background()
	shadow := &recordingShadowRetriever{matches: []ShadowMatch{shadowTestMatch("shadow_chunk")}}
	service := Service{
		Retriever:       &recordingServiceRetriever{},
		Model:           &scriptedServiceModel{},
		Packer:          ContextPacker{MaxTokens: 512, TopN: 4},
		PromptStrategy:  prompt.NewStrategy("auto"),
		DefaultProfile:  ProfileRealtime,
		NoContextAnswer: "no context",
		TopK:            4,
		Shadow:          ShadowOptions{Enabled: true, Inject: false, Limit: 3},
		ShadowRetriever: shadow,
	}

	if _, err := service.Execute(ctx, QueryRequest{
		TenantID:           "tenant_default",
		KnowledgeBaseID:    "kb_default",
		Query:              "qdrant vector search",
		ScopedShadowItemID: "opt_scoped",
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(shadow.requests) != 1 || shadow.requests[0].ScopedItemID != "opt_scoped" {
		t.Fatalf("shadow requests = %#v, want scoped item opt_scoped", shadow.requests)
	}
}

func TestExecuteScopedShadowSourceMismatchFailsExplicitly(t *testing.T) {
	ctx := context.Background()
	service := Service{
		Retriever:       &recordingServiceRetriever{},
		Model:           &scriptedServiceModel{},
		Packer:          ContextPacker{MaxTokens: 512, TopN: 4},
		PromptStrategy:  prompt.NewStrategy("auto"),
		DefaultProfile:  ProfileRealtime,
		NoContextAnswer: "no context",
		TopK:            4,
		Shadow:          ShadowOptions{Enabled: true, Inject: true, Limit: 3},
		ShadowRetriever: &recordingShadowRetriever{
			matches: []ShadowMatch{{
				ItemID:   "opt_scoped",
				ItemType: "answer_item",
				Source:   "optimization_library",
				Score:    1,
				Rank:     1,
				AnswerItem: &ShadowAnswerItem{
					SourceFingerprints: []ShadowSourceFingerprint{{
						DocID:            "shadow_doc",
						DocVersion:       "v1",
						ChunkID:          "shadow_chunk",
						ChunkContentHash: "sha256:expected",
					}},
					Evidence: []ShadowEvidence{{ChunkID: "shadow_chunk", DocID: "shadow_doc", Quote: "real source"}},
				},
			}},
		},
		ShadowSourceReader: staticShadowSourceReader{
			chunks: map[string]ShadowSourceChunk{
				"shadow_chunk": {
					TenantID:         "tenant_default",
					KBID:             "kb_default",
					DocID:            "shadow_doc",
					DocVersion:       "v1",
					ChunkID:          "shadow_chunk",
					ChunkContentHash: "sha256:stale",
					Text:             "real shadow source chunk",
				},
			},
		},
	}

	_, err := service.Execute(ctx, QueryRequest{
		TenantID:           "tenant_default",
		KnowledgeBaseID:    "kb_default",
		Query:              "qdrant vector search",
		ScopedShadowItemID: "opt_scoped",
	})
	if !errors.Is(err, ErrScopedShadowSourceMismatch) {
		t.Fatalf("Execute() error = %v, want %v", err, ErrScopedShadowSourceMismatch)
	}
}

type semanticCacheStub struct {
	lookupReq  SemanticCacheLookupRequest
	storeEntry SemanticCacheEntry
	resp       QueryResponse
	hit        bool
	err        error
}

type fixedQueryRouter struct {
	decision RouteDecision
	err      error
}

func (r fixedQueryRouter) Route(context.Context, QueryRequest) (RouteDecision, error) {
	return r.decision, r.err
}

func (s *semanticCacheStub) Lookup(_ context.Context, req SemanticCacheLookupRequest) (QueryResponse, bool, error) {
	s.lookupReq = req
	return s.resp, s.hit, s.err
}

func (s *semanticCacheStub) Store(_ context.Context, entry SemanticCacheEntry) error {
	s.storeEntry = entry
	return nil
}

type recordingServiceRetriever struct {
	requests []kb.SearchRequest
}

type emptyServiceRetriever struct{}

func (emptyServiceRetriever) Retrieve(context.Context, kb.SearchRequest) ([]kb.SearchResult, error) {
	return nil, nil
}

func (r *recordingServiceRetriever) Retrieve(_ context.Context, req kb.SearchRequest) ([]kb.SearchResult, error) {
	r.requests = append(r.requests, req)
	id := fmt.Sprintf("chk_service_%d", len(r.requests))
	return []kb.SearchResult{{
		Chunk: kb.Chunk{
			ID:         id,
			DocumentID: "doc_service",
			Content:    req.Query + " context",
			SourceURI:  "memory://service",
		},
		Score: 1,
		Rank:  1,
		From:  "stub",
	}}, nil
}

type rrfKServiceRetriever struct {
	calls int
}

func (r *rrfKServiceRetriever) Retrieve(_ context.Context, req kb.SearchRequest) ([]kb.SearchResult, error) {
	r.calls++
	return []kb.SearchResult{{
		Chunk: kb.Chunk{
			ID:              "chk_rrf",
			TenantID:        req.TenantID,
			KnowledgeBaseID: req.KnowledgeBaseID,
			DocumentID:      "doc_rrf",
			Content:         req.Query + " context",
			SourceURI:       "memory://rrf",
		},
		Score: 1,
		Rank:  r.calls,
		From:  "stub",
	}}, nil
}

type topKServiceRetriever struct {
	prefix   string
	max      int
	requests []kb.SearchRequest
}

func (r *topKServiceRetriever) Retrieve(_ context.Context, req kb.SearchRequest) ([]kb.SearchResult, error) {
	r.requests = append(r.requests, req)
	limit := req.TopK
	if limit <= 0 || limit > r.max {
		limit = r.max
	}
	results := make([]kb.SearchResult, 0, limit)
	for i := 1; i <= limit; i++ {
		id := fmt.Sprintf("%s_candidate_%03d", r.prefix, i)
		results = append(results, kb.SearchResult{
			Chunk: kb.Chunk{
				ID:              id,
				TenantID:        req.TenantID,
				KnowledgeBaseID: req.KnowledgeBaseID,
				DocumentID:      r.prefix + "_doc",
				Content:         fmt.Sprintf("%s context %03d", req.Query, i),
				SourceURI:       "memory://" + r.prefix,
			},
			Score: float64(limit - i + 1),
			Rank:  i,
			From:  r.prefix,
		})
	}
	return results, nil
}

type scriptedServiceModel struct {
	systemPrompts []string
	userPrompts   []string
	rerankTopNs   []int
}

func (m *scriptedServiceModel) Chat(_ context.Context, messages []ark.ChatMessage) (string, error) {
	system := ""
	for _, message := range messages {
		if message.Role == "system" {
			system = message.Content
		}
		if message.Role == "user" {
			m.userPrompts = append(m.userPrompts, message.Content)
		}
	}
	m.systemPrompts = append(m.systemPrompts, system)
	switch {
	case strings.Contains(system, "多查询"):
		return `["qdrant hybrid retrieval", "dense sparse fusion"]`, nil
	case strings.Contains(system, "HyDE"):
		return "qdrant vector search stores documents for hybrid retrieval", nil
	default:
		return "answer [chk_service_1]", nil
	}
}

func (m *scriptedServiceModel) Embed(_ context.Context, texts []string) ([][]float64, error) {
	out := make([][]float64, len(texts))
	for i, text := range texts {
		out[i] = []float64{float64(len(text)%7) + 1, float64(len(text)%5) + 1}
	}
	return out, nil
}

func (m *scriptedServiceModel) Rerank(_ context.Context, _ string, docs []ark.RerankDocument, topN int) ([]ark.RerankResult, error) {
	m.rerankTopNs = append(m.rerankTopNs, topN)
	if topN <= 0 || topN > len(docs) {
		topN = len(docs)
	}
	out := make([]ark.RerankResult, 0, topN)
	for i := 0; i < topN; i++ {
		out = append(out, ark.RerankResult{Index: i, Score: 1 / float64(i+1)})
	}
	return out, nil
}

func (m *scriptedServiceModel) sawSystemPrompt(value string) bool {
	for _, prompt := range m.systemPrompts {
		if strings.Contains(prompt, value) {
			return true
		}
	}
	return false
}

func (m *scriptedServiceModel) sawUserPrompt(value string) bool {
	for _, prompt := range m.userPrompts {
		if strings.Contains(prompt, value) {
			return true
		}
	}
	return false
}

type recordingShadowRetriever struct {
	requests []ShadowRetrieveRequest
	matches  []ShadowMatch
	err      error
}

func (r *recordingShadowRetriever) RetrieveShadow(_ context.Context, req ShadowRetrieveRequest) ([]ShadowMatch, error) {
	r.requests = append(r.requests, req)
	if r.err != nil {
		return nil, r.err
	}
	return r.matches, nil
}

type staticShadowSourceReader struct {
	chunks map[string]ShadowSourceChunk
	err    error
}

func (r staticShadowSourceReader) ReadShadowSourceChunk(_ context.Context, _, _, chunkID string) (ShadowSourceChunk, bool, error) {
	if r.err != nil {
		return ShadowSourceChunk{}, false, r.err
	}
	chunk, ok := r.chunks[chunkID]
	return chunk, ok, nil
}

func shadowTestMatch(chunkID string) ShadowMatch {
	return ShadowMatch{
		ItemID:   "item_shadow",
		ItemType: "answer_item",
		Source:   "optimization_library",
		Score:    1,
		Rank:     1,
		AnswerItem: &ShadowAnswerItem{
			Evidence: []ShadowEvidence{{ChunkID: chunkID, DocID: "shadow_doc", Quote: "shadow source"}},
			GuidanceMetadata: map[string]any{
				"canonical_question": "What is ORAG?",
			},
		},
	}
}
