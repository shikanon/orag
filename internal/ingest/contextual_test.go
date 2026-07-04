package ingest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shikanon/orag/internal/ingest/chunker"
	"github.com/shikanon/orag/internal/llm/ark"
)

type contextualChatModel struct {
	answers []string
	err     error
	calls   int
}

func (m *contextualChatModel) Chat(_ context.Context, messages []ark.ChatMessage) (string, error) {
	m.calls++
	if m.err != nil {
		return "", m.err
	}
	if len(messages) == 0 || !strings.Contains(messages[len(messages)-1].Content, "chunk") {
		return "", errors.New("missing chunk prompt")
	}
	if m.calls-1 < len(m.answers) {
		return m.answers[m.calls-1], nil
	}
	return "context", nil
}

func TestLLMContextualizerGeneratesContextPerChunk(t *testing.T) {
	model := &contextualChatModel{answers: []string{
		"Qdrant rollout benchmark section.",
		"Second context with extra trailing text that should be trimmed.",
	}}
	contextualizer := LLMContextualizer{
		Model:            model,
		MaxDocumentChars: 80,
		MaxChunkChars:    40,
		MaxContextChars:  64,
		FailureMode:      ContextualFailureFallback,
	}

	contexts, warnings, err := contextualizer.Contextualize(context.Background(), ContextualizationRequest{
		DocumentName: "bench.md",
		DocumentText: strings.Repeat("document ", 20),
		Chunks: []chunker.Chunk{
			{Content: "chunk one"},
			{Content: "chunk two"},
		},
	})
	if err != nil {
		t.Fatalf("Contextualize() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(contexts) != 2 {
		t.Fatalf("contexts len = %d", len(contexts))
	}
	if contexts[0] != "Qdrant rollout benchmark section." {
		t.Fatalf("context[0] = %q", contexts[0])
	}
	if len([]rune(contexts[1])) > 64 {
		t.Fatalf("context[1] was not truncated: %q", contexts[1])
	}
	if model.calls != 2 {
		t.Fatalf("calls = %d", model.calls)
	}
}

func TestLLMContextualizerFallbackKeepsIngestionSearchable(t *testing.T) {
	contextualizer := LLMContextualizer{
		Model:       &contextualChatModel{err: errors.New("llm down")},
		FailureMode: ContextualFailureFallback,
	}

	contexts, warnings, err := contextualizer.Contextualize(context.Background(), ContextualizationRequest{
		DocumentName: "doc.md",
		DocumentText: "document",
		Chunks:       []chunker.Chunk{{Content: "chunk one"}},
	})
	if err != nil {
		t.Fatalf("Contextualize() error = %v", err)
	}
	if len(contexts) != 1 || contexts[0] != "" {
		t.Fatalf("contexts = %#v", contexts)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "contextualization failed") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestLLMContextualizerFailModeReturnsError(t *testing.T) {
	contextualizer := LLMContextualizer{
		Model:       &contextualChatModel{err: errors.New("llm down")},
		FailureMode: ContextualFailureFail,
	}

	_, _, err := contextualizer.Contextualize(context.Background(), ContextualizationRequest{
		DocumentName: "doc.md",
		DocumentText: "document",
		Chunks:       []chunker.Chunk{{Content: "chunk one"}},
	})
	if err == nil {
		t.Fatal("expected fail mode error")
	}
}
