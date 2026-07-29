package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/audit"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/taskqueue"
)

const (
	taskMaturityHeader = "X-Orag-Maturity"
	taskMaturityValue  = "experimental"
)

type TaskService interface {
	EnqueueTask(ctx context.Context, tenantID string, task taskqueue.Task) (taskqueue.Task, error)
	GetTask(ctx context.Context, tenantID, taskID string) (taskqueue.Task, bool, error)
	ListTasks(ctx context.Context, tenantID string, filter taskqueue.TaskFilter) ([]taskqueue.Task, string, error)
	CancelTask(ctx context.Context, tenantID, taskID, actorType, actorID, traceID string) error
	RetryTask(ctx context.Context, tenantID, taskID, actorType, actorID, traceID string) (taskqueue.Task, error)
	ListTaskEvents(ctx context.Context, tenantID, taskID string, cursor string, limit int) ([]TaskEvent, string, error)
	GetTaskStats(ctx context.Context, tenantID string) (TaskStats, error)
}

type TaskEvent struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Type      string    `json:"type"`
	Status    string    `json:"status,omitempty"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type TaskStats struct {
	Pools map[string]TaskPoolStats `json:"pools"`
}

type TaskPoolStats struct {
	Queued     int `json:"queued"`
	Running    int `json:"running"`
	Succeeded  int `json:"succeeded"`
	Failed     int `json:"failed"`
	DeadLetter int `json:"dead_letter"`
}

type createTaskRequest struct {
	Type               string          `json:"type"`
	Pool               string          `json:"pool"`
	Payload            json.RawMessage `json:"payload,omitempty"`
	IdempotencyKey     string          `json:"idempotency_key,omitempty"`
	MaxAttempts        int             `json:"max_attempts,omitempty"`
	Priority           int             `json:"priority,omitempty"`
	RunAfter           *time.Time      `json:"run_after,omitempty"`
	ProjectID          string          `json:"project_id,omitempty"`
	LockedResourceType string          `json:"locked_resource_type,omitempty"`
	LockedResourceID   string          `json:"locked_resource_id,omitempty"`
}

func (s *Server) createTask(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}

	var req createTaskRequest
	if !bindJSON(c, &req) {
		return
	}

	taskType := strings.TrimSpace(req.Type)
	if taskType != core.DocumentImportTaskType {
		writeError(c, consts.StatusBadRequest, "invalid_request", "type must be document.import")
		return
	}
	pool := strings.TrimSpace(req.Pool)
	if pool != core.IngestionCorePool {
		writeError(c, consts.StatusBadRequest, "invalid_request", "pool must be ingestion_core")
		return
	}

	var payload core.DocumentImportTaskPayload
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid_request", "payload must be a document import payload")
		return
	}
	payload.KnowledgeBaseID = strings.TrimSpace(payload.KnowledgeBaseID)
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.KnowledgeBaseID == "" || payload.Name == "" || payload.ContentBase64 == "" {
		writeError(c, consts.StatusBadRequest, "invalid_request", "payload knowledge_base_id, name, and content_base64 are required")
		return
	}
	if content, err := base64.StdEncoding.DecodeString(payload.ContentBase64); err != nil || len(content) == 0 {
		writeError(c, consts.StatusBadRequest, "invalid_request", "payload content_base64 must encode non-empty content")
		return
	}
	if s.App == nil || s.App.KBStore == nil {
		writeError(c, consts.StatusInternalServerError, "knowledge_base_lookup_failed", "knowledge base repository is not configured")
		return
	}
	knowledgeBase, found, err := s.App.KBStore.GetKnowledgeBase(ctx, principal.TenantID, payload.KnowledgeBaseID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "knowledge_base_lookup_failed", err.Error())
		return
	}
	if !found {
		writeKnowledgeBaseNotFound(c)
		return
	}
	if !authorizeRequest(c, auth.ActionResourceWrite, principal.TenantID, knowledgeBase.ProjectID) {
		return
	}
	if (req.ProjectID != "" && req.ProjectID != knowledgeBase.ProjectID) ||
		(req.LockedResourceType != "" && req.LockedResourceType != audit.ResourceTypeKnowledgeBase) ||
		(req.LockedResourceID != "" && req.LockedResourceID != knowledgeBase.ID) {
		writeError(c, consts.StatusBadRequest, "invalid_request", "project and locked resource assertions must match the target knowledge base")
		return
	}
	canonicalPayload, err := json.Marshal(payload)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "task_payload_encode_failed", err.Error())
		return
	}
	task := taskqueue.Task{
		TenantID:           principal.TenantID,
		ProjectID:          knowledgeBase.ProjectID,
		Type:               taskType,
		Pool:               pool,
		Payload:            canonicalPayload,
		IdempotencyKey:     strings.TrimSpace(req.IdempotencyKey),
		MaxAttempts:        req.MaxAttempts,
		Priority:           req.Priority,
		LockedResourceType: audit.ResourceTypeKnowledgeBase,
		LockedResourceID:   knowledgeBase.ID,
		TraceID:            requestTraceID(c),
		CreatedBy:          string(principal.Kind) + ":" + principal.SubjectID,
	}
	if req.RunAfter != nil {
		task.RunAfter = *req.RunAfter
	}

	created, err := s.taskService().EnqueueTask(ctx, principal.TenantID, task)
	if err != nil {
		writeTaskError(c, err)
		return
	}

	setMaturityHeader(c)
	c.JSON(consts.StatusCreated, map[string]any{
		"task_id":    created.ID,
		"status":     created.Status,
		"created_at": created.CreatedAt,
	})
}

func (s *Server) listTasks(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}

	filter, ok := parseTaskListFilter(c)
	if !ok {
		return
	}
	if !scopeTaskFilter(c, principal, &filter) {
		return
	}

	tasks, nextCursor, err := s.taskService().ListTasks(ctx, principal.TenantID, filter)
	if err != nil {
		writeTaskError(c, err)
		return
	}

	setMaturityHeader(c)
	c.JSON(consts.StatusOK, map[string]any{
		"data":        tasks,
		"next_cursor": nextCursor,
	})
}

func (s *Server) getTask(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}

	taskID := c.Param("task_id")
	task, found, err := s.taskService().GetTask(ctx, principal.TenantID, taskID)
	if err != nil {
		writeTaskError(c, err)
		return
	}
	if !found {
		writeTaskNotFound(c)
		return
	}
	if !authorizeRequest(c, auth.ActionResourceRead, principal.TenantID, task.ProjectID) {
		return
	}

	setMaturityHeader(c)
	c.JSON(consts.StatusOK, task)
}

func (s *Server) cancelTask(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}

	taskID := taskIDFromRequest(c)
	if !s.authorizeTask(ctx, c, principal, taskID, auth.ActionResourceWrite) {
		return
	}
	err := s.taskService().CancelTask(ctx, principal.TenantID, taskID, string(principal.Kind), principal.SubjectID, requestTraceID(c))
	if err != nil {
		writeTaskError(c, err)
		return
	}

	setMaturityHeader(c)
	c.JSON(consts.StatusOK, map[string]any{
		"task_id": taskID,
		"status":  "cancelling",
	})
}

func (s *Server) retryTask(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}

	taskID := taskIDFromRequest(c)
	if !s.authorizeTask(ctx, c, principal, taskID, auth.ActionResourceWrite) {
		return
	}
	task, err := s.taskService().RetryTask(ctx, principal.TenantID, taskID, string(principal.Kind), principal.SubjectID, requestTraceID(c))
	if err != nil {
		writeTaskError(c, err)
		return
	}

	setMaturityHeader(c)
	c.JSON(consts.StatusOK, map[string]any{
		"task_id": task.ID,
		"status":  task.Status,
	})
}

func (s *Server) taskAction(ctx context.Context, c *app.RequestContext) {
	action := strings.TrimPrefix(c.Param("action"), "/")
	_, operation, found := strings.Cut(action, ":")
	if !found {
		writeError(c, consts.StatusNotFound, "task_action_not_found", "task action not found")
		return
	}
	switch operation {
	case "cancel":
		s.cancelTask(ctx, c)
	case "retry":
		s.retryTask(ctx, c)
	default:
		writeError(c, consts.StatusNotFound, "task_action_not_found", "task action not found")
	}
}

func taskIDFromRequest(c *app.RequestContext) string {
	if taskID := c.Param("task_id"); taskID != "" {
		return taskID
	}
	action := strings.TrimPrefix(c.Param("action"), "/")
	taskID, _, _ := strings.Cut(action, ":")
	return taskID
}

func scopeTaskFilter(c *app.RequestContext, principal auth.Principal, filter *taskqueue.TaskFilter) bool {
	if principal.ProjectID == "" {
		return true
	}
	if filter.ProjectID != "" && filter.ProjectID != principal.ProjectID {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return false
	}
	filter.ProjectID = principal.ProjectID
	return true
}

func (s *Server) authorizeTask(ctx context.Context, c *app.RequestContext, principal auth.Principal, taskID string, action auth.Action) bool {
	task, found, err := s.taskService().GetTask(ctx, principal.TenantID, taskID)
	if err != nil {
		writeTaskError(c, err)
		return false
	}
	if !found {
		writeTaskNotFound(c)
		return false
	}
	return authorizeRequest(c, action, principal.TenantID, task.ProjectID)
}

func (s *Server) listTaskEvents(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}

	taskID := c.Param("task_id")
	if !s.authorizeTask(ctx, c, principal, taskID, auth.ActionResourceRead) {
		return
	}
	cursor := strings.TrimSpace(c.Query("cursor"))
	limit := 20
	if limitStr := strings.TrimSpace(c.Query("limit")); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed <= 0 {
			writeError(c, consts.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return
		}
		if parsed > 100 {
			parsed = 100
		}
		limit = parsed
	}

	events, nextCursor, err := s.taskService().ListTaskEvents(ctx, principal.TenantID, taskID, cursor, limit)
	if err != nil {
		writeTaskError(c, err)
		return
	}

	setMaturityHeader(c)
	c.JSON(consts.StatusOK, map[string]any{
		"data":        events,
		"next_cursor": nextCursor,
	})
}

func (s *Server) getTaskStats(ctx context.Context, c *app.RequestContext) {
	principal, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}
	if principal.ProjectID != "" {
		writeError(c, consts.StatusForbidden, "forbidden", "project-scoped task statistics require a project filter endpoint")
		return
	}

	stats, err := s.taskService().GetTaskStats(ctx, principal.TenantID)
	if err != nil {
		writeTaskError(c, err)
		return
	}

	setMaturityHeader(c)
	c.JSON(consts.StatusOK, stats)
}

func parseTaskListFilter(c *app.RequestContext) (taskqueue.TaskFilter, bool) {
	var filter taskqueue.TaskFilter

	if status := strings.TrimSpace(c.Query("status")); status != "" {
		filter.Status = taskqueue.TaskStatus(status)
	}
	if typeStr := strings.TrimSpace(c.Query("type")); typeStr != "" {
		filter.Type = typeStr
	}
	if pool := strings.TrimSpace(c.Query("pool")); pool != "" {
		filter.Pool = pool
	}
	if projectID := strings.TrimSpace(c.Query("project_id")); projectID != "" {
		filter.ProjectID = projectID
	}

	limit := 20
	if limitStr := strings.TrimSpace(c.Query("limit")); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed <= 0 {
			writeError(c, consts.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return taskqueue.TaskFilter{}, false
		}
		if parsed > 100 {
			parsed = 100
		}
		limit = parsed
	}
	filter.Limit = limit

	if cursor := strings.TrimSpace(c.Query("cursor")); cursor != "" {
		filter.Cursor = cursor
	}

	return filter, true
}

func setMaturityHeader(c *app.RequestContext) {
	c.Header(taskMaturityHeader, taskMaturityValue)
}

func writeTaskNotFound(c *app.RequestContext) {
	writeError(c, consts.StatusNotFound, "task_not_found", "task not found")
}

func writeTaskError(c *app.RequestContext, err error) {
	writeError(c, consts.StatusInternalServerError, "task_operation_failed", err.Error())
}

var taskServiceInstance TaskService = &noopTaskService{}

func SetTaskService(svc TaskService) {
	taskServiceInstance = svc
}

func (s *Server) taskService() TaskService {
	if s.Tasks != nil {
		return s.Tasks
	}
	return taskServiceInstance
}

type taskServiceAdapter struct{ service *taskqueue.Service }

func newTaskServiceAdapter(service *taskqueue.Service) TaskService {
	return &taskServiceAdapter{service: service}
}
func (a *taskServiceAdapter) EnqueueTask(ctx context.Context, tenantID string, task taskqueue.Task) (taskqueue.Task, error) {
	return a.service.Enqueue(ctx, tenantID, task)
}
func (a *taskServiceAdapter) GetTask(ctx context.Context, tenantID, taskID string) (taskqueue.Task, bool, error) {
	return a.service.Get(ctx, tenantID, taskID)
}
func (a *taskServiceAdapter) ListTasks(ctx context.Context, tenantID string, filter taskqueue.TaskFilter) ([]taskqueue.Task, string, error) {
	return a.service.List(ctx, tenantID, filter)
}
func (a *taskServiceAdapter) CancelTask(ctx context.Context, tenantID, taskID, actorType, actorID, traceID string) error {
	return a.service.Cancel(ctx, tenantID, taskID, actorType, actorID, traceID)
}
func (a *taskServiceAdapter) RetryTask(ctx context.Context, tenantID, taskID, actorType, actorID, traceID string) (taskqueue.Task, error) {
	return a.service.Retry(ctx, tenantID, taskID, actorType, actorID, traceID)
}
func (a *taskServiceAdapter) ListTaskEvents(ctx context.Context, tenantID, taskID, cursor string, limit int) ([]TaskEvent, string, error) {
	events, next, err := a.service.ListEvents(ctx, tenantID, taskID, cursor, limit)
	if err != nil {
		return nil, "", err
	}
	out := make([]TaskEvent, 0, len(events))
	for _, event := range events {
		out = append(out, TaskEvent{ID: event.ID, TaskID: event.TaskID, Type: event.Type, CreatedAt: event.CreatedAt})
	}
	return out, next, nil
}
func (a *taskServiceAdapter) GetTaskStats(ctx context.Context, tenantID string) (TaskStats, error) {
	stats, err := a.service.Stats(ctx, tenantID)
	if err != nil {
		return TaskStats{}, err
	}
	out := TaskStats{Pools: make(map[string]TaskPoolStats, len(stats))}
	for pool, stat := range stats {
		out.Pools[pool] = TaskPoolStats{Queued: stat.Queued, Running: stat.Running, Succeeded: stat.Succeeded, Failed: stat.Failed, DeadLetter: stat.DeadLetter}
	}
	return out, nil
}

type noopTaskService struct{}

func (n *noopTaskService) EnqueueTask(_ context.Context, _ string, task taskqueue.Task) (taskqueue.Task, error) {
	return taskqueue.Task{}, errTaskServiceUnavailable
}

func (n *noopTaskService) GetTask(_ context.Context, _, _ string) (taskqueue.Task, bool, error) {
	return taskqueue.Task{}, false, errTaskServiceUnavailable
}

func (n *noopTaskService) ListTasks(_ context.Context, _ string, _ taskqueue.TaskFilter) ([]taskqueue.Task, string, error) {
	return nil, "", errTaskServiceUnavailable
}

func (n *noopTaskService) CancelTask(_ context.Context, _, _, _, _, _ string) error {
	return errTaskServiceUnavailable
}

func (n *noopTaskService) RetryTask(_ context.Context, _, _, _, _, _ string) (taskqueue.Task, error) {
	return taskqueue.Task{}, errTaskServiceUnavailable
}

func (n *noopTaskService) ListTaskEvents(_ context.Context, _, _, _ string, _ int) ([]TaskEvent, string, error) {
	return nil, "", errTaskServiceUnavailable
}

func (n *noopTaskService) GetTaskStats(_ context.Context, _ string) (TaskStats, error) {
	return TaskStats{}, errTaskServiceUnavailable
}

var errTaskServiceUnavailable = &taskServiceUnavailableError{}

type taskServiceUnavailableError struct{}

func (e *taskServiceUnavailableError) Error() string {
	return "task queue service is not configured"
}
