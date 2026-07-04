package ingest

import (
	"context"
	"strings"
	"testing"

	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/llm/ark"
)

func TestLLMRAPTORBuilderCreatesRecursiveSummaryChunks(t *testing.T) {
	builder := LLMRAPTORBuilder{
		Model:           &raptorModel{},
		BranchFactor:    2,
		MaxLevels:       2,
		MaxSummaryChars: 120,
	}
	doc := kb.Document{
		ID:              "doc_raptor",
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		SourceURI:       "memory://raptor",
	}
	chunks := []kb.Chunk{
		{ID: "chunk_1", Content: "alpha vector search", SourceURI: doc.SourceURI},
		{ID: "chunk_2", Content: "beta sparse search", SourceURI: doc.SourceURI},
		{ID: "chunk_3", Content: "gamma rerank", SourceURI: doc.SourceURI},
		{ID: "chunk_4", Content: "delta prompt cache", SourceURI: doc.SourceURI},
	}

	summaries, warnings, err := builder.Build(context.Background(), RAPTORRequest{Document: doc, Chunks: chunks})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(summaries) != 3 {
		t.Fatalf("summaries = %d, want 3: %#v", len(summaries), summaries)
	}
	for _, summary := range summaries {
		if summary.TenantID != doc.TenantID || summary.KnowledgeBaseID != doc.KnowledgeBaseID || summary.DocumentID != doc.ID {
			t.Fatalf("summary ownership not copied: %#v", summary)
		}
		if summary.Metadata["kind"] != "raptor_summary" {
			t.Fatalf("summary kind metadata = %#v", summary.Metadata)
		}
		if summary.Metadata["child_chunk_ids"] == "" {
			t.Fatalf("summary missing child ids: %#v", summary.Metadata)
		}
		if !strings.Contains(summary.Content, "summary") {
			t.Fatalf("summary content = %q, want generated summary", summary.Content)
		}
	}
	if summaries[len(summaries)-1].Metadata["level"] != "2" {
		t.Fatalf("last summary level = %q, want 2", summaries[len(summaries)-1].Metadata["level"])
	}
}

type raptorModel struct{}

func (m *raptorModel) Chat(_ context.Context, messages []ark.ChatMessage) (string, error) {
	var user string
	for _, message := range messages {
		if message.Role == "user" {
			user = message.Content
		}
	}
	return "summary: " + user, nil
}
