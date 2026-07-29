package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/ingest/chunker"
	"github.com/shikanon/orag/internal/ingest/parser"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/taskqueue"
)

func TestDocumentImportTaskHandlerProjectGuard(t *testing.T) {
	const (
		tenantID        = "tenant_1"
		knowledgeBaseID = "kb_1"
		projectID       = "prj_1"
	)

	t.Run("rejects project mismatch without side effects", func(t *testing.T) {
		handler, store, jobs, indexer := newDocumentImportTaskHandlerFixture(t, kb.KnowledgeBase{
			ID: knowledgeBaseID, TenantID: tenantID, ProjectID: projectID, Name: "Guarded KB",
		})
		err := handler.Handle(context.Background(), documentImportTask(
			t, tenantID, "prj_other", knowledgeBaseID,
		), nil)
		if err == nil {
			t.Fatal("Handle() error=nil, want project mismatch")
		}
		retryable, ok := err.(interface{ Retryable() bool })
		if !ok || retryable.Retryable() {
			t.Fatalf("Handle() error=%T %v, want non-retryable", err, err)
		}
		if jobs.createCalls != 0 {
			t.Fatalf("CreateJob calls=%d want=0", jobs.createCalls)
		}
		if indexer.storeCalls != 0 {
			t.Fatalf("Indexer Store calls=%d want=0", indexer.storeCalls)
		}
		if chunks := store.Chunks(tenantID, knowledgeBaseID); len(chunks) != 0 {
			t.Fatalf("chunks=%#v want none", chunks)
		}
	})

	t.Run("imports same project task", func(t *testing.T) {
		handler, store, jobs, indexer := newDocumentImportTaskHandlerFixture(t, kb.KnowledgeBase{
			ID: knowledgeBaseID, TenantID: tenantID, ProjectID: projectID, Name: "Guarded KB",
		})
		if err := handler.Handle(context.Background(), documentImportTask(
			t, tenantID, projectID, knowledgeBaseID,
		), nil); err != nil {
			t.Fatalf("Handle() error=%v", err)
		}
		if jobs.createCalls != 1 {
			t.Fatalf("CreateJob calls=%d want=1", jobs.createCalls)
		}
		if indexer.storeCalls != 1 {
			t.Fatalf("Indexer Store calls=%d want=1", indexer.storeCalls)
		}
		if chunks := store.Chunks(tenantID, knowledgeBaseID); len(chunks) == 0 {
			t.Fatal("same-project import stored no chunks")
		}
	})
}

func newDocumentImportTaskHandlerFixture(
	t *testing.T,
	knowledgeBase kb.KnowledgeBase,
) (documentImportTaskHandler, *kb.MemoryStore, *taskHandlerJobStore, *taskHandlerIndexer) {
	t.Helper()
	store := kb.NewMemoryStore()
	if err := store.PutKnowledgeBase(context.Background(), knowledgeBase); err != nil {
		t.Fatal(err)
	}
	jobs := &taskHandlerJobStore{delegate: ingest.NewMemoryJobStore()}
	indexer := &taskHandlerIndexer{delegate: store}
	service := &ingest.Service{
		Parser:         parser.BasicParser{},
		Splitter:       chunker.Recursive{SizeTokens: 100, OverlapTokens: 10},
		Embedder:       taskHandlerEmbedder{},
		KnowledgeBases: store,
		Indexer:        indexer,
		Jobs:           jobs,
	}
	return documentImportTaskHandler{ingest: service}, store, jobs, indexer
}

func documentImportTask(t *testing.T, tenantID, projectID, knowledgeBaseID string) taskqueue.Task {
	t.Helper()
	payload, err := json.Marshal(DocumentImportTaskPayload{
		KnowledgeBaseID: knowledgeBaseID,
		SourceURI:       "test://project-guard",
		Name:            "project-guard.md",
		ContentBase64:   base64.StdEncoding.EncodeToString([]byte("# Project guard\nValid same-project content.")),
	})
	if err != nil {
		t.Fatal(err)
	}
	return taskqueue.Task{
		ID:        "task_1",
		TenantID:  tenantID,
		ProjectID: projectID,
		Type:      DocumentImportTaskType,
		Pool:      IngestionCorePool,
		Payload:   payload,
	}
}

type taskHandlerEmbedder struct{}

func (taskHandlerEmbedder) Embed(_ context.Context, texts []string) ([][]float64, error) {
	vectors := make([][]float64, len(texts))
	for i := range texts {
		vectors[i] = []float64{1, 0, 0, 0}
	}
	return vectors, nil
}

type taskHandlerJobStore struct {
	delegate    ingest.JobStore
	createCalls int
}

func (s *taskHandlerJobStore) CreateJob(ctx context.Context, job ingest.Job) (ingest.Job, error) {
	s.createCalls++
	return s.delegate.CreateJob(ctx, job)
}

func (s *taskHandlerJobStore) UpdateJob(ctx context.Context, job ingest.Job) error {
	return s.delegate.UpdateJob(ctx, job)
}

func (s *taskHandlerJobStore) GetJob(ctx context.Context, tenantID, id string) (ingest.Job, bool, error) {
	return s.delegate.GetJob(ctx, tenantID, id)
}

type taskHandlerIndexer struct {
	delegate   kb.Indexer
	storeCalls int
}

func (i *taskHandlerIndexer) Store(ctx context.Context, document kb.Document, chunks []kb.Chunk) error {
	i.storeCalls++
	return i.delegate.Store(ctx, document, chunks)
}
