package http

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/pipeline"
)

func (s *Server) listPipelineNodeDefinitions(_ context.Context, c *app.RequestContext) {
	if _, ok := requestPrincipal(c); !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"items": pipeline.BuiltinRegistry().Definitions()})
}
