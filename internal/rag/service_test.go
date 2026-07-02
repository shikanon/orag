package rag

import (
	"context"
	"fmt"
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

type recordingServiceRetriever struct {
	requests []kb.SearchRequest
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

type scriptedServiceModel struct {
	systemPrompts []string
}

func (m *scriptedServiceModel) Chat(_ context.Context, messages []ark.ChatMessage) (string, error) {
	system := ""
	for _, message := range messages {
		if message.Role == "system" {
			system = message.Content
			break
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
