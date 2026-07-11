package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
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
	"github.com/shikanon/orag/internal/offlineknowledge"
	"github.com/shikanon/orag/internal/optimizer"
	"github.com/shikanon/orag/internal/platform/apperrors"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/storage/postgres"
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
	v1.POST("/projects", s.createProject)
	v1.GET("/projects", s.listProjects)
	v1.GET("/projects/:project_id", s.getProject)
	v1.PATCH("/projects/:project_id", s.updateProject)
	v1.POST("/knowledge-bases", s.createKnowledgeBase)
	v1.GET("/knowledge-bases", s.listKnowledgeBases)
	v1.GET("/knowledge-bases/:id", s.getKnowledgeBase)
	v1.DELETE("/knowledge-bases/:id", s.deleteKnowledgeBase)
	v1.POST("/knowledge-bases/:id/documents", s.uploadDocument)
	v1.POST("/knowledge-bases/:id/documents:import", s.importDocument)
	v1.POST("/knowledge-bases/:id/uploads", s.createUploadSession)
	v1.GET("/uploads/:id", s.getUploadSession)
	v1.PUT("/uploads/:id", s.appendUploadChunk)
	v1.POST("/uploads/*action", s.uploadAction)
	v1.DELETE("/uploads/:id", s.cancelUploadSession)
	v1.GET("/ingestion-jobs/:id", s.getIngestionJob)
	v1.POST("/query", s.query)
	v1.POST("/query:stream", s.queryStream)
	v1.GET("/traces", s.listTraces)
	v1.GET("/traces:stats", s.traceStats)
	v1.GET("/traces/:trace_id", s.getTrace)
	v1.POST("/datasets", s.createDataset)
	v1.POST("/datasets/:id/items", s.addDatasetItem)
	v1.POST("/evaluations", s.runEvaluation)
	v1.GET("/evaluations/:id", s.getEvaluation)
	v1.POST("/optimizations", s.optimize)
	v1.GET("/optimizations/:id", s.getOptimization)
	v1.POST("/optimizations/*action", s.optimizationAction)
	v1.POST("/offline-knowledge/runs", s.createOfflineKnowledgeRun)
	v1.POST("/offline-knowledge/scheduler:trigger", s.triggerOfflineKnowledgeScheduler)
	v1.GET("/offline-knowledge/runs", s.listOfflineKnowledgeRuns)
	v1.GET("/offline-knowledge/runs/:id", s.getOfflineKnowledgeRun)
	v1.POST("/offline-knowledge/runs/:id/:action", s.offlineKnowledgeRunAction)
	v1.GET("/offline-knowledge/runs/:id/questions", s.listOfflineKnowledgeQuestions)
	v1.GET("/optimization-items", s.listOptimizationItems)
	v1.POST("/optimization-items/revalidate", s.bulkRevalidateOptimizationItems)
	v1.GET("/optimization-items/:id", s.getOptimizationItem)
	v1.POST("/optimization-items/:id/:action", s.optimizationItemAction)
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

func (s *Server) listKnowledgeBases(ctx context.Context, c *app.RequestContext) {
	items, err := s.App.KBStore.ListKnowledgeBases(ctx, tenantID(c))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "knowledge_base_list_failed", err.Error())
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"items": items})
}

func (s *Server) getKnowledgeBase(ctx context.Context, c *app.RequestContext) {
	item, ok, err := s.App.KBStore.GetKnowledgeBase(ctx, tenantID(c), c.Param("id"))
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
	if !s.requireKnowledgeBase(ctx, c, kbID) {
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
	if !s.requireKnowledgeBase(ctx, c, kbID) {
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

type createUploadRequest struct {
	Name       string `json:"name"`
	SourceURI  string `json:"source_uri"`
	TotalBytes int64  `json:"total_bytes,omitempty"`
}

func (s *Server) createUploadSession(ctx context.Context, c *app.RequestContext) {
	kbID := c.Param("id")
	if !s.requireKnowledgeBase(ctx, c, kbID) {
		return
	}
	var req createUploadRequest
	if !bindJSON(c, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(c, consts.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if req.TotalBytes < 0 {
		writeError(c, consts.StatusBadRequest, "invalid_request", "total_bytes must be non-negative")
		return
	}
	if maxBytes := s.App.Config.Ingestion.MaxDocumentBytes; maxBytes > 0 && req.TotalBytes > maxBytes {
		writeError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "document exceeds max size")
		return
	}
	sourceURI := strings.TrimSpace(req.SourceURI)
	if sourceURI == "" {
		sourceURI = "upload://" + req.Name
	}
	session, err := s.uploadStore().CreateUpload(ctx, ingest.UploadSession{
		TenantID:        tenantID(c),
		KnowledgeBaseID: kbID,
		Name:            req.Name,
		SourceURI:       sourceURI,
		TotalBytes:      req.TotalBytes,
	})
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "upload_create_failed", err.Error())
		return
	}
	c.JSON(consts.StatusCreated, uploadSessionResponse(session))
}

func (s *Server) getUploadSession(ctx context.Context, c *app.RequestContext) {
	session, ok, err := s.uploadStore().GetUpload(ctx, tenantID(c), c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "upload_lookup_failed", err.Error())
		return
	}
	if !ok {
		writeUploadNotFound(c)
		return
	}
	c.JSON(consts.StatusOK, uploadSessionResponse(session))
}

func (s *Server) appendUploadChunk(ctx context.Context, c *app.RequestContext) {
	offsetHeader := strings.TrimSpace(string(c.GetHeader("Upload-Offset")))
	if offsetHeader == "" {
		writeError(c, consts.StatusBadRequest, "invalid_request", "Upload-Offset header is required")
		return
	}
	offset, err := strconv.ParseInt(offsetHeader, 10, 64)
	if err != nil || offset < 0 {
		writeError(c, consts.StatusBadRequest, "invalid_request", "Upload-Offset must be a non-negative integer")
		return
	}
	session, err := s.uploadStore().AppendUpload(ctx, tenantID(c), c.Param("id"), offset, c.Request.Body(), s.App.Config.Ingestion.MaxDocumentBytes)
	if err != nil {
		writeUploadError(c, err, session)
		return
	}
	c.JSON(consts.StatusOK, uploadSessionResponse(session))
}

func (s *Server) uploadAction(ctx context.Context, c *app.RequestContext) {
	action := strings.TrimPrefix(c.Param("action"), "/")
	if strings.HasSuffix(action, ":complete") {
		s.completeUploadSession(ctx, c, strings.TrimSuffix(action, ":complete"))
		return
	}
	writeError(c, consts.StatusNotFound, "upload_action_not_found", "upload action not found")
}

func (s *Server) completeUploadSession(ctx context.Context, c *app.RequestContext, uploadID string) {
	session, body, err := s.uploadStore().ReadUpload(ctx, tenantID(c), uploadID)
	if err != nil {
		writeUploadError(c, err, session)
		return
	}
	result, err := s.App.Ingest.Ingest(ctx, ingest.Request{
		TenantID:        tenantID(c),
		KnowledgeBaseID: session.KnowledgeBaseID,
		SourceURI:       session.SourceURI,
		Name:            session.Name,
		Content:         body,
	})
	if err != nil {
		writeIngestError(c, err)
		return
	}
	completed, err := s.uploadStore().CompleteUpload(ctx, tenantID(c), uploadID)
	if err != nil {
		writeUploadError(c, err, completed)
		return
	}
	c.JSON(consts.StatusAccepted, map[string]any{
		"upload":   uploadSessionResponse(completed),
		"document": result.Document,
		"chunks":   len(result.Chunks),
		"job":      result.Job,
	})
}

func (s *Server) cancelUploadSession(ctx context.Context, c *app.RequestContext) {
	if err := s.uploadStore().CancelUpload(ctx, tenantID(c), c.Param("id")); err != nil {
		writeUploadError(c, err, ingest.UploadSession{})
		return
	}
	c.Status(consts.StatusNoContent)
}

func (s *Server) uploadStore() ingest.UploadStore {
	if s.App.Ingest.Uploads == nil {
		s.App.Ingest.Uploads = ingest.NewMemoryUploadStore()
	}
	return s.App.Ingest.Uploads
}

func uploadSessionResponse(session ingest.UploadSession) map[string]any {
	return map[string]any{
		"id":                session.ID,
		"tenant_id":         session.TenantID,
		"knowledge_base_id": session.KnowledgeBaseID,
		"name":              session.Name,
		"source_uri":        session.SourceURI,
		"total_bytes":       session.TotalBytes,
		"received_bytes":    session.ReceivedBytes,
		"status":            session.Status,
		"created_at":        session.CreatedAt,
		"updated_at":        session.UpdatedAt,
		"upload_url":        "/v1/uploads/" + session.ID,
		"complete_url":      "/v1/uploads/" + session.ID + ":complete",
	}
}

func writeUploadError(c *app.RequestContext, err error, session ingest.UploadSession) {
	switch {
	case errors.Is(err, ingest.ErrUploadNotFound):
		writeUploadNotFound(c)
	case errors.Is(err, ingest.ErrUploadOffsetMismatch):
		writeErrorDetails(c, consts.StatusConflict, "upload_offset_mismatch", "Upload-Offset does not match received bytes", map[string]any{"received_bytes": session.ReceivedBytes})
	case errors.Is(err, ingest.ErrUploadAlreadyClosed):
		writeError(c, consts.StatusConflict, "upload_closed", "upload is already closed")
	case errors.Is(err, ingest.ErrUploadIncomplete):
		writeErrorDetails(c, consts.StatusConflict, "upload_incomplete", "upload has not received the declared total bytes", map[string]any{"received_bytes": session.ReceivedBytes, "total_bytes": session.TotalBytes})
	case errors.Is(err, ingest.ErrUploadTooLarge):
		writeError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "document exceeds max size")
	default:
		writeError(c, consts.StatusInternalServerError, "upload_failed", err.Error())
	}
}

func writeUploadNotFound(c *app.RequestContext) {
	writeError(c, consts.StatusNotFound, "upload_not_found", "upload session not found")
}

func (s *Server) requireKnowledgeBase(ctx context.Context, c *app.RequestContext, kbID string) bool {
	if _, ok, err := s.App.KBStore.GetKnowledgeBase(ctx, tenantID(c), kbID); err != nil {
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
	if !s.requireKnowledgeBase(ctx, c, ragReq.KnowledgeBaseID) {
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
	if !s.requireKnowledgeBase(ctx, c, ragReq.KnowledgeBaseID) {
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

func (s *Server) getTrace(ctx context.Context, c *app.RequestContext) {
	if s.App.Traces == nil {
		writeError(c, consts.StatusInternalServerError, "trace_repository_missing", "trace repository is not configured")
		return
	}
	traceID := strings.TrimSpace(c.Param("trace_id"))
	if traceID == "" {
		writeError(c, consts.StatusBadRequest, "invalid_request", "trace_id is required")
		return
	}
	trace, found, err := s.App.Traces.GetTraceForTenant(ctx, tenantID(c), traceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "trace_lookup_failed", err.Error())
		return
	}
	if !found {
		writeError(c, consts.StatusNotFound, "trace_not_found", "trace not found")
		return
	}
	normalizeTraceResponse(&trace)
	c.JSON(consts.StatusOK, trace)
}

func (s *Server) listTraces(ctx context.Context, c *app.RequestContext) {
	if s.App.Traces == nil {
		writeError(c, consts.StatusInternalServerError, "trace_repository_missing", "trace repository is not configured")
		return
	}
	filter, ok := parseTraceListFilter(c)
	if !ok {
		return
	}
	filter.TenantID = tenantID(c)
	traces, err := s.App.Traces.ListTraces(ctx, filter)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "trace_list_failed", err.Error())
		return
	}
	for i := range traces {
		normalizeTraceResponse(&traces[i])
	}
	c.JSON(consts.StatusOK, map[string]any{"items": traces})
}

func (s *Server) traceStats(ctx context.Context, c *app.RequestContext) {
	if s.App.Traces == nil {
		writeError(c, consts.StatusInternalServerError, "trace_repository_missing", "trace repository is not configured")
		return
	}
	filter, ok := parseTraceListFilter(c)
	if !ok {
		return
	}
	filter.TenantID = tenantID(c)
	stats, err := s.App.Traces.TraceNodeStats(ctx, filter)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "trace_stats_failed", err.Error())
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"tenant_id": filter.TenantID, "items": stats})
}

func normalizeTraceResponse(trace *postgres.TraceRecord) {
	if trace.NodeSpans == nil {
		trace.NodeSpans = []postgres.TraceNodeSpan{}
	}
}

func parseTraceListFilter(c *app.RequestContext) (postgres.TraceListFilter, bool) {
	var filter postgres.TraceListFilter
	if profile := strings.TrimSpace(c.Query("profile")); profile != "" {
		switch rag.Profile(profile) {
		case rag.ProfileRealtime, rag.ProfileHighPrecision:
			filter.Profile = rag.Profile(profile)
		default:
			writeError(c, consts.StatusBadRequest, "invalid_request", "profile must be realtime or high_precision")
			return postgres.TraceListFilter{}, false
		}
	}
	for _, field := range []struct {
		name string
		dst  *time.Time
	}{
		{name: "since", dst: &filter.Since},
		{name: "until", dst: &filter.Until},
	} {
		value := strings.TrimSpace(c.Query(field.name))
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			writeError(c, consts.StatusBadRequest, "invalid_request", field.name+" must be RFC3339")
			return postgres.TraceListFilter{}, false
		}
		*field.dst = parsed
	}
	if value := strings.TrimSpace(c.Query("has_error")); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			writeError(c, consts.StatusBadRequest, "invalid_request", "has_error must be true or false")
			return postgres.TraceListFilter{}, false
		}
		filter.HasError = &parsed
	}
	if value := strings.TrimSpace(c.Query("slow_ms")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			writeError(c, consts.StatusBadRequest, "invalid_request", "slow_ms must be a non-negative integer")
			return postgres.TraceListFilter{}, false
		}
		filter.SlowMS = parsed
	}
	if value := strings.TrimSpace(c.Query("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			writeError(c, consts.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return postgres.TraceListFilter{}, false
		}
		filter.Limit = parsed
	}
	return filter, true
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
	if !s.requireDatasetForEvaluation(ctx, c, req.DatasetID) {
		return
	}
	if strings.TrimSpace(req.DatasetID) != "" && strings.TrimSpace(req.KnowledgeBaseID) != "" {
		if !s.requireKnowledgeBase(ctx, c, req.KnowledgeBaseID) {
			return
		}
	}
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

func (s *Server) requireDatasetForEvaluation(ctx context.Context, c *app.RequestContext, datasetID string) bool {
	if strings.TrimSpace(datasetID) == "" {
		return true
	}
	if _, ok, err := s.App.Datasets.Get(ctx, tenantID(c), datasetID); err != nil {
		writeDatasetError(c, "dataset_lookup_failed", err)
		return false
	} else if !ok {
		writeDatasetNotFound(c)
		return false
	}
	return true
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
	HoldoutGate         eval.HoldoutGateConfig  `json:"holdout_gate,omitempty"`
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
	HoldoutGate             eval.HoldoutGateResult   `json:"holdout_gate,omitempty"`
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
	req := optimizationRequestFromSubmit(status.Run.StoredSubmitRequest())
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
	if !s.requireKnowledgeBase(ctx, c, req.KnowledgeBaseID) {
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
		HoldoutGate:     req.HoldoutGate,
		Runner:          runner,
	}, true
}

func optimizationRequestFromSubmit(req optimizer.SubmitRequest) optimizeRequest {
	return cloneOptimizeRequest(optimizeRequest{
		DatasetID:           req.DatasetID,
		KnowledgeBaseID:     req.KnowledgeBaseID,
		Objective:           req.Objective,
		SearchSpace:         req.SearchSpace,
		Search:              req.Search,
		Budget:              req.Budget,
		Profile:             req.Profile,
		TopK:                req.TopK,
		NamespaceTTLSeconds: int(req.NamespaceTTL / time.Second),
		SelectionSplit:      req.SelectionSplit,
		HoldoutSplit:        req.HoldoutSplit,
		HoldoutGate:         req.HoldoutGate,
		Runner:              req.Runner,
	})
}

func cloneOptimizeRequest(req optimizeRequest) optimizeRequest {
	req.Objective.Constraints = cloneSlice(req.Objective.Constraints)
	req.Objective.TieBreakers = cloneSlice(req.Objective.TieBreakers)
	req.SearchSpace = cloneOptimizerSearchSpace(req.SearchSpace)
	req.Runner = cloneAnyMap(req.Runner)
	req.Profiles = cloneSlice(req.Profiles)
	req.TopKs = cloneSlice(req.TopKs)
	return req
}

func cloneOptimizerSearchSpace(space optimizer.SearchSpace) optimizer.SearchSpace {
	space.Prompts = cloneSlice(space.Prompts)
	space.Chunking.SizeTokens = cloneSlice(space.Chunking.SizeTokens)
	space.Chunking.OverlapTokens = cloneSlice(space.Chunking.OverlapTokens)
	space.Chunking.ParserMethods = cloneSlice(space.Chunking.ParserMethods)
	space.Embedding.Models = cloneSlice(space.Embedding.Models)
	space.Embedding.Dimensions = cloneSlice(space.Embedding.Dimensions)
	space.Reranker.Providers = cloneSlice(space.Reranker.Providers)
	space.Reranker.Models = cloneSlice(space.Reranker.Models)
	space.Reranker.TopN = cloneSlice(space.Reranker.TopN)
	space.Reranker.ProviderModels = cloneStringSliceMap(space.Reranker.ProviderModels)
	space.Retrieval.DenseTopK = cloneSlice(space.Retrieval.DenseTopK)
	space.Retrieval.SparseTopK = cloneSlice(space.Retrieval.SparseTopK)
	space.Retrieval.RRFK = cloneSlice(space.Retrieval.RRFK)
	space.Retrieval.SemanticCacheThresholds = cloneSlice(space.Retrieval.SemanticCacheThresholds)
	space.Graph.QueryRewriteEnabled = cloneSlice(space.Graph.QueryRewriteEnabled)
	space.Graph.HyDEEnabled = cloneSlice(space.Graph.HyDEEnabled)
	space.Graph.MultiQueryCount = cloneSlice(space.Graph.MultiQueryCount)
	space.Graph.Modules = cloneStringMatrix(space.Graph.Modules)
	space.Harness = cloneHarnessCandidates(space.Harness)
	return space
}

func cloneHarnessCandidates(in []optimizer.HarnessCandidate) []optimizer.HarnessCandidate {
	if in == nil {
		return nil
	}
	out := make([]optimizer.HarnessCandidate, len(in))
	copy(out, in)
	for i := range out {
		out[i].Argv = cloneSlice(out[i].Argv)
	}
	return out
}

func cloneStringSliceMap(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for key, value := range in {
		out[key] = cloneSlice(value)
	}
	return out
}

func cloneStringMatrix(in [][]string) [][]string {
	if in == nil {
		return nil
	}
	out := make([][]string, len(in))
	for i := range in {
		out[i] = cloneSlice(in[i])
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneSlice[T any](in []T) []T {
	if in == nil {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
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
			HoldoutGate:             run.HoldoutGate,
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

type offlineKnowledgeRunRequest struct {
	KnowledgeBaseID string         `json:"knowledge_base_id"`
	KBID            string         `json:"kb_id"`
	WindowStart     time.Time      `json:"window_start"`
	WindowEnd       time.Time      `json:"window_end"`
	ConfigHash      string         `json:"config_hash"`
	ConfigJSON      map[string]any `json:"config_json,omitempty"`
	MaxQuestions    int            `json:"max_questions,omitempty"`
	MaxClusters     int            `json:"max_clusters,omitempty"`
}

func (s *Server) createOfflineKnowledgeRun(ctx context.Context, c *app.RequestContext) {
	if !s.requireOfflineKnowledge(c) {
		return
	}
	var req offlineKnowledgeRunRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.WindowStart.IsZero() || req.WindowEnd.IsZero() || !req.WindowEnd.After(req.WindowStart) {
		writeError(c, consts.StatusBadRequest, "invalid_request", "window_start and window_end are required and window_end must be after window_start")
		return
	}
	kbID := firstNonEmpty(req.KBID, req.KnowledgeBaseID)
	run, deduped, err := s.App.OfflineKnowledge.CreateRun(ctx, offlineknowledge.RunRequest{
		TenantID:     tenantID(c),
		KBID:         kbID,
		WindowStart:  req.WindowStart,
		WindowEnd:    req.WindowEnd,
		ConfigHash:   req.ConfigHash,
		ConfigJSON:   req.ConfigJSON,
		MaxQuestions: req.MaxQuestions,
		MaxClusters:  req.MaxClusters,
	})
	if err != nil {
		writeOfflineKnowledgeError(c, err, "offline_knowledge_run_create_failed")
		return
	}
	status := consts.StatusAccepted
	if deduped {
		status = consts.StatusOK
	}
	c.JSON(status, map[string]any{"run": run, "deduplicated": deduped})
}

func (s *Server) listOfflineKnowledgeRuns(ctx context.Context, c *app.RequestContext) {
	if !s.requireOfflineKnowledge(c) {
		return
	}
	runs, err := s.App.OfflineKnowledge.ListRuns(ctx, offlineknowledge.RunFilter{
		TenantID: tenantID(c),
		KBID:     strings.TrimSpace(firstNonEmpty(c.Query("kb_id"), c.Query("knowledge_base_id"))),
		Status:   offlineknowledge.RunStatus(strings.TrimSpace(c.Query("status"))),
		Limit:    queryPositiveInt(c, "limit"),
	})
	if err != nil {
		writeOfflineKnowledgeError(c, err, "offline_knowledge_run_list_failed")
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"items": runs})
}

func (s *Server) getOfflineKnowledgeRun(ctx context.Context, c *app.RequestContext) {
	if !s.requireOfflineKnowledge(c) {
		return
	}
	run, found, err := s.App.OfflineKnowledge.GetRun(ctx, tenantID(c), c.Param("id"))
	if err != nil {
		writeOfflineKnowledgeError(c, err, "offline_knowledge_run_lookup_failed")
		return
	}
	if !found {
		writeError(c, consts.StatusNotFound, "offline_knowledge_run_not_found", "offline knowledge run not found")
		return
	}
	c.JSON(consts.StatusOK, run)
}

func (s *Server) offlineKnowledgeRunAction(ctx context.Context, c *app.RequestContext) {
	if !s.requireOfflineKnowledge(c) {
		return
	}
	switch strings.TrimSpace(c.Param("action")) {
	case "execute":
		result, err := s.App.OfflineKnowledge.ExecuteRun(ctx, tenantID(c), c.Param("id"))
		if err != nil {
			writeOfflineKnowledgeError(c, err, "offline_knowledge_run_execute_failed")
			return
		}
		c.JSON(consts.StatusAccepted, offlineKnowledgeRunResultResponse(result))
	default:
		writeError(c, consts.StatusNotFound, "offline_knowledge_run_action_not_found", "offline knowledge run action not found")
	}
}

func (s *Server) triggerOfflineKnowledgeScheduler(ctx context.Context, c *app.RequestContext) {
	if s.App == nil || s.App.OfflineScheduler == nil || !s.App.OfflineScheduler.Enabled() {
		writeError(c, consts.StatusServiceUnavailable, "offline_knowledge_scheduler_disabled", "offline knowledge scheduler is disabled")
		return
	}
	var req struct {
		ScheduledAt time.Time `json:"scheduled_at"`
	}
	if len(c.Request.Body()) > 0 && !bindJSON(c, &req) {
		return
	}
	scheduledAt := req.ScheduledAt
	if scheduledAt.IsZero() {
		scheduledAt = time.Now().UTC()
	}
	results := s.App.OfflineScheduler.Trigger(ctx, scheduledAt)
	for _, result := range results {
		if result.Err != nil {
			writeOfflineKnowledgeError(c, result.Err, "offline_knowledge_scheduler_trigger_failed")
			return
		}
	}
	items := make([]map[string]any, 0, len(results))
	for _, result := range results {
		items = append(items, map[string]any{
			"target": map[string]any{
				"tenant_id": result.Target.TenantID,
				"kb_id":     result.Target.KBID,
			},
			"request": map[string]any{
				"kb_id":         result.Request.KBID,
				"window_start":  result.Request.WindowStart,
				"window_end":    result.Request.WindowEnd,
				"config_hash":   result.Request.ConfigHash,
				"config_json":   result.Request.ConfigJSON,
				"max_questions": result.Request.MaxQuestions,
				"max_clusters":  result.Request.MaxClusters,
			},
			"result":       offlineKnowledgeRunResultResponse(result.Result),
			"deduplicated": result.Deduplicated,
		})
	}
	c.JSON(consts.StatusAccepted, map[string]any{"items": items})
}

func offlineKnowledgeRunResultResponse(result offlineknowledge.RunResult) map[string]any {
	createdItems := result.CreatedItems
	if createdItems == nil {
		createdItems = []offlineknowledge.OptimizationItem{}
	}
	return map[string]any{
		"run":               result.Run,
		"deduplicated":      result.Deduplicated,
		"processed_cluster": result.ProcessedCluster,
		"created_items":     createdItems,
	}
}

func (s *Server) listOfflineKnowledgeQuestions(ctx context.Context, c *app.RequestContext) {
	if !s.requireOfflineKnowledge(c) {
		return
	}
	clusters, err := s.App.OfflineKnowledge.ListQuestionClusters(ctx, offlineknowledge.QuestionClusterFilter{
		TenantID: tenantID(c),
		RunID:    c.Param("id"),
		KBID:     strings.TrimSpace(firstNonEmpty(c.Query("kb_id"), c.Query("knowledge_base_id"))),
		Limit:    queryPositiveInt(c, "limit"),
	})
	if err != nil {
		writeOfflineKnowledgeError(c, err, "offline_knowledge_questions_list_failed")
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"items": clusters})
}

func (s *Server) listOptimizationItems(ctx context.Context, c *app.RequestContext) {
	if !s.requireOfflineKnowledge(c) {
		return
	}
	items, err := s.App.OfflineKnowledge.ListOptimizationItems(ctx, offlineknowledge.OptimizationItemFilter{
		TenantID: tenantID(c),
		KBID:     strings.TrimSpace(firstNonEmpty(c.Query("kb_id"), c.Query("knowledge_base_id"))),
		RunID:    strings.TrimSpace(c.Query("run_id")),
		Status:   offlineknowledge.ItemStatus(strings.TrimSpace(c.Query("status"))),
		ItemType: offlineknowledge.ItemType(strings.TrimSpace(c.Query("item_type"))),
		Limit:    queryPositiveInt(c, "limit"),
	})
	if err != nil {
		writeOfflineKnowledgeError(c, err, "optimization_item_list_failed")
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"items": items})
}

func (s *Server) getOptimizationItem(ctx context.Context, c *app.RequestContext) {
	if !s.requireOfflineKnowledge(c) {
		return
	}
	item, found, err := s.App.OfflineKnowledge.GetOptimizationItem(ctx, tenantID(c), c.Param("id"))
	if err != nil {
		writeOfflineKnowledgeError(c, err, "optimization_item_lookup_failed")
		return
	}
	if !found {
		writeOptimizationItemNotFound(c)
		return
	}
	c.JSON(consts.StatusOK, item)
}

func (s *Server) optimizationItemAction(ctx context.Context, c *app.RequestContext) {
	if !s.requireOfflineKnowledge(c) {
		return
	}
	action := strings.TrimSpace(c.Param("action"))
	switch action {
	case "verify", "reject", "enable-shadow", "publish", "disable":
		item, err := s.App.OfflineKnowledge.TransitionOptimizationItem(ctx, tenantID(c), c.Param("id"), offlineKnowledgeActionStatus(action))
		if err != nil {
			writeOfflineKnowledgeError(c, err, "optimization_item_action_failed")
			return
		}
		c.JSON(consts.StatusAccepted, item)
	case "revalidate":
		result, err := s.App.OfflineKnowledge.RevalidateItem(ctx, tenantID(c), c.Param("id"))
		if err != nil {
			writeOfflineKnowledgeError(c, err, "optimization_item_revalidate_failed")
			return
		}
		c.JSON(consts.StatusAccepted, result)
	case "run-regression":
		item, err := s.App.OfflineKnowledge.RunRegressionForItem(ctx, tenantID(c), c.Param("id"))
		if err != nil {
			writeOfflineKnowledgeError(c, err, "optimization_item_regression_failed")
			return
		}
		c.JSON(consts.StatusAccepted, item)
	default:
		writeError(c, consts.StatusNotFound, "optimization_item_action_not_found", "optimization item action not found")
	}
}

func (s *Server) bulkRevalidateOptimizationItems(ctx context.Context, c *app.RequestContext) {
	if !s.requireOfflineKnowledge(c) {
		return
	}
	var req struct {
		KnowledgeBaseID   string                             `json:"knowledge_base_id"`
		KBID              string                             `json:"kb_id"`
		Status            offlineknowledge.ItemStatus        `json:"status"`
		SourceFingerprint offlineknowledge.SourceFingerprint `json:"source_fingerprint"`
		SourceDocID       string                             `json:"source_doc_id"`
		SourceChunkID     string                             `json:"source_chunk_id"`
		SourceContentHash string                             `json:"source_content_hash"`
		Limit             int                                `json:"limit"`
	}
	if len(c.Request.Body()) > 0 && !bindJSON(c, &req) {
		return
	}
	result, err := s.App.OfflineKnowledge.BulkRevalidate(ctx, offlineknowledge.BulkRevalidateRequest{
		TenantID:          tenantID(c),
		KBID:              firstNonEmpty(req.KBID, req.KnowledgeBaseID),
		Status:            req.Status,
		SourceFingerprint: req.SourceFingerprint,
		SourceDocID:       req.SourceDocID,
		SourceChunkID:     req.SourceChunkID,
		SourceContentHash: req.SourceContentHash,
		Limit:             req.Limit,
	})
	if err != nil {
		writeOfflineKnowledgeError(c, err, "optimization_item_revalidate_failed")
		return
	}
	c.JSON(consts.StatusAccepted, result)
}

func (s *Server) requireOfflineKnowledge(c *app.RequestContext) bool {
	if s.App == nil || s.App.OfflineKnowledge == nil {
		writeError(c, consts.StatusNotFound, "offline_knowledge_not_configured", "offline knowledge service is not configured")
		return false
	}
	return true
}

func offlineKnowledgeActionStatus(action string) offlineknowledge.ItemStatus {
	switch action {
	case "verify":
		return offlineknowledge.ItemStatusVerified
	case "reject":
		return offlineknowledge.ItemStatusRejected
	case "enable-shadow":
		return offlineknowledge.ItemStatusShadowEnabled
	case "publish":
		return offlineknowledge.ItemStatusPublished
	case "disable":
		return offlineknowledge.ItemStatusDeprecated
	default:
		return ""
	}
}

func writeOptimizationItemNotFound(c *app.RequestContext) {
	writeError(c, consts.StatusNotFound, "optimization_item_not_found", "optimization item not found")
}

func writeOfflineKnowledgeError(c *app.RequestContext, err error, fallbackCode string) {
	switch {
	case errors.Is(err, offlineknowledge.ErrRunNotFound):
		writeError(c, consts.StatusNotFound, "offline_knowledge_run_not_found", "offline knowledge run not found")
	case errors.Is(err, offlineknowledge.ErrRunExecutionConflict):
		writeError(c, consts.StatusConflict, "offline_knowledge_run_execution_conflict", err.Error())
	case errors.Is(err, offlineknowledge.ErrOptimizationItemNotFound):
		writeOptimizationItemNotFound(c)
	case errors.Is(err, offlineknowledge.ErrInvalidItemTransition):
		writeError(c, consts.StatusConflict, "invalid_optimization_item_transition", err.Error())
	case errors.Is(err, offlineknowledge.ErrServiceRepositoryRequired):
		writeError(c, consts.StatusServiceUnavailable, "offline_knowledge_not_configured", "offline knowledge service is not configured")
	case errors.Is(err, offlineknowledge.ErrValidatorRequired), errors.Is(err, offlineknowledge.ErrValidatorDisabled):
		writeError(c, consts.StatusServiceUnavailable, "offline_knowledge_validator_missing", "offline knowledge validator is not configured")
	case errors.Is(err, offlineknowledge.ErrHistorySourceRequired),
		errors.Is(err, offlineknowledge.ErrQuestionClustererRequired),
		errors.Is(err, offlineknowledge.ErrRecallReplayerRequired),
		errors.Is(err, offlineknowledge.ErrCodexAnalyzerRequired),
		errors.Is(err, offlineknowledge.ErrCodexDisabled),
		errors.Is(err, offlineknowledge.ErrCodexUnavailable),
		errors.Is(err, offlineknowledge.ErrSourceReaderUnavailable),
		errors.Is(err, offlineknowledge.ErrConclusionDisabled),
		errors.Is(err, offlineknowledge.ErrConclusionUnavailable),
		errors.Is(err, offlineknowledge.ErrRegressionRunnerRequired),
		errors.Is(err, offlineknowledge.ErrRegressionDisabled),
		errors.Is(err, offlineknowledge.ErrRegressionUnavailable),
		errors.Is(err, offlineknowledge.ErrRegressionDatasetRequired),
		errors.Is(err, offlineknowledge.ErrSchedulerServiceRequired),
		errors.Is(err, offlineknowledge.ErrSchedulerTargetRequired):
		writeError(c, consts.StatusServiceUnavailable, "offline_knowledge_dependency_unavailable", err.Error())
	default:
		writeError(c, consts.StatusInternalServerError, fallbackCode, err.Error())
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func queryPositiveInt(c *app.RequestContext, key string) int {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
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
