package http

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/project"
)

func (s *Server) createProject(ctx context.Context, c *app.RequestContext) {
	var req project.CreateInput
	if !bindJSON(c, &req) {
		return
	}
	item, err := s.App.Projects.Create(ctx, tenantID(c), req)
	if err != nil {
		writeProjectError(c, err)
		return
	}
	c.JSON(consts.StatusCreated, item)
}

func (s *Server) listProjects(ctx context.Context, c *app.RequestContext) {
	items, err := s.App.Projects.List(ctx, tenantID(c))
	if err != nil {
		writeProjectError(c, err)
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"projects": items})
}

func (s *Server) getProject(ctx context.Context, c *app.RequestContext) {
	item, err := s.App.Projects.Get(ctx, tenantID(c), c.Param("project_id"))
	if err != nil {
		writeProjectError(c, err)
		return
	}
	c.JSON(consts.StatusOK, item)
}

func (s *Server) updateProject(ctx context.Context, c *app.RequestContext) {
	var req project.UpdateInput
	if !bindJSON(c, &req) {
		return
	}
	item, err := s.App.Projects.Update(ctx, tenantID(c), c.Param("project_id"), req)
	if err != nil {
		writeProjectError(c, err)
		return
	}
	c.JSON(consts.StatusOK, item)
}

func writeProjectError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, project.ErrTenantRequired), errors.Is(err, project.ErrNameRequired):
		writeError(c, consts.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, project.ErrNotFound):
		writeError(c, consts.StatusNotFound, "project_not_found", "project not found")
	default:
		writeError(c, consts.StatusConflict, "project_conflict", "project operation conflicts with current state")
	}
}
