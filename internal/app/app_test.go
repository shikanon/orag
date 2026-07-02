package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/kb"
)

func TestKnowledgeBaseStoreWithPointCleanupDeletesPointsBeforeMetadata(t *testing.T) {
	calls := []string{}
	repo := newFakeKnowledgeBaseRepo(&calls, kb.KnowledgeBase{
		ID:        "kb_1",
		TenantID:  "tenant_1",
		Name:      "KB",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	store := knowledgeBaseStore{
		primary:       repo,
		vectorDeleter: recordingVectorDeleter{name: "chunks", calls: &calls},
	}

	deleted, err := store.DeleteKnowledgeBase(context.Background(), "tenant_1", "kb_1")
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("DeleteKnowledgeBase() deleted = false, want true")
	}
	wantCalls := []string{"chunks:tenant_1/kb_1", "metadata:tenant_1/kb_1"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
	if _, ok, err := repo.GetKnowledgeBase("tenant_1", "kb_1"); err != nil || ok {
		t.Fatal("knowledge base metadata still exists")
	}
}

func TestKnowledgeBaseStoreWithPointCleanupSkipsMissingKBAndStopsOnPointError(t *testing.T) {
	calls := []string{}
	repo := newFakeKnowledgeBaseRepo(&calls, kb.KnowledgeBase{
		ID:        "kb_1",
		TenantID:  "tenant_1",
		Name:      "KB",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	store := knowledgeBaseStore{
		primary:       repo,
		vectorDeleter: recordingVectorDeleter{name: "chunks", calls: &calls, err: errors.New("qdrant delete failed")},
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
	if deleted {
		t.Fatal("DeleteKnowledgeBase() deleted = true after point delete error")
	}
	if _, ok, err := repo.GetKnowledgeBase("tenant_1", "kb_1"); err != nil || !ok {
		t.Fatal("metadata was deleted after point delete error")
	}
}

type fakeKnowledgeBaseRepo struct {
	items map[string]kb.KnowledgeBase
	calls *[]string
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

func (r *fakeKnowledgeBaseRepo) ListKnowledgeBases(tenantID string) ([]kb.KnowledgeBase, error) {
	var out []kb.KnowledgeBase
	for _, item := range r.items {
		if item.TenantID == tenantID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *fakeKnowledgeBaseRepo) GetKnowledgeBase(tenantID, id string) (kb.KnowledgeBase, bool, error) {
	item, ok := r.items[tenantID+"/"+id]
	return item, ok, nil
}

func (r *fakeKnowledgeBaseRepo) DeleteKnowledgeBase(_ context.Context, tenantID, id string) (bool, error) {
	if _, ok, err := r.GetKnowledgeBase(tenantID, id); err != nil || !ok {
		return false, nil
	}
	*r.calls = append(*r.calls, "metadata:"+tenantID+"/"+id)
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
