package chunker

import (
	"strings"
	"unicode"
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
	flush := func() {
		if strings.TrimSpace(current.String()) == "" {
			return
		}
		content := strings.TrimSpace(current.String())
		chunks = append(chunks, Chunk{Content: content, Section: section, Offset: offset})
		offset += utf8.RuneCountInString(content)
		current.Reset()
		if overlap > 0 {
			current.WriteString(tailTokens(content, overlap))
			current.WriteString("\n\n")
		}
	}
	for _, p := range paragraphs {
		trimmed := strings.TrimSpace(p)
		if strings.HasPrefix(trimmed, "#") {
			section = strings.TrimSpace(strings.TrimLeft(trimmed, "# "))
		}
		if tokenCount(trimmed) > size {
			flush()
			for _, part := range splitLongParagraph(trimmed, size, overlap) {
				chunks = append(chunks, Chunk{Content: part, Section: section, Offset: offset})
				offset += utf8.RuneCountInString(part)
			}
			current.Reset()
			continue
		}
		if tokenCount(current.String())+tokenCount(trimmed) > size && current.Len() > 0 {
			flush()
		}
		current.WriteString(trimmed)
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
	return len(textUnits(s))
}

type textUnit struct {
	text string
	cjk  bool
}

func splitLongParagraph(s string, size, overlap int) []string {
	units := textUnits(s)
	if len(units) <= size {
		return []string{strings.TrimSpace(s)}
	}
	step := size - overlap
	if step <= 0 {
		step = size
	}
	out := make([]string, 0, (len(units)+step-1)/step)
	for start := 0; start < len(units); start += step {
		end := start + size
		if end > len(units) {
			end = len(units)
		}
		part := strings.TrimSpace(joinUnits(units[start:end]))
		if part != "" {
			out = append(out, part)
		}
		if end == len(units) {
			break
		}
	}
	return out
}

func tailTokens(s string, n int) string {
	units := textUnits(s)
	if len(units) <= n {
		return joinUnits(units)
	}
	return joinUnits(units[len(units)-n:])
}

func textUnits(s string) []textUnit {
	var units []textUnit
	var word strings.Builder
	flushWord := func() {
		if word.Len() == 0 {
			return
		}
		units = append(units, textUnit{text: word.String()})
		word.Reset()
	}
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			flushWord()
		case isCJK(r):
			flushWord()
			units = append(units, textUnit{text: string(r), cjk: true})
		case unicode.IsPunct(r), unicode.IsSymbol(r):
			if word.Len() > 0 {
				word.WriteRune(r)
			}
		default:
			word.WriteRune(r)
		}
	}
	flushWord()
	return units
}

func joinUnits(units []textUnit) string {
	var out strings.Builder
	for i, unit := range units {
		if i > 0 && !units[i-1].cjk && !unit.cjk {
			out.WriteByte(' ')
		}
		out.WriteString(unit.text)
	}
	return out.String()
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}
