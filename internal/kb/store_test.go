package kb

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryStoreFailingIndexer struct {
	err error
}

func (i memoryStoreFailingIndexer) Store(context.Context, Document, []Chunk) error {
	return i.err
}

type memoryStoreNoopIndexer struct{}

func (memoryStoreNoopIndexer) Store(context.Context, Document, []Chunk) error {
	return nil
}

func TestMemoryStoreDeleteKnowledgeBaseIsTenantScopedAndCleansChunks(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	if err := store.PutKnowledgeBase(context.Background(), KnowledgeBase{
		ID:        "kb_delete",
		TenantID:  "tenant_owner",
		Name:      "Delete me",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Store(context.Background(), Document{
		ID:              "doc_delete",
		TenantID:        "tenant_owner",
		KnowledgeBaseID: "kb_delete",
		SourceURI:       "test://delete",
		Title:           "delete.md",
		ContentHash:     "hash_delete",
		CreatedAt:       now,
	}, []Chunk{{
		ID:              "chunk_delete",
		TenantID:        "tenant_owner",
		KnowledgeBaseID: "kb_delete",
		DocumentID:      "doc_delete",
		Content:         "delete cleanup marker",
		SourceURI:       "test://delete",
	}}); err != nil {
		t.Fatal(err)
	}

	deleted, err := store.DeleteKnowledgeBase(context.Background(), "tenant_other", "kb_delete")
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("DeleteKnowledgeBase() deleted a knowledge base for the wrong tenant")
	}
	if _, ok, err := store.GetKnowledgeBase(context.Background(), "tenant_owner", "kb_delete"); err != nil || !ok {
		t.Fatal("knowledge base was removed by wrong-tenant delete")
	}
	if got := store.Chunks("tenant_owner", "kb_delete"); len(got) != 1 {
		t.Fatalf("chunks after wrong-tenant delete = %#v", got)
	}

	deleted, err = store.DeleteKnowledgeBase(context.Background(), "tenant_owner", "kb_delete")
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("DeleteKnowledgeBase() deleted = false, want true")
	}
	if _, ok, err := store.GetKnowledgeBase(context.Background(), "tenant_owner", "kb_delete"); err != nil || ok {
		t.Fatal("deleted knowledge base is still visible")
	}
	if got := store.Chunks("tenant_owner", "kb_delete"); len(got) != 0 {
		t.Fatalf("chunks after delete = %#v", got)
	}
}

func TestMemoryStoreStoreReplacesChunksForSameSource(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	firstDoc := Document{
		ID:              "doc_old",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		SourceURI:       "test://same.md",
		Title:           "same.md",
		ContentHash:     "hash_old",
		CreatedAt:       now,
	}
	if err := store.Store(ctx, firstDoc, []Chunk{{
		ID:              "chunk_old",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      firstDoc.ID,
		Content:         "old content",
		SourceURI:       firstDoc.SourceURI,
	}}); err != nil {
		t.Fatal(err)
	}

	secondDoc := firstDoc
	secondDoc.ID = "doc_new"
	secondDoc.ContentHash = "hash_new"
	if err := store.Store(ctx, secondDoc, []Chunk{{
		ID:              "chunk_new",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      secondDoc.ID,
		Content:         "new content",
		SourceURI:       secondDoc.SourceURI,
	}}); err != nil {
		t.Fatal(err)
	}

	got := store.Chunks("tenant_1", "kb_1")
	if len(got) != 1 || got[0].ID != "chunk_new" || got[0].Content != "new content" {
		t.Fatalf("chunks after same-source replacement = %#v", got)
	}
}

func TestCompositeIndexerFailedReplacementKeepsMemoryStorePreviousSource(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()
	oldDoc := Document{
		ID:              "doc_old",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		SourceURI:       "memory://replace.md",
		Title:           "replace.md",
		ContentHash:     "hash_old",
		CreatedAt:       now,
	}
	oldChunks := []Chunk{{
		ID:              "chunk_old",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      oldDoc.ID,
		Content:         "old replacement marker",
		SourceURI:       oldDoc.SourceURI,
	}}
	if err := store.Store(ctx, oldDoc, oldChunks); err != nil {
		t.Fatal(err)
	}

	newDoc := oldDoc
	newDoc.ID = "doc_new"
	newDoc.ContentHash = "hash_new"
	newChunks := []Chunk{{
		ID:              "chunk_new",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      newDoc.ID,
		Content:         "new replacement marker",
		SourceURI:       newDoc.SourceURI,
	}}
	want := errors.New("qdrant upsert failed")
	err := (CompositeIndexer{Indexers: []Indexer{
		store,
		memoryStoreFailingIndexer{err: want},
	}}).Store(ctx, newDoc, newChunks)
	if !errors.Is(err, want) {
		t.Fatalf("Store() error = %v, want %v", err, want)
	}
	got := store.Chunks("tenant_1", "kb_1")
	if len(got) != 1 || got[0].ID != "chunk_old" || got[0].Content != "old replacement marker" {
		t.Fatalf("chunks after failed replacement = %#v", got)
	}

	err = (CompositeIndexer{Indexers: []Indexer{
		store,
		memoryStoreNoopIndexer{},
	}}).Store(ctx, newDoc, newChunks)
	if err != nil {
		t.Fatalf("successful replacement Store() error = %v", err)
	}
	got = store.Chunks("tenant_1", "kb_1")
	if len(got) != 1 || got[0].ID != "chunk_new" || got[0].Content != "new replacement marker" {
		t.Fatalf("chunks after successful replacement = %#v", got)
	}
}
