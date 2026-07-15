package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrNotFound          = errors.New("pipeline not found")
	ErrRevisionConflict  = errors.New("pipeline draft revision conflict")
	ErrInvalidDefinition = errors.New("invalid pipeline definition")
)

type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	return fmt.Sprintf("pipeline definition has %d validation errors", len(e))
}
func (e ValidationErrors) Unwrap() error { return ErrInvalidDefinition }

type Pipeline struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Draft struct {
	PipelineID    string     `json:"pipeline_id"`
	ProjectID     string     `json:"project_id"`
	Revision      int64      `json:"revision"`
	SchemaVersion int        `json:"schema_version"`
	Definition    Definition `json:"definition"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type Repository interface {
	CreatePipeline(context.Context, Pipeline) error
	ListPipelines(context.Context, string) ([]Pipeline, error)
	GetDraft(context.Context, string, string) (Draft, error)
	SaveDraft(context.Context, string, string, int64, Draft) (Draft, error)
}

type Service struct {
	repo     Repository
	registry Registry
}

func NewService(repo Repository, registry Registry) *Service {
	return &Service{repo: repo, registry: registry}
}
func (s *Service) CreatePipeline(ctx context.Context, item Pipeline) error {
	if item.ID == "" || item.ProjectID == "" || item.Name == "" {
		return fmt.Errorf("%w: id, project, and name are required", ErrInvalidDefinition)
	}
	return s.repo.CreatePipeline(ctx, item)
}
func (s *Service) ListPipelines(ctx context.Context, projectID string) ([]Pipeline, error) {
	return s.repo.ListPipelines(ctx, projectID)
}
func (s *Service) GetDraft(ctx context.Context, projectID, pipelineID string) (Draft, error) {
	return s.repo.GetDraft(ctx, projectID, pipelineID)
}

// ValidateDefinition returns node-addressable validation diagnostics without
// mutating the draft repository. Callers use this before operations that
// materialize an immutable version from a draft.
func (s *Service) ValidateDefinition(definition Definition) ValidationErrors {
	return ValidationErrors(Validate(definition, s.registry))
}
func (s *Service) SaveDraft(ctx context.Context, projectID, pipelineID string, expectedRevision int64, definition Definition) (Draft, error) {
	validation := Validate(definition, s.registry)
	if len(validation) > 0 {
		return Draft{}, ValidationErrors(validation)
	}
	draft := Draft{PipelineID: pipelineID, ProjectID: projectID, Revision: expectedRevision + 1, SchemaVersion: 1, Definition: definition, UpdatedAt: time.Now().UTC()}
	return s.repo.SaveDraft(ctx, projectID, pipelineID, expectedRevision, draft)
}

type MemoryRepository struct {
	mu        sync.RWMutex
	pipelines map[string]Pipeline
	drafts    map[string]Draft
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{pipelines: map[string]Pipeline{}, drafts: map[string]Draft{}}
}
func (r *MemoryRepository) CreatePipeline(_ context.Context, item Pipeline) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.pipelines[item.ID]; ok {
		return ErrRevisionConflict
	}
	r.pipelines[item.ID] = item
	r.drafts[item.ID] = Draft{PipelineID: item.ID, ProjectID: item.ProjectID, Revision: 0, SchemaVersion: 1, Definition: Definition{}}
	return nil
}
func (r *MemoryRepository) ListPipelines(_ context.Context, projectID string) ([]Pipeline, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []Pipeline{}
	for _, item := range r.pipelines {
		if item.ProjectID == projectID {
			out = append(out, item)
		}
	}
	return out, nil
}
func (r *MemoryRepository) GetDraft(_ context.Context, projectID, pipelineID string) (Draft, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.drafts[pipelineID]
	if !ok || item.ProjectID != projectID {
		return Draft{}, ErrNotFound
	}
	return cloneDraft(item), nil
}
func (r *MemoryRepository) SaveDraft(_ context.Context, projectID, pipelineID string, expected int64, draft Draft) (Draft, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.drafts[pipelineID]
	if !ok || current.ProjectID != projectID {
		return Draft{}, ErrNotFound
	}
	if current.Revision != expected {
		return current, ErrRevisionConflict
	}
	r.drafts[pipelineID] = cloneDraft(draft)
	return cloneDraft(draft), nil
}
func cloneDraft(item Draft) Draft {
	body, _ := json.Marshal(item.Definition)
	_ = json.Unmarshal(body, &item.Definition)
	return item
}
