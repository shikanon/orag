package tutorial

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

func TestNewCatalogLoadsApprovedTemplates(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatal(err)
	}

	items := catalog.List()
	if len(items) != 3 {
		t.Fatalf("List() len = %d, want 3", len(items))
	}
	want := []Modality{ModalityText, ModalityVideo, ModalityVisualDocument}
	got := make([]Modality, 0, len(items))
	for _, item := range items {
		got = append(got, item.Modality)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("modalities = %v, want %v", got, want)
	}
}

func TestCatalogGetVersion(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	current, err := catalog.Get("text-rag", "")
	if err != nil {
		t.Fatal(err)
	}
	versioned, err := catalog.Get("text-rag", current.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current, versioned) {
		t.Fatalf("versioned = %#v, current %#v", versioned, current)
	}
}

func TestCatalogDistinguishesMissingTemplateAndVersion(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Get("missing", ""); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("missing template error = %v", err)
	}
	if _, err := catalog.Get("text-rag", "9.9.9"); !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("missing version error = %v", err)
	}
}

func TestNewCatalogRejectsDuplicateVersion(t *testing.T) {
	templates := []Template{
		validTemplate("text-rag", "1.0.0", ModalityText),
		validTemplate("text-rag", "1.0.0", ModalityText),
	}
	if _, err := newCatalog(templates); err == nil {
		t.Fatal("newCatalog() error = nil, want duplicate version error")
	}
}

func TestCatalogReturnsDefensiveCopies(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	items := catalog.List()
	items[0].ScenarioDimensions[0] = "mutated"
	items[0].Packs[0].Tier = "mutated"

	refreshed, err := catalog.Get(items[0].ID, items[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.ScenarioDimensions[0] == "mutated" || refreshed.Packs[0].Tier == "mutated" {
		t.Fatal("catalog values were mutated through List()")
	}
}

func validTemplate(id, version string, modality Modality) Template {
	return Template{
		ID:                       id,
		Slug:                     id,
		Title:                    "Tutorial",
		Summary:                  "Summary",
		Version:                  version,
		Status:                   "published",
		Modality:                 modality,
		Difficulty:               "intermediate",
		EstimatedDurationMinutes: 30,
		SourceBenchmark:          "Benchmark",
		SourceURL:                "https://example.test/benchmark",
		ScenarioDimensions:       []string{"negative_semantics"},
		PipelineStages:           []string{"P0"},
		RequiredCapabilities:     []string{"retrieval"},
		Packs: []PackRef{
			{Tier: "quick", ManifestPath: id + "/" + version + "/quick/manifest.json"},
			{Tier: "benchmark", ManifestPath: id + "/" + version + "/benchmark/manifest.json"},
		},
		ReplayAvailable: true,
	}
}
