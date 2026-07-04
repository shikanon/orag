package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestRunPrintsMemoryQueryAndTraceMetadata(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), &out); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"document_id=doc_",
		"chunks=3",
		"answer=Found",
		"trace_id=trace_example_memory",
		"cache_status=disabled",
		"trace_summary=node_count:2",
		"slowest_node:",
		"trace_spans=2",
		"citations=",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() output missing %q:\n%s", want, got)
		}
	}
}

func TestExampleDoesNotImportInternalPackages(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if strings.Contains(string(source), "/internal/") {
		t.Fatalf("memory example must not import internal packages:\n%s", source)
	}
}
