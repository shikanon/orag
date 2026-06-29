package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
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
	metadata := map[string]string{
		"filename":      name,
		"ext":           ext,
		"parser_method": MethodBasic,
	}
	switch ext {
	case ".txt", ".md", ".csv", ".json":
		text = string(content)
	case ".html", ".htm":
		text = htmlToMarkdown(string(content))
	case ".docx":
		text = extractOfficeText(content)
		descriptions, imageCount := p.describeOfficeImages(ctx, content)
		if imageCount > 0 {
			metadata["embedded_image_count"] = strconv.Itoa(imageCount)
		}
		if len(descriptions) > 0 {
			metadata["described_image_count"] = strconv.Itoa(len(descriptions))
			text = joinMarkdown(text, strings.Join(descriptions, "\n\n"))
		}
	case ".pptx", ".xlsx":
		text = extractOfficeText(content)
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
		Metadata: metadata,
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

func extractOfficeText(content []byte) string {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return extractXMLText(content)
	}
	var parts []string
	for _, file := range reader.File {
		name := strings.ToLower(file.Name)
		if !strings.HasSuffix(name, ".xml") || strings.Contains(name, "/_rels/") || strings.HasPrefix(name, "docprops/") {
			continue
		}
		body, err := readZipFile(file)
		if err != nil {
			continue
		}
		if text := extractXMLText(body); strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return extractXMLText(content)
	}
	return strings.Join(parts, " ")
}

func (p BasicParser) describeOfficeImages(ctx context.Context, content []byte) ([]string, int) {
	if p.Multimodal == nil {
		return nil, 0
	}
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, 0
	}
	var descriptions []string
	imageCount := 0
	for _, file := range reader.File {
		if !isOfficeImage(file.Name) {
			continue
		}
		imageCount++
		body, err := readZipFile(file)
		if err != nil {
			continue
		}
		md, err := p.Multimodal.MultimodalParse(ctx, file.Name, body)
		if err != nil || strings.TrimSpace(md) == "" {
			continue
		}
		descriptions = append(descriptions, fmt.Sprintf("## Image: %s\n\n%s", path.Base(file.Name), strings.TrimSpace(md)))
	}
	return descriptions, imageCount
}

func isOfficeImage(name string) bool {
	lower := strings.ToLower(name)
	if !strings.HasPrefix(lower, "word/media/") && !strings.HasPrefix(lower, "ppt/media/") && !strings.HasPrefix(lower, "xl/media/") {
		return false
	}
	switch filepath.Ext(lower) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp":
		return true
	default:
		return false
	}
}

func readZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func joinMarkdown(parts ...string) string {
	var out []string
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return strings.Join(out, "\n\n")
}
