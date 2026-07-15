package http

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/project"
)

type createAPIKeyRequest struct {
	Name      string     `json:"name"`
	Role      auth.Role  `json:"role"`
	ProjectID string     `json:"project_id,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (s *Server) createAPIKey(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok || !authorizeRequest(c, auth.ActionAPIKeyManage, principal.TenantID, "") {
		return
	}
	var req createAPIKeyRequest
	if !bindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.ProjectID) != "" {
		if _, err := s.App.Projects.Get(ctx, principal.TenantID, req.ProjectID); err != nil {
			writeAPIKeyProjectError(c, err)
			return
		}
	}
	created, err := s.App.APIKeys.Create(ctx, auth.APIKeyCreateInput{
		TenantID:  principal.TenantID,
		ProjectID: req.ProjectID,
		Name:      req.Name,
		Role:      req.Role,
		CreatedBy: string(principal.Kind) + ":" + principal.SubjectID,
		ExpiresAt: req.ExpiresAt,
	})
	if err != nil {
		writeAPIKeyError(c, err, "api_key_create_failed")
		return
	}
	c.JSON(consts.StatusCreated, created)
}

func (s *Server) listAPIKeys(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok || !authorizeRequest(c, auth.ActionAPIKeyManage, principal.TenantID, "") {
		return
	}
	items, err := s.App.APIKeys.List(ctx, principal.TenantID)
	if err != nil {
		writeAPIKeyError(c, err, "api_key_list_failed")
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"api_keys": items})
}

func (s *Server) revokeAPIKey(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok || !authorizeRequest(c, auth.ActionAPIKeyManage, principal.TenantID, "") {
		return
	}
	if err := s.App.APIKeys.Revoke(ctx, principal.TenantID, c.Param("api_key_id")); err != nil {
		writeAPIKeyError(c, err, "api_key_revoke_failed")
		return
	}
	c.Status(consts.StatusNoContent)
}

func writeAPIKeyProjectError(c *app.RequestContext, err error) {
	if errors.Is(err, project.ErrNotFound) {
		writeError(c, consts.StatusNotFound, "project_not_found", "project not found")
		return
	}
	writeError(c, consts.StatusInternalServerError, "project_lookup_failed", "project operation failed")
}

func writeAPIKeyError(c *app.RequestContext, err error, internalCode string) {
	switch {
	case errors.Is(err, auth.ErrAPIKeyInvalid):
		writeError(c, consts.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, auth.ErrAPIKeyNotFound):
		writeError(c, consts.StatusNotFound, "api_key_not_found", "api key not found")
	default:
		writeError(c, consts.StatusInternalServerError, internalCode, "api key operation failed")
	}
}
