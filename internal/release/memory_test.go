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
