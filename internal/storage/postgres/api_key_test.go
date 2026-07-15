package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestAPIKeyRBACMigrationDefinesSecurityConstraints(t *testing.T) {
	body, err := os.ReadFile("../../../migrations/000017_api_key_rbac.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, required := range []string{
		"projects_tenant_id_id_unique",
		"FOREIGN KEY (tenant_id, project_id)",
		"api_keys_role_check",
		"api_keys_project_role_check",
		"api_keys_expiry_check",
		"api_keys_hash_unique",
		"WHERE revoked_at IS NULL",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}
