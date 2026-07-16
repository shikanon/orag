package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/tutorial"
)

func (s *Server) importTutorialVideoSource(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok || !authorizeRequest(c, auth.ActionTutorialCloneCreate, tenantID(c), c.Param("project_id")) {
		return
	}
	if s.App.VideoImports == nil || string(c.FormValue("license_confirmed")) != "true" {
		writeError(c, consts.StatusBadRequest, "invalid_video_import", "license_confirmed must be true")
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid_video_import", "multipart field file is required")
		return
	}
	duration, err := strconv.ParseInt(string(c.FormValue("duration_ms")), 10, 64)
	if err != nil || duration <= 0 {
		writeError(c, consts.StatusBadRequest, "invalid_video_import", "duration_ms must be positive")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid_video_import", "file is unavailable")
		return
	}
	defer file.Close()
	source := tutorial.VideoSource{Alias: string(c.FormValue("alias")), SHA256: string(c.FormValue("sha256")), Bytes: fileHeader.Size, ContentType: string(c.FormValue("content_type")), DurationMS: duration}
	_, segments, err := s.App.VideoImports.Import(ctx, tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, c.Param("project_id"), source, file)
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid_video_import", "video source could not be verified")
		return
	}
	c.JSON(consts.StatusCreated, map[string]any{"source_alias": source.Alias, "temporal_segment_count": len(segments)})
}

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

type tutorialExperimentRunRequest struct {
	Variant        string `json:"variant"`
	IdempotencyKey string `json:"idempotency_key"`
}

type tutorialExperimentRunAcceptedResponse struct {
	RunID   string                 `json:"run_id"`
	PollURL string                 `json:"poll_url"`
	Run     tutorial.ExperimentRun `json:"run"`
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

func (s *Server) startTutorialExperimentRun(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}
	projectID := c.Param("project_id")
	experiment, err := s.App.TutorialClones.GetExperiment(ctx, tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, projectID)
	if err != nil {
		writeTutorialRunError(c, err)
		return
	}
	if experiment.ID != c.Param("experiment_id") {
		writeError(c, consts.StatusNotFound, "tutorial_experiment_not_found", "tutorial experiment not found")
		return
	}
	if !authorizeRequest(c, auth.ActionTutorialRunCreate, principal.TenantID, experiment.ProjectID) {
		return
	}
	var req tutorialExperimentRunRequest
	if !bindTutorialExperimentRunRequest(c, &req) {
		return
	}
	run, duplicate, err := s.App.TutorialRuns.StartVariant(ctx, tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, projectID, req.Variant, req.IdempotencyKey)
	if err != nil {
		writeTutorialRunError(c, err)
		return
	}
	c.JSON(consts.StatusAccepted, tutorialExperimentRunAcceptedResponse{
		RunID: run.ID, PollURL: "/v1/projects/" + projectID + "/tutorial-experiments/" + experiment.ID + "/runs/" + run.ID, Run: run,
	})
	if !duplicate && s.App.TutorialRunRunner != nil {
		s.App.TutorialRunRunner.Schedule(principal.TenantID, run.ID)
	}
}

func (s *Server) getTutorialExperimentRunComparison(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}
	projectID, experimentID, runID := c.Param("project_id"), c.Param("experiment_id"), c.Param("run_id")
	run, err := s.App.TutorialRuns.Get(ctx, tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, runID)
	if err != nil {
		writeTutorialRunError(c, err)
		return
	}
	if run.ProjectID != projectID || run.ExperimentID != experimentID {
		writeError(c, consts.StatusNotFound, "tutorial_experiment_run_not_found", "tutorial experiment run not found")
		return
	}
	if !authorizeRequest(c, auth.ActionTutorialRunRead, principal.TenantID, run.ProjectID) {
		return
	}
	comparison, err := s.App.TutorialRuns.Compare(ctx, tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, projectID, experimentID, runID)
	if err != nil {
		writeTutorialRunError(c, err)
		return
	}
	c.JSON(consts.StatusOK, comparison)
}

func (s *Server) getTutorialExperimentRun(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}
	run, err := s.App.TutorialRuns.Get(ctx, tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, c.Param("run_id"))
	if err != nil {
		writeTutorialRunError(c, err)
		return
	}
	if run.ProjectID != c.Param("project_id") || run.ExperimentID != c.Param("experiment_id") {
		writeError(c, consts.StatusNotFound, "tutorial_experiment_run_not_found", "tutorial experiment run not found")
		return
	}
	if !authorizeRequest(c, auth.ActionTutorialRunRead, principal.TenantID, run.ProjectID) {
		return
	}
	c.JSON(consts.StatusOK, run)
}

func (s *Server) tutorialExperimentRunAction(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}
	action := strings.TrimPrefix(c.Param("action"), "/")
	if !strings.HasSuffix(action, ":cancel") {
		writeError(c, consts.StatusNotFound, "tutorial_experiment_run_not_found", "tutorial experiment run not found")
		return
	}
	runID := strings.TrimSuffix(action, ":cancel")
	run, err := s.App.TutorialRuns.Get(ctx, tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, runID)
	if err != nil {
		writeTutorialRunError(c, err)
		return
	}
	if run.ProjectID != c.Param("project_id") || run.ExperimentID != c.Param("experiment_id") {
		writeError(c, consts.StatusNotFound, "tutorial_experiment_run_not_found", "tutorial experiment run not found")
		return
	}
	if !authorizeRequest(c, auth.ActionTutorialRunCancel, principal.TenantID, run.ProjectID) {
		return
	}
	cancelled, err := s.App.TutorialRuns.Cancel(ctx, tutorial.Subject{TenantID: principal.TenantID, ID: principal.SubjectID}, runID)
	if err != nil {
		writeTutorialRunError(c, err)
		return
	}
	c.JSON(consts.StatusAccepted, cancelled)
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

func writeTutorialRunError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, tutorial.ErrCloneExperimentAbsent):
		writeError(c, consts.StatusNotFound, "tutorial_experiment_not_found", "tutorial experiment not found")
	case errors.Is(err, tutorial.ErrExperimentRunNotFound):
		writeError(c, consts.StatusNotFound, "tutorial_experiment_run_not_found", "tutorial experiment run not found")
	case errors.Is(err, tutorial.ErrExperimentRunKey):
		writeError(c, consts.StatusBadRequest, "invalid_tutorial_experiment_run_request", "tutorial experiment run request is invalid")
	case errors.Is(err, tutorial.ErrExperimentRunVariant):
		writeError(c, consts.StatusBadRequest, "invalid_tutorial_experiment_run_request", "tutorial experiment run request is invalid")
	case errors.Is(err, tutorial.ErrBaselineRequired):
		writeError(c, consts.StatusConflict, "tutorial_baseline_required", "tutorial candidate requires a compatible completed baseline")
	case errors.Is(err, tutorial.ErrExperimentComparisonUnavailable):
		writeError(c, consts.StatusConflict, "tutorial_experiment_comparison_unavailable", "tutorial experiment comparison is unavailable")
	case errors.Is(err, tutorial.ErrRuntimeUnavailable):
		writeError(c, consts.StatusConflict, "tutorial_runtime_unavailable", "tutorial Pack does not declare a runnable runtime")
	case errors.Is(err, tutorial.ErrPackNotInstalled):
		writeError(c, consts.StatusConflict, "tutorial_pack_not_installed", "tutorial Pack is not installed")
	case errors.Is(err, tutorial.ErrExperimentRunCancelled):
		writeError(c, consts.StatusConflict, "tutorial_experiment_run_cancelled", "tutorial experiment run is cancelled")
	default:
		writeError(c, consts.StatusInternalServerError, "tutorial_experiment_run_failed", "tutorial experiment run operation failed")
	}
}

func bindTutorialExperimentRunRequest(c *app.RequestContext, dst *tutorialExperimentRunRequest) bool {
	decoder := json.NewDecoder(bytes.NewReader(c.Request.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid_tutorial_experiment_run_request", "tutorial experiment run request is invalid")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(c, consts.StatusBadRequest, "invalid_tutorial_experiment_run_request", "tutorial experiment run request is invalid")
		return false
	}
	return true
}
