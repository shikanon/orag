package offlineknowledge

import (
	"context"
	"sort"
	"sync"
	"time"
)

type MemoryRepository struct {
	mu sync.RWMutex

	runs     map[string]OfflineKnowledgeRun
	clusters map[string]QuestionCluster
	items    map[string]OptimizationItem
	events   []OptimizationItemEvent
	shadow   []ShadowRetrievalEvent
	audit    []CodexToolAuditEvent
	feedback []NegativeFeedback
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		runs:     make(map[string]OfflineKnowledgeRun),
		clusters: make(map[string]QuestionCluster),
		items:    make(map[string]OptimizationItem),
	}
}

func (r *MemoryRepository) AddNegativeFeedback(_ context.Context, item NegativeFeedback) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item = copyNegativeFeedback(item)
	if item.ID == "" {
		item.ID = stableID("negfb", item.TenantID, item.KBID, item.TraceID, item.Query, item.Reason, item.CreatedAt.Format(time.RFC3339Nano))
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	r.feedback = append(r.feedback, item)
	return nil
}

func (r *MemoryRepository) ListNegativeFeedback(_ context.Context, filter NegativeFeedbackFilter) ([]NegativeFeedback, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	traceIDs := make(map[string]struct{}, len(filter.TraceIDs))
	for _, traceID := range filter.TraceIDs {
		if traceID != "" {
			traceIDs[traceID] = struct{}{}
		}
	}
	out := make([]NegativeFeedback, 0, len(r.feedback))
	for _, item := range r.feedback {
		if filter.TenantID != "" && item.TenantID != filter.TenantID {
			continue
		}
		if filter.KBID != "" && item.KBID != filter.KBID {
			continue
		}
		if !filter.Since.IsZero() && item.CreatedAt.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && !item.CreatedAt.Before(filter.Until) {
			continue
		}
		if len(traceIDs) > 0 && item.TraceID != "" {
			if _, ok := traceIDs[item.TraceID]; !ok {
				continue
			}
		}
		out = append(out, copyNegativeFeedback(item))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return limitNegativeFeedback(out, filter.Limit), nil
}

func (r *MemoryRepository) CreateRun(_ context.Context, run OfflineKnowledgeRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = copyRun(run)
	return nil
}

func (r *MemoryRepository) GetRun(_ context.Context, tenantID, runID string) (OfflineKnowledgeRun, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID {
		return OfflineKnowledgeRun{}, false, nil
	}
	return copyRun(run), true, nil
}

func (r *MemoryRepository) ListRuns(_ context.Context, filter RunFilter) ([]OfflineKnowledgeRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]OfflineKnowledgeRun, 0, len(r.runs))
	for _, run := range r.runs {
		if filter.TenantID != "" && run.TenantID != filter.TenantID {
			continue
		}
		if filter.KBID != "" && run.KBID != filter.KBID {
			continue
		}
		if filter.Status != "" && run.Status != filter.Status {
			continue
		}
		out = append(out, copyRun(run))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return limitRuns(out, filter.Limit), nil
}

func (r *MemoryRepository) UpdateRun(_ context.Context, run OfflineKnowledgeRun) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.runs[run.ID]
	if !ok || current.TenantID != run.TenantID {
		return false, nil
	}
	r.runs[run.ID] = copyRun(run)
	return true, nil
}

func (r *MemoryRepository) UpsertQuestionCluster(_ context.Context, cluster QuestionCluster) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clusters[cluster.ID] = copyCluster(cluster)
	return nil
}

func (r *MemoryRepository) GetQuestionCluster(_ context.Context, tenantID, clusterID string) (QuestionCluster, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cluster, ok := r.clusters[clusterID]
	if !ok || cluster.TenantID != tenantID {
		return QuestionCluster{}, false, nil
	}
	return copyCluster(cluster), true, nil
}

func (r *MemoryRepository) ListQuestionClusters(_ context.Context, filter QuestionClusterFilter) ([]QuestionCluster, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]QuestionCluster, 0, len(r.clusters))
	for _, cluster := range r.clusters {
		if filter.TenantID != "" && cluster.TenantID != filter.TenantID {
			continue
		}
		if filter.KBID != "" && cluster.KBID != filter.KBID {
			continue
		}
		if filter.RunID != "" && cluster.RunID != filter.RunID {
			continue
		}
		if filter.QuestionHash != "" && cluster.QuestionHash != filter.QuestionHash {
			continue
		}
		out = append(out, copyCluster(cluster))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return limitClusters(out, filter.Limit), nil
}

func (r *MemoryRepository) CreateOptimizationItem(_ context.Context, item OptimizationItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.ID] = copyItem(item)
	return nil
}

func (r *MemoryRepository) GetOptimizationItem(_ context.Context, tenantID, itemID string) (OptimizationItem, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[itemID]
	if !ok || item.TenantID != tenantID {
		return OptimizationItem{}, false, nil
	}
	return copyItem(item), true, nil
}

func (r *MemoryRepository) ListOptimizationItems(_ context.Context, filter OptimizationItemFilter) ([]OptimizationItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]OptimizationItem, 0, len(r.items))
	for _, item := range r.items {
		if filter.TenantID != "" && item.TenantID != filter.TenantID {
			continue
		}
		if filter.KBID != "" && item.KBID != filter.KBID {
			continue
		}
		if filter.RunID != "" && item.RunID != filter.RunID {
			continue
		}
		if filter.QuestionClusterID != "" && item.QuestionClusterID != filter.QuestionClusterID {
			continue
		}
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		if filter.ItemType != "" && item.ItemType != filter.ItemType {
			continue
		}
		out = append(out, copyItem(item))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return limitItems(out, filter.Limit), nil
}

func (r *MemoryRepository) UpdateOptimizationItem(_ context.Context, item OptimizationItem) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.items[item.ID]
	if !ok || current.TenantID != item.TenantID {
		return false, nil
	}
	r.items[item.ID] = copyItem(item)
	return true, nil
}

func (r *MemoryRepository) UpdateOptimizationItemStatus(_ context.Context, tenantID, itemID string, status ItemStatus, updatedAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[itemID]
	if !ok || item.TenantID != tenantID {
		return false, nil
	}
	item.Status = status
	item.UpdatedAt = updatedAt
	r.items[itemID] = copyItem(item)
	return true, nil
}

func (r *MemoryRepository) AppendItemEvent(_ context.Context, event OptimizationItemEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, copyItemEvent(event))
	return nil
}

func (r *MemoryRepository) ListItemEvents(_ context.Context, filter OptimizationItemEventFilter) ([]OptimizationItemEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]OptimizationItemEvent, 0, len(r.events))
	for _, event := range r.events {
		if filter.TenantID != "" && event.TenantID != filter.TenantID {
			continue
		}
		if filter.ItemID != "" && event.ItemID != filter.ItemID {
			continue
		}
		if filter.EventType != "" && event.EventType != filter.EventType {
			continue
		}
		if !filter.Since.IsZero() && event.CreatedAt.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && !event.CreatedAt.Before(filter.Until) {
			continue
		}
		out = append(out, copyItemEvent(event))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return limitItemEvents(out, filter.Limit), nil
}

func (r *MemoryRepository) RecordShadowEvent(_ context.Context, event ShadowRetrievalEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shadow = append(r.shadow, event)
	return nil
}

func (r *MemoryRepository) ListShadowEvents(_ context.Context, filter ShadowRetrievalEventFilter) ([]ShadowRetrievalEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ShadowRetrievalEvent, 0, len(r.shadow))
	for _, event := range r.shadow {
		if filter.TenantID != "" && event.TenantID != filter.TenantID {
			continue
		}
		if filter.KBID != "" && event.KBID != filter.KBID {
			continue
		}
		if filter.ItemID != "" && event.ItemID != filter.ItemID {
			continue
		}
		if filter.TraceID != "" && event.TraceID != filter.TraceID {
			continue
		}
		if !filter.Since.IsZero() && event.CreatedAt.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && !event.CreatedAt.Before(filter.Until) {
			continue
		}
		out = append(out, event)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return limitShadowEvents(out, filter.Limit), nil
}

func (r *MemoryRepository) RecordCodexToolAudit(_ context.Context, event CodexToolAuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audit = append(r.audit, event)
	return nil
}

func (r *MemoryRepository) ListCodexToolAuditEvents(_ context.Context, filter CodexToolAuditFilter) ([]CodexToolAuditEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]CodexToolAuditEvent, 0, len(r.audit))
	for _, event := range r.audit {
		if filter.TenantID != "" && event.TenantID != filter.TenantID {
			continue
		}
		if filter.KBID != "" && event.KBID != filter.KBID {
			continue
		}
		if filter.SessionID != "" && event.SessionID != filter.SessionID {
			continue
		}
		if filter.Tool != "" && event.Tool != filter.Tool {
			continue
		}
		if !filter.Since.IsZero() && event.StartedAt.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && !event.StartedAt.Before(filter.Until) {
			continue
		}
		out = append(out, event)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return limitCodexToolAuditEvents(out, filter.Limit), nil
}

func copyRun(run OfflineKnowledgeRun) OfflineKnowledgeRun {
	run.ConfigJSON = copyMap(run.ConfigJSON)
	return run
}

func copyCluster(cluster QuestionCluster) QuestionCluster {
	cluster.EmbeddingJSON = append([]float64(nil), cluster.EmbeddingJSON...)
	cluster.SampleQuestions = append([]string(nil), cluster.SampleQuestions...)
	cluster.TraceIDs = append([]string(nil), cluster.TraceIDs...)
	return cluster
}

func copyItem(item OptimizationItem) OptimizationItem {
	item.SourceFingerprints = append([]SourceFingerprint(nil), item.SourceFingerprints...)
	item.Evidence = append([]Evidence(nil), item.Evidence...)
	item.DeepSearchSteps = append([]DeepSearchStep(nil), item.DeepSearchSteps...)
	item.EvalReportJSON = append([]byte(nil), item.EvalReportJSON...)
	return item
}

func copyItemEvent(event OptimizationItemEvent) OptimizationItemEvent {
	event.Payload = copyMap(event.Payload)
	return event
}

func copyNegativeFeedback(item NegativeFeedback) NegativeFeedback {
	return item
}

func copyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func limitRuns(in []OfflineKnowledgeRun, limit int) []OfflineKnowledgeRun {
	if limit > 0 && len(in) > limit {
		return in[:limit]
	}
	return in
}

func limitClusters(in []QuestionCluster, limit int) []QuestionCluster {
	if limit > 0 && len(in) > limit {
		return in[:limit]
	}
	return in
}

func limitItems(in []OptimizationItem, limit int) []OptimizationItem {
	if limit > 0 && len(in) > limit {
		return in[:limit]
	}
	return in
}

func limitItemEvents(in []OptimizationItemEvent, limit int) []OptimizationItemEvent {
	if limit > 0 && len(in) > limit {
		return in[:limit]
	}
	return in
}

func limitShadowEvents(in []ShadowRetrievalEvent, limit int) []ShadowRetrievalEvent {
	if limit > 0 && len(in) > limit {
		return in[:limit]
	}
	return in
}

func limitNegativeFeedback(in []NegativeFeedback, limit int) []NegativeFeedback {
	if limit > 0 && len(in) > limit {
		return in[:limit]
	}
	return in
}

func limitCodexToolAuditEvents(in []CodexToolAuditEvent, limit int) []CodexToolAuditEvent {
	if limit > 0 && len(in) > limit {
		return in[:limit]
	}
	return in
}
