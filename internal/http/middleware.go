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
	claims, err := s.App.Auth.ParseToken(token)
	if err != nil {
		writeError(c, consts.StatusUnauthorized, "invalid_bearer_token", "invalid bearer token")
		c.Abort()
		return
	}
	principal := claims.Principal()
	c.Set("principal", principal)
	c.Set("tenant_id", principal.TenantID)
	c.Next(ctx)
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
