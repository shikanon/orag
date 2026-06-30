package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/ingest/chunker"
	"github.com/shikanon/orag/internal/ingest/parser"
	"github.com/shikanon/orag/internal/kb"
)

type fakeEmbedder struct{}

func (fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float64, error) {
	out := make([][]float64, len(texts))
	for i := range texts {
		out[i] = []float64{1, 0, 0, 0}
	}
	return out, nil
}

func TestIngestCreatesJobAndStableIDs(t *testing.T) {
	ctx := context.Background()
	store := kb.NewMemoryStore()
	store.PutKnowledgeBase(kb.KnowledgeBase{
		ID:        "kb_default",
		TenantID:  "tenant_default",
		Name:      "Default",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	jobs := NewMemoryJobStore()
	svc := &Service{
		Parser:         parser.BasicParser{},
		Splitter:       chunker.Recursive{SizeTokens: 20, OverlapTokens: 0},
		Embedder:       fakeEmbedder{},
		KnowledgeBases: store,
		Indexer:        store,
		Jobs:           jobs,
	}
	req := Request{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		SourceURI:       "memory://doc.md",
		Name:            "doc.md",
		Content:         []byte("# Title\n\nqdrant vector search"),
	}
	first, err := svc.Ingest(ctx, req)
	if err != nil {
		t.Fatalf("first Ingest() error = %v", err)
	}
	second, err := svc.Ingest(ctx, req)
	if err != nil {
		t.Fatalf("second Ingest() error = %v", err)
	}
	if first.Document.ID != second.Document.ID {
		t.Fatalf("document ids differ: %q vs %q", first.Document.ID, second.Document.ID)
	}
	if len(first.Chunks) != 1 || len(second.Chunks) != 1 || first.Chunks[0].ID != second.Chunks[0].ID {
		t.Fatalf("chunk ids are not stable: %#v %#v", first.Chunks, second.Chunks)
	}
	if got := store.Chunks("tenant_default", "kb_default"); len(got) != 1 {
		t.Fatalf("memory chunks len = %d", len(got))
	}
	if first.Job.Status != JobStatusSucceeded || first.Job.ChunkCount != 1 {
		t.Fatalf("job = %#v", first.Job)
	}
	if _, ok, err := jobs.GetJob(ctx, "tenant_default", first.Job.ID); err != nil || !ok {
		t.Fatalf("job lookup ok=%v err=%v", ok, err)
	}
}

func TestIngestRejectsMissingKnowledgeBaseBeforeCreatingJob(t *testing.T) {
	ctx := context.Background()
	store := kb.NewMemoryStore()
	jobs := NewMemoryJobStore()
	svc := &Service{
		Parser:         parser.BasicParser{},
		Splitter:       chunker.Recursive{SizeTokens: 20, OverlapTokens: 0},
		Embedder:       fakeEmbedder{},
		KnowledgeBases: store,
		Indexer:        store,
		Jobs:           jobs,
	}

	res, err := svc.Ingest(ctx, Request{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_missing",
		SourceURI:       "memory://missing.md",
		Name:            "missing.md",
		Content:         []byte("orphan chunks must not be created"),
	})
	if !errors.Is(err, ErrKnowledgeBaseNotFound) {
		t.Fatalf("Ingest() error = %v, want ErrKnowledgeBaseNotFound", err)
	}
	if res.Job.ID != "" {
		t.Fatalf("unexpected job result: %#v", res.Job)
	}
	if len(jobs.jobs) != 0 {
		t.Fatalf("jobs created for missing knowledge base: %#v", jobs.jobs)
	}
	if got := store.Chunks("tenant_default", "kb_missing"); len(got) != 0 {
		t.Fatalf("chunks created for missing knowledge base: %#v", got)
	}
}

func TestIngestRejectsOversizedDocumentAndMarksJobFailed(t *testing.T) {
	ctx := context.Background()
	jobs := NewMemoryJobStore()
	svc := &Service{
		Parser:           parser.BasicParser{},
		Splitter:         chunker.Recursive{SizeTokens: 20},
		Embedder:         fakeEmbedder{},
		Indexer:          kb.NewMemoryStore(),
		Jobs:             jobs,
		MaxDocumentBytes: 4,
	}
	res, err := svc.Ingest(ctx, Request{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		SourceURI:       "memory://big.md",
		Name:            "big.md",
		Content:         []byte("too large"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if res.Job.Status != JobStatusFailed {
		t.Fatalf("job status = %q", res.Job.Status)
	}
}

func TestIngestMarksJobFailedOnParseError(t *testing.T) {
	ctx := context.Background()
	svc := &Service{
		Parser:   parser.BasicParser{},
		Splitter: chunker.Recursive{SizeTokens: 20},
		Embedder: fakeEmbedder{},
		Indexer:  kb.NewMemoryStore(),
		Jobs:     NewMemoryJobStore(),
	}
	res, err := svc.Ingest(ctx, Request{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		SourceURI:       "memory://empty.md",
		Name:            "empty.md",
	})
	if err == nil {
		t.Fatal("expected parse error")
	}
	if res.Job.Status != JobStatusFailed {
		t.Fatalf("job status = %q", res.Job.Status)
	}
}
