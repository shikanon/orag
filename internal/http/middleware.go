package http

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/observability"
)

func (s *Server) traceMiddleware(ctx context.Context, c *app.RequestContext) {
	traceID := strings.TrimSpace(string(c.GetHeader(observability.TraceIDHeader)))
	if traceID == "" {
		traceID = observability.EnsureTraceID(ctx)
	}
	c.Set("trace_id", traceID)
	c.Header(observability.TraceIDHeader, traceID)
	c.Next(observability.WithTraceID(ctx, traceID))
}

func (s *Server) authMiddleware(ctx context.Context, c *app.RequestContext) {
	header := string(c.GetHeader("Authorization"))
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if token == "" {
		writeError(c, consts.StatusUnauthorized, "missing_bearer_token", "missing bearer token")
		c.Abort()
		return
	}
	var principal auth.Principal
	var err error
	if strings.HasPrefix(token, "orag_sk_") {
		principal, err = s.App.APIKeys.Authenticate(ctx, token)
	} else {
		var claims auth.Claims
		claims, err = s.App.Auth.ParseToken(token)
		if err == nil {
			principal = claims.Principal()
		}
	}
	if err != nil || !principal.Valid() {
		writeError(c, consts.StatusUnauthorized, "invalid_bearer_token", "invalid bearer token")
		c.Abort()
		return
	}
	c.Set("principal", principal)
	c.Set("tenant_id", principal.TenantID)
	if principal.ProjectID != "" && !projectScopedRouteSupported(c) {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		c.Abort()
		return
	}
	c.Next(ctx)
}

func projectScopedRouteSupported(c *app.RequestContext) bool {
	switch c.FullPath() {
	case "/v1/projects/:project_id",
		"/v1/projects/:project_id/environments", "/v1/projects/:project_id/releases", "/v1/projects/:project_id/releases:promote", "/v1/projects/:project_id/environments/:environment/rollback",
		"/v1/projects/:project_id/versions", "/v1/projects/:project_id/versions/:version_id/validations",
		"/v1/knowledge-bases", "/v1/knowledge-bases/:id",
		"/v1/knowledge-bases/:id/documents", "/v1/knowledge-bases/:id/documents:import",
		"/v1/knowledge-bases/:id/uploads", "/v1/uploads/:id", "/v1/uploads/*action",
		"/v1/query", "/v1/query:stream", "/v1/datasets", "/v1/datasets/:id/items",
		"/v1/evaluations", "/v1/evaluations/:id",
		"/v1/optimizations", "/v1/optimizations/:id", "/v1/optimizations/*action":
		return true
	default:
		return false
	}
}

func authorizeRequest(c *app.RequestContext, action auth.Action, resourceTenantID, resourceProjectID string) bool {
	principal, ok := requestPrincipal(c)
	if !ok || auth.Authorize(principal, action, resourceTenantID, resourceProjectID) != nil {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return false
	}
	return true
}

func (s *Server) metricsMiddleware(ctx context.Context, c *app.RequestContext) {
	start := time.Now()
	c.Next(ctx)
	status := c.Response.StatusCode()
	latencyMS := time.Since(start).Milliseconds()
	if s.App != nil && s.App.Metrics != nil {
		s.App.Metrics.ObserveHTTP(string(c.Method()), requestRoute(c), status, latencyMS)
	}
	if s.App != nil && s.App.Logger != nil {
		attrs := []slog.Attr{
			slog.String("method", string(c.Method())),
			slog.String("route", requestRoute(c)),
			slog.String("path", string(c.Path())),
			slog.Int("status", status),
			slog.Int64("latency", latencyMS),
			slog.String("trace_id", requestTraceID(c)),
		}
		if code := requestErrorCode(c); code != "" {
			attrs = append(attrs, slog.String("error_code", code))
		}
		s.App.Logger.LogAttrs(ctx, slog.LevelInfo, "http_request_completed", attrs...)
	}
}

func requestRoute(c *app.RequestContext) string {
	if route := c.FullPath(); route != "" {
		return route
	}
	return string(c.Path())
}

func requestErrorCode(c *app.RequestContext) string {
	v, ok := c.Get("error_code")
	if !ok {
		return ""
	}
	code, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(code)
}

func tenantID(c *app.RequestContext) string {
	principal, ok := requestPrincipal(c)
	if !ok {
		return ""
	}
	return principal.TenantID
}

func requestPrincipal(c *app.RequestContext) (auth.Principal, bool) {
	v, ok := c.Get("principal")
	if !ok {
		return auth.Principal{}, false
	}
	principal, ok := v.(auth.Principal)
	return principal, ok && principal.Valid()
}
