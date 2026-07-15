package http

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/release"
)

func (s *Server) listReleaseEnvironments(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceRead, principal.TenantID, projectID) {
		return
	}
	items, err := s.App.Release.Environments(ctx, projectID)
	if err != nil {
		writeReleaseError(c, err)
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"items": items})
}

func (s *Server) listReleases(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceRead, principal.TenantID, projectID) {
		return
	}
	items, err := s.App.Release.Releases(ctx, projectID)
	if err != nil {
		writeReleaseError(c, err)
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"items": items})
}

func (s *Server) promoteRelease(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceWrite, principal.TenantID, projectID) {
		return
	}
	var req release.PromoteRequest
	if !bindJSON(c, &req) {
		return
	}
	req.ProjectID, req.Actor = projectID, principal.SubjectID
	item, err := s.App.Release.Promote(ctx, req)
	if err != nil {
		writeReleaseError(c, err)
		return
	}
	c.JSON(consts.StatusCreated, item)
}

func (s *Server) rollbackRelease(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceWrite, principal.TenantID, projectID) {
		return
	}
	var req release.RollbackRequest
	if !bindJSON(c, &req) {
		return
	}
	req.ProjectID, req.Actor = projectID, principal.SubjectID
	req.Environment = release.EnvironmentKind(c.Param("environment"))
	item, err := s.App.Release.Rollback(ctx, req)
	if err != nil {
		writeReleaseError(c, err)
		return
	}
	c.JSON(consts.StatusCreated, item)
}

func releaseProjectRequest(c *app.RequestContext) (string, auth.Principal, bool) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return "", auth.Principal{}, false
	}
	projectID := c.Param("project_id")
	if principal.ProjectID != "" && principal.ProjectID != projectID {
		writeError(c, consts.StatusNotFound, "project_not_found", "project not found")
		return "", auth.Principal{}, false
	}
	return projectID, principal, true
}

func writeReleaseError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, release.ErrConflict):
		writeError(c, consts.StatusConflict, "release_conflict", err.Error())
	case errors.Is(err, release.ErrGateFailed):
		writeError(c, consts.StatusUnprocessableEntity, "release_gate_failed", err.Error())
	case errors.Is(err, release.ErrBindingMissing):
		writeError(c, consts.StatusUnprocessableEntity, "release_binding_missing", err.Error())
	case errors.Is(err, release.ErrInvalidTransition), errors.Is(err, release.ErrRollbackTarget):
		writeError(c, consts.StatusBadRequest, "invalid_release_request", err.Error())
	case errors.Is(err, release.ErrNotFound):
		writeError(c, consts.StatusNotFound, "release_resource_not_found", err.Error())
	default:
		writeError(c, consts.StatusInternalServerError, "release_operation_failed", "release operation failed")
	}
}
