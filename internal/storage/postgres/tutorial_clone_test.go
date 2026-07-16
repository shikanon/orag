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

func TestTutorialP2ChunkCandidateMigrationPersistsAuditFields(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/000031_tutorial_p2_chunk_candidate.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"ADD COLUMN chunk_size_tokens INTEGER NOT NULL DEFAULT 0",
		"ADD COLUMN chunk_overlap_tokens INTEGER NOT NULL DEFAULT 0",
		"ADD COLUMN indexed_chunk_count INTEGER NOT NULL DEFAULT 0",
		"ADD COLUMN average_chunk_tokens DOUBLE PRECISION NOT NULL DEFAULT 0",
		"DROP COLUMN average_chunk_tokens",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestTutorialP3ContextualCandidateMigrationPersistsAuditFields(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/000032_tutorial_p3_contextual_candidate.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"ADD COLUMN contextual_retrieval_enabled BOOLEAN NOT NULL DEFAULT FALSE",
		"ADD COLUMN contextualized_chunk_count INTEGER NOT NULL DEFAULT 0",
		"ADD COLUMN average_context_tokens DOUBLE PRECISION NOT NULL DEFAULT 0",
		"DROP COLUMN average_context_tokens",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
