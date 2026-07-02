package chunker

import (
	"strings"
	"testing"
)

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

func TestRecursiveSplitCountsCJKText(t *testing.T) {
	splitter := Recursive{SizeTokens: 8, OverlapTokens: 0}
	chunks := splitter.Split("这是一个用于检索增强生成系统的数据入库切分测试文本")
	if len(chunks) < 2 {
		t.Fatalf("expected CJK text to split into multiple chunks, got %#v", chunks)
	}
	for _, chunk := range chunks {
		if got := tokenCount(chunk.Content); got > 8 {
			t.Fatalf("chunk token count = %d, chunk = %#v", got, chunk)
		}
	}
}

func TestRecursiveSplitLongParagraph(t *testing.T) {
	splitter := Recursive{SizeTokens: 5, OverlapTokens: 1}
	chunks := splitter.Split(strings.Join([]string{
		"alpha", "bravo", "charlie", "delta", "echo",
		"foxtrot", "golf", "hotel", "india", "juliet",
		"kilo", "lima",
	}, " "))
	if len(chunks) < 3 {
		t.Fatalf("expected long paragraph to split into multiple chunks, got %#v", chunks)
	}
	for _, chunk := range chunks {
		if got := tokenCount(chunk.Content); got > 5 {
			t.Fatalf("chunk token count = %d, chunk = %#v", got, chunk)
		}
	}
}
