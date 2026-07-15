package release

import (
	"context"
	"testing"
)

func TestMemoryRepositoryVersionsCloneDefinitionsAndScopeProject(t *testing.T) {
	repo := NewMemoryRepository("project_a")
	definition := []byte(`{"nodes":[]}`)
	if err := repo.CreateVersion(context.Background(), Version{
		ID:         "pv_a",
		ProjectID:  "project_a",
		PipelineID: "pipe_a",
		Definition: definition,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVersion(context.Background(), Version{ID: "pv_b", ProjectID: "project_b"}); err != nil {
		t.Fatal(err)
	}

	definition[2] = 'X'
	version, err := repo.Version(context.Background(), "project_a", "pv_a")
	if err != nil || string(version.Definition) != `{"nodes":[]}` {
		t.Fatalf("stored definition = %q, err = %v", version.Definition, err)
	}
	version.Definition[2] = 'Y'
	stored, err := repo.Version(context.Background(), "project_a", "pv_a")
	if err != nil || string(stored.Definition) != `{"nodes":[]}` {
		t.Fatalf("read definition = %q, err = %v", stored.Definition, err)
	}
	if _, err := repo.Version(context.Background(), "project_b", "pv_a"); err != ErrNotFound {
		t.Fatalf("cross-project version error = %v, want ErrNotFound", err)
	}
	versions, err := repo.Versions(context.Background(), "project_a")
	if err != nil || len(versions) != 1 || versions[0].ID != "pv_a" {
		t.Fatalf("versions = %#v, err = %v", versions, err)
	}
}

func TestMemoryRepositoryScopesEnvironmentsAndEvidenceByProject(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository("project_a")
	envA, err := repo.Environment(ctx, "project_a", Production)
	if err != nil {
		t.Fatal(err)
	}
	envA.ActiveVersionID = "pv_a"
	repo.SetEnvironment(envA)
	envB, err := repo.Environment(ctx, "project_b", Production)
	if err != nil {
		t.Fatal(err)
	}
	if envB.ActiveVersionID != "" || envB.ProjectID != "project_b" {
		t.Fatalf("project_b environment leaked project_a state: %#v", envB)
	}
	repo.PutEvidence(Evidence{ProjectID: "project_a", VersionID: "pv_same", EnvironmentID: string(Production), Passed: true, DatasetID: "ds_a"})
	repo.PutEvidence(Evidence{ProjectID: "project_b", VersionID: "pv_same", EnvironmentID: string(Production), Passed: true, DatasetID: "ds_b"})
	evidenceA, _ := repo.Evidence(ctx, "project_a", "pv_same", Production)
	evidenceB, _ := repo.Evidence(ctx, "project_b", "pv_same", Production)
	if evidenceA.DatasetID != "ds_a" || evidenceB.DatasetID != "ds_b" {
		t.Fatalf("project evidence not isolated: a=%#v b=%#v", evidenceA, evidenceB)
	}
}
