package memory

import (
	"context"
	"strings"
	"time"

	orag "github.com/shikanon/orag"
)

const (
	defaultTenantID        = "tenant_default"
	defaultKnowledgeBaseID = "kb_default"
)

// Client adapts the legacy memory API to the root ORAG SDK.
//
// Deprecated: use orag.Client with orag.MockConfig.
type Client struct {
	sdk      *orag.Client
	initErr  error
	tenantID string
	kbID     string
	sdkKBID  string
}

// Option customizes a Client.
//
// Deprecated: configure orag.Config directly.
type Option func(*Client)

// WithTenantID overrides the default tenant identifier used by the memory client.
//
// Deprecated: set orag.Config.TenantID.
func WithTenantID(tenantID string) Option {
	return func(c *Client) {
		if strings.TrimSpace(tenantID) != "" {
			c.tenantID = strings.TrimSpace(tenantID)
		}
	}
}

// WithKnowledgeBaseID overrides the legacy knowledge base identifier exposed
// by compatibility responses.
//
// Deprecated: create or select a knowledge base through orag.Client.
func WithKnowledgeBaseID(kbID string) Option {
	return func(c *Client) {
		if strings.TrimSpace(kbID) != "" {
			c.kbID = strings.TrimSpace(kbID)
		}
	}
}

// New creates a dependency-free compatibility client backed by orag.MockConfig.
//
// Deprecated: call orag.New(ctx, orag.MockConfig()).
func New(opts ...Option) *Client {
	c := &Client{
		tenantID: defaultTenantID,
		kbID:     defaultKnowledgeBaseID,
		sdkKBID:  defaultKnowledgeBaseID,
	}
	for _, opt := range opts {
		opt(c)
	}
	cfg := orag.MockConfig()
	cfg.TenantID = c.tenantID
	c.sdk, c.initErr = orag.New(context.Background(), cfg)
	return c
}

// Close releases resources owned by the underlying root SDK client.
func (c *Client) Close() error {
	if c == nil || c.sdk == nil {
		return nil
	}
	return c.sdk.Close()
}

// Document is a text document to add to the in-memory knowledge base.
type Document struct {
	ID        string
	Title     string
	SourceURI string
	Content   string
	Metadata  map[string]string
}

// DocumentRecord describes a document stored by the in-memory facade.
type DocumentRecord struct {
	ID              string
	TenantID        string
	KnowledgeBaseID string
	Title           string
	SourceURI       string
	ContentHash     string
	Metadata        map[string]string
	CreatedAt       time.Time
	Chunks          []Chunk
}

// Chunk is a searchable document segment.
type Chunk struct {
	ID              string
	TenantID        string
	KnowledgeBaseID string
	DocumentID      string
	Content         string
	SourceURI       string
	Section         string
	Metadata        map[string]string
}

// QueryRequest asks the in-memory client to search indexed documents.
type QueryRequest struct {
	Query   string
	TopK    int
	TraceID string
	Profile string
}

// QueryResponse contains an answer and response metadata from the root SDK.
type QueryResponse struct {
	Answer          string
	Citations       []Citation
	RetrievedChunks []SearchResult
	TraceID         string
	CacheStatus     string
	Profile         string
	Warnings        []string
	TraceSummary    TraceSummary
	LatencyMS       int64
	CreatedAt       time.Time
}

// Citation points back to the chunk used to form the answer.
type Citation struct {
	ChunkID    string
	DocumentID string
	SourceURI  string
	Section    string
	Quote      string
}

// SearchResult is a ranked memory-search hit.
type SearchResult struct {
	Chunk Chunk
	Score float64
	Rank  int
	From  string
}

// TraceSummary summarizes the node spans recorded for a query.
type TraceSummary struct {
	NodeCount        int
	SlowestNode      string
	SlowestLatencyMS int64
}

// TraceRecord stores the query trace in memory.
type TraceRecord struct {
	ID         string
	TenantID   string
	Profile    string
	LatencyMS  int64
	CreatedAt  time.Time
	HasError   bool
	ErrorCount int
	NodeSpans  []TraceNodeSpan
}

// TraceNodeSpan describes one logical step in the memory pipeline.
type TraceNodeSpan struct {
	ID        string
	NodeName  string
	Sequence  int
	LatencyMS int64
	Error     string
	StartedAt time.Time
	EndedAt   time.Time
	CreatedAt time.Time
}

// AddDocument stores a text document through the root SDK ingestion service.
func (c *Client) AddDocument(ctx context.Context, doc Document) (DocumentRecord, error) {
	if c.initErr != nil {
		return DocumentRecord{}, c.initErr
	}
	result, err := c.sdk.IngestText(ctx, orag.IngestTextRequest{
		TenantID:        c.tenantID,
		KnowledgeBaseID: c.sdkKBID,
		Name:            doc.Title,
		SourceURI:       doc.SourceURI,
		Text:            doc.Content,
	})
	if err != nil {
		return DocumentRecord{}, err
	}
	record := DocumentRecord{
		ID:              result.Document.ID,
		TenantID:        c.tenantID,
		KnowledgeBaseID: c.kbID,
		Title:           result.Document.Title,
		SourceURI:       result.Document.SourceURI,
		ContentHash:     result.Document.ContentHash,
		Metadata:        cloneMap(doc.Metadata),
		CreatedAt:       result.Document.CreatedAt,
		Chunks:          make([]Chunk, len(result.Chunks)),
	}
	for index := range result.Chunks {
		record.Chunks[index] = fromSDKChunk(result.Chunks[index], c.kbID, doc.Metadata)
	}
	return record, nil
}

// Query runs the root SDK query workflow with deterministic mock providers.
func (c *Client) Query(ctx context.Context, req QueryRequest) (QueryResponse, error) {
	if c.initErr != nil {
		return QueryResponse{}, c.initErr
	}
	result, err := c.sdk.Query(ctx, orag.QueryRequest{
		TenantID:        c.tenantID,
		KnowledgeBaseID: c.sdkKBID,
		Query:           req.Query,
		Profile:         req.Profile,
		TopK:            req.TopK,
		TraceID:         req.TraceID,
	})
	if err != nil {
		return QueryResponse{}, err
	}
	response := QueryResponse{
		Answer:          result.Answer,
		TraceID:         result.TraceID,
		CacheStatus:     result.CacheStatus,
		Profile:         result.Profile,
		Warnings:        append([]string(nil), result.Warnings...),
		LatencyMS:       result.LatencyMS,
		CreatedAt:       result.CreatedAt,
		Citations:       make([]Citation, len(result.Citations)),
		RetrievedChunks: make([]SearchResult, len(result.RetrievedChunks)),
	}
	if result.TraceSummary != nil {
		response.TraceSummary = TraceSummary{
			NodeCount:        result.TraceSummary.NodeCount,
			SlowestNode:      result.TraceSummary.SlowestNode,
			SlowestLatencyMS: result.TraceSummary.SlowestLatencyMS,
		}
	} else if trace, found, traceErr := c.sdk.GetTrace(ctx, orag.GetTraceRequest{TenantID: c.tenantID, ID: result.TraceID}); traceErr == nil && found {
		response.TraceSummary = summarizeSDKSpans(trace.NodeSpans)
	}
	for index, citation := range result.Citations {
		response.Citations[index] = Citation{
			ChunkID: citation.ChunkID, DocumentID: citation.DocumentID,
			SourceURI: citation.SourceURI, Section: citation.Section, Quote: citation.Quote,
		}
	}
	for index, item := range result.RetrievedChunks {
		response.RetrievedChunks[index] = SearchResult{
			Chunk: fromSDKChunk(item.Chunk, c.kbID, item.Chunk.Metadata),
			Score: item.Score, Rank: item.Rank, From: item.From,
		}
	}
	return response, nil
}

// Trace returns a trace recorded by the root SDK.
func (c *Client) Trace(ctx context.Context, traceID string) (TraceRecord, bool) {
	if c.initErr != nil {
		return TraceRecord{}, false
	}
	result, found, err := c.sdk.GetTrace(ctx, orag.GetTraceRequest{TenantID: c.tenantID, ID: traceID})
	if err != nil || !found {
		return TraceRecord{}, false
	}
	trace := TraceRecord{
		ID: result.ID, TenantID: result.TenantID, Profile: result.Profile,
		LatencyMS: result.LatencyMS, CreatedAt: result.CreatedAt,
		HasError: result.HasError, ErrorCount: result.ErrorCount,
		NodeSpans: make([]TraceNodeSpan, len(result.NodeSpans)),
	}
	for index, span := range result.NodeSpans {
		trace.NodeSpans[index] = TraceNodeSpan{
			ID: span.ID, NodeName: span.NodeName, Sequence: span.Sequence,
			LatencyMS: span.LatencyMS, Error: span.Error,
			StartedAt: span.StartedAt, EndedAt: span.EndedAt, CreatedAt: span.CreatedAt,
		}
	}
	return trace, true
}

func fromSDKChunk(chunk orag.Chunk, knowledgeBaseID string, fallbackMetadata map[string]string) Chunk {
	metadata := chunk.Metadata
	if len(metadata) == 0 {
		metadata = fallbackMetadata
	}
	return Chunk{
		ID: chunk.ID, TenantID: chunk.TenantID, KnowledgeBaseID: knowledgeBaseID,
		DocumentID: chunk.DocumentID, Content: chunk.Content, SourceURI: chunk.SourceURI,
		Section: chunk.Section, Metadata: cloneMap(metadata),
	}
}

func summarizeSDKSpans(spans []orag.TraceNodeSpan) TraceSummary {
	summary := TraceSummary{NodeCount: len(spans)}
	for _, span := range spans {
		if span.LatencyMS >= summary.SlowestLatencyMS {
			summary.SlowestNode = span.NodeName
			summary.SlowestLatencyMS = span.LatencyMS
		}
	}
	return summary
}

func cloneMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
