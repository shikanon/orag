package graph

import (
	"time"

	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
)

type NodeSpan struct {
	NodeName  string
	LatencyMS int64
	Error     string
}

type State struct {
	Request          rag.QueryRequest
	Response         rag.QueryResponse
	Start            time.Time
	TraceID          string
	Profile          rag.Profile
	TopK             int
	Cached           bool
	Embedding        []float64
	RewrittenQuery   string
	RetrievalQueries []rag.RetrievalQuery
	Results          []kb.SearchResult
	Context          string
	Citations        []rag.Citation
	PromptText       string
	Warnings         []string
	Spans            []NodeSpan
}
