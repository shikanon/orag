package http

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

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
	c.Set("tenant_id", claims.TenantID)
	c.Next(ctx)
}

func (s *Server) metricsMiddleware(ctx context.Context, c *app.RequestContext) {
	c.Next(ctx)
	if s.App != nil && s.App.Metrics != nil {
		s.App.Metrics.IncHTTPRequests()
	}
}

func tenantID(c *app.RequestContext) string {
	v, ok := c.Get("tenant_id")
	if !ok {
		return "tenant_default"
	}
	tenantID, ok := v.(string)
	if !ok || tenantID == "" {
		return "tenant_default"
	}
	return tenantID
}
