package orag

import (
	"io"
	"time"
)

// KnowledgeBase is the public SDK representation of an ORAG knowledge base.
type KnowledgeBase struct {
	ID          string
	TenantID    string
	ProjectID   string
	Name        string
	Description string
	Metadata    map[string]string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CreateKnowledgeBaseRequest struct {
	TenantID    string
	ProjectID   string
	Name        string
	Description string
	Metadata    map[string]string
}

type ListKnowledgeBasesRequest struct{ TenantID string }

type GetKnowledgeBaseRequest struct {
	TenantID string
	ID       string
}

type DeleteKnowledgeBaseRequest struct {
	TenantID string
	ID       string
}

type IngestTextRequest struct {
	TenantID        string
	KnowledgeBaseID string
	Name            string
	SourceURI       string
	Text            string
}

type IngestFileRequest struct {
	TenantID        string
	KnowledgeBaseID string
	Name            string
	SourceURI       string
	Reader          io.Reader
}

type IngestResult struct {
	Document Document
	Job      IngestionJob
	Chunks   []Chunk
}

type Document struct {
	ID              string
	TenantID        string
	KnowledgeBaseID string
	SourceURI       string
	Title           string
	ContentHash     string
	Metadata        map[string]string
	CreatedAt       time.Time
}

type Chunk struct {
	ID              string
	TenantID        string
	KnowledgeBaseID string
	DocumentID      string
	Content         string
	ContextualText  string
	SourceURI       string
	Page            int
	Section         string
	Offset          int
	Metadata        map[string]string
}

type IngestionStatus string

const (
	IngestionRunning   IngestionStatus = "running"
	IngestionSucceeded IngestionStatus = "succeeded"
	IngestionFailed    IngestionStatus = "failed"
)

type IngestionJob struct {
	ID              string
	TenantID        string
	KnowledgeBaseID string
	Status          IngestionStatus
	SourceURI       string
	DocumentID      string
	ChunkCount      int
	Error           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type GetIngestionJobRequest struct {
	TenantID string
	ID       string
}

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

type GetTraceRequest struct {
	TenantID string
	ID       string
}

type ListTracesRequest struct {
	TenantID        string
	KnowledgeBaseID string
	Profile         string
	Since           time.Time
	Until           time.Time
	HasError        *bool
	SlowMS          int64
	Limit           int
}

type TraceRecord struct {
	ID              string
	TenantID        string
	KnowledgeBaseID string
	Query           string
	Profile         string
	Answer          string
	RetrievedChunks []string
	LatencyMS       int64
	CreatedAt       time.Time
	HasError        bool
	ErrorCount      int
	NodeSpans       []TraceNodeSpan
}

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
