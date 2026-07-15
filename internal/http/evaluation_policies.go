package http

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/evaluationpolicy"
	"github.com/shikanon/orag/internal/release"
)

type recordProjectEvaluationEvidenceRequest struct {
	PolicyID        string `json:"policy_id"`
	EvaluationRunID string `json:"evaluation_run_id"`
	Environment     string `json:"environment"`
}

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

// recordProjectEvaluationEvidence derives immutable gate evidence from a
// stored evaluation run. The client supplies only resource IDs; metric values
// and pass/fail results always come from the server-side run and policy.
func (s *Server) recordProjectEvaluationEvidence(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceWrite, principal.TenantID, projectID) {
		return
	}
	var req recordProjectEvaluationEvidenceRequest
	if !bindJSON(c, &req) {
		return
	}
	environment := release.EnvironmentKind(strings.TrimSpace(req.Environment))
	if environment != release.Development && environment != release.Staging && environment != release.Production {
		writeError(c, consts.StatusBadRequest, "invalid_release_request", "environment must be development, staging, or production")
		return
	}
	policy, err := s.App.EvaluationPolicy.Get(ctx, principal.TenantID, projectID, strings.TrimSpace(req.PolicyID))
	if err != nil {
		writeEvaluationPolicyError(c, err)
		return
	}
	run, found, err := s.App.Eval.GetInProject(ctx, principal.TenantID, projectID, strings.TrimSpace(req.EvaluationRunID))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "evaluation_lookup_failed", "evaluation lookup failed")
		return
	}
	if !found {
		writeError(c, consts.StatusNotFound, "evaluation_not_found", "evaluation not found")
		return
	}
	version, err := s.App.Release.Version(ctx, projectID, c.Param("version_id"))
	if err != nil {
		writeReleaseError(c, err)
		return
	}
	evidence, err := evaluationpolicy.Freeze(policy, run, version.ID, version.ContentHash, time.Now().UTC())
	if err != nil {
		writeEvaluationPolicyError(c, err)
		return
	}
	evidence.Environment = string(environment)
	evidence.FrozenInput.Environment = string(environment)
	if err := s.App.EvaluationPolicy.RecordEvidence(ctx, evidence); err != nil {
		writeEvaluationPolicyError(c, err)
		return
	}
	if err := s.App.Release.Validate(ctx, projectID, version.ID, release.Evidence{EnvironmentID: string(environment), Passed: evidence.Passed, ContentHash: version.ContentHash}); err != nil {
		writeReleaseError(c, err)
		return
	}
	c.JSON(consts.StatusCreated, evidence)
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
