package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestExtractGooseUp(t *testing.T) {
	got, err := extractGooseUp(`-- +goose Up
CREATE TABLE example(id TEXT);
-- +goose Down
DROP TABLE example;`)
	if err != nil {
		t.Fatalf("extractGooseUp() error = %v", err)
	}
	if strings.Contains(got, "DROP TABLE") {
		t.Fatalf("up migration contains down section: %q", got)
	}
	if !strings.Contains(got, "CREATE TABLE example") {
		t.Fatalf("up migration missing create statement: %q", got)
	}
}

func TestStringMapRoundTrip(t *testing.T) {
	body := mustJSON(map[string]string{"source": "test"})
	got := stringMap(body)
	if got["source"] != "test" {
		t.Fatalf("stringMap() = %#v", got)
	}
}

func TestIngestionJobResultMigration(t *testing.T) {
	body, err := os.ReadFile("../../../migrations/000002_ingestion_job_result.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{"document_id", "chunk_count", "ADD COLUMN IF NOT EXISTS"} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q: %s", required, text)
		}
	}
}
