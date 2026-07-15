package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestPipelineVersionDefinitionMigrationPreservesLegacyVersions(t *testing.T) {
	body, err := os.ReadFile("../../../migrations/000023_pipeline_version_definitions.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS pipeline_id TEXT REFERENCES pipelines(id) ON DELETE RESTRICT",
		"ADD COLUMN IF NOT EXISTS definition JSONB",
		"pipeline_versions_project_pipeline_created_idx",
		"DROP COLUMN IF EXISTS definition",
		"DROP COLUMN IF EXISTS pipeline_id",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}
