package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
)

// StructuredJSONParser renders JSON objects into stable Markdown. It is used
// by tutorial P1 so a candidate can differ from the P0 basic parser without
// requiring a remote parser service or model credentials.
type StructuredJSONParser struct {
	Fallback BasicParser
}

func (p StructuredJSONParser) Parse(ctx context.Context, name string, content []byte) (ParsedDocument, error) {
	if strings.ToLower(filepath.Ext(name)) != ".json" {
		return p.Fallback.Parse(ctx, name, content)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return ParsedDocument{}, fmt.Errorf("parse JSON %s: %w", name, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ParsedDocument{}, fmt.Errorf("parse JSON %s: multiple JSON values", name)
		}
		return ParsedDocument{}, fmt.Errorf("parse JSON %s: %w", name, err)
	}
	markdown := strings.TrimSpace(renderStructuredJSON(value, 1))
	if markdown == "" {
		return ParsedDocument{}, fmt.Errorf("no text extracted from %s", name)
	}
	return ParsedDocument{
		Markdown: markdown,
		Metadata: map[string]string{
			"filename":      name,
			"ext":           ".json",
			"parser_method": MethodStructuredJSON,
		},
	}, nil
}

func renderStructuredJSON(value any, depth int) string {
	switch item := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(item))
		for key := range item {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		parts := make([]string, 0, len(keys)*2)
		for _, key := range keys {
			parts = append(parts, strings.Repeat("#", min(depth, 6))+" "+key)
			parts = append(parts, renderStructuredJSON(item[key], depth+1))
		}
		return strings.Join(parts, "\n\n")
	case []any:
		parts := make([]string, 0, len(item))
		for _, entry := range item {
			rendered := renderStructuredJSON(entry, depth)
			if structuredJSONScalar(entry) {
				parts = append(parts, "- "+rendered)
				continue
			}
			parts = append(parts, "-\n"+indentStructuredJSON(rendered))
		}
		return strings.Join(parts, "\n")
	case string:
		return item
	case json.Number:
		return item.String()
	case float64:
		return fmt.Sprintf("%v", item)
	case bool:
		return fmt.Sprintf("%t", item)
	case nil:
		return "null"
	default:
		return fmt.Sprint(item)
	}
}

func structuredJSONScalar(value any) bool {
	switch value.(type) {
	case map[string]any, []any:
		return false
	default:
		return true
	}
}

func indentStructuredJSON(value string) string {
	return "  " + strings.ReplaceAll(value, "\n", "\n  ")
}
