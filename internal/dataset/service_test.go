package dataset

import (
	"context"
	"errors"
	"testing"
)

func TestServiceItemsAreTenantAware(t *testing.T) {
	ctx := context.Background()
	svc := NewService()
	ds, err := svc.Create(ctx, "tenant_a", "tenant a regression", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddItem(ctx, "tenant_b", ds.ID, Item{
		Query:       "cross tenant query",
		GroundTruth: "cross tenant answer",
	}); !errors.Is(err, ErrDatasetNotFound) {
		t.Fatalf("AddItem() error = %v, want ErrDatasetNotFound", err)
	}

	created, err := svc.AddItem(ctx, "tenant_a", ds.ID, Item{
		Query:       "tenant a query",
		GroundTruth: "tenant a answer",
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := svc.Items(ctx, "tenant_a", ds.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("Items() = %#v, want created item", items)
	}
	if _, err := svc.Items(ctx, "tenant_b", ds.ID); !errors.Is(err, ErrDatasetNotFound) {
		t.Fatalf("Items() cross-tenant error = %v, want ErrDatasetNotFound", err)
	}
}
