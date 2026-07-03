package rag

import (
	"time"

	"github.com/shikanon/orag/internal/kb"
)

type Profile string

const (
	ProfileRealtime      Profile = "realtime"
	ProfileHighPrecision Profile = "high_precision"
)

type QueryRequest struct {
	TenantID        string  `json:"-"`
	TraceID         string  `json:"-"`
	KnowledgeBaseID string  `json:"knowledge_base_id"`
	Query           string  `json:"query"`
	Profile         Profile `json:"profile,omitempty"`
	SessionID       string  `json:"session_id,omitempty"`
	TopK            int     `json:"top_k,omitempty"`
}

type Citation struct {
	ChunkID    string `json:"chunk_id"`
	DocumentID string `json:"document_id"`
	SourceURI  string `json:"source_uri"`
	Section    string `json:"section,omitempty"`
	Quote      string `json:"quote,omitempty"`
}

const WarningCodeTraceStoreFailed = "trace_store_failed"

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type TraceSummary struct {
	NodeCount        int    `json:"node_count"`
	SlowestNode      string `json:"slowest_node,omitempty"`
	SlowestLatencyMS int64  `json:"slowest_latency_ms"`
}

type QueryResponse struct {
	Answer          string            `json:"answer"`
	Citations       []Citation        `json:"citations"`
	RetrievedChunks []kb.SearchResult `json:"retrieved_chunks"`
	TraceID         string            `json:"trace_id"`
	CacheStatus     string            `json:"cache_status"`
	Profile         Profile           `json:"profile"`
	Warnings        []string          `json:"warnings,omitempty"`
	TraceWarnings   []Warning         `json:"trace_warnings,omitempty"`
	TraceSummary    *TraceSummary     `json:"trace_summary,omitempty"`
	LatencyMS       int64             `json:"latency_ms"`
	CreatedAt       time.Time         `json:"created_at"`
}
