package kb

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStoreDeleteKnowledgeBaseIsTenantScopedAndCleansChunks(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	store.PutKnowledgeBase(KnowledgeBase{
		ID:        "kb_delete",
		TenantID:  "tenant_owner",
		Name:      "Delete me",
		CreatedAt: now,
		UpdatedAt: now,
	})
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
	if _, ok, err := store.GetKnowledgeBase("tenant_owner", "kb_delete"); err != nil || !ok {
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
	if _, ok, err := store.GetKnowledgeBase("tenant_owner", "kb_delete"); err != nil || ok {
		t.Fatal("deleted knowledge base is still visible")
	}
	if got := store.Chunks("tenant_owner", "kb_delete"); len(got) != 0 {
		t.Fatalf("chunks after delete = %#v", got)
	}
}
