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

func TestTutorialP4SparseCandidateMigrationPersistsAuditFields(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/000033_tutorial_p4_sparse_candidate.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"ADD COLUMN retrieval_strategy TEXT NOT NULL DEFAULT 'hybrid'",
		"ADD COLUMN reused_baseline_index BOOLEAN NOT NULL DEFAULT FALSE",
		"DROP COLUMN reused_baseline_index",
		"DROP COLUMN retrieval_strategy",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestTutorialP5MultiQueryCandidateMigrationPersistsAuditFields(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/000034_tutorial_p5_multi_query_candidate.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"ADD COLUMN query_expansion_mode TEXT NOT NULL DEFAULT 'none'",
		"ADD COLUMN multi_query_count INTEGER NOT NULL DEFAULT 0",
		"DROP COLUMN multi_query_count",
		"DROP COLUMN query_expansion_mode",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestTutorialP6RerankCandidateMigrationPersistsAuditFields(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/000035_tutorial_p6_rerank_candidate.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"ADD COLUMN rerank_enabled BOOLEAN NOT NULL DEFAULT FALSE",
		"DROP COLUMN rerank_enabled",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestTutorialP7GraphCandidateMigrationPersistsAuditFields(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/000036_tutorial_p7_graph_candidate.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"ADD COLUMN graph_retrieval_enabled BOOLEAN NOT NULL DEFAULT FALSE",
		"DROP COLUMN graph_retrieval_enabled",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestTutorialP8ContextPackCandidateMigrationPersistsAuditFields(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/000037_tutorial_p8_context_pack_candidate.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"ADD COLUMN context_pack_top_n INTEGER NOT NULL DEFAULT 0",
		"ADD COLUMN context_pack_max_tokens INTEGER NOT NULL DEFAULT 0",
		"DROP COLUMN context_pack_max_tokens",
		"DROP COLUMN context_pack_top_n",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
