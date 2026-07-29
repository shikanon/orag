package http

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/eval"
)

func (s *Server) compareEvaluation(ctx context.Context, c *app.RequestContext) {
	baselineID := strings.TrimSpace(string(c.QueryArgs().Peek("baseline_id")))
	candidateID := strings.TrimSpace(c.Param("id"))
	if baselineID == "" {
		writeError(c, consts.StatusBadRequest, "invalid_request", "baseline_id is required")
		return
	}
	baseline, baselineFound, err := s.getEvaluationForRequest(ctx, c, baselineID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "evaluation_lookup_failed", err.Error())
		return
	}
	if !baselineFound || !authorizeRequest(c, auth.ActionResourceRead, tenantID(c), baseline.ProjectID) {
		if baselineFound {
			return
		}
		writeError(c, consts.StatusNotFound, "evaluation_not_found", "evaluation not found")
		return
	}
	candidate, candidateFound, err := s.getEvaluationForRequest(ctx, c, candidateID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "evaluation_lookup_failed", err.Error())
		return
	}
	if !candidateFound || !authorizeRequest(c, auth.ActionResourceRead, tenantID(c), candidate.ProjectID) {
		if candidateFound {
			return
		}
		writeError(c, consts.StatusNotFound, "evaluation_not_found", "evaluation not found")
		return
	}
	if baseline.ProjectID != candidate.ProjectID {
		writeError(c, consts.StatusBadRequest, "evaluation_project_mismatch", "evaluations must belong to the same project")
		return
	}
	comparability := eval.CompareRuns(baseline, candidate)
	response := map[string]any{"baseline_id": baselineID, "candidate_id": candidateID, "comparability": comparability}
	if comparability.Comparable {
		baselineDetail, _, detailErr := s.App.Eval.GetDetail(ctx, tenantID(c), baselineID, eval.EvaluationDetailOptions{IncludeItems: true})
		if detailErr != nil {
			writeError(c, consts.StatusInternalServerError, "evaluation_lookup_failed", detailErr.Error())
			return
		}
		candidateDetail, _, detailErr := s.App.Eval.GetDetail(ctx, tenantID(c), candidateID, eval.EvaluationDetailOptions{IncludeItems: true})
		if detailErr != nil {
			writeError(c, consts.StatusInternalServerError, "evaluation_lookup_failed", detailErr.Error())
			return
		}
		response["metric_comparisons"] = eval.ComparePairedMetrics(baseline, candidate, baselineDetail.Items, candidateDetail.Items)
	}
	c.JSON(consts.StatusOK, response)
}
