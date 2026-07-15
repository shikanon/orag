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

func TestProductionExecutionLineageMigration(t *testing.T) {
	body, err := os.ReadFile("../../../migrations/000025_production_execution_lineage.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS active_release_id TEXT",
		"ALTER TABLE project_release_validations",
		"ADD COLUMN IF NOT EXISTS dataset_id TEXT NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS evaluation_run_id TEXT NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS project_id TEXT NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS pipeline_version_id TEXT NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS release_id TEXT NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS retrieval_params JSONB NOT NULL DEFAULT '{}'::jsonb",
		"rag_traces_project_version_created_idx",
		"DROP COLUMN IF EXISTS active_release_id",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}
