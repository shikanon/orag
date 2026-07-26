package taskqueue

import (
	"context"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/audit"
	"github.com/shikanon/orag/internal/platform/apperrors"
	"github.com/shikanon/orag/internal/platform/id"
)

// Service is the tenant-aware control-plane facade for a QueueRepository.
// Workers use the repository directly; external callers must use this type so
// task IDs cannot cross tenant boundaries.
type Service struct {
	repo  QueueRepository
	audit interface {
		Record(context.Context, audit.AuditEvent) error
	}
	now func() time.Time
}

func NewService(repo QueueRepository, auditService interface {
	Record(context.Context, audit.AuditEvent) error
}) *Service {
	return &Service{repo: repo, audit: auditService, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Enqueue(ctx context.Context, tenantID string, task Task) (Task, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(task.Type) == "" || strings.TrimSpace(task.Pool) == "" {
		return Task{}, apperrors.New(apperrors.CodeValidation, "tenant_id, type, and pool are required")
	}
	if task.TenantID != "" && task.TenantID != tenantID {
		return Task{}, apperrors.New(apperrors.CodeForbidden, "task tenant does not match caller tenant")
	}
	now := s.now()
	task.TenantID = tenantID
	if task.ID == "" {
		task.ID = id.New("task")
	}
	if task.MaxAttempts <= 0 {
		task.MaxAttempts = 3
	}
	if task.RunAfter.IsZero() {
		task.RunAfter = now
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	task.UpdatedAt = now
	created, err := s.repo.Enqueue(ctx, task)
	if err != nil {
		return Task{}, err
	}
	return created, nil
}

func (s *Service) Get(ctx context.Context, tenantID, taskID string) (Task, bool, error) {
	task, found, err := s.repo.Get(ctx, taskID)
	if err != nil || !found || task.TenantID != tenantID {
		return Task{}, false, err
	}
	return task, true, nil
}

func (s *Service) List(ctx context.Context, tenantID string, filter TaskFilter) ([]Task, string, error) {
	filter.TenantID = tenantID
	return s.repo.List(ctx, filter)
}

func (s *Service) Cancel(ctx context.Context, tenantID, taskID, actorType, actorID, traceID string) error {
	task, found, err := s.Get(ctx, tenantID, taskID)
	if err != nil {
		return err
	}
	if !found {
		return apperrors.New(apperrors.CodeNotFound, "task not found")
	}
	if err := s.repo.Cancel(ctx, task.ID); err != nil {
		return err
	}
	s.record(ctx, audit.ActionTaskCancelled, task, actorType, actorID, traceID)
	return nil
}

func (s *Service) Retry(ctx context.Context, tenantID, taskID, actorType, actorID, traceID string) (Task, error) {
	task, found, err := s.Get(ctx, tenantID, taskID)
	if err != nil {
		return Task{}, err
	}
	if !found {
		return Task{}, apperrors.New(apperrors.CodeNotFound, "task not found")
	}
	retried, err := s.repo.Retry(ctx, task.ID)
	if err != nil {
		return Task{}, err
	}
	s.record(ctx, audit.ActionTaskRetried, retried, actorType, actorID, traceID)
	return retried, nil
}

func (s *Service) ListEvents(ctx context.Context, tenantID, taskID, cursor string, limit int) ([]Event, string, error) {
	if _, found, err := s.Get(ctx, tenantID, taskID); err != nil || !found {
		if err != nil {
			return nil, "", err
		}
		return nil, "", apperrors.New(apperrors.CodeNotFound, "task not found")
	}
	return s.repo.ListEvents(ctx, taskID, cursor, limit)
}

func (s *Service) Stats(ctx context.Context, tenantID string) (map[string]PoolStats, error) {
	return s.repo.Stats(ctx, tenantID)
}

func (s *Service) record(ctx context.Context, action string, task Task, actorType, actorID, traceID string) {
	if s.audit == nil {
		return
	}
	if traceID == "" {
		traceID = task.TraceID
	}
	_ = s.audit.Record(ctx, audit.AuditEvent{
		ID: id.New("audit"), TenantID: task.TenantID, ProjectID: task.ProjectID,
		ActorType: actorType, ActorID: actorID, Action: action,
		ResourceType: audit.ResourceTypeTask, ResourceID: task.ID,
		Outcome: audit.OutcomeSuccess, TraceID: traceID, TaskID: task.ID,
		CreatedAt: s.now(),
	})
}
