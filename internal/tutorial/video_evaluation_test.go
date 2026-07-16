package tutorial

import (
	"context"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/kb"
)

func TestVideoEvaluationActivatesFrozenProjectSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryCloneRepository()
	job := CloneJob{ID: "j", TenantID: "t", ProjectID: "p", TemplateID: "video-rag", TemplateVersion: "1.0.0", Tier: "benchmark"}
	if _, _, err := repo.CreateOrGet(ctx, job); err != nil {
		t.Fatal(err)
	}
	protocol, err := ParseVideoProtocol([]byte(validVideoProtocol), videoProtocolTemplate(t), videoProtocolPack(t))
	if err != nil {
		t.Fatal(err)
	}
	experiment := Experiment{ID: "e", TenantID: "t", ProjectID: "p", CloneJobID: "j", TemplateID: "video-rag", TemplateVersion: "1.0.0", Tier: "benchmark", PackStatus: PackStatusInstalled, UpdatedAt: time.Now().UTC(), PackManifest: Manifest{VideoProtocol: &protocol, VideoSource: &VideoSource{Alias: "clip"}, TemporalSegments: []TemporalSegment{{EvidenceID: "clip@0-10000"}}, TemporalAssets: []PackObject{{Path: temporalIndexPath, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Bytes: 1, ContentType: "text/plain"}}}}
	if err := repo.EnsureExperiment(ctx, experiment); err != nil {
		t.Fatal(err)
	}
	datasets := dataset.NewService(dataset.NewMemoryRepository())
	source, err := datasets.CreateInProject(ctx, "t", "p", "authorized", "private")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := datasets.AddItem(ctx, "t", source.ID, dataset.Item{Query: "what happened", GroundTruth: "answer", ExpectedEvidence: []string{"clip@0-10000"}}); err != nil {
		t.Fatal(err)
	}
	service := NewVideoEvaluationService(repo, datasets, ResourceInitializer{KnowledgeBases: kb.NewMemoryStore(), Datasets: datasets})
	resources, err := service.Activate(ctx, Subject{TenantID: "t", ID: "u"}, "p", source.ID)
	if err != nil || resources.Status != "ready" || resources.DatasetID == source.ID {
		t.Fatalf("resources=%#v err=%v", resources, err)
	}
	got, found, err := repo.GetExperiment(ctx, "t", "p")
	if err != nil || !found || got.PackManifest.Runtime == nil || len(got.PackManifest.Runtime.Dataset.Items) != 1 || got.PackManifest.Runtime.Dataset.Items[0].Query != "what happened" {
		t.Fatalf("experiment=%#v err=%v", got, err)
	}
}
