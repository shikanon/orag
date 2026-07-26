package audit

import (
	"context"
	"time"
)

const (
	ActionKBCreated                = "kb.created"
	ActionKBUpdated                = "kb.updated"
	ActionDocumentImportRequested  = "document.import_requested"
	ActionDocumentImportCompleted  = "document.import_completed"
	ActionDocumentReparseRequested = "document.reparse_requested"
	ActionPipelineUpdated          = "pipeline.updated"
	ActionPipelineVersionCreated   = "pipeline_version.created"
	ActionReleasePromoted          = "release.promoted"
	ActionReleaseRolledBack        = "release.rolled_back"
	ActionEvaluationRequested      = "evaluation.requested"
	ActionEvaluationCompleted      = "evaluation.completed"
	ActionTaskCancelled            = "task.cancelled"
	ActionTaskRetried              = "task.retried"
	ActionModelReadinessTested     = "model.readiness_tested"
	ActionAuthAccessDenied         = "auth.access_denied"
	ActionAPIKeyCreated            = "api_key.created"
	ActionAPIKeyRevoked            = "api_key.revoked"
)

const (
	OutcomeSuccess   = "success"
	OutcomeFailure   = "failure"
	OutcomeCancelled = "cancelled"
)

const (
	ActorTypeUser    = "user"
	ActorTypeAPIKey  = "api_key"
	ActorTypeSystem  = "system"
	ActorTypeService = "service"
)

const (
	ResourceTypeKnowledgeBase = "knowledge_base"
	ResourceTypeDocument      = "document"
	ResourceTypePipeline      = "pipeline"
	ResourceTypeRelease       = "release"
	ResourceTypeTask          = "task"
	ResourceTypeModel         = "model"
	ResourceTypeAPIKey        = "api_key"
)

type AuditEvent struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenant_id"`
	ProjectID      string            `json:"project_id,omitempty"`
	ActorType      string            `json:"actor_type"`
	ActorID        string            `json:"actor_id"`
	Action         string            `json:"action"`
	ResourceType   string            `json:"resource_type"`
	ResourceID     string            `json:"resource_id"`
	Outcome        string            `json:"outcome"`
	TraceID        string            `json:"trace_id,omitempty"`
	TaskID         string            `json:"task_id,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	ErrorCode      string            `json:"error_code,omitempty"`
	ErrorMessage   string            `json:"error_message,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
}

type AuditFilter struct {
	TenantID     string
	ProjectID    string
	ResourceType string
	ResourceID   string
	Action       string
	ActorID      string
	Outcome      string
	Limit        int
	Cursor       string
}

type AuditRepository interface {
	Record(ctx context.Context, event AuditEvent) error
	List(ctx context.Context, filter AuditFilter) ([]AuditEvent, string, error)
	Get(ctx context.Context, eventID string) (AuditEvent, bool, error)
}
