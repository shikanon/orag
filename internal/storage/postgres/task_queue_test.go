package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestTaskQueueLeaseGenerationMigrationIsDurableAndReversible(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/000042_task_queue_lease_generation.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"ADD COLUMN lease_generation BIGINT NOT NULL DEFAULT 0",
		"CHECK (lease_generation >= 0)",
		"DROP COLUMN IF EXISTS lease_generation",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("lease generation migration missing %q", required)
		}
	}
}
