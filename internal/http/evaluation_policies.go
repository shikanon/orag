package http

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/evaluationpolicy"
)

func (s *Server) listProjectEvaluationPolicies(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceRead, principal.TenantID, projectID) {
		return
	}
	items, err := s.App.EvaluationPolicy.List(ctx, principal.TenantID, projectID)
	if err != nil {
		writeEvaluationPolicyError(c, err)
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"items": items})
}

func (s *Server) createProjectEvaluationPolicy(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceWrite, principal.TenantID, projectID) {
		return
	}
	var req evaluationpolicy.CreateInput
	if !bindJSON(c, &req) {
		return
	}
	item, err := s.App.EvaluationPolicy.Create(ctx, principal.TenantID, projectID, req)
	if err != nil {
		writeEvaluationPolicyError(c, err)
		return
	}
	c.JSON(consts.StatusCreated, item)
}

func writeEvaluationPolicyError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, evaluationpolicy.ErrDatasetNotFound):
		writeError(c, consts.StatusNotFound, "evaluation_policy_dataset_not_found", "evaluation policy dataset was not found in this project")
	case errors.Is(err, evaluationpolicy.ErrPolicyNotFound):
		writeError(c, consts.StatusNotFound, "evaluation_policy_not_found", "evaluation policy was not found")
	case errors.Is(err, evaluationpolicy.ErrInvalidPolicy), errors.Is(err, evaluationpolicy.ErrEvaluationMismatch):
		writeError(c, consts.StatusUnprocessableEntity, "invalid_evaluation_policy", err.Error())
	default:
		writeError(c, consts.StatusInternalServerError, "evaluation_policy_operation_failed", "evaluation policy operation failed")
	}
}
