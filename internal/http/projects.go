package http

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/project"
)

func (s *Server) createProject(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok || !authorizeRequest(c, auth.ActionProjectCreate, principal.TenantID, "") {
		return
	}
	var req project.CreateInput
	if !bindJSON(c, &req) {
		return
	}
	item, err := s.App.Projects.Create(ctx, tenantID(c), req)
	if err != nil {
		writeProjectError(c, err, "project_create_failed")
		return
	}
	c.JSON(consts.StatusCreated, item)
}

func (s *Server) listProjects(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok || !authorizeRequest(c, auth.ActionProjectList, principal.TenantID, "") {
		return
	}
	items, err := s.App.Projects.List(ctx, tenantID(c))
	if err != nil {
		writeProjectError(c, err, "project_list_failed")
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"projects": items})
}

func (s *Server) getProject(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}
	projectID := c.Param("project_id")
	if principal.ProjectID != "" && principal.ProjectID != projectID {
		writeError(c, consts.StatusNotFound, "project_not_found", "project not found")
		return
	}
	item, err := s.App.Projects.Get(ctx, principal.TenantID, projectID)
	if err != nil {
		writeProjectError(c, err, "project_lookup_failed")
		return
	}
	if !authorizeRequest(c, auth.ActionProjectRead, principal.TenantID, item.ID) {
		return
	}
	c.JSON(consts.StatusOK, item)
}

func (s *Server) updateProject(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}
	projectID := c.Param("project_id")
	if principal.ProjectID != "" && principal.ProjectID != projectID {
		writeError(c, consts.StatusNotFound, "project_not_found", "project not found")
		return
	}
	existing, err := s.App.Projects.Get(ctx, principal.TenantID, projectID)
	if err != nil {
		writeProjectError(c, err, "project_lookup_failed")
		return
	}
	if !authorizeRequest(c, auth.ActionProjectUpdate, principal.TenantID, existing.ID) {
		return
	}
	var req project.UpdateInput
	if !bindJSON(c, &req) {
		return
	}
	item, err := s.App.Projects.Update(ctx, principal.TenantID, projectID, req)
	if err != nil {
		writeProjectError(c, err, "project_update_failed")
		return
	}
	c.JSON(consts.StatusOK, item)
}

func writeProjectError(c *app.RequestContext, err error, internalCode string) {
	switch {
	case errors.Is(err, project.ErrTenantRequired), errors.Is(err, project.ErrNameRequired):
		writeError(c, consts.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, project.ErrNotFound):
		writeError(c, consts.StatusNotFound, "project_not_found", "project not found")
	case errors.Is(err, project.ErrConflict):
		writeError(c, consts.StatusConflict, "project_conflict", "project operation conflicts with current state")
	default:
		writeError(c, consts.StatusInternalServerError, internalCode, "project operation failed")
	}
}
