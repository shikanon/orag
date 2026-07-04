package ingest

import (
	"context"
	"errors"
	"sort"
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

type failingIndexer struct {
	err error
}

func (i failingIndexer) Store(context.Context, kb.Document, []kb.Chunk) error {
	return i.err
}

type noopIndexer struct{}

func (noopIndexer) Store(context.Context, kb.Document, []kb.Chunk) error {
	return nil
}

type stagedSearchStore struct {
	pending map[string]kb.Chunk
	active  map[string]kb.Chunk
}

func newStagedSearchStore() *stagedSearchStore {
	return &stagedSearchStore{
		pending: map[string]kb.Chunk{},
		active:  map[string]kb.Chunk{},
	}
}

func (s *stagedSearchStore) Store(_ context.Context, _ kb.Document, chunks []kb.Chunk) error {
	for _, chunk := range chunks {
		s.pending[chunk.ID] = chunk
	}
	return nil
}

func (s *stagedSearchStore) Activate(_ context.Context, _ kb.Document, chunks []kb.Chunk) error {
	for _, chunk := range chunks {
		if staged, ok := s.pending[chunk.ID]; ok {
			s.active[chunk.ID] = staged
			delete(s.pending, chunk.ID)
		}
	}
	return nil
}

func (s *stagedSearchStore) Chunks(tenantID, kbID string) []kb.Chunk {
	out := make([]kb.Chunk, 0, len(s.active))
	for _, chunk := range s.active {
		if chunk.TenantID == tenantID && chunk.KnowledgeBaseID == kbID {
			out = append(out, chunk)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func TestIngestCreatesJobAndStableIDs(t *testing.T) {
	ctx := context.Background()
	store := kb.NewMemoryStore()
	if err := store.PutKnowledgeBase(ctx, kb.KnowledgeBase{
		ID:        "kb_default",
		TenantID:  "tenant_default",
		Name:      "Default",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
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

func TestIngestReplacesOldChunksForSameSource(t *testing.T) {
	ctx := context.Background()
	store := kb.NewMemoryStore()
	if err := store.PutKnowledgeBase(ctx, kb.KnowledgeBase{
		ID:        "kb_default",
		TenantID:  "tenant_default",
		Name:      "Default",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		Parser:         parser.BasicParser{},
		Splitter:       chunker.Recursive{SizeTokens: 20, OverlapTokens: 0},
		Embedder:       fakeEmbedder{},
		KnowledgeBases: store,
		Indexer:        store,
		Jobs:           NewMemoryJobStore(),
	}
	req := Request{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		SourceURI:       "memory://replace.md",
		Name:            "replace.md",
		Content:         []byte("old replacement marker"),
	}
	first, err := svc.Ingest(ctx, req)
	if err != nil {
		t.Fatalf("first Ingest() error = %v", err)
	}
	req.Content = []byte("new replacement marker")
	second, err := svc.Ingest(ctx, req)
	if err != nil {
		t.Fatalf("second Ingest() error = %v", err)
	}
	if first.Document.ID == second.Document.ID {
		t.Fatalf("document IDs should differ after content change: %s", first.Document.ID)
	}
	got := store.Chunks("tenant_default", "kb_default")
	if len(got) != 1 || got[0].DocumentID != second.Document.ID || got[0].Content != "new replacement marker" {
		t.Fatalf("chunks after re-ingest = %#v", got)
	}
}

func TestIngestFailedCompositeIndexDoesNotExposeSparseChunks(t *testing.T) {
	ctx := context.Background()
	store := newStagedSearchStore()
	jobs := NewMemoryJobStore()
	svc := &Service{
		Parser:   parser.BasicParser{},
		Splitter: chunker.Recursive{SizeTokens: 20, OverlapTokens: 0},
		Embedder: fakeEmbedder{},
		Indexer: kb.CompositeIndexer{Indexers: []kb.Indexer{
			store,
			failingIndexer{err: errors.New("qdrant upsert failed")},
		}},
		Jobs: jobs,
	}

	res, err := svc.Ingest(ctx, Request{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		SourceURI:       "memory://failed.md",
		Name:            "failed.md",
		Content:         []byte("failed ingestion hidden marker"),
	})
	if err == nil {
		t.Fatal("expected indexer error")
	}
	if res.Job.Status != JobStatusFailed {
		t.Fatalf("job status = %q", res.Job.Status)
	}
	updated, ok, err := jobs.GetJob(ctx, "tenant_default", res.Job.ID)
	if err != nil || !ok {
		t.Fatalf("job lookup ok=%v err=%v", ok, err)
	}
	if updated.Status != JobStatusFailed {
		t.Fatalf("stored job status = %q", updated.Status)
	}

	results, err := (kb.SparseRetriever{Store: store}).Retrieve(ctx, kb.SearchRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "hidden marker",
		TopK:            8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("failed ingestion is searchable: %#v", results)
	}
}

func TestIngestSuccessfulCompositeIndexExposesSparseChunks(t *testing.T) {
	ctx := context.Background()
	store := newStagedSearchStore()
	svc := &Service{
		Parser:   parser.BasicParser{},
		Splitter: chunker.Recursive{SizeTokens: 20, OverlapTokens: 0},
		Embedder: fakeEmbedder{},
		Indexer: kb.CompositeIndexer{Indexers: []kb.Indexer{
			store,
			noopIndexer{},
		}},
		Jobs: NewMemoryJobStore(),
	}

	res, err := svc.Ingest(ctx, Request{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		SourceURI:       "memory://success.md",
		Name:            "success.md",
		Content:         []byte("successful ingestion visible marker"),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if res.Job.Status != JobStatusSucceeded {
		t.Fatalf("job status = %q", res.Job.Status)
	}

	results, err := (kb.SparseRetriever{Store: store}).Retrieve(ctx, kb.SearchRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "visible marker",
		TopK:            8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].Chunk.DocumentID != res.Document.ID {
		t.Fatalf("successful ingestion is not searchable: %#v", results)
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
