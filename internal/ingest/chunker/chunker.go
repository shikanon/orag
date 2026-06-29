package chunker

import (
	"strings"
	"unicode/utf8"
)

type Chunk struct {
	Content string
	Section string
	Offset  int
}

type Recursive struct {
	SizeTokens    int
	OverlapTokens int
}

func (c Recursive) Split(markdown string) []Chunk {
	size := c.SizeTokens
	if size <= 0 {
		size = 800
	}
	overlap := c.OverlapTokens
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		overlap = size / 5
	}
	paragraphs := splitParagraphs(markdown)
	var chunks []Chunk
	var current strings.Builder
	offset := 0
	section := ""
	for _, p := range paragraphs {
		if strings.HasPrefix(strings.TrimSpace(p), "#") {
			section = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(p), "# "))
		}
		if tokenCount(current.String())+tokenCount(p) > size && current.Len() > 0 {
			content := strings.TrimSpace(current.String())
			chunks = append(chunks, Chunk{Content: content, Section: section, Offset: offset})
			offset += utf8.RuneCountInString(content)
			current.Reset()
			if overlap > 0 {
				current.WriteString(tailWords(content, overlap))
				current.WriteString("\n\n")
			}
		}
		current.WriteString(p)
		current.WriteString("\n\n")
	}
	if strings.TrimSpace(current.String()) != "" {
		chunks = append(chunks, Chunk{Content: strings.TrimSpace(current.String()), Section: section, Offset: offset})
	}
	return chunks
}

func splitParagraphs(s string) []string {
	parts := strings.Split(s, "\n\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func tokenCount(s string) int {
	return len(strings.Fields(s))
}

func tailWords(s string, n int) string {
	words := strings.Fields(s)
	if len(words) <= n {
		return strings.Join(words, " ")
	}
	return strings.Join(words[len(words)-n:], " ")
}
