package dataset

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryRepositoryItemsRequireOwningTenant(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryRepository())
	ds, err := svc.Create(ctx, "tenant_owner", "regression", "golden")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.AddItem(ctx, "tenant_other", ds.ID, Item{Query: "wrong tenant"}); !errors.Is(err, ErrDatasetNotFound) {
		t.Fatalf("AddItem() error = %v, want ErrDatasetNotFound", err)
	}
	if _, err := svc.AddItem(ctx, "tenant_owner", ds.ID, Item{Query: "owned tenant"}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Items(ctx, "tenant_other", ds.ID); !errors.Is(err, ErrDatasetNotFound) {
		t.Fatalf("Items() error = %v, want ErrDatasetNotFound", err)
	}
	items, err := svc.Items(ctx, "tenant_owner", ds.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Query != "owned tenant" {
		t.Fatalf("Items() = %#v, want only owned tenant item", items)
	}
}
