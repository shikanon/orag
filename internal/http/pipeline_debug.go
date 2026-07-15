package http

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/observability"
	"github.com/shikanon/orag/internal/pipeline"
)

type pipelineDebugRequest struct {
	PipelineID       string       `json:"pipeline_id"`
	ExpectedRevision int64        `json:"expected_revision"`
	Environment      string       `json:"environment,omitempty"`
	Query            queryRequest `json:"query"`
}

// debugProjectQuery executes only a frozen development draft. Production and
// staging must use an immutable released version, never mutable draft state.
func (s *Server) debugProjectQuery(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceRead, principal.TenantID, projectID) {
		return
	}
	var req pipelineDebugRequest
	if !bindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.PipelineID) == "" || req.ExpectedRevision < 0 {
		writeError(c, consts.StatusBadRequest, "invalid_request", "pipeline_id and a non-negative expected_revision are required")
		return
	}
	if environment := strings.TrimSpace(req.Environment); environment != "" && environment != "development" {
		writeError(c, consts.StatusConflict, "pipeline_draft_environment_forbidden", "mutable drafts can only be debugged in development")
		return
	}
	if !validateQueryRequest(c, req.Query) {
		return
	}
	if s.App.PipelineDebug == nil {
		writeError(c, consts.StatusServiceUnavailable, "pipeline_debug_unavailable", "pipeline debug runner is not configured")
		return
	}
	ragReq := req.Query.ragRequest()
	if _, ok := s.authorizedKnowledgeBase(ctx, c, ragReq.KnowledgeBaseID, auth.ActionResourceRead); !ok {
		return
	}
	ragReq.TenantID = principal.TenantID
	ragReq.TraceID = requestTraceID(c)
	ctx = observability.WithTraceID(ctx, ragReq.TraceID)
	result, err := s.App.PipelineDebug.Run(ctx, pipeline.DebugRequest{
		ProjectID: projectID, PipelineID: req.PipelineID, ExpectedRevision: req.ExpectedRevision, Query: ragReq,
	})
	if err != nil {
		writePipelineError(c, err)
		return
	}
	s.observeRAGSuccess(result.Response.Profile, result.Response.CacheStatus, result.Response.LatencyMS)
	c.JSON(consts.StatusOK, result)
}
