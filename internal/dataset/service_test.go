package dataset

import (
	"context"
	"errors"
	"testing"
)

func TestServiceRequiresTenantForItems(t *testing.T) {
	ctx := context.Background()
	svc := NewService()
	ds, err := svc.Create(ctx, "tenant_a", "golden", "golden")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.AddItem(ctx, "tenant_b", ds.ID, Item{Query: "q", GroundTruth: "a"}); !errors.Is(err, ErrDatasetNotFound) {
		t.Fatalf("AddItem() error = %v, want dataset not found", err)
	}
	if _, err := svc.Items(ctx, "tenant_b", ds.ID); !errors.Is(err, ErrDatasetNotFound) {
		t.Fatalf("Items() error = %v, want dataset not found", err)
	}

	items, err := svc.Items(ctx, "tenant_a", ds.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("cross-tenant AddItem inserted %d items", len(items))
	}
}
