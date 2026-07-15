package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/shikanon/orag/internal/graph"
	"github.com/shikanon/orag/internal/rag"
)

type DebugRequest struct {
	ProjectID        string
	PipelineID       string
	ExpectedRevision int64
	Query            rag.QueryRequest
}

type DiagnosticEvent struct {
	Sequence  int    `json:"sequence"`
	NodeID    string `json:"node_id"`
	NodeType  string `json:"node_type"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

type DebugResponse struct {
	Revision int64             `json:"revision"`
	TraceID  string            `json:"trace_id"`
	Response rag.QueryResponse `json:"response"`
	Events   []DiagnosticEvent `json:"events"`
}

// DebugRunner executes exactly the draft revision loaded at the beginning of
// the request. It never mutates the draft and rejects stale revisions.
type DebugRunner struct {
	Drafts   *Service
	Compiler Compiler
}

func (r DebugRunner) Run(ctx context.Context, request DebugRequest) (DebugResponse, error) {
	if r.Drafts == nil {
		return DebugResponse{}, fmt.Errorf("debug runner requires a pipeline service")
	}
	draft, err := r.Drafts.GetDraft(ctx, request.ProjectID, request.PipelineID)
	if err != nil {
		return DebugResponse{}, err
	}
	if request.ExpectedRevision != draft.Revision {
		return DebugResponse{}, ErrRevisionConflict
	}
	runnable, err := r.Compiler.Compile(ctx, draft.Definition)
	if err != nil {
		return DebugResponse{}, err
	}
	started := time.Now()
	state, err := runnable.Invoke(ctx, graph.State{Request: request.Query})
	if err != nil {
		return DebugResponse{}, err
	}
	if state.Response.LatencyMS == 0 {
		state.Response.LatencyMS = time.Since(started).Milliseconds()
	}
	events := make([]DiagnosticEvent, 0, len(state.Spans))
	for _, span := range state.Spans {
		events = append(events, DiagnosticEvent{Sequence: span.Sequence, NodeID: span.NodeName, NodeType: span.NodeName, LatencyMS: span.LatencyMS, Error: span.Error})
	}
	return DebugResponse{Revision: draft.Revision, TraceID: state.TraceID, Response: state.Response, Events: events}, nil
}
