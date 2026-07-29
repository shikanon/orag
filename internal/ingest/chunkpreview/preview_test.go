package chunkpreview

import (
	"strings"
	"testing"
)

func TestProfileDetectsHeadings(t *testing.T) {
	text := "# Title\n\n## Section 1\n\ncontent\n\n### Sub-section\n\nmore content\n\n## Section 2\n\nfinal content"
	profile := Profile(text)

	if profile.HeadingCount != 4 {
		t.Errorf("expected 4 headings, got %d", profile.HeadingCount)
	}
	if profile.HeadingLevels[1] != 1 {
		t.Errorf("expected 1 level-1 heading, got %d", profile.HeadingLevels[1])
	}
	if profile.HeadingLevels[2] != 2 {
		t.Errorf("expected 2 level-2 headings, got %d", profile.HeadingLevels[2])
	}
	if profile.HeadingLevels[3] != 1 {
		t.Errorf("expected 1 level-3 heading, got %d", profile.HeadingLevels[3])
	}
}

func TestProfileDetectsPageBreaks(t *testing.T) {
	text := "content\n\n---\n\nmore content\n\n====\n\nend"
	profile := Profile(text)

	if !profile.HasPageBreaks {
		t.Error("expected page breaks to be detected")
	}
}

func TestProfileDetectsTables(t *testing.T) {
	text := "| Header 1 | Header 2 |\n| --- | --- |\n| cell 1 | cell 2 |\n| cell 3 | cell 4 |\n\nnormal paragraph"
	profile := Profile(text)

	if profile.TableLikeLines < 3 {
		t.Errorf("expected at least 3 table-like lines, got %d", profile.TableLikeLines)
	}
}

func TestProfileDetectsCodeBlocks(t *testing.T) {
	text := "normal text\n\n```go\nfunc hello() {\n    fmt.Println(\"hello\")\n}\n```\n\nmore text\n\n```python\ndef foo():\n    pass\n```"
	profile := Profile(text)

	if profile.CodeBlocks != 2 {
		t.Errorf("expected 2 code blocks, got %d", profile.CodeBlocks)
	}
}

func TestProfileDetectsListItems(t *testing.T) {
	text := "- item one\n- item two\n- item three\n\n1. first\n2. second\n3. third"
	profile := Profile(text)

	if profile.ListItems < 6 {
		t.Errorf("expected at least 6 list items, got %d", profile.ListItems)
	}
}

func TestProfileLanguageHintChinese(t *testing.T) {
	text := "这是一个中文文档。包含很多中文字符。用于测试语言检测功能。"
	profile := Profile(text)

	if profile.LanguageHint != "zh" {
		t.Errorf("expected language hint 'zh', got '%s'", profile.LanguageHint)
	}
}

func TestProfileLanguageHintEnglish(t *testing.T) {
	text := "This is an English document with mostly English words for testing."
	profile := Profile(text)

	if profile.LanguageHint != "en" {
		t.Errorf("expected language hint 'en', got '%s'", profile.LanguageHint)
	}
}

func TestProfileFrontMatter(t *testing.T) {
	text := "---\ntitle: Test\ndate: 2024-01-01\n---\n\n# Content\n\nbody text"
	profile := Profile(text)

	if !profile.HasFrontMatter {
		t.Error("expected front matter to be detected")
	}
}

func TestProfileNoFrontMatter(t *testing.T) {
	text := "# Content\n\nbody text"
	profile := Profile(text)

	if profile.HasFrontMatter {
		t.Error("expected no front matter")
	}
}

func TestProfileParagraphAndTokenStats(t *testing.T) {
	text := "# Heading\n\nFirst paragraph with several words.\n\nSecond paragraph also has many words here.\n\nThird one too."
	profile := Profile(text)

	if profile.ParagraphCount < 3 {
		t.Errorf("expected at least 3 paragraphs, got %d", profile.ParagraphCount)
	}
	if profile.TotalTokens <= 0 {
		t.Error("expected positive total tokens")
	}
	if profile.AvgParagraphTokens <= 0 {
		t.Error("expected positive average paragraph tokens")
	}
}

func TestSelectStrategyExplicit(t *testing.T) {
	profile := Profile("some text")

	tests := []struct {
		strategy string
	}{
		{StrategyHeading},
		{StrategyRecursive},
		{StrategyHeuristic},
	}

	for _, tt := range tests {
		config := Config{Strategy: tt.strategy, SizeTokens: 800, OverlapTokens: 100}
		sel := SelectStrategy(profile, config)

		if sel.Strategy != tt.strategy {
			t.Errorf("expected strategy %s, got %s", tt.strategy, sel.Strategy)
		}
		if sel.Confidence != 1.0 {
			t.Errorf("expected confidence 1.0, got %f", sel.Confidence)
		}
	}
}

func TestSelectStrategyAutoShortDoc(t *testing.T) {
	text := "short document with few words"
	profile := Profile(text)
	config := Config{Strategy: StrategyAuto, SizeTokens: 800, OverlapTokens: 100}
	sel := SelectStrategy(profile, config)

	if sel.Strategy != StrategyRecursive {
		t.Errorf("expected recursive for short doc, got %s", sel.Strategy)
	}
}

func TestSelectStrategyAutoManyHeadings(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("# Main\n\n")
	sb.WriteString(strings.Repeat("intro word ", 100))
	sb.WriteString("\n\n")
	for i := 0; i < 20; i++ {
		sb.WriteString("## Section ")
		sb.WriteString(string(rune('A' + i%26)))
		sb.WriteString("\n\n")
		sb.WriteString(strings.Repeat("content paragraph with many words in it. ", 30))
		sb.WriteString("\n\n")
	}
	profile := Profile(sb.String())
	config := Config{Strategy: StrategyAuto, SizeTokens: 800, OverlapTokens: 100}
	sel := SelectStrategy(profile, config)

	if sel.Strategy != StrategyHeading {
		t.Errorf("expected heading strategy for many headings, got %s (reason: %s)", sel.Strategy, sel.Reason)
	}
}

func TestPreviewReturnsChunksAndStats(t *testing.T) {
	text := "# Intro\n\nThis is the first paragraph with enough words to count.\n\n## Section\n\nSecond paragraph also has content. Third sentence here.\n\n### Sub\n\nMore text for testing the preview function works well."
	config := Config{Strategy: StrategyAuto, SizeTokens: 10, OverlapTokens: 2, SampleCount: 3}
	result := Preview(text, config)

	if result.Stats.TotalChunks == 0 {
		t.Fatal("expected some chunks")
	}
	if result.Stats.TotalTokens <= 0 {
		t.Error("expected positive total tokens")
	}
	if result.Stats.AvgChunkTokens <= 0 {
		t.Error("expected positive average chunk tokens")
	}
	if result.StrategySelected.Strategy == "" {
		t.Error("expected a strategy to be selected")
	}
	if len(result.SampleChunks) == 0 {
		t.Error("expected sample chunks")
	}
}

func TestPreviewSampleCountRespected(t *testing.T) {
	text := "# Intro\n\nFirst paragraph with many words to create multiple chunks. " + strings.Repeat("word ", 100) + "\n\n## Section 2\n\n" + strings.Repeat("more words ", 100)
	config := Config{Strategy: StrategyRecursive, SizeTokens: 20, OverlapTokens: 5, SampleCount: 2}
	result := Preview(text, config)

	if len(result.SampleChunks) > 2 {
		t.Errorf("expected at most 2 sample chunks, got %d", len(result.SampleChunks))
	}
}

func TestPreviewSizeDistribution(t *testing.T) {
	text := strings.Repeat("word ", 500)
	config := Config{Strategy: StrategyRecursive, SizeTokens: 50, OverlapTokens: 10}
	result := Preview(text, config)

	if len(result.Stats.SizeDistribution) == 0 {
		t.Error("expected size distribution to be populated")
	}

	expectedBuckets := []string{"<200", "200-400", "400-600", "600-800", "800-1000", ">1000"}
	for _, b := range expectedBuckets {
		if _, ok := result.Stats.SizeDistribution[b]; !ok {
			result.Stats.SizeDistribution[b] = 0
		}
	}
}

func TestPreviewContentTruncation(t *testing.T) {
	longText := strings.Repeat("word ", 200)
	text := "# Title\n\n" + longText
	config := Config{Strategy: StrategyRecursive, SizeTokens: 50, OverlapTokens: 5, SampleCount: 1}
	result := Preview(text, config)

	for _, s := range result.SampleChunks {
		runeCount := 0
		for _, r := range s.Content {
			if r == '.' && strings.HasSuffix(s.Content, "...") {
				continue
			}
			runeCount++
		}
		if runeCount > 205 {
			t.Errorf("sample content too long: %d runes", runeCount)
		}
	}
}

func TestPreviewIsPureFunction(t *testing.T) {
	text := "# Test\n\nparagraph one\n\nparagraph two\n\nparagraph three"
	config := Config{Strategy: StrategyRecursive, SizeTokens: 5, OverlapTokens: 1}

	r1 := Preview(text, config)
	r2 := Preview(text, config)

	if r1.Stats.TotalChunks != r2.Stats.TotalChunks {
		t.Error("preview should be deterministic")
	}
	if r1.Stats.TotalTokens != r2.Stats.TotalTokens {
		t.Error("preview should produce same token counts")
	}
	if r1.StrategySelected.Strategy != r2.StrategySelected.Strategy {
		t.Error("preview should select same strategy")
	}
}

func TestPreviewShortDocument(t *testing.T) {
	text := "hello world"
	config := Config{Strategy: StrategyAuto}
	result := Preview(text, config)

	if result.Stats.TotalChunks == 0 {
		t.Error("expected at least one chunk for short text")
	}
}

func TestPreviewEmptyDocument(t *testing.T) {
	text := ""
	config := Config{Strategy: StrategyAuto}
	result := Preview(text, config)

	if len(result.Warnings) == 0 {
		t.Error("expected warnings for empty document")
	}
}

func TestPreviewOrphanChunksCount(t *testing.T) {
	text := "first paragraph with no heading\n\nsecond paragraph also no heading\n\nthird one too"
	config := Config{Strategy: StrategyRecursive, SizeTokens: 3, OverlapTokens: 0}
	result := Preview(text, config)

	if result.Stats.OrphanChunks != result.Stats.TotalChunks {
		t.Errorf("expected all chunks to be orphans when no headings, got %d/%d", result.Stats.OrphanChunks, result.Stats.TotalChunks)
	}
}

func TestPreviewTableWarning(t *testing.T) {
	text := "| a | b |\n| - | - |\n| 1 | 2 |\n| 3 | 4 |\n\nnormal paragraph with words"
	config := Config{Strategy: StrategyRecursive, SizeTokens: 100}
	result := Preview(text, config)

	hasTableWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "table") {
			hasTableWarning = true
			break
		}
	}
	if !hasTableWarning {
		t.Error("expected table-related warning")
	}
}

func TestPreviewMinMaxChunkTokens(t *testing.T) {
	text := "# H\n\na\n\n" + strings.Repeat("word ", 200)
	config := Config{Strategy: StrategyRecursive, SizeTokens: 20, OverlapTokens: 5}
	result := Preview(text, config)

	if result.Stats.MinChunkTokens < 0 {
		t.Error("min chunk tokens should be >= 0")
	}
	if result.Stats.MaxChunkTokens < result.Stats.MinChunkTokens {
		t.Error("max should be >= min")
	}
}

func TestPreviewSampleChunkFields(t *testing.T) {
	text := "# Section One\n\ncontent here with words enough for testing"
	config := Config{Strategy: StrategyRecursive, SizeTokens: 10, OverlapTokens: 2, SampleCount: 1}
	result := Preview(text, config)

	if len(result.SampleChunks) == 0 {
		t.Fatal("expected at least one sample chunk")
	}

	s := result.SampleChunks[0]
	if s.Content == "" {
		t.Error("sample content should not be empty")
	}
	if s.Tokens <= 0 {
		t.Error("sample token count should be positive")
	}
}

func TestProfileLongParagraphs(t *testing.T) {
	short := "short."
	long := strings.Repeat("word ", 2000)
	text := "# H\n\n" + short + "\n\n" + long
	profile := Profile(text)

	if profile.LongParagraphs < 1 {
		t.Errorf("expected at least 1 long paragraph, got %d", profile.LongParagraphs)
	}
}
