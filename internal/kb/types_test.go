package kb

import "testing"

func TestChunkSearchTextUsesContextualTextWhenPresent(t *testing.T) {
	chunk := Chunk{
		Content:        "it reduced latency by 30 percent",
		ContextualText: "This chunk describes the Qdrant rollout in the performance section.",
	}

	got := chunk.SearchText()
	want := "This chunk describes the Qdrant rollout in the performance section.\n\nit reduced latency by 30 percent"
	if got != want {
		t.Fatalf("SearchText() = %q, want %q", got, want)
	}
}

func TestChunkSearchTextFallsBackToContent(t *testing.T) {
	chunk := Chunk{Content: "plain chunk"}
	if got := chunk.SearchText(); got != "plain chunk" {
		t.Fatalf("SearchText() = %q", got)
	}
}
