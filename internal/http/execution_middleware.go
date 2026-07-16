package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/execution"
)

// executionMiddleware is deliberately fail-fast. It never retains waiting
// requests in an unbounded in-memory queue: a full class responds with 429 and
// lets the caller retry after its own bounded backoff.
func (s *Server) executionMiddleware(ctx context.Context, c *app.RequestContext) {
	operation, ok := executionOperation(c.FullPath(), string(c.Method()))
	if !ok || s.Execution == nil {
		c.Next(ctx)
		return
	}
	runCtx, release, admitted := s.Execution.Start(ctx, operation)
	if !admitted {
		c.Header("Retry-After", "1")
		writeErrorDetails(c, http.StatusTooManyRequests, "execution_capacity_exhausted", "operation capacity is currently exhausted; retry with backoff", map[string]string{"operation": string(operation)})
		c.Abort()
		return
	}
	defer release()
	c.Next(runCtx)
	if runCtx.Err() == context.DeadlineExceeded {
		writeErrorDetails(c, http.StatusGatewayTimeout, "execution_deadline_exceeded", "operation exceeded its configured execution deadline", map[string]string{"operation": string(operation)})
		c.Abort()
	}
}

func executionOperation(route, method string) (execution.Operation, bool) {
	if method == consts.MethodGet || method == consts.MethodHead || method == consts.MethodOptions {
		return "", false
	}
	switch route {
	case "/v1/knowledge-bases/:id/documents", "/v1/knowledge-bases/:id/documents:import", "/v1/uploads/*action":
		return execution.Ingestion, true
	case "/v1/query", "/v1/query:stream":
		return execution.Query, true
	case "/v1/evaluations":
		return execution.Evaluation, true
	}
	if strings.HasPrefix(route, "/v1/projects/:project_id/") &&
		(strings.Contains(route, "/releases") || strings.Contains(route, "/environments/") || strings.Contains(route, "/versions")) {
		return execution.Release, true
	}
	return "", false
}
