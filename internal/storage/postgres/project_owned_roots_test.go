package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestProjectOwnedRootsMigrationBackfillsAndScopesForeignKeys(t *testing.T) {
	body, err := os.ReadFile("../../../migrations/000018_project_owned_roots.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, required := range []string{
		"'prj_default_' || id",
		"UPDATE knowledge_bases SET project_id",
		"UPDATE datasets SET project_id",
		"FOREIGN KEY (tenant_id, project_id)",
		"knowledge_bases_tenant_project_created_idx",
		"datasets_tenant_project_created_idx",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}
