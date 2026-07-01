package kb

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStoreDeleteKnowledgeBase(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	for _, item := range []KnowledgeBase{
		{ID: "kb_delete", TenantID: "tenant_1", Name: "Delete", CreatedAt: now, UpdatedAt: now},
		{ID: "kb_keep", TenantID: "tenant_1", Name: "Keep", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
		{ID: "kb_other", TenantID: "tenant_2", Name: "Other", CreatedAt: now.Add(2 * time.Minute), UpdatedAt: now.Add(2 * time.Minute)},
	} {
		if err := store.PutKnowledgeBase(item); err != nil {
			t.Fatalf("PutKnowledgeBase(%s) error = %v", item.ID, err)
		}
	}
	for _, doc := range []Document{
		{ID: "doc_delete", TenantID: "tenant_1", KnowledgeBaseID: "kb_delete"},
		{ID: "doc_keep", TenantID: "tenant_1", KnowledgeBaseID: "kb_keep"},
		{ID: "doc_other", TenantID: "tenant_2", KnowledgeBaseID: "kb_other"},
	} {
		if err := store.Store(context.Background(), doc, []Chunk{
			{ID: "chunk_" + doc.ID, TenantID: doc.TenantID, KnowledgeBaseID: doc.KnowledgeBaseID, DocumentID: doc.ID},
		}); err != nil {
			t.Fatalf("Store(%s) error = %v", doc.ID, err)
		}
	}

	found, err := store.DeleteKnowledgeBase(context.Background(), "tenant_1", "kb_delete")
	if err != nil {
		t.Fatalf("DeleteKnowledgeBase() error = %v", err)
	}
	if !found {
		t.Fatal("DeleteKnowledgeBase() found = false, want true")
	}
	if _, ok, err := store.GetKnowledgeBase("tenant_1", "kb_delete"); err != nil || ok {
		t.Fatalf("deleted KB lookup ok=%v err=%v, want ok=false nil error", ok, err)
	}
	if chunks := store.Chunks("tenant_1", "kb_delete"); len(chunks) != 0 {
		t.Fatalf("deleted KB chunks = %#v, want none", chunks)
	}
	if _, ok := store.documents["doc_delete"]; ok {
		t.Fatal("deleted KB document remains")
	}

	for _, tc := range []struct {
		tenantID string
		kbID     string
		docID    string
		chunkID  string
	}{
		{tenantID: "tenant_1", kbID: "kb_keep", docID: "doc_keep", chunkID: "chunk_doc_keep"},
		{tenantID: "tenant_2", kbID: "kb_other", docID: "doc_other", chunkID: "chunk_doc_other"},
	} {
		if _, ok, err := store.GetKnowledgeBase(tc.tenantID, tc.kbID); err != nil || !ok {
			t.Fatalf("kept KB %s/%s ok=%v err=%v, want ok=true nil error", tc.tenantID, tc.kbID, ok, err)
		}
		if _, ok := store.documents[tc.docID]; !ok {
			t.Fatalf("kept document %s missing", tc.docID)
		}
		if _, ok := store.chunks[tc.chunkID]; !ok {
			t.Fatalf("kept chunk %s missing", tc.chunkID)
		}
	}
	items, err := store.ListKnowledgeBases("tenant_1")
	if err != nil {
		t.Fatalf("ListKnowledgeBases() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "kb_keep" {
		t.Fatalf("tenant_1 KBs after delete = %#v, want only kb_keep", items)
	}

	found, err = store.DeleteKnowledgeBase(context.Background(), "tenant_1", "kb_delete")
	if err != nil {
		t.Fatalf("repeated DeleteKnowledgeBase() error = %v", err)
	}
	if found {
		t.Fatal("repeated DeleteKnowledgeBase() found = true, want false")
	}
}
