package http

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/eval"
)

func (s *Server) listEvaluationMetrics(_ context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusOK, map[string]any{"items": eval.DefaultMetricRegistry.Definitions()})
}
