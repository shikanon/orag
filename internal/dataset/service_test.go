package dataset

import (
	"context"
	"testing"

	"github.com/shikanon/orag/internal/platform/apperrors"
)

func TestServiceItemsEnforceTenantMemoryRepository(t *testing.T) {
	ctx := context.Background()
	svc := NewService()

	ds, err := svc.Create(ctx, "tenant_a", "regression", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddItem(ctx, "tenant_b", ds.ID, Item{Query: "q", GroundTruth: "a"}); !apperrors.IsCode(err, apperrors.CodeNotFound) {
		t.Fatalf("AddItem() cross tenant err = %v, want not_found", err)
	}

	created, err := svc.AddItem(ctx, "tenant_a", ds.ID, Item{Query: "q", GroundTruth: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if created.DatasetID != ds.ID || created.ID == "" {
		t.Fatalf("created item = %#v", created)
	}

	items, err := svc.Items(ctx, "tenant_a", ds.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("Items() = %#v", items)
	}
	if _, err := svc.Items(ctx, "tenant_b", ds.ID); !apperrors.IsCode(err, apperrors.CodeNotFound) {
		t.Fatalf("Items() cross tenant err = %v, want not_found", err)
	}
}
