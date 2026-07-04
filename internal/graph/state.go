package graph

import (
	"time"

	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
)

type NodeSpan struct {
	NodeName  string
	Sequence  int
	LatencyMS int64
	Error     string
	StartedAt time.Time
	EndedAt   time.Time
}

type State struct {
	Request          rag.QueryRequest
	Response         rag.QueryResponse
	Start            time.Time
	TraceID          string
	Profile          rag.Profile
	TopK             int
	Route            *rag.RouteDecision
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

func (s State) isDirectRoute() bool {
	return s.Route != nil && s.Route.Route == rag.QueryRouteDirect
}

func (s State) isSingleRoute() bool {
	return s.Route != nil && s.Route.Route == rag.QueryRouteSingleRetrieval
}
