package http

import (
	"context"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/modelreadiness"
)

const (
	modelReadinessMaturityHeader = "X-Orag-Maturity"
	modelReadinessMaturityValue  = "experimental"
)

type ModelReadinessService interface {
	RunProbes(ctx context.Context, cfg modelreadiness.ProbeConfig) (*modelreadiness.ReadinessRun, error)
	GetRun(ctx context.Context, tenantID, runID string) (*modelreadiness.ReadinessRun, bool, error)
}

type runModelReadinessRequest struct {
	Provider              string   `json:"provider"`
	Capabilities          []string `json:"capabilities"`
	ExpectedEmbeddingDims int      `json:"expected_embedding_dimensions"`
	TimeoutSeconds        int      `json:"timeout_seconds"`
}

func (s *Server) runModelReadiness(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}

	var req runModelReadinessRequest
	if !bindJSON(c, &req) {
		return
	}

	if strings.TrimSpace(req.Provider) == "" {
		writeError(c, consts.StatusBadRequest, "invalid_request", "provider is required")
		return
	}
	if len(req.Capabilities) == 0 {
		writeError(c, consts.StatusBadRequest, "invalid_request", "at least one capability is required")
		return
	}

	cfg := modelreadiness.ProbeConfig{
		TenantID:           principal.TenantID,
		ActorType:          string(principal.Kind),
		ActorID:            principal.SubjectID,
		TraceID:            requestTraceID(c),
		Provider:           strings.TrimSpace(req.Provider),
		ExpectedDimensions: req.ExpectedEmbeddingDims,
		SkipChat:           true,
		SkipEmbedding:      true,
		SkipRerank:         true,
		SkipMultimodal:     true,
		SkipProviderAuth:   true,
	}

	for _, cap := range req.Capabilities {
		switch strings.ToLower(strings.TrimSpace(cap)) {
		case "chat":
			cfg.SkipChat = false
		case "embedding":
			cfg.SkipEmbedding = false
		case "rerank":
			cfg.SkipRerank = false
		case "multimodal":
			cfg.SkipMultimodal = false
		case "provider_auth":
			cfg.SkipProviderAuth = false
		}
	}

	if req.TimeoutSeconds > 0 {
		cfg.Timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}

	run, err := s.modelReadinessService().RunProbes(ctx, cfg)
	if err != nil {
		writeModelReadinessError(c, err)
		return
	}

	setModelReadinessMaturityHeader(c)
	c.JSON(consts.StatusOK, readinessRunResponse(run))
}

func (s *Server) getModelReadinessRun(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}

	runID := c.Param("run_id")
	run, found, err := s.modelReadinessService().GetRun(ctx, principal.TenantID, runID)
	if err != nil {
		writeModelReadinessError(c, err)
		return
	}
	if !found {
		writeModelReadinessNotFound(c)
		return
	}

	setModelReadinessMaturityHeader(c)
	c.JSON(consts.StatusOK, readinessRunResponse(run))
}

func readinessRunResponse(run *modelreadiness.ReadinessRun) map[string]any {
	results := make([]map[string]any, 0, len(run.Results))
	for _, r := range run.Results {
		result := map[string]any{
			"capability":     r.Capability,
			"status":         string(r.Status),
			"latency_ms":     r.LatencyMs,
			"model":          r.Model,
			"error_category": string(r.ErrorCategory),
			"error_message":  r.ErrorMessage,
			"warnings":       r.Warnings,
		}
		if r.Dimensions > 0 {
			result["dimensions"] = r.Dimensions
		}
		results = append(results, result)
	}

	return map[string]any{
		"run_id":         run.ID,
		"provider":       run.Provider,
		"overall_status": string(run.OverallStatus),
		"started_at":     run.StartedAt,
		"completed_at":   run.CompletedAt,
		"results":        results,
	}
}

func setModelReadinessMaturityHeader(c *app.RequestContext) {
	c.Header(modelReadinessMaturityHeader, modelReadinessMaturityValue)
}

func writeModelReadinessNotFound(c *app.RequestContext) {
	writeError(c, consts.StatusNotFound, "model_readiness_run_not_found", "model readiness run not found")
}

func writeModelReadinessError(c *app.RequestContext, err error) {
	writeError(c, consts.StatusInternalServerError, "model_readiness_operation_failed", err.Error())
}

var modelReadinessServiceInstance ModelReadinessService = &noopModelReadinessService{}

func SetModelReadinessService(svc ModelReadinessService) {
	modelReadinessServiceInstance = svc
}

func (s *Server) modelReadinessService() ModelReadinessService {
	if s.Readiness != nil {
		return s.Readiness
	}
	return modelReadinessServiceInstance
}

type noopModelReadinessService struct{}

func (n *noopModelReadinessService) RunProbes(_ context.Context, _ modelreadiness.ProbeConfig) (*modelreadiness.ReadinessRun, error) {
	return nil, errModelReadinessServiceUnavailable
}

func (n *noopModelReadinessService) GetRun(_ context.Context, _, _ string) (*modelreadiness.ReadinessRun, bool, error) {
	return nil, false, errModelReadinessServiceUnavailable
}

var errModelReadinessServiceUnavailable = &modelReadinessServiceUnavailableError{}

type modelReadinessServiceUnavailableError struct{}

func (e *modelReadinessServiceUnavailableError) Error() string {
	return "model readiness service is not configured"
}
