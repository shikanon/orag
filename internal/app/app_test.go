package app

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/config"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/project"
	"github.com/shikanon/orag/internal/storage/postgres"
	qdrantstore "github.com/shikanon/orag/internal/storage/qdrant"
)

func TestBuildKnowledgeBackendWiresVectorVisibility(t *testing.T) {
	client := &qdrantstore.Client{}
	repo := &postgres.Repository{}
	got := newPostgresVectorStore(client, "chunks", repo)
	if got.Client != client {
		t.Fatal("client was not preserved")
	}
	if got.Collection != "chunks" {
		t.Fatalf("collection = %q", got.Collection)
	}
	if got.Visibility != repo {
		t.Fatal("postgres visibility filter was not wired")
	}
}

func TestNewWiresProjectServiceForMemoryBackend(t *testing.T) {
	t.Setenv("STORAGE_BACKEND", "memory")
	t.Setenv("ALLOW_DETERMINISTIC_MOCK", "true")
	t.Setenv("LLM_CHAT_PROVIDER", "mock")
	t.Setenv("LLM_EMBEDDING_PROVIDER", "mock")
	t.Setenv("LLM_RERANK_PROVIDER", "mock")
	t.Setenv("LLM_MULTIMODAL_PROVIDER", "mock")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(context.Background(), cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if app.Projects == nil {
		t.Fatal("Projects service is nil")
	}
	if _, err := app.Projects.Create(context.Background(), "tenant_a", project.CreateInput{Name: "Console"}); err != nil {
		t.Fatal(err)
	}
}

func TestNewWiresTutorialCatalogForMemoryBackend(t *testing.T) {
	t.Setenv("STORAGE_BACKEND", "memory")
	t.Setenv("ALLOW_DETERMINISTIC_MOCK", "true")
	t.Setenv("LLM_CHAT_PROVIDER", "mock")
	t.Setenv("LLM_EMBEDDING_PROVIDER", "mock")
	t.Setenv("LLM_RERANK_PROVIDER", "mock")
	t.Setenv("LLM_MULTIMODAL_PROVIDER", "mock")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	application, err := New(context.Background(), cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	if application.Tutorials == nil {
		t.Fatal("Tutorials catalog is nil")
	}
	if got := len(application.Tutorials.List()); got != 3 {
		t.Fatalf("Tutorials.List() len = %d, want 3", got)
	}
}

func TestKnowledgeBaseStoreDeleteKnowledgeBaseDeletesMetadataBeforeCleanup(t *testing.T) {
	calls := []string{}
	repo := newFakeKnowledgeBaseRepo(&calls, kb.KnowledgeBase{
		ID:        "kb_1",
		TenantID:  "tenant_1",
		Name:      "KB",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	store := knowledgeBaseStore{
		primary:              repo,
		vectorDeleter:        recordingVectorDeleter{name: "chunks", calls: &calls},
		semanticCacheDeleter: recordingSemanticCacheDeleter{name: "semantic_cache", calls: &calls},
	}

	deleted, err := store.DeleteKnowledgeBase(context.Background(), "tenant_1", "kb_1")
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("DeleteKnowledgeBase() deleted = false, want true")
	}
	wantCalls := []string{"metadata:tenant_1/kb_1", "chunks:tenant_1/kb_1", "semantic_cache:tenant_1/kb_1"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
	if _, ok, err := repo.GetKnowledgeBase(context.Background(), "tenant_1", "kb_1"); err != nil || ok {
		t.Fatal("knowledge base metadata still exists")
	}
}

func TestKnowledgeBaseStoreDeleteKnowledgeBaseSkipsMissingKBAndStopsOnVectorError(t *testing.T) {
	calls := []string{}
	repo := newFakeKnowledgeBaseRepo(&calls, kb.KnowledgeBase{
		ID:        "kb_1",
		TenantID:  "tenant_1",
		Name:      "KB",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	store := knowledgeBaseStore{
		primary:              repo,
		vectorDeleter:        recordingVectorDeleter{name: "chunks", calls: &calls, err: errors.New("qdrant delete failed")},
		semanticCacheDeleter: recordingSemanticCacheDeleter{name: "semantic_cache", calls: &calls},
	}

	deleted, err := store.DeleteKnowledgeBase(context.Background(), "tenant_other", "kb_1")
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("wrong tenant delete unexpectedly succeeded")
	}
	if len(calls) != 0 {
		t.Fatalf("calls for missing KB = %#v, want none", calls)
	}

	deleted, err = store.DeleteKnowledgeBase(context.Background(), "tenant_1", "kb_1")
	if err == nil {
		t.Fatal("DeleteKnowledgeBase() error = nil, want qdrant delete error")
	}
	if !deleted {
		t.Fatal("DeleteKnowledgeBase() deleted = false after metadata delete and point delete error")
	}
	wantCalls := []string{"metadata:tenant_1/kb_1", "chunks:tenant_1/kb_1"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
	if _, ok, err := repo.GetKnowledgeBase(context.Background(), "tenant_1", "kb_1"); err != nil || ok {
		t.Fatal("metadata still exists after point delete error")
	}
}

func TestKnowledgeBaseStoreDoesNotDeleteExternalIndexesWhenMetadataDeleteFails(t *testing.T) {
	metadataErr := errors.New("metadata delete failed")
	calls := []string{}
	repo := newFakeKnowledgeBaseRepo(&calls, kb.KnowledgeBase{
		ID:        "kb_1",
		TenantID:  "tenant_1",
		Name:      "KB",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	repo.deleteErr = metadataErr
	store := knowledgeBaseStore{
		primary:              repo,
		vectorDeleter:        recordingVectorDeleter{name: "chunks", calls: &calls},
		semanticCacheDeleter: recordingSemanticCacheDeleter{name: "semantic_cache", calls: &calls},
	}

	deleted, err := store.DeleteKnowledgeBase(context.Background(), "tenant_1", "kb_1")
	if !errors.Is(err, metadataErr) {
		t.Fatalf("DeleteKnowledgeBase() error = %v, want %v", err, metadataErr)
	}
	if deleted {
		t.Fatal("DeleteKnowledgeBase() deleted = true, want false")
	}
	if _, ok, err := repo.GetKnowledgeBase(context.Background(), "tenant_1", "kb_1"); err != nil || !ok {
		t.Fatal("metadata was deleted after metadata delete failed")
	}
	wantCalls := []string{"metadata:tenant_1/kb_1"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
}

func TestKnowledgeBaseStoreDeleteKnowledgeBaseStopsWhenMetadataDeleteFails(t *testing.T) {
	metadataErr := errors.New("metadata delete failed")
	tests := []struct {
		name    string
		deleted bool
		err     error
	}{
		{name: "delete false", deleted: false},
		{name: "delete error", deleted: false, err: metadataErr},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := []string{}
			repo := newFakeKnowledgeBaseRepo(&calls, kb.KnowledgeBase{
				ID:        "kb_1",
				TenantID:  "tenant_1",
				Name:      "KB",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			})
			repo.deleteDeleted = &tt.deleted
			repo.deleteErr = tt.err
			store := knowledgeBaseStore{
				primary:              repo,
				vectorDeleter:        recordingVectorDeleter{name: "chunks", calls: &calls},
				semanticCacheDeleter: recordingSemanticCacheDeleter{name: "semantic_cache", calls: &calls},
			}

			deleted, err := store.DeleteKnowledgeBase(context.Background(), "tenant_1", "kb_1")
			if !errors.Is(err, tt.err) {
				t.Fatalf("DeleteKnowledgeBase() error = %v, want %v", err, tt.err)
			}
			if deleted != tt.deleted {
				t.Fatalf("DeleteKnowledgeBase() deleted = %v, want %v", deleted, tt.deleted)
			}
			wantCalls := []string{"metadata:tenant_1/kb_1"}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
			}
			if _, ok, err := repo.GetKnowledgeBase(context.Background(), "tenant_1", "kb_1"); err != nil || !ok {
				t.Fatal("metadata was deleted after metadata delete failed")
			}
		})
	}
}

func TestKnowledgeBaseStoreDeleteKnowledgeBaseStopsOnSemanticCacheError(t *testing.T) {
	calls := []string{}
	repo := newFakeKnowledgeBaseRepo(&calls, kb.KnowledgeBase{
		ID:        "kb_1",
		TenantID:  "tenant_1",
		Name:      "KB",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	store := knowledgeBaseStore{
		primary:              repo,
		vectorDeleter:        recordingVectorDeleter{name: "chunks", calls: &calls},
		semanticCacheDeleter: recordingSemanticCacheDeleter{name: "semantic_cache", calls: &calls, err: errors.New("semantic cache delete failed")},
	}

	deleted, err := store.DeleteKnowledgeBase(context.Background(), "tenant_1", "kb_1")
	if err == nil {
		t.Fatal("DeleteKnowledgeBase() error = nil, want semantic cache delete error")
	}
	if !deleted {
		t.Fatal("DeleteKnowledgeBase() deleted = false after metadata delete and semantic cache delete error")
	}
	wantCalls := []string{"metadata:tenant_1/kb_1", "chunks:tenant_1/kb_1", "semantic_cache:tenant_1/kb_1"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
	if _, ok, err := repo.GetKnowledgeBase(context.Background(), "tenant_1", "kb_1"); err != nil || ok {
		t.Fatal("metadata still exists after semantic cache delete error")
	}
}

type fakeKnowledgeBaseRepo struct {
	items         map[string]kb.KnowledgeBase
	calls         *[]string
	deleteDeleted *bool
	deleteErr     error
}

func newFakeKnowledgeBaseRepo(calls *[]string, items ...kb.KnowledgeBase) *fakeKnowledgeBaseRepo {
	repo := &fakeKnowledgeBaseRepo{items: map[string]kb.KnowledgeBase{}, calls: calls}
	for _, item := range items {
		repo.items[item.TenantID+"/"+item.ID] = item
	}
	return repo
}

func (r *fakeKnowledgeBaseRepo) PutKnowledgeBase(_ context.Context, item kb.KnowledgeBase) error {
	r.items[item.TenantID+"/"+item.ID] = item
	return nil
}

func (r *fakeKnowledgeBaseRepo) ListKnowledgeBases(_ context.Context, tenantID string) ([]kb.KnowledgeBase, error) {
	var out []kb.KnowledgeBase
	for _, item := range r.items {
		if item.TenantID == tenantID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *fakeKnowledgeBaseRepo) GetKnowledgeBase(_ context.Context, tenantID, id string) (kb.KnowledgeBase, bool, error) {
	item, ok := r.items[tenantID+"/"+id]
	return item, ok, nil
}

func (r *fakeKnowledgeBaseRepo) DeleteKnowledgeBase(_ context.Context, tenantID, id string) (bool, error) {
	if _, ok, err := r.GetKnowledgeBase(context.Background(), tenantID, id); err != nil || !ok {
		return false, nil
	}
	*r.calls = append(*r.calls, "metadata:"+tenantID+"/"+id)
	if r.deleteDeleted != nil || r.deleteErr != nil {
		if r.deleteDeleted == nil {
			return false, r.deleteErr
		}
		return *r.deleteDeleted, r.deleteErr
	}
	delete(r.items, tenantID+"/"+id)
	return true, nil
}

type recordingVectorDeleter struct {
	name  string
	calls *[]string
	err   error
}

func (d recordingVectorDeleter) DeleteKnowledgeBaseVectors(_ context.Context, tenantID, kbID string) error {
	*d.calls = append(*d.calls, d.name+":"+tenantID+"/"+kbID)
	return d.err
}

type recordingSemanticCacheDeleter struct {
	name  string
	calls *[]string
	err   error
}

func (d recordingSemanticCacheDeleter) DeleteKnowledgeBaseSemanticCache(_ context.Context, tenantID, kbID string) error {
	*d.calls = append(*d.calls, d.name+":"+tenantID+"/"+kbID)
	return d.err
}
