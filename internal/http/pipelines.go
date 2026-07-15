package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/pipeline"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/release"
)

type createPipelineRequest struct {
	Name string `json:"name"`
}
type savePipelineDraftRequest struct {
	ExpectedRevision int64               `json:"expected_revision"`
	Definition       pipeline.Definition `json:"definition"`
}
type createPipelineVersionFromDraftRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

func (s *Server) listPipelines(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceRead, principal.TenantID, projectID) {
		return
	}
	items, err := s.App.Pipeline.ListPipelines(ctx, projectID)
	if err != nil {
		writePipelineError(c, err)
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"items": items})
}
func (s *Server) createPipeline(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceWrite, principal.TenantID, projectID) {
		return
	}
	var req createPipelineRequest
	if !bindJSON(c, &req) {
		return
	}
	item := pipeline.Pipeline{ID: id.New("pipe"), ProjectID: projectID, Name: strings.TrimSpace(req.Name), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := s.App.Pipeline.CreatePipeline(ctx, item); err != nil {
		writePipelineError(c, err)
		return
	}
	c.JSON(consts.StatusCreated, item)
}
func (s *Server) getPipelineDraft(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceRead, principal.TenantID, projectID) {
		return
	}
	item, err := s.App.Pipeline.GetDraft(ctx, projectID, c.Param("pipeline_id"))
	if err != nil {
		writePipelineError(c, err)
		return
	}
	c.JSON(consts.StatusOK, item)
}
func (s *Server) savePipelineDraft(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceWrite, principal.TenantID, projectID) {
		return
	}
	var req savePipelineDraftRequest
	if !bindJSON(c, &req) {
		return
	}
	item, err := s.App.Pipeline.SaveDraft(ctx, projectID, c.Param("pipeline_id"), req.ExpectedRevision, req.Definition)
	if err != nil {
		writePipelineError(c, err)
		return
	}
	c.JSON(consts.StatusOK, item)
}

func (s *Server) createPipelineVersionFromDraft(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceWrite, principal.TenantID, projectID) {
		return
	}
	var req createPipelineVersionFromDraftRequest
	if !bindJSON(c, &req) {
		return
	}
	draft, err := s.App.Pipeline.GetDraft(ctx, projectID, c.Param("pipeline_id"))
	if err != nil {
		writePipelineError(c, err)
		return
	}
	if draft.Revision != req.ExpectedRevision {
		writePipelineError(c, pipeline.ErrRevisionConflict)
		return
	}
	payload, err := json.Marshal(draft.Definition)
	if err != nil {
		writePipelineError(c, err)
		return
	}
	sum := sha256.Sum256(payload)
	contentHash := hex.EncodeToString(sum[:])
	version := release.Version{ID: id.New("pv"), ProjectID: projectID, ContentHash: contentHash, CreatedAt: time.Now().UTC()}
	if err := s.App.Release.CreateVersion(ctx, version); err != nil {
		writeReleaseError(c, err)
		return
	}
	c.JSON(consts.StatusCreated, map[string]any{"version": version, "draft_revision": draft.Revision})
}

func writePipelineError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, pipeline.ErrRevisionConflict):
		writeError(c, consts.StatusConflict, "pipeline_revision_conflict", err.Error())
	case errors.Is(err, pipeline.ErrNotFound):
		writeError(c, consts.StatusNotFound, "pipeline_not_found", err.Error())
	case errors.Is(err, pipeline.ErrInvalidDefinition):
		writeError(c, consts.StatusUnprocessableEntity, "pipeline_invalid_definition", err.Error())
	default:
		writeError(c, consts.StatusInternalServerError, "pipeline_operation_failed", "pipeline operation failed")
	}
}

func (s *Server) listPipelineNodeDefinitions(_ context.Context, c *app.RequestContext) {
	if _, ok := requestPrincipal(c); !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"items": pipeline.BuiltinRegistry().Definitions()})
}
