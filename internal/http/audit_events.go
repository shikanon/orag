package http

import (
	"context"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/audit"
	"github.com/shikanon/orag/internal/auth"
)

const (
	auditMaturityHeader = "X-Orag-Maturity"
	auditMaturityValue  = "experimental"
)

type AuditService interface {
	ListAuditEvents(ctx context.Context, tenantID string, filter audit.AuditFilter) ([]audit.AuditEvent, string, error)
	GetAuditEvent(ctx context.Context, tenantID, eventID string) (audit.AuditEvent, bool, error)
}

func (s *Server) listAuditEvents(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}

	filter, ok := parseAuditListFilter(c)
	if !ok {
		return
	}
	if principal.ProjectID != "" {
		if filter.ProjectID != "" && filter.ProjectID != principal.ProjectID {
			writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
			return
		}
		filter.ProjectID = principal.ProjectID
	}

	events, nextCursor, err := s.auditService().ListAuditEvents(ctx, principal.TenantID, filter)
	if err != nil {
		writeAuditError(c, err)
		return
	}

	setAuditMaturityHeader(c)
	c.JSON(consts.StatusOK, map[string]any{
		"data":        events,
		"next_cursor": nextCursor,
	})
}

func (s *Server) listProjectAuditEvents(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}

	projectID := c.Param("project_id")
	if !authorizeRequest(c, auth.ActionProjectRead, principal.TenantID, projectID) {
		return
	}
	filter, ok := parseAuditListFilter(c)
	if !ok {
		return
	}
	filter.ProjectID = projectID

	events, nextCursor, err := s.auditService().ListAuditEvents(ctx, principal.TenantID, filter)
	if err != nil {
		writeAuditError(c, err)
		return
	}

	setAuditMaturityHeader(c)
	c.JSON(consts.StatusOK, map[string]any{
		"data":        events,
		"next_cursor": nextCursor,
	})
}

func (s *Server) listKnowledgeBaseAuditEvents(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}

	kbID := c.Param("id")
	kb, found := s.authorizedKnowledgeBase(ctx, c, kbID, auth.ActionResourceRead)
	if !found {
		return
	}
	filter, ok := parseAuditListFilter(c)
	if !ok {
		return
	}
	filter.ResourceType = audit.ResourceTypeKnowledgeBase
	filter.ResourceID = kbID
	filter.ProjectID = kb.ProjectID

	events, nextCursor, err := s.auditService().ListAuditEvents(ctx, principal.TenantID, filter)
	if err != nil {
		writeAuditError(c, err)
		return
	}

	setAuditMaturityHeader(c)
	c.JSON(consts.StatusOK, map[string]any{
		"data":        events,
		"next_cursor": nextCursor,
	})
}

func (s *Server) listReleaseAuditEvents(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}
	if principal.ProjectID != "" {
		writeError(c, consts.StatusForbidden, "forbidden", "project-scoped release audit requires the project activity endpoint")
		return
	}

	releaseID := c.Param("id")
	filter, ok := parseAuditListFilter(c)
	if !ok {
		return
	}
	filter.ResourceType = audit.ResourceTypeRelease
	filter.ResourceID = releaseID

	events, nextCursor, err := s.auditService().ListAuditEvents(ctx, principal.TenantID, filter)
	if err != nil {
		writeAuditError(c, err)
		return
	}

	setAuditMaturityHeader(c)
	c.JSON(consts.StatusOK, map[string]any{
		"data":        events,
		"next_cursor": nextCursor,
	})
}

func parseAuditListFilter(c *app.RequestContext) (audit.AuditFilter, bool) {
	var filter audit.AuditFilter

	if resourceType := strings.TrimSpace(c.Query("resource_type")); resourceType != "" {
		filter.ResourceType = resourceType
	}
	if resourceID := strings.TrimSpace(c.Query("resource_id")); resourceID != "" {
		filter.ResourceID = resourceID
	}
	if action := strings.TrimSpace(c.Query("action")); action != "" {
		filter.Action = action
	}
	if actorID := strings.TrimSpace(c.Query("actor_id")); actorID != "" {
		filter.ActorID = actorID
	}
	if outcome := strings.TrimSpace(c.Query("outcome")); outcome != "" {
		filter.Outcome = outcome
	}
	if projectID := strings.TrimSpace(c.Query("project_id")); projectID != "" {
		filter.ProjectID = projectID
	}

	limit := 20
	if limitStr := strings.TrimSpace(c.Query("limit")); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed <= 0 {
			writeError(c, consts.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return audit.AuditFilter{}, false
		}
		if parsed > 100 {
			parsed = 100
		}
		limit = parsed
	}
	filter.Limit = limit

	if cursor := strings.TrimSpace(c.Query("cursor")); cursor != "" {
		filter.Cursor = cursor
	}

	return filter, true
}

func setAuditMaturityHeader(c *app.RequestContext) {
	c.Header(auditMaturityHeader, auditMaturityValue)
}

func writeAuditError(c *app.RequestContext, err error) {
	writeError(c, consts.StatusInternalServerError, "audit_operation_failed", err.Error())
}

var auditServiceInstance AuditService = &noopAuditService{}

func SetAuditService(svc AuditService) {
	auditServiceInstance = svc
}

func (s *Server) auditService() AuditService {
	if s.Audit != nil {
		return s.Audit
	}
	return auditServiceInstance
}

type auditServiceAdapter struct{ service *audit.AuditService }

func newAuditServiceAdapter(service *audit.AuditService) AuditService {
	return &auditServiceAdapter{service: service}
}
func (a *auditServiceAdapter) ListAuditEvents(ctx context.Context, tenantID string, filter audit.AuditFilter) ([]audit.AuditEvent, string, error) {
	filter.TenantID = tenantID
	return a.service.List(ctx, filter)
}
func (a *auditServiceAdapter) GetAuditEvent(ctx context.Context, tenantID, eventID string) (audit.AuditEvent, bool, error) {
	event, found, err := a.service.Get(ctx, eventID)
	if err != nil || !found || event.TenantID != tenantID {
		return audit.AuditEvent{}, false, err
	}
	return event, true, nil
}

type noopAuditService struct{}

func (n *noopAuditService) ListAuditEvents(_ context.Context, _ string, _ audit.AuditFilter) ([]audit.AuditEvent, string, error) {
	return nil, "", errAuditServiceUnavailable
}

func (n *noopAuditService) GetAuditEvent(_ context.Context, _, _ string) (audit.AuditEvent, bool, error) {
	return audit.AuditEvent{}, false, errAuditServiceUnavailable
}

var errAuditServiceUnavailable = &auditServiceUnavailableError{}

type auditServiceUnavailableError struct{}

func (e *auditServiceUnavailableError) Error() string {
	return "audit service is not configured"
}
