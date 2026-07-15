package http

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/tutorial"
)

type tutorialCloneRequest struct {
	Version  string `json:"version"`
	PackTier string `json:"pack_tier"`
	Project  struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"project"`
	IdempotencyKey  string `json:"idempotency_key"`
	LicenseAccepted bool   `json:"license_accepted"`
}

type tutorialCloneAcceptedResponse struct {
	JobID     string            `json:"job_id"`
	ProjectID string            `json:"project_id"`
	PollURL   string            `json:"poll_url"`
	Job       tutorial.CloneJob `json:"job"`
}

func (s *Server) createTutorialClone(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok || !authorizeRequest(c, auth.ActionTutorialCloneCreate, tenantID(c), "") {
		return
	}
	var req tutorialCloneRequest
	if !bindJSON(c, &req) {
		return
	}
	job, duplicate, err := s.App.TutorialClones.Start(ctx, tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, tutorial.CloneRequest{
		TemplateID: c.Param("template_id"), Version: req.Version, Tier: req.PackTier,
		ProjectName: req.Project.Name, ProjectDescription: req.Project.Description,
		IdempotencyKey: req.IdempotencyKey, LicenseAccepted: req.LicenseAccepted,
	})
	if err != nil {
		writeTutorialCloneError(c, err)
		return
	}
	response := tutorialCloneAcceptedResponse{
		JobID: job.ID, ProjectID: job.ProjectID, PollURL: "/v1/tutorial-clone-jobs/" + job.ID, Job: job,
	}
	c.JSON(consts.StatusAccepted, response)
	if !duplicate && s.App.TutorialCloneRunner != nil {
		s.App.TutorialCloneRunner.Schedule(tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, job.ID)
	}
}

func (s *Server) getTutorialCloneJob(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}
	job, err := s.App.TutorialClones.GetJob(ctx, tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, c.Param("job_id"))
	if err != nil {
		writeTutorialCloneError(c, err)
		return
	}
	if !authorizeRequest(c, auth.ActionTutorialCloneRead, principal.TenantID, job.ProjectID) {
		return
	}
	c.JSON(consts.StatusOK, job)
}

func (s *Server) retryTutorialClone(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}
	action := strings.TrimPrefix(c.Param("action"), "/")
	if !strings.HasSuffix(action, ":retry") || strings.TrimSuffix(action, ":retry") == "" {
		writeError(c, consts.StatusNotFound, "tutorial_clone_not_found", "tutorial clone resource not found")
		return
	}
	jobID := strings.TrimSuffix(action, ":retry")
	current, err := s.App.TutorialClones.GetJob(ctx, tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, jobID)
	if err != nil {
		writeTutorialCloneError(c, err)
		return
	}
	if !authorizeRequest(c, auth.ActionTutorialCloneRetry, principal.TenantID, current.ProjectID) {
		return
	}
	job, err := s.App.TutorialClones.Retry(ctx, tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, current.ID)
	if err != nil {
		writeTutorialCloneError(c, err)
		return
	}
	c.JSON(consts.StatusAccepted, job)
	if s.App.TutorialCloneRunner != nil {
		s.App.TutorialCloneRunner.Schedule(tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, job.ID)
	}
}

func (s *Server) getProjectTutorialExperiment(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}
	projectID := c.Param("project_id")
	if principal.ProjectID != "" && principal.ProjectID != projectID {
		writeError(c, consts.StatusNotFound, "project_not_found", "project not found")
		return
	}
	experiment, err := s.App.TutorialClones.GetExperiment(ctx, tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, projectID)
	if err != nil {
		writeTutorialCloneError(c, err)
		return
	}
	if !authorizeRequest(c, auth.ActionTutorialCloneRead, principal.TenantID, experiment.ProjectID) {
		return
	}
	c.JSON(consts.StatusOK, experiment)
}

func writeTutorialCloneError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, tutorial.ErrTemplateNotFound):
		writeError(c, consts.StatusNotFound, "tutorial_not_found", "tutorial template not found")
	case errors.Is(err, tutorial.ErrVersionNotFound):
		writeError(c, consts.StatusNotFound, "tutorial_version_not_found", "tutorial template version not found")
	case errors.Is(err, tutorial.ErrCloneJobNotFound), errors.Is(err, tutorial.ErrCloneExperimentAbsent):
		writeError(c, consts.StatusNotFound, "tutorial_clone_not_found", "tutorial clone resource not found")
	case errors.Is(err, tutorial.ErrCloneSubjectRequired), errors.Is(err, tutorial.ErrCloneProjectName), errors.Is(err, tutorial.ErrCloneIdempotencyKey), errors.Is(err, tutorial.ErrCloneLicenseRequired), errors.Is(err, tutorial.ErrManifestInvalid):
		writeError(c, consts.StatusBadRequest, "invalid_tutorial_clone_request", "tutorial clone request is invalid")
	case errors.Is(err, tutorial.ErrCloneNotRetryable):
		writeError(c, consts.StatusConflict, "tutorial_clone_not_retryable", "tutorial clone job cannot be retried")
	default:
		if strings.TrimSpace(err.Error()) == "" {
			writeError(c, consts.StatusInternalServerError, "tutorial_clone_failed", "tutorial clone operation failed")
			return
		}
		writeError(c, consts.StatusInternalServerError, "tutorial_clone_failed", "tutorial clone operation failed")
	}
}
