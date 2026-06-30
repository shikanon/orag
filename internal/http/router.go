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
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/rag"
)

type Server struct {
	App *core.App
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

func (s *Server) createKnowledgeBase(_ context.Context, c *app.RequestContext) {
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
	if err := s.App.KBStore.PutKnowledgeBase(item); err != nil {
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

func (s *Server) deleteKnowledgeBase(_ context.Context, c *app.RequestContext) {
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
	var req rag.QueryRequest
	if !bindJSON(c, &req) {
		return
	}
	start := time.Now()
	traceID := requestTraceID(c)
	ctx = observability.WithTraceID(ctx, traceID)
	req.TenantID = tenantID(c)
	req.TraceID = traceID
	resp, err := s.App.RAG.Query(ctx, req)
	if err != nil {
		s.observeRAGError(req.Profile, "query_failed", time.Since(start).Milliseconds())
		writeError(c, consts.StatusInternalServerError, "query_failed", err.Error())
		return
	}
	s.observeRAGSuccess(resp.Profile, resp.CacheStatus, resp.LatencyMS)
	c.JSON(consts.StatusOK, resp)
}

func (s *Server) queryStream(ctx context.Context, c *app.RequestContext) {
	var req rag.QueryRequest
	if !bindJSON(c, &req) {
		return
	}
	start := time.Now()
	traceID := requestTraceID(c)
	ctx = observability.WithTraceID(ctx, traceID)
	req.TenantID = tenantID(c)
	req.TraceID = traceID
	resp, err := s.App.RAG.Query(ctx, req)
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	if err != nil {
		c.Set("error_code", "query_failed")
		s.observeRAGError(req.Profile, "query_failed", time.Since(start).Milliseconds())
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
	created, err := s.App.Datasets.AddItem(ctx, c.Param("id"), item)
	if err != nil {
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
		writeError(c, consts.StatusInternalServerError, "evaluation_failed", err.Error())
		return
	}
	c.JSON(consts.StatusAccepted, resp)
}

func (s *Server) getEvaluation(ctx context.Context, c *app.RequestContext) {
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

func (s *Server) optimize(ctx context.Context, c *app.RequestContext) {
	var req eval.OptimizeRequest
	if !bindJSON(c, &req) {
		return
	}
	req.TenantID = tenantID(c)
	result, err := eval.Optimizer{Runner: s.App.Eval}.Optimize(ctx, req)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "optimization_failed", err.Error())
		return
	}
	c.JSON(consts.StatusAccepted, result)
}

func bindJSON(c *app.RequestContext, dst any) bool {
	if err := json.Unmarshal(c.Request.Body(), dst); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", "invalid json body")
		return false
	}
	return true
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
