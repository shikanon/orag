package ingest

import (
	"context"
	"sync"
	"time"
)

type JobStatus string

const (
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
)

type Job struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	KnowledgeBaseID string    `json:"knowledge_base_id"`
	Status          JobStatus `json:"status"`
	SourceURI       string    `json:"source_uri"`
	DocumentID      string    `json:"document_id,omitempty"`
	ChunkCount      int       `json:"chunk_count"`
	Error           string    `json:"error,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type JobStore interface {
	CreateJob(ctx context.Context, job Job) (Job, error)
	UpdateJob(ctx context.Context, job Job) error
	GetJob(ctx context.Context, tenantID, id string) (Job, bool, error)
}

type MemoryJobStore struct {
	mu   sync.RWMutex
	jobs map[string]Job
}

func NewMemoryJobStore() *MemoryJobStore {
	return &MemoryJobStore{jobs: map[string]Job{}}
}

func (s *MemoryJobStore) CreateJob(_ context.Context, job Job) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	return job, nil
}

func (s *MemoryJobStore) UpdateJob(_ context.Context, job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	return nil
}

func (s *MemoryJobStore) GetJob(_ context.Context, tenantID, id string) (Job, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok || job.TenantID != tenantID {
		return Job{}, false, nil
	}
	return job, true, nil
}
