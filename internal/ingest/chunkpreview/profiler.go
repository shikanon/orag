package chunkpreview

import (
	"strings"
	"unicode"

	"github.com/shikanon/orag/internal/ingest/chunker"
)

// DocumentProfile captures the structural and statistical characteristics of a
// markdown document so that a chunking strategy can be selected intelligently.
type DocumentProfile struct {
	HeadingCount       int
	HeadingLevels      map[int]int
	HasPageBreaks      bool
	TableLikeLines     int
	CodeBlocks         int
	ListItems          int
	TotalTokens        int
	ParagraphCount     int
	AvgParagraphTokens float64
	LongParagraphs     int
	LanguageHint       string
	HasFrontMatter     bool
}

// Profile analyzes a markdown document and returns a DocumentProfile containing
// structural statistics and language hint.
func Profile(text string) DocumentProfile {
	profile := DocumentProfile{
		HeadingLevels: make(map[int]int),
	}

	lines := strings.Split(text, "\n")
	paragraphs := splitParagraphs(text)
	profile.ParagraphCount = len(paragraphs)
	profile.TotalTokens = chunker.TokenCount(text)

	if profile.ParagraphCount > 0 {
		profile.AvgParagraphTokens = float64(profile.TotalTokens) / float64(profile.ParagraphCount)
	}

	inCodeBlock := false
	codeBlockStart := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if !inCodeBlock {
				inCodeBlock = true
				codeBlockStart = true
				profile.CodeBlocks++
			} else {
				inCodeBlock = false
			}
			continue
		}

		if inCodeBlock {
			continue
		}

		if codeBlockStart {
			codeBlockStart = false
		}

		if strings.HasPrefix(trimmed, "#") {
			level := countHeadingLevel(trimmed)
			if level > 0 {
				profile.HeadingCount++
				profile.HeadingLevels[level]++
			}
		}

		if isPageBreak(trimmed) {
			profile.HasPageBreaks = true
		}

		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
			profile.TableLikeLines++
		}

		if isListItem(trimmed) {
			profile.ListItems++
		}
	}

	sizeTokens := 800
	for _, p := range paragraphs {
		if chunker.TokenCount(p) > sizeTokens {
			profile.LongParagraphs++
		}
	}

	profile.LanguageHint = detectLanguage(text)
	profile.HasFrontMatter = hasFrontMatter(text)

	return profile
}

func countHeadingLevel(line string) int {
	level := 0
	for _, r := range line {
		if r == '#' {
			level++
		} else {
			break
		}
	}
	if level > 0 && level <= 6 {
		rest := line[level:]
		if len(rest) > 0 && rest[0] == ' ' {
			return level
		}
	}
	return 0
}

func isPageBreak(line string) bool {
	if len(line) < 3 {
		return false
	}
	allDash := true
	for _, r := range line {
		if r != '-' {
			allDash = false
			break
		}
	}
	if allDash && len(line) >= 3 {
		return true
	}
	allEqual := true
	for _, r := range line {
		if r != '=' {
			allEqual = false
			break
		}
	}
	return allEqual && len(line) >= 3
}

func isListItem(line string) bool {
	if len(line) == 0 {
		return false
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
		return true
	}
	for i := 0; i < len(line); i++ {
		if line[i] < '0' || line[i] > '9' {
			if line[i] == '.' && i > 0 && i+1 < len(line) && line[i+1] == ' ' {
				return true
			}
			break
		}
	}
	return false
}

func splitParagraphs(s string) []string {
	parts := strings.Split(s, "\n\n")
	out := make([]string, 0, len(parts))
	inCodeBlock := false
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "```") {
			inCodeBlock = !inCodeBlock
		}
		if !inCodeBlock {
			out = append(out, p)
		} else {
			if len(out) > 0 {
				out[len(out)-1] = out[len(out)-1] + "\n\n" + p
			} else {
				out = append(out, p)
			}
		}
	}
	return out
}

func detectLanguage(text string) string {
	var cjkCount int
	var totalCount int
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		totalCount++
		if isCJK(r) {
			cjkCount++
		}
	}
	if totalCount == 0 {
		return "en"
	}
	ratio := float64(cjkCount) / float64(totalCount)
	if ratio > 0.5 {
		return "zh"
	}
	if ratio > 0.1 {
		return "mixed"
	}
	return "en"
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func hasFrontMatter(text string) bool {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "---") {
		return false
	}
	lines := strings.SplitN(trimmed, "\n", -1)
	if len(lines) < 2 {
		return false
	}
	if strings.TrimSpace(lines[0]) != "---" {
		return false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return true
		}
	}
	return false
}
