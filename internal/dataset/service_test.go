package dataset

import (
	"context"
	"testing"

	"github.com/shikanon/orag/internal/platform/apperrors"
)

func TestServiceRejectsCrossTenantDatasetItems(t *testing.T) {
	ctx := context.Background()
	svc := NewService()

	dsA, err := svc.Create(ctx, "tenant_a", "a", "golden")
	if err != nil {
		t.Fatal(err)
	}
	dsB, err := svc.Create(ctx, "tenant_b", "b", "golden")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.AddItem(ctx, "tenant_a", dsA.ID, Item{Query: "owned", GroundTruth: "truth"}); err != nil {
		t.Fatalf("tenant A AddItem() error = %v", err)
	}
	if _, err := svc.AddItem(ctx, "tenant_b", dsA.ID, Item{Query: "cross", GroundTruth: "bad"}); !apperrors.IsCode(err, apperrors.CodeNotFound) {
		t.Fatalf("tenant B AddItem() error = %v, want not_found", err)
	}
	if _, err := svc.Items(ctx, "tenant_b", dsA.ID); !apperrors.IsCode(err, apperrors.CodeNotFound) {
		t.Fatalf("tenant B Items() error = %v, want not_found", err)
	}

	itemsA, err := svc.Items(ctx, "tenant_a", dsA.ID)
	if err != nil {
		t.Fatalf("tenant A Items() error = %v", err)
	}
	if len(itemsA) != 1 || itemsA[0].Query != "owned" {
		t.Fatalf("tenant A items = %#v, want only owned item", itemsA)
	}
	itemsB, err := svc.Items(ctx, "tenant_b", dsB.ID)
	if err != nil {
		t.Fatalf("tenant B Items() error = %v", err)
	}
	if len(itemsB) != 0 {
		t.Fatalf("tenant B items = %#v, want empty tenant-owned dataset", itemsB)
	}
}
