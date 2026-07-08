package offlineknowledge

import (
	"context"
	"errors"
	"time"
)

var ErrRunConflict = errors.New("offline knowledge run already exists for tenant, knowledge base, window, and config")

type RunFilter struct {
	TenantID string
	KBID     string
	Status   RunStatus
	Limit    int
}

type QuestionClusterFilter struct {
	TenantID     string
	KBID         string
	RunID        string
	QuestionHash string
	Limit        int
}

type OptimizationItemFilter struct {
	TenantID          string
	KBID              string
	RunID             string
	QuestionClusterID string
	Status            ItemStatus
	ItemType          ItemType
	Limit             int
}

type OptimizationItemEvent struct {
	ID        string         `json:"id"`
	TenantID  string         `json:"tenant_id"`
	ItemID    string         `json:"item_id"`
	EventType string         `json:"event_type"`
	Operator  string         `json:"operator,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type OptimizationItemEventFilter struct {
	TenantID  string
	ItemID    string
	EventType string
	Since     time.Time
	Until     time.Time
	Limit     int
}

type ShadowRetrievalEventFilter struct {
	TenantID string
	KBID     string
	ItemID   string
	TraceID  string
	Since    time.Time
	Until    time.Time
	Limit    int
}

type CodexToolAuditFilter struct {
	TenantID  string
	KBID      string
	SessionID string
	Tool      ReadOnlyToolName
	Since     time.Time
	Until     time.Time
	Limit     int
}

type Repository interface {
	NegativeFeedbackRepository

	CreateRun(ctx context.Context, run OfflineKnowledgeRun) error
	GetRun(ctx context.Context, tenantID, runID string) (OfflineKnowledgeRun, bool, error)
	ListRuns(ctx context.Context, filter RunFilter) ([]OfflineKnowledgeRun, error)
	UpdateRun(ctx context.Context, run OfflineKnowledgeRun) (bool, error)

	UpsertQuestionCluster(ctx context.Context, cluster QuestionCluster) error
	GetQuestionCluster(ctx context.Context, tenantID, clusterID string) (QuestionCluster, bool, error)
	ListQuestionClusters(ctx context.Context, filter QuestionClusterFilter) ([]QuestionCluster, error)

	CreateOptimizationItem(ctx context.Context, item OptimizationItem) error
	GetOptimizationItem(ctx context.Context, tenantID, itemID string) (OptimizationItem, bool, error)
	ListOptimizationItems(ctx context.Context, filter OptimizationItemFilter) ([]OptimizationItem, error)
	UpdateOptimizationItem(ctx context.Context, item OptimizationItem) (bool, error)
	UpdateOptimizationItemStatus(ctx context.Context, tenantID, itemID string, status ItemStatus, updatedAt time.Time) (bool, error)

	AppendItemEvent(ctx context.Context, event OptimizationItemEvent) error
	ListItemEvents(ctx context.Context, filter OptimizationItemEventFilter) ([]OptimizationItemEvent, error)

	RecordShadowEvent(ctx context.Context, event ShadowRetrievalEvent) error
	ListShadowEvents(ctx context.Context, filter ShadowRetrievalEventFilter) ([]ShadowRetrievalEvent, error)

	RecordCodexToolAudit(ctx context.Context, event CodexToolAuditEvent) error
	ListCodexToolAuditEvents(ctx context.Context, filter CodexToolAuditFilter) ([]CodexToolAuditEvent, error)
}

type SourceChunk struct {
	TenantID         string
	KBID             string
	DocID            string
	DocVersion       string
	ChunkID          string
	ChunkContentHash string
	Text             string
}

type SourceReader interface {
	ReadSourceChunk(ctx context.Context, tenantID, kbID, chunkID string) (SourceChunk, bool, error)
}

type ConclusionJudge interface {
	JudgeConclusion(ctx context.Context, item OptimizationItem, evidence []Evidence) (bool, error)
}
