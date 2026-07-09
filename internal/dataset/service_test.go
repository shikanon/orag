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

func TestServiceDatasetItemEvaluationMetadata(t *testing.T) {
	ctx := context.Background()
	svc := NewService()
	ds, err := svc.Create(ctx, "tenant_a", "golden", "golden")
	if err != nil {
		t.Fatal(err)
	}

	oldItem, err := svc.AddItem(ctx, "tenant_a", ds.ID, Item{Query: "legacy q", GroundTruth: "legacy a"})
	if err != nil {
		t.Fatal(err)
	}
	if oldItem.Split != DatasetSplitEval || oldItem.Weight != 1 {
		t.Fatalf("legacy item metadata = %#v, want eval split and weight 1", oldItem)
	}

	_, err = svc.AddItem(ctx, "tenant_a", ds.ID, Item{
		Query:            "calibration q",
		GroundTruth:      "calibration a",
		Split:            DatasetSplitGold,
		Weight:           2.5,
		ExpectedEvidence: []string{"chunk_1", "chunk_2"},
		HumanScores:      map[string]float64{"faithfulness": 0.9},
	})
	if err != nil {
		t.Fatal(err)
	}

	items, err := svc.Items(ctx, "tenant_a", ds.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("Items() len = %d, want 2", len(items))
	}
	got := items[1]
	if got.Split != DatasetSplitGold || got.Weight != 2.5 {
		t.Fatalf("metadata = split:%s weight:%v, want gold/2.5", got.Split, got.Weight)
	}
	if len(got.ExpectedEvidence) != 2 || got.ExpectedEvidence[0] != "chunk_1" {
		t.Fatalf("expected evidence = %#v", got.ExpectedEvidence)
	}
	if got.HumanScores["faithfulness"] != 0.9 {
		t.Fatalf("human scores = %#v", got.HumanScores)
	}
}

func TestServiceItemsBySplitFiltersAndNormalizesLegacyMetadata(t *testing.T) {
	ctx := context.Background()
	svc := NewService()
	ds, err := svc.Create(ctx, "tenant_a", "golden", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddItem(ctx, "tenant_a", ds.ID, Item{Query: "legacy", GroundTruth: "a", Weight: 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddItem(ctx, "tenant_a", ds.ID, Item{Query: "holdout", GroundTruth: "b", Split: DatasetSplitHoldout, Weight: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddItem(ctx, "tenant_a", ds.ID, Item{Query: "eval-negative", GroundTruth: "c", Split: DatasetSplitEval, Weight: -3}); err != nil {
		t.Fatal(err)
	}

	evalItems, err := svc.ItemsBySplit(ctx, "tenant_a", ds.ID, DatasetSplitEval)
	if err != nil {
		t.Fatal(err)
	}
	if len(evalItems) != 2 {
		t.Fatalf("eval items len = %d, want 2: %#v", len(evalItems), evalItems)
	}
	for _, item := range evalItems {
		if item.Split != DatasetSplitEval || item.Weight != 1 {
			t.Fatalf("eval item metadata = %#v, want eval split and normalized weight 1", item)
		}
	}

	holdoutItems, err := svc.ItemsBySplit(ctx, "tenant_a", ds.ID, DatasetSplitHoldout)
	if err != nil {
		t.Fatal(err)
	}
	if len(holdoutItems) != 1 || holdoutItems[0].Query != "holdout" || holdoutItems[0].Weight != 2 {
		t.Fatalf("holdout items = %#v, want single weighted holdout item", holdoutItems)
	}

	allItems, err := svc.ItemsBySplit(ctx, "tenant_a", ds.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(allItems) != 3 {
		t.Fatalf("all items len = %d, want 3", len(allItems))
	}
}
