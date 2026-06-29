package parser

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"strings"
)

type ParsedDocument struct {
	Markdown string
	Metadata map[string]string
}

type Parser interface {
	Parse(ctx context.Context, name string, content []byte) (ParsedDocument, error)
}

type BasicParser struct {
	Multimodal interface {
		MultimodalParse(ctx context.Context, name string, content []byte) (string, error)
	}
}

func (p BasicParser) Parse(ctx context.Context, name string, content []byte) (ParsedDocument, error) {
	ext := strings.ToLower(filepath.Ext(name))
	var text string
	switch ext {
	case ".txt", ".md", ".csv", ".json":
		text = string(content)
	case ".html", ".htm":
		text = htmlToMarkdown(string(content))
	case ".docx", ".pptx", ".xlsx":
		text = extractXMLText(content)
	case ".pdf", ".png", ".jpg", ".jpeg":
		if p.Multimodal != nil {
			md, err := p.Multimodal.MultimodalParse(ctx, name, content)
			if err == nil && strings.TrimSpace(md) != "" {
				text = md
			}
		}
		if text == "" {
			text = string(content)
		}
	default:
		text = string(content)
	}
	if strings.TrimSpace(text) == "" {
		return ParsedDocument{}, fmt.Errorf("no text extracted from %s", name)
	}
	return ParsedDocument{
		Markdown: strings.TrimSpace(text),
		Metadata: map[string]string{
			"filename": name,
			"ext":      ext,
		},
	}, nil
}

var tagRe = regexp.MustCompile(`<[^>]+>`)

func htmlToMarkdown(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

func extractXMLText(content []byte) string {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	var parts []string
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if char, ok := tok.(xml.CharData); ok {
			text := strings.TrimSpace(string(char))
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	if len(parts) == 0 {
		return string(content)
	}
	return strings.Join(parts, " ")
}
