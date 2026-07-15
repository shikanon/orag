package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestTutorialCloneMigrationDefinesDurableIdempotentState(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/000026_tutorial_clone_jobs.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS tutorial_clone_jobs",
		"UNIQUE (tenant_id, subject_id, template_id, template_version, idempotency_key)",
		"CREATE TABLE IF NOT EXISTS tutorial_experiments",
		"REFERENCES projects(id) ON DELETE CASCADE",
		"CREATE TABLE IF NOT EXISTS tutorial_clone_stage_events",
		"ON DELETE CASCADE",
		"tutorial_clone_jobs_tenant_updated_idx",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
