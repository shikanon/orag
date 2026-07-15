package orag

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/storage/postgres"
)

func (c *Client) CreateKnowledgeBase(ctx context.Context, req CreateKnowledgeBaseRequest) (KnowledgeBase, error) {
	if err := c.requireOpen("create_knowledge_base"); err != nil {
		return KnowledgeBase{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return KnowledgeBase{}, newError(CodeInvalidArgument, "create_knowledge_base", "knowledge_base", "", false, errors.New("name is required"))
	}
	tenantID := c.tenant(req.TenantID)
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID != "" {
		if _, err := c.app.Projects.Get(ctx, tenantID, projectID); err != nil {
			return KnowledgeBase{}, controlPlaneError("create_knowledge_base", projectID, err)
		}
	}
	now := time.Now().UTC()
	item := kb.KnowledgeBase{
		ID:          id.New("kb"),
		TenantID:    tenantID,
		ProjectID:   projectID,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Metadata:    cloneStrings(req.Metadata),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := c.app.KBStore.PutKnowledgeBase(ctx, item); err != nil {
		return KnowledgeBase{}, wrapError("create_knowledge_base", item.ID, "", err)
	}
	return fromKnowledgeBase(item), nil
}

func (c *Client) ListKnowledgeBases(ctx context.Context, req ListKnowledgeBasesRequest) ([]KnowledgeBase, error) {
	if err := c.requireOpen("list_knowledge_bases"); err != nil {
		return nil, err
	}
	items, err := c.app.KBStore.ListKnowledgeBases(ctx, c.tenant(req.TenantID))
	if err != nil {
		return nil, wrapError("list_knowledge_bases", "", "", err)
	}
	result := make([]KnowledgeBase, len(items))
	for index := range items {
		result[index] = fromKnowledgeBase(items[index])
	}
	return result, nil
}

func (c *Client) GetKnowledgeBase(ctx context.Context, req GetKnowledgeBaseRequest) (KnowledgeBase, bool, error) {
	if err := c.requireOpen("get_knowledge_base"); err != nil {
		return KnowledgeBase{}, false, err
	}
	item, found, err := c.app.KBStore.GetKnowledgeBase(ctx, c.tenant(req.TenantID), strings.TrimSpace(req.ID))
	if err != nil {
		return KnowledgeBase{}, false, wrapError("get_knowledge_base", req.ID, "", err)
	}
	if !found {
		return KnowledgeBase{}, false, nil
	}
	return fromKnowledgeBase(item), true, nil
}

func (c *Client) DeleteKnowledgeBase(ctx context.Context, req DeleteKnowledgeBaseRequest) error {
	if err := c.requireOpen("delete_knowledge_base"); err != nil {
		return err
	}
	deleted, err := c.app.KBStore.DeleteKnowledgeBase(ctx, c.tenant(req.TenantID), strings.TrimSpace(req.ID))
	if err != nil {
		return wrapError("delete_knowledge_base", req.ID, "", err)
	}
	if !deleted {
		return newError(CodeNotFound, "delete_knowledge_base", req.ID, "", false, errors.New("knowledge base not found"))
	}
	return nil
}

func (c *Client) IngestText(ctx context.Context, req IngestTextRequest) (IngestResult, error) {
	return c.ingest(ctx, req.TenantID, req.KnowledgeBaseID, req.Name, req.SourceURI, []byte(req.Text))
}

func (c *Client) IngestFile(ctx context.Context, req IngestFileRequest) (IngestResult, error) {
	if req.Reader == nil {
		return IngestResult{}, newError(CodeInvalidArgument, "ingest_file", req.KnowledgeBaseID, "", false, errors.New("reader is required"))
	}
	if err := c.requireOpen("ingest_file"); err != nil {
		return IngestResult{}, err
	}
	limit := c.app.Config.Ingestion.MaxDocumentBytes
	reader := req.Reader
	if limit > 0 {
		reader = io.LimitReader(req.Reader, limit+1)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return IngestResult{}, wrapError("ingest_file", req.KnowledgeBaseID, "", err)
	}
	if limit > 0 && int64(len(body)) > limit {
		return IngestResult{}, newError(CodeInvalidArgument, "ingest_file", req.KnowledgeBaseID, "", false, fmt.Errorf("document exceeds max size %d bytes", limit))
	}
	return c.ingest(ctx, req.TenantID, req.KnowledgeBaseID, req.Name, req.SourceURI, body)
}

func (c *Client) ingest(ctx context.Context, tenantID, knowledgeBaseID, name, sourceURI string, body []byte) (IngestResult, error) {
	if err := c.requireOpen("ingest"); err != nil {
		return IngestResult{}, err
	}
	if strings.TrimSpace(knowledgeBaseID) == "" || strings.TrimSpace(name) == "" || len(body) == 0 {
		return IngestResult{}, newError(CodeInvalidArgument, "ingest", knowledgeBaseID, "", false, errors.New("knowledge_base_id, name, and content are required"))
	}
	result, err := c.app.Ingest.Ingest(ctx, ingest.Request{
		TenantID:        c.tenant(tenantID),
		KnowledgeBaseID: knowledgeBaseID,
		SourceURI:       strings.TrimSpace(sourceURI),
		Name:            strings.TrimSpace(name),
		Content:         append([]byte(nil), body...),
	})
	if err != nil {
		if errors.Is(err, ingest.ErrKnowledgeBaseNotFound) {
			return IngestResult{}, newError(CodeNotFound, "ingest", knowledgeBaseID, "", false, err)
		}
		return IngestResult{}, wrapError("ingest", knowledgeBaseID, "", err)
	}
	return fromIngestResult(result), nil
}

func (c *Client) GetIngestionJob(ctx context.Context, req GetIngestionJobRequest) (IngestionJob, bool, error) {
	if err := c.requireOpen("get_ingestion_job"); err != nil {
		return IngestionJob{}, false, err
	}
	job, found, err := c.app.Ingest.Jobs.GetJob(ctx, c.tenant(req.TenantID), strings.TrimSpace(req.ID))
	if err != nil {
		return IngestionJob{}, false, wrapError("get_ingestion_job", req.ID, "", err)
	}
	if !found {
		return IngestionJob{}, false, nil
	}
	return fromIngestionJob(job), true, nil
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

func (c *Client) GetTrace(ctx context.Context, req GetTraceRequest) (TraceRecord, bool, error) {
	if err := c.requireOpen("get_trace"); err != nil {
		return TraceRecord{}, false, err
	}
	trace, found, err := c.app.Traces.GetTraceForTenant(ctx, c.tenant(req.TenantID), strings.TrimSpace(req.ID))
	if err != nil {
		return TraceRecord{}, false, wrapError("get_trace", req.ID, req.ID, err)
	}
	if !found {
		return TraceRecord{}, false, nil
	}
	return fromTraceRecord(trace), true, nil
}

func (c *Client) ListTraces(ctx context.Context, req ListTracesRequest) ([]TraceRecord, error) {
	if err := c.requireOpen("list_traces"); err != nil {
		return nil, err
	}
	items, err := c.app.Traces.ListTraces(ctx, postgres.TraceListFilter{
		TenantID: c.tenant(req.TenantID),
		KBID:     req.KnowledgeBaseID,
		Profile:  rag.Profile(req.Profile),
		Since:    req.Since,
		Until:    req.Until,
		HasError: req.HasError,
		SlowMS:   req.SlowMS,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, wrapError("list_traces", "", "", err)
	}
	result := make([]TraceRecord, len(items))
	for index := range items {
		result[index] = fromTraceRecord(items[index])
	}
	return result, nil
}

func (c *Client) tenant(value string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return c.tenantID
}

func (c *Client) requireOpen(operation string) error {
	if c == nil || c.app == nil {
		return newError(CodeUnavailable, operation, "", "", false, errClientClosed)
	}
	if c.closed.Load() {
		return newError(CodeUnavailable, operation, "", "", false, errClientClosed)
	}
	return nil
}

func fromKnowledgeBase(item kb.KnowledgeBase) KnowledgeBase {
	return KnowledgeBase{ID: item.ID, TenantID: item.TenantID, ProjectID: item.ProjectID, Name: item.Name, Description: item.Description, Metadata: cloneStrings(item.Metadata), CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func fromIngestResult(result ingest.Result) IngestResult {
	chunks := make([]Chunk, len(result.Chunks))
	for index := range result.Chunks {
		chunks[index] = fromChunk(result.Chunks[index])
	}
	return IngestResult{Document: fromDocument(result.Document), Job: fromIngestionJob(result.Job), Chunks: chunks}
}

func fromDocument(item kb.Document) Document {
	return Document{ID: item.ID, TenantID: item.TenantID, KnowledgeBaseID: item.KnowledgeBaseID, SourceURI: item.SourceURI, Title: item.Title, ContentHash: item.ContentHash, Metadata: cloneStrings(item.Metadata), CreatedAt: item.CreatedAt}
}

func fromChunk(item kb.Chunk) Chunk {
	return Chunk{ID: item.ID, TenantID: item.TenantID, KnowledgeBaseID: item.KnowledgeBaseID, DocumentID: item.DocumentID, Content: item.Content, ContextualText: item.ContextualText, SourceURI: item.SourceURI, Page: item.Page, Section: item.Section, Offset: item.Offset, Metadata: cloneStrings(item.Metadata)}
}

func fromIngestionJob(item ingest.Job) IngestionJob {
	return IngestionJob{ID: item.ID, TenantID: item.TenantID, KnowledgeBaseID: item.KnowledgeBaseID, Status: IngestionStatus(item.Status), SourceURI: item.SourceURI, DocumentID: item.DocumentID, ChunkCount: item.ChunkCount, Error: item.Error, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
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

func fromTraceRecord(item postgres.TraceRecord) TraceRecord {
	spans := make([]TraceNodeSpan, len(item.NodeSpans))
	for index := range item.NodeSpans {
		value := item.NodeSpans[index]
		spans[index] = TraceNodeSpan{ID: value.ID, NodeName: value.NodeName, Sequence: value.Sequence, LatencyMS: value.LatencyMS, Error: value.Error, StartedAt: value.StartedAt, EndedAt: value.EndedAt, CreatedAt: value.CreatedAt}
	}
	return TraceRecord{ID: item.ID, TenantID: item.TenantID, KnowledgeBaseID: item.KBID, Query: item.Query, Profile: string(item.Profile), Answer: item.Answer, RetrievedChunks: append([]string(nil), item.RetrievedChunks...), LatencyMS: item.LatencyMS, CreatedAt: item.CreatedAt, HasError: item.HasError, ErrorCount: item.ErrorCount, NodeSpans: spans}
}
