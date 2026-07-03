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
	Request        rag.QueryRequest
	Response       rag.QueryResponse
	Start          time.Time
	TraceID        string
	Profile        rag.Profile
	TopK           int
	Cached         bool
	Embedding      []float64
	RewrittenQuery string
	Results        []kb.SearchResult
	Context        string
	Citations      []rag.Citation
	PromptText     string
	Warnings       []string
	Spans          []NodeSpan
	spanRecorder   *spanRecorder
}

type spanRecorder struct {
	spans []NodeSpan
}

func (r *spanRecorder) append(span NodeSpan) {
	if r == nil {
		return
	}
	r.spans = append(r.spans, span)
}

func (r *spanRecorder) snapshot() []NodeSpan {
	if r == nil || len(r.spans) == 0 {
		return nil
	}
	out := make([]NodeSpan, len(r.spans))
	copy(out, r.spans)
	return out
}
