package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/observability"
	"github.com/shikanon/orag/internal/optimizer"
	"github.com/shikanon/orag/internal/platform/apperrors"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/rag"
)

type Server struct {
	App *core.App
}

const maxQueryTopK = 100

type queryRequest struct {
	KnowledgeBaseID string      `json:"knowledge_base_id"`
	Query           string      `json:"query"`
	Profile         rag.Profile `json:"profile,omitempty"`
	SessionID       string      `json:"session_id,omitempty"`
	TopK            *int        `json:"top_k,omitempty"`
}

func NewServer(app *core.App) *Server {
	return &Server{App: app}
}

func (s *Server) Hertz() *server.Hertz {
	h := server.Default(server.WithHostPorts(s.App.Config.Server.Addr()))
	h.Use(s.traceMiddleware)
	h.Use(s.metricsMiddleware)
	h.GET("/healthz", s.health)
	h.GET("/readyz", s.ready)
	h.GET("/metrics", s.metrics)
	h.GET("/docs", s.docs)
	h.POST("/v1/auth/login", s.login)

	v1 := h.Group("/v1", s.authMiddleware)
	v1.POST("/knowledge-bases", s.createKnowledgeBase)
	v1.GET("/knowledge-bases", s.listKnowledgeBases)
	v1.GET("/knowledge-bases/:id", s.getKnowledgeBase)
	v1.DELETE("/knowledge-bases/:id", s.deleteKnowledgeBase)
	v1.POST("/knowledge-bases/:id/documents", s.uploadDocument)
	v1.POST("/knowledge-bases/:id/documents:import", s.importDocument)
	v1.GET("/ingestion-jobs/:id", s.getIngestionJob)
	v1.POST("/query", s.query)
	v1.POST("/query:stream", s.queryStream)
	v1.POST("/datasets", s.createDataset)
	v1.POST("/datasets/:id/items", s.addDatasetItem)
	v1.POST("/evaluations", s.runEvaluation)
	v1.GET("/evaluations/:id", s.getEvaluation)
	v1.POST("/optimizations", s.optimize)
	v1.GET("/optimizations/:id", s.getOptimization)
	v1.POST("/optimizations/*action", s.optimizationAction)
	return h
}

func (s *Server) health(_ context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(ctx context.Context, c *app.RequestContext) {
	checks, ok := s.App.Readiness(ctx)
	status := consts.StatusOK
	state := "ready"
	if !ok {
		status = consts.StatusServiceUnavailable
		state = "not_ready"
	}
	c.JSON(status, map[string]any{"status": state, "checks": checks})
}

func (s *Server) metrics(_ context.Context, c *app.RequestContext) {
	c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	c.String(consts.StatusOK, s.App.Metrics.Render())
}

func (s *Server) docs(_ context.Context, c *app.RequestContext) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(consts.StatusOK, `<html><body><h1>ORAG API</h1><p>OpenAPI source: <code>api/openapi.yaml</code></p></body></html>`)
}

func (s *Server) login(_ context.Context, c *app.RequestContext) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Username) == "" || req.Password == "" {
		writeError(c, consts.StatusBadRequest, "invalid_credentials", "username and password are required")
		return
	}
	if req.Username != s.App.Config.Auth.AdminDefaultUsername || req.Password != s.App.Config.Auth.AdminDefaultPassword {
		writeError(c, consts.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
	token, err := s.App.Auth.IssueToken("tenant_default", "user_admin")
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "token_issue_failed", err.Error())
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"access_token": token, "token_type": "Bearer", "expires_in": int(s.App.Config.Auth.TokenTTL.Seconds())})
}

func (s *Server) createKnowledgeBase(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Metadata    map[string]string `json:"metadata"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(c, consts.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	now := time.Now().UTC()
	item := kb.KnowledgeBase{
		ID:          id.New("kb"),
		TenantID:    tenantID(c),
		Name:        req.Name,
		Description: req.Description,
		Metadata:    req.Metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.App.KBStore.PutKnowledgeBase(ctx, item); err != nil {
		writeError(c, consts.StatusInternalServerError, "knowledge_base_create_failed", err.Error())
		return
	}
	c.JSON(consts.StatusCreated, item)
}

func (s *Server) listKnowledgeBases(_ context.Context, c *app.RequestContext) {
	items, err := s.App.KBStore.ListKnowledgeBases(tenantID(c))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "knowledge_base_list_failed", err.Error())
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"items": items})
}

func (s *Server) getKnowledgeBase(_ context.Context, c *app.RequestContext) {
	item, ok, err := s.App.KBStore.GetKnowledgeBase(tenantID(c), c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "knowledge_base_lookup_failed", err.Error())
		return
	}
	if !ok {
		writeKnowledgeBaseNotFound(c)
		return
	}
	c.JSON(consts.StatusOK, item)
}

func (s *Server) deleteKnowledgeBase(ctx context.Context, c *app.RequestContext) {
	deleted, err := s.App.KBStore.DeleteKnowledgeBase(ctx, tenantID(c), c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "knowledge_base_delete_failed", err.Error())
		return
	}
	if !deleted {
		writeKnowledgeBaseNotFound(c)
		return
	}
	c.Status(consts.StatusNoContent)
}

func (s *Server) uploadDocument(ctx context.Context, c *app.RequestContext) {
	kbID := c.Param("id")
	if !s.requireKnowledgeBase(c, kbID) {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid_request", "multipart field file is required")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid_upload", err.Error())
		return
	}
	defer file.Close()
	maxBytes := s.App.Config.Ingestion.MaxDocumentBytes
	if maxBytes > 0 && fileHeader.Size > maxBytes {
		writeError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "document exceeds max size")
		return
	}
	body, err := readLimited(file, maxBytes)
	if err != nil {
		writeError(c, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error())
		return
	}
	result, err := s.App.Ingest.Ingest(ctx, ingest.Request{
		TenantID:        tenantID(c),
		KnowledgeBaseID: kbID,
		SourceURI:       "upload://" + fileHeader.Filename,
		Name:            fileHeader.Filename,
		Content:         body,
	})
	if err != nil {
		writeIngestError(c, err)
		return
	}
	c.JSON(consts.StatusAccepted, map[string]any{"document": result.Document, "chunks": len(result.Chunks), "job": result.Job})
}

func (s *Server) importDocument(ctx context.Context, c *app.RequestContext) {
	kbID := c.Param("id")
	if !s.requireKnowledgeBase(c, kbID) {
		return
	}
	var req struct {
		SourceURI string `json:"source_uri"`
		Name      string `json:"name"`
		Content   string `json:"content"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeError(c, consts.StatusBadRequest, "invalid_request", "content is required")
		return
	}
	if req.Name == "" {
		req.Name = "imported.md"
	}
	if maxBytes := s.App.Config.Ingestion.MaxDocumentBytes; maxBytes > 0 && int64(len(req.Content)) > maxBytes {
		writeError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "document exceeds max size")
		return
	}
	result, err := s.App.Ingest.Ingest(ctx, ingest.Request{
		TenantID:        tenantID(c),
		KnowledgeBaseID: kbID,
		SourceURI:       req.SourceURI,
		Name:            req.Name,
		Content:         []byte(req.Content),
	})
	if err != nil {
		writeIngestError(c, err)
		return
	}
	c.JSON(consts.StatusAccepted, map[string]any{"document": result.Document, "chunks": len(result.Chunks), "job": result.Job})
}

func (s *Server) requireKnowledgeBase(c *app.RequestContext, kbID string) bool {
	if _, ok, err := s.App.KBStore.GetKnowledgeBase(tenantID(c), kbID); err != nil {
		writeError(c, consts.StatusInternalServerError, "knowledge_base_lookup_failed", err.Error())
		return false
	} else if ok {
		return true
	}
	writeKnowledgeBaseNotFound(c)
	return false
}

func writeKnowledgeBaseNotFound(c *app.RequestContext) {
	writeError(c, consts.StatusNotFound, "knowledge_base_not_found", "knowledge base not found")
}

func writeIngestError(c *app.RequestContext, err error) {
	if errors.Is(err, ingest.ErrKnowledgeBaseNotFound) {
		writeKnowledgeBaseNotFound(c)
		return
	}
	writeError(c, consts.StatusInternalServerError, "ingest_failed", err.Error())
}

func (s *Server) getIngestionJob(ctx context.Context, c *app.RequestContext) {
	if s.App.Ingest.Jobs == nil {
		writeError(c, consts.StatusNotFound, "ingestion_job_store_missing", "ingestion job store is not configured")
		return
	}
	job, ok, err := s.App.Ingest.Jobs.GetJob(ctx, tenantID(c), c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "ingestion_job_lookup_failed", err.Error())
		return
	}
	if !ok {
		writeError(c, consts.StatusNotFound, "ingestion_job_not_found", "ingestion job not found")
		return
	}
	c.JSON(consts.StatusOK, job)
}

func (s *Server) query(ctx context.Context, c *app.RequestContext) {
	var req queryRequest
	if !bindJSON(c, &req) {
		return
	}
	if !validateQueryRequest(c, req) {
		return
	}
	ragReq := req.ragRequest()
	if !s.requireKnowledgeBase(c, ragReq.KnowledgeBaseID) {
		return
	}
	start := time.Now()
	traceID := requestTraceID(c)
	ctx = observability.WithTraceID(ctx, traceID)
	ragReq.TenantID = tenantID(c)
	ragReq.TraceID = traceID
	resp, err := s.App.RAG.Query(ctx, ragReq)
	if err != nil {
		s.observeRAGError(ragReq.Profile, "query_failed", time.Since(start).Milliseconds())
		writeError(c, consts.StatusInternalServerError, "query_failed", err.Error())
		return
	}
	s.observeRAGSuccess(resp.Profile, resp.CacheStatus, resp.LatencyMS)
	c.JSON(consts.StatusOK, resp)
}

func (s *Server) queryStream(ctx context.Context, c *app.RequestContext) {
	var req queryRequest
	if !bindJSON(c, &req) {
		return
	}
	if !validateQueryRequest(c, req) {
		return
	}
	ragReq := req.ragRequest()
	if !s.requireKnowledgeBase(c, ragReq.KnowledgeBaseID) {
		return
	}
	start := time.Now()
	traceID := requestTraceID(c)
	ctx = observability.WithTraceID(ctx, traceID)
	ragReq.TenantID = tenantID(c)
	ragReq.TraceID = traceID
	resp, err := s.App.RAG.Query(ctx, ragReq)
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	if err != nil {
		c.Set("error_code", "query_failed")
		s.observeRAGError(ragReq.Profile, "query_failed", time.Since(start).Milliseconds())
		c.Response.SetStatusCode(consts.StatusInternalServerError)
		c.Response.Header.SetContentType("text/event-stream; charset=utf-8")
		c.Response.SetBodyString(errorSSE("query_failed", err.Error(), traceID))
		return
	}
	s.observeRAGSuccess(resp.Profile, resp.CacheStatus, resp.LatencyMS)
	c.Response.SetStatusCode(consts.StatusOK)
	c.Response.Header.SetContentType("text/event-stream; charset=utf-8")
	c.Response.SetBodyString(querySSE(resp))
}

func (req queryRequest) ragRequest() rag.QueryRequest {
	topK := 0
	if req.TopK != nil {
		topK = *req.TopK
	}
	return rag.QueryRequest{
		KnowledgeBaseID: req.KnowledgeBaseID,
		Query:           req.Query,
		Profile:         req.Profile,
		SessionID:       req.SessionID,
		TopK:            topK,
	}
}

func validateQueryRequest(c *app.RequestContext, req queryRequest) bool {
	if strings.TrimSpace(req.KnowledgeBaseID) == "" {
		writeError(c, consts.StatusBadRequest, "invalid_request", "knowledge_base_id is required")
		return false
	}
	if strings.TrimSpace(req.Query) == "" {
		writeError(c, consts.StatusBadRequest, "invalid_request", "query is required")
		return false
	}
	if req.Profile != "" && req.Profile != rag.ProfileRealtime && req.Profile != rag.ProfileHighPrecision {
		writeError(c, consts.StatusBadRequest, "invalid_request", "profile must be realtime or high_precision")
		return false
	}
	if req.TopK != nil && *req.TopK <= 0 {
		writeError(c, consts.StatusBadRequest, "invalid_request", "top_k must be positive")
		return false
	}
	if req.TopK != nil && *req.TopK > maxQueryTopK {
		writeError(c, consts.StatusBadRequest, "invalid_request", "top_k must be at most 100")
		return false
	}
	return true
}

func (s *Server) observeRAGSuccess(profile rag.Profile, cacheStatus string, latencyMS int64) {
	if s.App == nil || s.App.Metrics == nil {
		return
	}
	s.App.Metrics.ObserveRAGQuery(string(profile), cacheStatus, "success", latencyMS)
}

func (s *Server) observeRAGError(profile rag.Profile, errorCode string, latencyMS int64) {
	if s.App == nil || s.App.Metrics == nil {
		return
	}
	s.App.Metrics.ObserveRAGQuery(string(profile), "unknown", "error", latencyMS)
	s.App.Metrics.IncRAGError(string(profile), errorCode)
}

func (s *Server) createDataset(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Kind == "" {
		req.Kind = "golden"
	}
	ds, err := s.App.Datasets.Create(ctx, tenantID(c), req.Name, req.Kind)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "dataset_create_failed", err.Error())
		return
	}
	c.JSON(consts.StatusCreated, ds)
}

func (s *Server) addDatasetItem(ctx context.Context, c *app.RequestContext) {
	var item dataset.Item
	if !bindJSON(c, &item) {
		return
	}
	created, err := s.App.Datasets.AddItem(ctx, tenantID(c), c.Param("id"), item)
	if err != nil {
		if errors.Is(err, dataset.ErrDatasetNotFound) {
			writeDatasetNotFound(c)
			return
		}
		writeError(c, consts.StatusInternalServerError, "dataset_item_create_failed", err.Error())
		return
	}
	c.JSON(consts.StatusCreated, created)
}

func (s *Server) runEvaluation(ctx context.Context, c *app.RequestContext) {
	var req eval.RunRequest
	if !bindJSON(c, &req) {
		return
	}
	req.TenantID = tenantID(c)
	resp, err := s.App.Eval.Run(ctx, req)
	if err != nil {
		if errors.Is(err, dataset.ErrDatasetNotFound) {
			writeDatasetNotFound(c)
			return
		}
		writeError(c, consts.StatusInternalServerError, "evaluation_failed", err.Error())
		return
	}
	c.JSON(consts.StatusAccepted, resp)
}

func (s *Server) getEvaluation(ctx context.Context, c *app.RequestContext) {
	options := eval.EvaluationDetailOptions{
		IncludeItems:    queryBool(c, "include_items"),
		IncludeJudge:    queryBool(c, "include_judge"),
		IncludePairwise: queryBool(c, "include_pairwise"),
	}
	if options.IncludeItems || options.IncludeJudge || options.IncludePairwise {
		detail, ok, err := s.App.Eval.GetDetail(ctx, tenantID(c), c.Param("id"), options)
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "evaluation_lookup_failed", err.Error())
			return
		}
		if !ok {
			writeError(c, consts.StatusNotFound, "evaluation_not_found", "evaluation not found")
			return
		}
		c.JSON(consts.StatusOK, detail)
		return
	}
	result, ok, err := s.App.Eval.Get(ctx, tenantID(c), c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "evaluation_lookup_failed", err.Error())
		return
	}
	if !ok {
		writeError(c, consts.StatusNotFound, "evaluation_not_found", "evaluation not found")
		return
	}
	c.JSON(consts.StatusOK, result)
}

func queryBool(c *app.RequestContext, key string) bool {
	value := strings.ToLower(strings.TrimSpace(string(c.QueryArgs().Peek(key))))
	return value == "true" || value == "1" || value == "yes"
}

type optimizeRequest struct {
	DatasetID           string                  `json:"dataset_id"`
	KnowledgeBaseID     string                  `json:"knowledge_base_id"`
	Objective           optimizer.ObjectiveSpec `json:"objective,omitempty"`
	SearchSpace         optimizer.SearchSpace   `json:"search_space,omitempty"`
	Search              optimizer.SearchSpec    `json:"search,omitempty"`
	Budget              optimizer.Budget        `json:"budget,omitempty"`
	Profile             rag.Profile             `json:"profile,omitempty"`
	TopK                int                     `json:"top_k,omitempty"`
	NamespaceTTLSeconds int                     `json:"namespace_ttl_seconds,omitempty"`
	SelectionSplit      string                  `json:"selection_split,omitempty"`
	HoldoutSplit        string                  `json:"holdout_split,omitempty"`
	Runner              map[string]any          `json:"runner,omitempty"`
	Profiles            []rag.Profile           `json:"profiles,omitempty"`
	TopKs               []int                   `json:"top_ks,omitempty"`
}

type optimizationAcceptedResponse struct {
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	PollURL   string `json:"poll_url"`
	CancelURL string `json:"cancel_url"`
	ResumeURL string `json:"resume_url"`
}

type optimizationStatusResponse struct {
	Run        optimizationRunResponse         `json:"run"`
	Candidates []optimizationCandidateResponse `json:"candidates"`
}

type optimizationRunResponse struct {
	ID                      string                   `json:"id"`
	DatasetID               string                   `json:"dataset_id"`
	KnowledgeBaseID         string                   `json:"knowledge_base_id"`
	Objective               optimizer.ObjectiveSpec  `json:"objective"`
	SearchSpace             optimizer.SearchSpace    `json:"search_space"`
	Runner                  map[string]any           `json:"runner,omitempty"`
	Status                  optimizer.RunStatus      `json:"status"`
	StatusReason            string                   `json:"status_reason,omitempty"`
	BestCandidateID         string                   `json:"best_candidate_id,omitempty"`
	HoldoutCandidateID      string                   `json:"holdout_candidate_id,omitempty"`
	SamplingStrategy        optimizer.SearchStrategy `json:"sampling_strategy"`
	SearchSpaceSize         int64                    `json:"search_space_size"`
	SampledCandidateCount   int                      `json:"sampled_candidate_count"`
	CompletedCandidateCount int                      `json:"completed_candidate_count"`
	Checkpoint              optimizer.Checkpoint     `json:"checkpoint"`
	CostUSD                 float64                  `json:"cost_usd,omitempty"`
	CostBudgetUSD           *float64                 `json:"cost_budget_usd,omitempty"`
	CreatedAt               time.Time                `json:"created_at"`
	UpdatedAt               time.Time                `json:"updated_at"`
}

type optimizationCandidateResponse struct {
	ID                string                    `json:"id"`
	OptimizationRunID string                    `json:"optimization_run_id"`
	Config            optimizer.CandidateConfig `json:"config"`
	Status            optimizer.CandidateStatus `json:"status"`
	EvaluationRunID   string                    `json:"evaluation_run_id,omitempty"`
	JudgeRunID        string                    `json:"judge_run_id,omitempty"`
	ObjectiveScore    float64                   `json:"objective_score,omitempty"`
	HoldoutScore      *float64                  `json:"holdout_score,omitempty"`
	Confidence        map[string]float64        `json:"confidence,omitempty"`
	Metrics           map[string]float64        `json:"metrics,omitempty"`
	TokenUsage        optimizer.TokenUsage      `json:"token_usage,omitempty"`
	CostUSD           float64                   `json:"cost_usd,omitempty"`
	Artifacts         map[string]any            `json:"artifacts,omitempty"`
	TempNamespaces    []optimizer.TempNamespace `json:"temp_namespaces,omitempty"`
	CleanupStatus     optimizer.CleanupStatus   `json:"cleanup_status,omitempty"`
	ExpiresAt         *time.Time                `json:"expires_at,omitempty"`
	Error             string                    `json:"error,omitempty"`
	CreatedAt         time.Time                 `json:"created_at"`
	UpdatedAt         time.Time                 `json:"updated_at"`
}

func (s *Server) optimize(ctx context.Context, c *app.RequestContext) {
	var req optimizeRequest
	if !bindJSON(c, &req) {
		return
	}
	submitReq, ok := s.optimizationSubmitRequest(ctx, c, req)
	if !ok {
		return
	}
	run, err := s.App.Optimizer.Submit(ctx, submitReq)
	if err != nil {
		writeOptimizationError(c, err)
		return
	}
	c.JSON(consts.StatusAccepted, optimizationAccepted(run.ID, run.Status))
}

func (s *Server) getOptimization(ctx context.Context, c *app.RequestContext) {
	status, ok, err := s.App.Optimizer.Get(ctx, tenantID(c), c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "optimization_lookup_failed", err.Error())
		return
	}
	if !ok {
		writeOptimizationNotFound(c)
		return
	}
	c.JSON(consts.StatusOK, optimizationStatus(status))
}

func (s *Server) optimizationAction(ctx context.Context, c *app.RequestContext) {
	switch action := optimizationActionName(c); action {
	case "cancel":
		s.cancelOptimization(ctx, c)
	case "resume":
		s.resumeOptimization(ctx, c)
	default:
		writeError(c, consts.StatusNotFound, "optimization_action_not_found", "optimization action not found")
	}
}

func (s *Server) cancelOptimization(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Reason string `json:"reason"`
	}
	if len(c.Request.Body()) > 0 && !bindJSON(c, &req) {
		return
	}
	runID := optimizationActionID(c, "cancel")
	run, err := s.App.Optimizer.Cancel(ctx, tenantID(c), runID, req.Reason)
	if err != nil {
		writeOptimizationError(c, err)
		return
	}
	c.JSON(consts.StatusAccepted, optimizationAccepted(run.ID, run.Status))
}

func (s *Server) resumeOptimization(ctx context.Context, c *app.RequestContext) {
	runID := optimizationActionID(c, "resume")
	status, ok, err := s.App.Optimizer.Get(ctx, tenantID(c), runID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "optimization_lookup_failed", err.Error())
		return
	}
	if !ok {
		writeOptimizationNotFound(c)
		return
	}
	req := optimizeRequest{
		DatasetID:       status.Run.DatasetID,
		KnowledgeBaseID: status.Run.KnowledgeBaseID,
		Objective:       status.Run.Objective,
		SearchSpace:     status.Run.SearchSpace,
		Runner:          status.Run.Runner,
	}
	if len(c.Request.Body()) > 0 && !bindJSON(c, &req) {
		return
	}
	submitReq, ok := s.optimizationSubmitRequest(ctx, c, req)
	if !ok {
		return
	}
	run, err := s.App.Optimizer.Resume(ctx, tenantID(c), runID, submitReq)
	if err != nil {
		writeOptimizationError(c, err)
		return
	}
	c.JSON(consts.StatusAccepted, optimizationAccepted(run.ID, run.Status))
}

func (s *Server) optimizationSubmitRequest(ctx context.Context, c *app.RequestContext, req optimizeRequest) (optimizer.SubmitRequest, bool) {
	if strings.TrimSpace(req.DatasetID) == "" {
		writeError(c, consts.StatusBadRequest, "invalid_request", "dataset_id is required")
		return optimizer.SubmitRequest{}, false
	}
	if strings.TrimSpace(req.KnowledgeBaseID) == "" {
		writeError(c, consts.StatusBadRequest, "invalid_request", "knowledge_base_id is required")
		return optimizer.SubmitRequest{}, false
	}
	if _, ok, err := s.App.Datasets.Get(ctx, tenantID(c), req.DatasetID); err != nil {
		writeDatasetError(c, "dataset_lookup_failed", err)
		return optimizer.SubmitRequest{}, false
	} else if !ok {
		writeDatasetNotFound(c)
		return optimizer.SubmitRequest{}, false
	}
	if !s.requireKnowledgeBase(c, req.KnowledgeBaseID) {
		return optimizer.SubmitRequest{}, false
	}
	req.applyLegacyShortcutDefaults()
	if req.Objective.Maximize == "" {
		req.Objective.Maximize = eval.PrimaryMetricPairwiseAccuracy
	}
	if req.Profile == "" && len(req.Profiles) > 0 {
		req.Profile = req.Profiles[0]
	}
	if req.Profile == "" {
		req.Profile = rag.ProfileRealtime
	}
	if req.TopK <= 0 && len(req.TopKs) > 0 {
		req.TopK = req.TopKs[0]
	}
	runner := req.Runner
	if runner == nil {
		runner = map[string]any{}
	}
	runner["type"] = "internal_rag"
	runner["profile"] = string(req.Profile)
	if req.TopK > 0 {
		runner["top_k"] = req.TopK
	}
	return optimizer.SubmitRequest{
		TenantID:        tenantID(c),
		DatasetID:       req.DatasetID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		Objective:       req.Objective,
		SearchSpace:     req.SearchSpace,
		Search:          req.Search,
		Budget:          req.Budget,
		Profile:         req.Profile,
		TopK:            req.TopK,
		NamespaceTTL:    time.Duration(req.NamespaceTTLSeconds) * time.Second,
		SelectionSplit:  req.SelectionSplit,
		HoldoutSplit:    req.HoldoutSplit,
		Runner:          runner,
	}, true
}

func (req *optimizeRequest) applyLegacyShortcutDefaults() {
	if len(req.SearchSpace.Retrieval.DenseTopK) == 0 && len(req.TopKs) > 0 {
		req.SearchSpace.Retrieval.DenseTopK = append([]int(nil), req.TopKs...)
	}
	if len(req.SearchSpace.Retrieval.DenseTopK) == 0 && req.TopK > 0 {
		req.SearchSpace.Retrieval.DenseTopK = []int{req.TopK}
	}
	if len(req.SearchSpace.Retrieval.DenseTopK) == 0 {
		req.SearchSpace.Retrieval.DenseTopK = []int{8}
	}
}

func optimizationAccepted(runID string, status optimizer.RunStatus) optimizationAcceptedResponse {
	return optimizationAcceptedResponse{
		RunID:     runID,
		Status:    string(status),
		PollURL:   "/v1/optimizations/" + runID,
		CancelURL: "/v1/optimizations/" + runID + ":cancel",
		ResumeURL: "/v1/optimizations/" + runID + ":resume",
	}
}

func optimizationStatus(status optimizer.OptimizationStatus) optimizationStatusResponse {
	candidates := make([]optimizationCandidateResponse, 0, len(status.Candidates))
	for _, candidate := range status.Candidates {
		candidates = append(candidates, optimizationCandidateResponse{
			ID:                candidate.ID,
			OptimizationRunID: candidate.OptimizationRunID,
			Config:            candidate.Config,
			Status:            candidate.Status,
			EvaluationRunID:   candidate.EvaluationRunID,
			JudgeRunID:        candidate.JudgeRunID,
			ObjectiveScore:    candidate.ObjectiveScore,
			HoldoutScore:      candidate.HoldoutScore,
			Confidence:        candidate.Confidence,
			Metrics:           candidate.Metrics,
			TokenUsage:        candidate.TokenUsage,
			CostUSD:           candidate.CostUSD,
			Artifacts:         candidate.Artifacts,
			TempNamespaces:    candidate.TempNamespaces,
			CleanupStatus:     candidate.CleanupStatus,
			ExpiresAt:         candidate.ExpiresAt,
			Error:             candidate.Error,
			CreatedAt:         candidate.CreatedAt,
			UpdatedAt:         candidate.UpdatedAt,
		})
	}
	run := status.Run
	return optimizationStatusResponse{
		Run: optimizationRunResponse{
			ID:                      run.ID,
			DatasetID:               run.DatasetID,
			KnowledgeBaseID:         run.KnowledgeBaseID,
			Objective:               run.Objective,
			SearchSpace:             run.SearchSpace,
			Runner:                  run.Runner,
			Status:                  run.Status,
			StatusReason:            run.StatusReason,
			BestCandidateID:         run.BestCandidateID,
			HoldoutCandidateID:      run.HoldoutCandidateID,
			SamplingStrategy:        run.SamplingStrategy,
			SearchSpaceSize:         run.SearchSpaceSize,
			SampledCandidateCount:   run.SampledCandidateCount,
			CompletedCandidateCount: run.CompletedCandidateCount,
			Checkpoint:              run.Checkpoint,
			CostUSD:                 run.CostUSD,
			CostBudgetUSD:           run.CostBudgetUSD,
			CreatedAt:               run.CreatedAt,
			UpdatedAt:               run.UpdatedAt,
		},
		Candidates: candidates,
	}
}

func optimizationActionID(c *app.RequestContext, action string) string {
	id := c.Param("id")
	if id != "" {
		return strings.TrimSuffix(id, ":"+action)
	}
	path := strings.TrimPrefix(string(c.Path()), "/v1/optimizations/")
	return strings.TrimSuffix(path, ":"+action)
}

func optimizationActionName(c *app.RequestContext) string {
	action := c.Param("action")
	if action == "" {
		action = strings.TrimPrefix(string(c.Path()), "/v1/optimizations/")
	}
	action = strings.TrimPrefix(action, "/")
	if strings.HasSuffix(action, ":cancel") {
		return "cancel"
	}
	if strings.HasSuffix(action, ":resume") {
		return "resume"
	}
	return ""
}

func writeOptimizationError(c *app.RequestContext, err error) {
	if errors.Is(err, dataset.ErrDatasetNotFound) {
		writeDatasetNotFound(c)
		return
	}
	if errors.Is(err, optimizer.ErrOptimizationNotFound) {
		writeOptimizationNotFound(c)
		return
	}
	switch {
	case apperrors.IsCode(err, apperrors.CodeValidation):
		writeError(c, consts.StatusBadRequest, "invalid_request", err.Error())
	case apperrors.IsCode(err, apperrors.CodeNotFound):
		writeError(c, consts.StatusNotFound, "not_found", err.Error())
	default:
		writeError(c, consts.StatusInternalServerError, "optimization_failed", err.Error())
	}
}

func writeOptimizationNotFound(c *app.RequestContext) {
	writeError(c, consts.StatusNotFound, "optimization_not_found", "optimization run not found")
}

func writeDatasetError(c *app.RequestContext, fallbackCode string, err error) {
	if apperrors.IsCode(err, apperrors.CodeNotFound) {
		writeError(c, consts.StatusNotFound, "dataset_not_found", "dataset not found")
		return
	}
	writeError(c, consts.StatusInternalServerError, fallbackCode, err.Error())
}

func bindJSON(c *app.RequestContext, dst any) bool {
	if err := json.Unmarshal(c.Request.Body(), dst); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", "invalid json body")
		return false
	}
	return true
}

func writeDatasetNotFound(c *app.RequestContext) {
	writeError(c, consts.StatusNotFound, "dataset_not_found", "dataset not found")
}

func readLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return io.ReadAll(r)
	}
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, errors.New("document exceeds max size")
	}
	return body, nil
}
