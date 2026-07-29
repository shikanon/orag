package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shikanon/orag/internal/audit"
	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/taskqueue"
)

const (
	DocumentImportTaskType = "document.import"
	IngestionCorePool      = "ingestion_core"
	EvaluationRunTaskType  = "evaluation.run"
	EvaluationPool         = "evaluation"
)

// DocumentImportTaskPayload is intentionally limited to the same data the
// synchronous import endpoint already accepts. It is an internal payload; the
// API returns only task metadata and never echoes its content.
type DocumentImportTaskPayload struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	SourceURI       string `json:"source_uri"`
	Name            string `json:"name"`
	ContentBase64   string `json:"content_base64"`
}

type documentImportTaskHandler struct {
	ingest *ingest.Service
	audit  interface {
		Record(context.Context, audit.AuditEvent) error
	}
}

func (h documentImportTaskHandler) Handle(ctx context.Context, task taskqueue.Task, _ taskqueue.ProgressReporter) error {
	var payload DocumentImportTaskPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("decode document import payload: %w", err)
	}
	if strings.TrimSpace(payload.KnowledgeBaseID) == "" || strings.TrimSpace(payload.Name) == "" || payload.ContentBase64 == "" {
		return fmt.Errorf("document import payload is incomplete")
	}
	content, err := base64.StdEncoding.DecodeString(payload.ContentBase64)
	if err != nil {
		return fmt.Errorf("decode document import content: %w", err)
	}
	result, err := h.ingest.Ingest(ctx, ingest.Request{
		TenantID:        task.TenantID,
		KnowledgeBaseID: payload.KnowledgeBaseID,
		SourceURI:       payload.SourceURI,
		Name:            payload.Name,
		Content:         content,
	})
	if err != nil {
		return err
	}
	if h.audit != nil {
		_ = h.audit.Record(ctx, audit.AuditEvent{
			ID: id.New("audit"), TenantID: task.TenantID, ProjectID: task.ProjectID,
			ActorType: audit.ActorTypeSystem, Action: audit.ActionDocumentImportCompleted,
			ResourceType: audit.ResourceTypeDocument, ResourceID: result.Document.ID,
			Outcome: audit.OutcomeSuccess, TraceID: task.TraceID, TaskID: task.ID,
			Metadata: map[string]string{"knowledge_base_id": payload.KnowledgeBaseID, "chunk_count": fmt.Sprintf("%d", len(result.Chunks))},
		})
	}
	return nil
}

type EvaluationTaskPayload struct {
	Request eval.RunRequest `json:"request"`
}

type evaluationTaskHandler struct {
	runner eval.Runner
	audit  interface {
		Record(context.Context, audit.AuditEvent) error
	}
}

func (h evaluationTaskHandler) Handle(ctx context.Context, task taskqueue.Task, _ taskqueue.ProgressReporter) error {
	var payload EvaluationTaskPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("decode evaluation payload: %w", err)
	}
	if strings.TrimSpace(payload.Request.DatasetID) == "" {
		return fmt.Errorf("evaluation payload dataset_id is required")
	}
	payload.Request.TenantID = task.TenantID
	payload.Request.ProjectID = task.ProjectID
	result, err := h.runner.Run(ctx, payload.Request)
	if err != nil {
		return err
	}
	if h.audit != nil {
		_ = h.audit.Record(ctx, audit.AuditEvent{
			ID: id.New("audit"), TenantID: task.TenantID, ProjectID: task.ProjectID,
			ActorType: audit.ActorTypeSystem, Action: audit.ActionEvaluationCompleted,
			ResourceType: "evaluation", ResourceID: result.ID, Outcome: audit.OutcomeSuccess,
			TraceID: task.TraceID, TaskID: task.ID,
		})
	}
	return nil
}
