package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultTenantID        = "tenant_default"
	defaultKnowledgeBaseID = "kb_default"
	defaultTopK            = 3
	defaultProfile         = "realtime"
)

// Client is a small in-memory ORAG facade intended for examples and local demos.
type Client struct {
	mu          sync.RWMutex
	tenantID    string
	kbID        string
	documents   map[string]DocumentRecord
	chunks      map[string]Chunk
	traceSeq    int
	traces      map[string]TraceRecord
	clock       func() time.Time
	idGenerator func(prefix, seed string) string
}

// Option customizes a Client.
type Option func(*Client)

// WithTenantID overrides the default tenant identifier used by the memory client.
func WithTenantID(tenantID string) Option {
	return func(c *Client) {
		if strings.TrimSpace(tenantID) != "" {
			c.tenantID = strings.TrimSpace(tenantID)
		}
	}
}

// WithKnowledgeBaseID overrides the default knowledge base identifier.
func WithKnowledgeBaseID(kbID string) Option {
	return func(c *Client) {
		if strings.TrimSpace(kbID) != "" {
			c.kbID = strings.TrimSpace(kbID)
		}
	}
}

// New creates a dependency-free in-memory client.
func New(opts ...Option) *Client {
	c := &Client{
		tenantID:  defaultTenantID,
		kbID:      defaultKnowledgeBaseID,
		documents: map[string]DocumentRecord{},
		chunks:    map[string]Chunk{},
		traces:    map[string]TraceRecord{},
		clock:     func() time.Time { return time.Now().UTC() },
		idGenerator: func(prefix, seed string) string {
			sum := sha256.Sum256([]byte(seed))
			return prefix + "_" + hex.EncodeToString(sum[:])[:16]
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
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

// QueryResponse contains a deterministic answer and response metadata.
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

// TraceNodeSpan describes one logical step in the example memory pipeline.
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

// AddDocument stores a text document and splits it into simple searchable chunks.
func (c *Client) AddDocument(ctx context.Context, doc Document) (DocumentRecord, error) {
	if err := ctx.Err(); err != nil {
		return DocumentRecord{}, err
	}
	content := strings.TrimSpace(doc.Content)
	if content == "" {
		return DocumentRecord{}, errors.New("memory: document content is required")
	}

	now := c.clock()
	docID := strings.TrimSpace(doc.ID)
	if docID == "" {
		docID = c.idGenerator("doc", doc.Title+"\n"+doc.SourceURI+"\n"+content)
	}
	record := DocumentRecord{
		ID:              docID,
		TenantID:        c.tenantID,
		KnowledgeBaseID: c.kbID,
		Title:           strings.TrimSpace(doc.Title),
		SourceURI:       strings.TrimSpace(doc.SourceURI),
		ContentHash:     stableHash(content),
		Metadata:        cloneMap(doc.Metadata),
		CreatedAt:       now,
	}
	record.Chunks = c.splitChunks(record, content)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.documents[record.ID] = record
	for _, chunk := range record.Chunks {
		c.chunks[chunk.ID] = chunk
	}
	return copyDocument(record), nil
}

// Query retrieves relevant chunks and creates a deterministic answer.
func (c *Client) Query(ctx context.Context, req QueryRequest) (QueryResponse, error) {
	if err := ctx.Err(); err != nil {
		return QueryResponse{}, err
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return QueryResponse{}, errors.New("memory: query is required")
	}

	start := c.clock()
	traceID := strings.TrimSpace(req.TraceID)
	if traceID == "" {
		traceID = c.nextTraceID(query)
	}
	profile := strings.TrimSpace(req.Profile)
	if profile == "" {
		profile = defaultProfile
	}
	topK := req.TopK
	if topK <= 0 {
		topK = defaultTopK
	}

	retrieveStart := c.clock()
	results := c.retrieve(query, topK)
	retrieveLatency := elapsedMillis(retrieveStart, c.clock())

	generateStart := c.clock()
	resp := QueryResponse{
		RetrievedChunks: results,
		TraceID:         traceID,
		CacheStatus:     "disabled",
		Profile:         profile,
		CreatedAt:       c.clock(),
	}
	if len(results) == 0 {
		resp.Answer = "No matching memory documents found."
		resp.Warnings = []string{"no_retrieved_context"}
	} else {
		resp.Citations = citationsFor(results)
		resp.Answer = answerFor(query, results)
	}
	generateLatency := elapsedMillis(generateStart, c.clock())

	resp.LatencyMS = elapsedMillis(start, c.clock())
	spans := []TraceNodeSpan{
		c.newSpan(traceID, 1, "retrieve", retrieveStart, retrieveLatency, ""),
		c.newSpan(traceID, 2, "generate_answer", generateStart, generateLatency, ""),
	}
	resp.TraceSummary = summarizeTrace(spans)
	c.storeTrace(TraceRecord{
		ID:        traceID,
		TenantID:  c.tenantID,
		Profile:   profile,
		LatencyMS: resp.LatencyMS,
		CreatedAt: resp.CreatedAt,
		NodeSpans: spans,
	})
	return resp, nil
}

// Trace returns a previously recorded query trace.
func (c *Client) Trace(_ context.Context, traceID string) (TraceRecord, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	trace, ok := c.traces[strings.TrimSpace(traceID)]
	if !ok {
		return TraceRecord{}, false
	}
	return copyTrace(trace), true
}

func (c *Client) splitChunks(doc DocumentRecord, content string) []Chunk {
	sections := splitSections(content)
	chunks := make([]Chunk, 0, len(sections))
	for i, section := range sections {
		chunkID := c.idGenerator("chunk", fmt.Sprintf("%s/%d/%s", doc.ID, i+1, section))
		chunks = append(chunks, Chunk{
			ID:              chunkID,
			TenantID:        doc.TenantID,
			KnowledgeBaseID: doc.KnowledgeBaseID,
			DocumentID:      doc.ID,
			Content:         section,
			SourceURI:       doc.SourceURI,
			Section:         fmt.Sprintf("section-%d", i+1),
			Metadata:        cloneMap(doc.Metadata),
		})
	}
	return chunks
}

func (c *Client) retrieve(query string, topK int) []SearchResult {
	terms := queryTerms(query)
	c.mu.RLock()
	defer c.mu.RUnlock()

	results := make([]SearchResult, 0, len(c.chunks))
	for _, chunk := range c.chunks {
		score := scoreChunk(terms, chunk.Content)
		if score <= 0 {
			continue
		}
		results = append(results, SearchResult{Chunk: copyChunk(chunk), Score: score, From: "memory"})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Chunk.ID < results[j].Chunk.ID
	})
	if len(results) > topK {
		results = results[:topK]
	}
	for i := range results {
		results[i].Rank = i + 1
	}
	return results
}

func (c *Client) nextTraceID(query string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.traceSeq++
	return c.idGenerator("trace", fmt.Sprintf("%d/%s", c.traceSeq, query))
}

func (c *Client) newSpan(traceID string, sequence int, nodeName string, startedAt time.Time, latencyMS int64, errText string) TraceNodeSpan {
	endedAt := startedAt.Add(time.Duration(latencyMS) * time.Millisecond)
	return TraceNodeSpan{
		ID:        c.idGenerator("span", fmt.Sprintf("%s/%d/%s", traceID, sequence, nodeName)),
		NodeName:  nodeName,
		Sequence:  sequence,
		LatencyMS: latencyMS,
		Error:     errText,
		StartedAt: startedAt,
		EndedAt:   endedAt,
		CreatedAt: c.clock(),
	}
}

func (c *Client) storeTrace(trace TraceRecord) {
	trace.HasError, trace.ErrorCount = traceErrors(trace.NodeSpans)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.traces[trace.ID] = copyTrace(trace)
}

func splitSections(content string) []string {
	paragraphs := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n\n")
	sections := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		section := strings.Join(strings.Fields(paragraph), " ")
		if section != "" {
			sections = append(sections, section)
		}
	}
	if len(sections) == 0 {
		return []string{strings.Join(strings.Fields(content), " ")}
	}
	return sections
}

func queryTerms(query string) []string {
	fields := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	seen := map[string]bool{}
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		if len(field) < 2 || seen[field] {
			continue
		}
		seen[field] = true
		terms = append(terms, field)
	}
	return terms
}

func scoreChunk(terms []string, content string) float64 {
	if len(terms) == 0 {
		return 0
	}
	lower := strings.ToLower(content)
	var score float64
	for _, term := range terms {
		if strings.Contains(lower, term) {
			score++
		}
	}
	return score / float64(len(terms))
}

func answerFor(query string, results []SearchResult) string {
	top := results[0]
	snippet := top.Chunk.Content
	if len(snippet) > 160 {
		snippet = strings.TrimSpace(snippet[:160]) + "..."
	}
	return fmt.Sprintf("Found %d relevant memory chunk(s) for %q. Top source: %s. Snippet: %s [%s]",
		len(results), query, sourceLabel(top.Chunk), snippet, top.Chunk.ID)
}

func citationsFor(results []SearchResult) []Citation {
	citations := make([]Citation, 0, len(results))
	for _, result := range results {
		quote := result.Chunk.Content
		if len(quote) > 120 {
			quote = strings.TrimSpace(quote[:120]) + "..."
		}
		citations = append(citations, Citation{
			ChunkID:    result.Chunk.ID,
			DocumentID: result.Chunk.DocumentID,
			SourceURI:  result.Chunk.SourceURI,
			Section:    result.Chunk.Section,
			Quote:      quote,
		})
	}
	return citations
}

func summarizeTrace(spans []TraceNodeSpan) TraceSummary {
	var summary TraceSummary
	for _, span := range spans {
		summary.NodeCount++
		if span.LatencyMS >= summary.SlowestLatencyMS {
			summary.SlowestLatencyMS = span.LatencyMS
			summary.SlowestNode = span.NodeName
		}
	}
	return summary
}

func traceErrors(spans []TraceNodeSpan) (bool, int) {
	var count int
	for _, span := range spans {
		if span.Error != "" {
			count++
		}
	}
	return count > 0, count
}

func sourceLabel(chunk Chunk) string {
	if chunk.SourceURI != "" {
		return chunk.SourceURI
	}
	return chunk.DocumentID
}

func elapsedMillis(start, end time.Time) int64 {
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func stableHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyDocument(in DocumentRecord) DocumentRecord {
	out := in
	out.Metadata = cloneMap(in.Metadata)
	out.Chunks = make([]Chunk, len(in.Chunks))
	for i, chunk := range in.Chunks {
		out.Chunks[i] = copyChunk(chunk)
	}
	return out
}

func copyChunk(in Chunk) Chunk {
	out := in
	out.Metadata = cloneMap(in.Metadata)
	return out
}

func copyTrace(in TraceRecord) TraceRecord {
	out := in
	out.NodeSpans = append([]TraceNodeSpan(nil), in.NodeSpans...)
	return out
}
