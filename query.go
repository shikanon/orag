package orag

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/rag"
)

type QueryRequest struct {
	TenantID        string
	KnowledgeBaseID string
	Query           string
	Profile         string
	SessionID       string
	TopK            int
	TraceID         string
}

type QueryResponse struct {
	Answer          string
	Citations       []Citation
	RetrievedChunks []SearchResult
	TraceID         string
	CacheStatus     string
	Profile         string
	Route           *RouteDecision
	Warnings        []string
	TraceWarnings   []Warning
	TraceSummary    *TraceSummary
	LatencyMS       int64
	CreatedAt       time.Time
}

type Citation struct {
	ChunkID    string
	DocumentID string
	SourceURI  string
	Section    string
	Quote      string
}

type SearchResult struct {
	Chunk Chunk
	Score float64
	Rank  int
	From  string
}

type RouteDecision struct {
	Route    string
	Reason   string
	Strategy string
	Signals  []string
}

type Warning struct {
	Code    string
	Message string
}

type TraceSummary struct {
	NodeCount        int
	SlowestNode      string
	SlowestLatencyMS int64
}

func (c *Client) Query(ctx context.Context, req QueryRequest) (QueryResponse, error) {
	if err := c.requireOpen("query"); err != nil {
		return QueryResponse{}, err
	}
	if strings.TrimSpace(req.KnowledgeBaseID) == "" || strings.TrimSpace(req.Query) == "" {
		return QueryResponse{}, newError(CodeInvalidArgument, "query", req.KnowledgeBaseID, req.TraceID, false, errors.New("knowledge_base_id and query are required"))
	}
	tenantID := c.tenant(req.TenantID)
	if _, found, err := c.app.KBStore.GetKnowledgeBase(ctx, tenantID, req.KnowledgeBaseID); err != nil {
		return QueryResponse{}, wrapError("query", req.KnowledgeBaseID, req.TraceID, err)
	} else if !found {
		return QueryResponse{}, newError(CodeNotFound, "query", req.KnowledgeBaseID, req.TraceID, false, errors.New("knowledge base not found"))
	}
	result, err := c.app.RAG.Query(ctx, rag.QueryRequest{
		TenantID:        tenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		Query:           req.Query,
		Profile:         rag.Profile(req.Profile),
		SessionID:       req.SessionID,
		TopK:            req.TopK,
		TraceID:         req.TraceID,
	})
	if err != nil {
		return QueryResponse{}, wrapError("query", req.KnowledgeBaseID, req.TraceID, err)
	}
	return fromQueryResponse(result), nil
}

func fromQueryResponse(item rag.QueryResponse) QueryResponse {
	citations := make([]Citation, len(item.Citations))
	for index := range item.Citations {
		value := item.Citations[index]
		citations[index] = Citation{ChunkID: value.ChunkID, DocumentID: value.DocumentID, SourceURI: value.SourceURI, Section: value.Section, Quote: value.Quote}
	}
	retrieved := make([]SearchResult, len(item.RetrievedChunks))
	for index := range item.RetrievedChunks {
		value := item.RetrievedChunks[index]
		retrieved[index] = SearchResult{Chunk: fromChunk(value.Chunk), Score: value.Score, Rank: value.Rank, From: value.From}
	}
	warnings := make([]Warning, len(item.TraceWarnings))
	for index := range item.TraceWarnings {
		warnings[index] = Warning{Code: item.TraceWarnings[index].Code, Message: item.TraceWarnings[index].Message}
	}
	result := QueryResponse{Answer: item.Answer, Citations: citations, RetrievedChunks: retrieved, TraceID: item.TraceID, CacheStatus: item.CacheStatus, Profile: string(item.Profile), Warnings: append([]string(nil), item.Warnings...), TraceWarnings: warnings, LatencyMS: item.LatencyMS, CreatedAt: item.CreatedAt}
	if item.Route != nil {
		result.Route = &RouteDecision{Route: string(item.Route.Route), Reason: item.Route.Reason, Strategy: item.Route.Strategy, Signals: append([]string(nil), item.Route.Signals...)}
	}
	if item.TraceSummary != nil {
		result.TraceSummary = &TraceSummary{NodeCount: item.TraceSummary.NodeCount, SlowestNode: item.TraceSummary.SlowestNode, SlowestLatencyMS: item.TraceSummary.SlowestLatencyMS}
	}
	return result
}
