package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	MethodBasic   = "basic"
	MethodMinerU  = "mineru"
	MethodDocling = "docling"
)

type Config struct {
	Method     string
	Multimodal interface {
		MultimodalParse(ctx context.Context, name string, content []byte) (string, error)
	}
	HTTPClient *http.Client
	MinerU     MinerUConfig
	Docling    DoclingConfig
}

type MinerUConfig struct {
	APIURL        string
	ServerURL     string
	Backend       string
	ParseMethod   string
	Lang          string
	Formula       bool
	Table         bool
	RequestZipOut bool
}

type DoclingConfig struct {
	ServerURL string
	Timeout   time.Duration
}

func New(cfg Config) Parser {
	method := normalizeMethod(cfg.Method)
	basic := BasicParser{Multimodal: cfg.Multimodal}
	if method == MethodBasic {
		return basic
	}
	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.Docling.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Minute
		}
		client = &http.Client{Timeout: timeout}
	}
	return methodParser{
		method:  method,
		basic:   basic,
		minerU:  minerUParser{cfg: cfg.MinerU, client: client},
		docling: doclingParser{cfg: cfg.Docling, client: client},
	}
}

func normalizeMethod(method string) string {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "", MethodBasic:
		return MethodBasic
	case MethodMinerU:
		return MethodMinerU
	case MethodDocling:
		return MethodDocling
	default:
		return strings.ToLower(strings.TrimSpace(method))
	}
}

type methodParser struct {
	method  string
	basic   BasicParser
	minerU  minerUParser
	docling doclingParser
}

func (p methodParser) Parse(ctx context.Context, name string, content []byte) (ParsedDocument, error) {
	ext := strings.ToLower(filepath.Ext(name))
	switch p.method {
	case MethodMinerU:
		if ext != ".pdf" {
			return p.basic.Parse(ctx, name, content)
		}
		return p.minerU.Parse(ctx, name, content)
	case MethodDocling:
		if ext != ".pdf" && ext != ".docx" {
			return p.basic.Parse(ctx, name, content)
		}
		return p.docling.Parse(ctx, name, content)
	default:
		return ParsedDocument{}, fmt.Errorf("unsupported ingestion parser method %q", p.method)
	}
}

type minerUParser struct {
	cfg    MinerUConfig
	client *http.Client
}

func (p minerUParser) Parse(ctx context.Context, name string, content []byte) (ParsedDocument, error) {
	apiURL := strings.TrimRight(strings.TrimSpace(p.cfg.APIURL), "/")
	if apiURL == "" {
		return ParsedDocument{}, fmt.Errorf("MINERU_APISERVER is required when INGEST_PARSER_METHOD=mineru")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("files", filepath.Base(name))
	if err != nil {
		return ParsedDocument{}, err
	}
	if _, err := fileWriter.Write(content); err != nil {
		return ParsedDocument{}, err
	}
	fields := map[string]string{
		"output_dir":          "./output",
		"lang_list":           minerULang(p.cfg.Lang),
		"backend":             defaultString(p.cfg.Backend, "pipeline"),
		"parse_method":        defaultString(p.cfg.ParseMethod, "auto"),
		"formula_enable":      boolString(p.cfg.Formula),
		"table_enable":        boolString(p.cfg.Table),
		"return_md":           "true",
		"return_middle_json":  "true",
		"return_model_output": "true",
		"return_content_list": "true",
		"return_images":       "true",
		"response_format_zip": boolString(p.cfg.RequestZipOut),
		"start_page_id":       "0",
		"end_page_id":         "99999",
	}
	if strings.TrimSpace(p.cfg.ServerURL) != "" {
		fields["server_url"] = strings.TrimSpace(p.cfg.ServerURL)
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return ParsedDocument{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return ParsedDocument{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/file_parse", &body)
	if err != nil {
		return ParsedDocument{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json, application/zip")
	resp, err := p.client.Do(req)
	if err != nil {
		return ParsedDocument{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ParsedDocument{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ParsedDocument{}, fmt.Errorf("MinerU parser status %d: %s", resp.StatusCode, string(respBody))
	}

	var markdown string
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "zip") || zipLooksLike(respBody) {
		markdown, err = markdownFromMinerUZip(respBody)
	} else {
		markdown, err = markdownFromMinerUJSON(respBody)
	}
	if err != nil {
		return ParsedDocument{}, err
	}
	if strings.TrimSpace(markdown) == "" {
		return ParsedDocument{}, fmt.Errorf("MinerU parser returned empty markdown for %s", name)
	}
	return parsedRemoteDocument(name, MethodMinerU, markdown), nil
}

func markdownFromMinerUZip(body []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", err
	}
	for _, file := range reader.File {
		lower := strings.ToLower(file.Name)
		if lower == "content_list.json" || strings.HasSuffix(lower, "_content_list.json") || strings.HasSuffix(lower, "/content_list.json") {
			raw, err := readZipFile(file)
			if err != nil {
				return "", err
			}
			return markdownFromMinerUJSON(raw)
		}
	}
	return "", fmt.Errorf("MinerU zip did not include content_list.json")
}

func markdownFromMinerUJSON(body []byte) (string, error) {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if items, ok := payload.([]any); ok {
		return minerUBlocksMarkdown(items), nil
	}
	if doc := mapFromAny(payload); doc != nil {
		for _, key := range []string{"md_content", "markdown", "text_content", "text"} {
			if value := stringField(doc, key); value != "" {
				return value, nil
			}
		}
		for _, key := range []string{"content_list", "blocks", "results"} {
			if items, ok := doc[key].([]any); ok {
				return minerUBlocksMarkdown(items), nil
			}
		}
		if nested := mapFromAny(doc["document"]); nested != nil {
			for _, key := range []string{"md_content", "markdown", "text_content", "text"} {
				if value := stringField(nested, key); value != "" {
					return value, nil
				}
			}
		}
	}
	return "", nil
}

func minerUBlocksMarkdown(items []any) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		block := mapFromAny(item)
		if block == nil {
			continue
		}
		var section string
		switch strings.ToLower(stringField(block, "type")) {
		case "text", "equation":
			section = stringField(block, "text")
		case "table":
			section = joinMarkdown(
				htmlFragmentToText(stringField(block, "table_body")),
				strings.Join(stringSliceField(block, "table_caption"), "\n"),
				strings.Join(stringSliceField(block, "table_footnote"), "\n"),
			)
		case "image":
			section = joinMarkdown(
				strings.Join(stringSliceField(block, "image_caption"), "\n"),
				strings.Join(stringSliceField(block, "image_footnote"), "\n"),
				stringField(block, "vlm_description"),
			)
		case "code":
			section = joinMarkdown(stringField(block, "code_body"), strings.Join(stringSliceField(block, "code_caption"), "\n"))
		case "list":
			section = strings.Join(stringSliceField(block, "list_items"), "\n")
		default:
			continue
		}
		if strings.TrimSpace(section) != "" {
			parts = append(parts, strings.TrimSpace(section))
		}
	}
	return strings.Join(parts, "\n\n")
}

type doclingParser struct {
	cfg    DoclingConfig
	client *http.Client
}

func (p doclingParser) Parse(ctx context.Context, name string, content []byte) (ParsedDocument, error) {
	serverURL := strings.TrimRight(strings.TrimSpace(p.cfg.ServerURL), "/")
	if serverURL == "" {
		return ParsedDocument{}, fmt.Errorf("DOCLING_SERVER_URL is required when INGEST_PARSER_METHOD=docling")
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if format == "" {
		format = "pdf"
	}
	payload := map[string]any{
		"options": map[string]any{
			"from_formats": []string{format},
			"to_formats":   []string{"json", "md", "text"},
		},
		"sources": []map[string]string{{
			"kind":          "file",
			"filename":      filepath.Base(name),
			"base64_string": base64.StdEncoding.EncodeToString(content),
		}},
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return ParsedDocument{}, err
	}

	var errors []string
	for _, endpoint := range []string{"/v1/convert/source", "/v1alpha/convert/source"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+endpoint, bytes.NewReader(rawPayload))
		if err != nil {
			return ParsedDocument{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := p.client.Do(req)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", endpoint, err))
			continue
		}
		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return ParsedDocument{}, readErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			errors = append(errors, fmt.Sprintf("%s: status %d: %s", endpoint, resp.StatusCode, string(respBody)))
			continue
		}
		markdown, err := markdownFromDoclingJSON(respBody)
		if err != nil {
			return ParsedDocument{}, err
		}
		if strings.TrimSpace(markdown) == "" {
			return ParsedDocument{}, fmt.Errorf("Docling parser returned empty markdown for %s", name)
		}
		return parsedRemoteDocument(name, MethodDocling, markdown), nil
	}
	return ParsedDocument{}, fmt.Errorf("Docling remote convert failed: %s", strings.Join(errors, " | "))
}

func markdownFromDoclingJSON(body []byte) (string, error) {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	return doclingMarkdown(payload), nil
}

func doclingMarkdown(payload any) string {
	if items, ok := payload.([]any); ok {
		return doclingChunksMarkdown(items)
	}
	doc := mapFromAny(payload)
	if doc == nil {
		return ""
	}
	for _, key := range []string{"md_content", "markdown", "text_content", "text"} {
		if value := stringField(doc, key); value != "" {
			return value
		}
	}
	if nested := mapFromAny(doc["document"]); nested != nil {
		if markdown := doclingMarkdown(nested); markdown != "" {
			return markdown
		}
	}
	for _, key := range []string{"documents", "results", "chunks"} {
		if items, ok := doc[key].([]any); ok {
			if markdown := doclingChunksMarkdown(items); markdown != "" {
				return markdown
			}
		}
	}
	return ""
}

func doclingChunksMarkdown(items []any) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		doc := mapFromAny(item)
		if doc == nil {
			continue
		}
		var text string
		for _, key := range []string{"md_content", "markdown", "text_content", "text"} {
			if value := stringField(doc, key); value != "" {
				text = value
				break
			}
		}
		if text == "" {
			if nested := mapFromAny(doc["document"]); nested != nil {
				text = doclingMarkdown(nested)
			}
		}
		if text == "" {
			if nested := mapFromAny(doc["chunk"]); nested != nil {
				text = doclingMarkdown(nested)
			}
		}
		if strings.TrimSpace(text) != "" {
			parts = append(parts, strings.TrimSpace(text))
		}
	}
	return strings.Join(parts, "\n\n")
}

func parsedRemoteDocument(name, method, markdown string) ParsedDocument {
	ext := strings.ToLower(filepath.Ext(name))
	return ParsedDocument{
		Markdown: strings.TrimSpace(markdown),
		Metadata: map[string]string{
			"filename":      name,
			"ext":           ext,
			"parser_method": method,
		},
	}
}

func mapFromAny(value any) map[string]any {
	if doc, ok := value.(map[string]any); ok {
		return doc
	}
	return nil
}

func stringField(doc map[string]any, key string) string {
	if value, ok := doc[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func stringSliceField(doc map[string]any, key string) []string {
	switch value := doc[key].(type) {
	case []string:
		return value
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func htmlFragmentToText(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	s = html.UnescapeString(s)
	s = regexp.MustCompile(`(?is)<\s*br\s*/?\s*>`).ReplaceAllString(s, "\n")
	s = regexp.MustCompile(`(?is)</\s*(p|div|li|tr|h[1-6]|table|caption)\s*>`).ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func minerULang(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "english", "en":
		return "en"
	case "chinese", "simplified chinese", "zh", "ch":
		return "ch"
	case "traditional chinese", "chinese_cht":
		return "chinese_cht"
	case "japanese", "japan", "ja":
		return "japan"
	case "korean", "ko":
		return "korean"
	case "russian", "ukrainian", "east_slavic":
		return "east_slavic"
	case "hindi", "devanagari":
		return "devanagari"
	case "greek", "el":
		return "el"
	case "thai", "th":
		return "th"
	case "tamil", "ta":
		return "ta"
	case "telugu", "te":
		return "te"
	case "kannada", "ka":
		return "ka"
	default:
		return strings.TrimSpace(value)
	}
}

func zipLooksLike(body []byte) bool {
	return len(body) >= 4 && body[0] == 'P' && body[1] == 'K'
}
