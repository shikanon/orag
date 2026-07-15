package tutorial

import (
	"context"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/kb"
)

func TestResourceInitializerCreatesStableProjectRoots(t *testing.T) {
	store := kb.NewMemoryStore()
	datasets := dataset.NewService(dataset.NewMemoryRepository())
	initializer := ResourceInitializer{KnowledgeBases: store, Datasets: datasets, Now: func() time.Time {
		return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	}}
	job := CloneJob{ID: "tclj_1", TenantID: "tenant_a", ProjectID: "prj_1", TemplateID: "text-rag", TemplateVersion: "1.0.0", Tier: "quick"}
	manifest := Manifest{Runtime: &RuntimeManifest{
		Baseline:  RuntimeBaseline{Profile: "realtime", TopK: 5},
		Documents: []RuntimeDocument{{ObjectPath: "corpus/data.txt", Name: "数据"}},
		Dataset:   RuntimeDataset{Name: "教程验证集", Items: []RuntimeDatasetItem{{Query: "问题", GroundTruth: "答案", Split: "eval"}}},
	}}
	first, err := initializer.Ensure(context.Background(), job, manifest)
	if err != nil || first.Status != "ready" || first.KnowledgeBaseID == "" || first.DatasetID == "" {
		t.Fatalf("resources=%#v err=%v", first, err)
	}
	second, err := initializer.Ensure(context.Background(), job, manifest)
	if err != nil || second != first {
		t.Fatalf("retry resources=%#v first=%#v err=%v", second, first, err)
	}
	base, found, err := store.GetKnowledgeBase(context.Background(), job.TenantID, first.KnowledgeBaseID)
	if err != nil || !found || base.ProjectID != job.ProjectID {
		t.Fatalf("knowledge base=%#v found=%v err=%v", base, found, err)
	}
	items, err := datasets.Items(context.Background(), job.TenantID, first.DatasetID)
	if err != nil || len(items) != 1 || items[0].Split != dataset.DatasetSplitEval {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestResourceInitializerMarksMissingRuntimeUnavailable(t *testing.T) {
	resources, err := (ResourceInitializer{}).Ensure(context.Background(), CloneJob{}, Manifest{})
	if err != nil || resources.Status != "runtime_unavailable" {
		t.Fatalf("resources=%#v err=%v", resources, err)
	}
}
