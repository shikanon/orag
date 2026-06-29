package rag

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/llm/ark"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/prompt"
)

type Pipeline interface {
	Invoke(ctx context.Context, req QueryRequest) (QueryResponse, error)
}

type Service struct {
	Retriever              kb.Retriever
	Model                  *ark.Client
	Cache                  SemanticCacheStore
	Packer                 ContextPacker
	PromptStrategy         prompt.CacheStrategy
	DefaultProfile         Profile
	NoContextAnswer        string
	TopK                   int
	Pipeline               Pipeline
	SemanticCacheThreshold float64

	QueryRewriteEnabled bool
	MultiQueryCount     int
	HyDEEnabled         bool
}

func (s *Service) Query(ctx context.Context, req QueryRequest) (QueryResponse, error) {
	if s.Pipeline != nil {
		return s.Pipeline.Invoke(ctx, req)
	}
	return s.Execute(ctx, req)
}

func (s *Service) Execute(ctx context.Context, req QueryRequest) (QueryResponse, error) {
	start := time.Now()
	traceID := id.New("trace")
	profile := s.Profile(req.Profile)
	topK := req.TopK
	if topK <= 0 {
		topK = s.TopK
	}
	if topK <= 0 {
		topK = 50
	}
	embeddings, err := s.Model.Embed(ctx, []string{req.Query})
	if err != nil {
		return QueryResponse{}, err
	}
	if len(embeddings) == 0 {
		return QueryResponse{}, fmt.Errorf("embedding response is empty")
	}
	var warnings []string
	if cached, ok, warning := s.LookupSemanticCache(ctx, req, embeddings[0], traceID, profile, start); ok {
		return cached, nil
	} else if warning != "" {
		warnings = append(warnings, warning)
	}
	searchReq := kb.SearchRequest{
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		Query:           req.Query,
		Vector:          embeddings[0],
		TopK:            topK,
	}
	var results []kb.SearchResult
	if retriever, ok := s.Retriever.(interface {
		RetrieveWithWarnings(context.Context, kb.SearchRequest) ([]kb.SearchResult, []string, error)
	}); ok {
		var retrievalWarnings []string
		results, retrievalWarnings, err = retriever.RetrieveWithWarnings(ctx, searchReq)
		warnings = append(warnings, retrievalWarnings...)
	} else {
		results, err = s.Retriever.Retrieve(ctx, searchReq)
	}
	if err != nil {
		return QueryResponse{}, err
	}
	if len(results) == 0 {
		return QueryResponse{
			Answer:      s.NoContextAnswer,
			TraceID:     traceID,
			CacheStatus: "miss",
			Profile:     profile,
			Warnings:    append(warnings, "no_retrieved_context"),
			CreatedAt:   time.Now().UTC(),
			LatencyMS:   time.Since(start).Milliseconds(),
		}, nil
	}
	results = s.ApplyRerank(ctx, req.Query, results)
	contextText, citations := s.Packer.Pack(results)
	system := "你是一个严格基于给定上下文回答的 RAG 助手。回答必须使用中文，并在事实来自上下文时引用 chunk id。"
	if profile == ProfileHighPrecision {
		system += " 当前为高精档，请更充分整合上下文。"
	}
	user := fmt.Sprintf("问题：%s\n\n上下文：\n%s", req.Query, contextText)
	promptText := s.PromptStrategy.Apply([]prompt.Segment{
		{Name: "system", Stable: true, Content: system},
		{Name: "context", Stable: true, Content: contextText},
		{Name: "question", Stable: false, Content: req.Query},
	})
	answer, err := s.Model.Chat(ctx, []ark.ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user + "\n\n缓存稳定前缀：\n" + promptText},
	})
	if err != nil {
		return QueryResponse{}, err
	}
	citations, validationWarnings := ValidateCitations(citations, results)
	warnings = append(warnings, validationWarnings...)
	resp := QueryResponse{
		Answer:          EnsureCitationHint(answer, citations),
		Citations:       citations,
		RetrievedChunks: results,
		TraceID:         traceID,
		CacheStatus:     "miss",
		Profile:         profile,
		Warnings:        warnings,
		CreatedAt:       time.Now().UTC(),
		LatencyMS:       time.Since(start).Milliseconds(),
	}
	if warning := s.StoreSemanticCache(ctx, req, embeddings[0], resp); warning != "" {
		resp.Warnings = append(resp.Warnings, warning)
	}
	return resp, nil
}

func (s *Service) LookupSemanticCache(ctx context.Context, req QueryRequest, vector []float64, traceID string, profile Profile, start time.Time) (QueryResponse, bool, string) {
	if s.Cache == nil || strings.TrimSpace(req.Query) == "" {
		return QueryResponse{}, false, ""
	}
	cached, ok, err := s.Cache.Lookup(ctx, SemanticCacheLookupRequest{
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		Query:           req.Query,
		Vector:          vector,
		Threshold:       s.SemanticCacheThreshold,
		Profile:         profile,
	})
	if err != nil {
		return QueryResponse{}, false, "semantic cache lookup failed: " + err.Error()
	}
	if !ok {
		return QueryResponse{}, false, ""
	}
	cached.CacheStatus = "hit"
	cached.TraceID = traceID
	cached.Profile = profile
	cached.LatencyMS = time.Since(start).Milliseconds()
	cached.CreatedAt = time.Now().UTC()
	return cached, true, ""
}

func (s *Service) StoreSemanticCache(ctx context.Context, req QueryRequest, vector []float64, resp QueryResponse) string {
	if s.Cache == nil || len(resp.Citations) == 0 {
		return ""
	}
	if err := s.Cache.Store(ctx, SemanticCacheEntry{
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		Query:           req.Query,
		Vector:          vector,
		Response:        resp,
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		return "semantic cache write failed: " + err.Error()
	}
	return ""
}

func (s *Service) Profile(requested Profile) Profile {
	if requested != "" {
		return requested
	}
	if s.DefaultProfile != "" {
		return s.DefaultProfile
	}
	return ProfileRealtime
}

func (s *Service) ApplyRerank(ctx context.Context, query string, results []kb.SearchResult) []kb.SearchResult {
	docs := make([]ark.RerankDocument, len(results))
	for i, result := range results {
		docs[i] = ark.RerankDocument{ID: result.Chunk.ID, Content: result.Chunk.Content}
	}
	reranked, err := s.Model.Rerank(ctx, query, docs, s.Packer.TopN)
	if err != nil || len(reranked) == 0 {
		return results
	}
	out := make([]kb.SearchResult, 0, len(reranked))
	for rank, item := range reranked {
		if item.Index < 0 || item.Index >= len(results) {
			continue
		}
		result := results[item.Index]
		result.Rank = rank + 1
		result.Score = item.Score
		result.From = "ark_rerank"
		out = append(out, result)
	}
	return out
}

func CacheKey(req QueryRequest) string {
	return cacheKey(req.TenantID, req.KnowledgeBaseID, strings.ToLower(strings.TrimSpace(req.Query)))
}

func EnsureCitationHint(answer string, citations []Citation) string {
	if len(citations) == 0 || strings.Contains(answer, citations[0].ChunkID) {
		return answer
	}
	return answer + " [" + citations[0].ChunkID + "]"
}
