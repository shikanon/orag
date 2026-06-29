package chunker

import "testing"

func TestRecursiveSplit(t *testing.T) {
	splitter := Recursive{SizeTokens: 5, OverlapTokens: 1}
	chunks := splitter.Split("# Intro\n\none two three four five\n\nsix seven eight")
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %#v", chunks)
	}
	if chunks[0].Content == "" {
		t.Fatal("empty chunk")
	}
}
