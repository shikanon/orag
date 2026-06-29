package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type recordingMultimodal struct {
	names []string
}

func (m *recordingMultimodal) MultimodalParse(_ context.Context, name string, _ []byte) (string, error) {
	m.names = append(m.names, name)
	return "vision: " + name, nil
}

func TestBasicParserText(t *testing.T) {
	doc, err := (BasicParser{}).Parse(context.Background(), "a.txt", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Markdown != "hello" {
		t.Fatalf("markdown = %q", doc.Markdown)
	}
}

func TestBasicParserHTML(t *testing.T) {
	doc, err := (BasicParser{}).Parse(context.Background(), "a.html", []byte("<h1>Hello</h1><p>RAG</p>"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Markdown != "Hello RAG" {
		t.Fatalf("markdown = %q", doc.Markdown)
	}
}

func TestBasicParserDescribesPDFWithMultimodalModel(t *testing.T) {
	mm := &recordingMultimodal{}
	doc, err := (BasicParser{Multimodal: mm}).Parse(context.Background(), "deck.pdf", []byte("%PDF-visual"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Markdown != "vision: deck.pdf" {
		t.Fatalf("markdown = %q", doc.Markdown)
	}
	if len(mm.names) != 1 || mm.names[0] != "deck.pdf" {
		t.Fatalf("multimodal calls = %#v", mm.names)
	}
}

func TestBasicParserExtractsDOCXTextAndDescribesEmbeddedImages(t *testing.T) {
	mm := &recordingMultimodal{}
	doc, err := (BasicParser{Multimodal: mm}).Parse(context.Background(), "report.docx", docxFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Quarterly revenue", "vision: word/media/chart.png"} {
		if !strings.Contains(doc.Markdown, want) {
			t.Fatalf("markdown missing %q: %q", want, doc.Markdown)
		}
	}
	if len(mm.names) != 1 || mm.names[0] != "word/media/chart.png" {
		t.Fatalf("multimodal calls = %#v", mm.names)
	}
	if doc.Metadata["embedded_image_count"] != "1" {
		t.Fatalf("embedded image metadata = %#v", doc.Metadata)
	}
}

func TestMinerUParserNormalizesContentListZip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/file_parse" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		if got := r.FormValue("backend"); got != "pipeline" {
			t.Fatalf("backend = %q", got)
		}
		if got := r.FormValue("parse_method"); got != "auto" {
			t.Fatalf("parse_method = %q", got)
		}

		w.Header().Set("Content-Type", "application/zip")
		zw := zip.NewWriter(w)
		f, err := zw.Create("demo/auto/demo_content_list.json")
		if err != nil {
			t.Fatal(err)
		}
		blocks := []map[string]any{
			{"type": "text", "text": "MinerU paragraph"},
			{"type": "image", "image_caption": []string{"Chart caption"}, "vlm_description": "Chart bars show visible growth"},
			{"type": "table", "table_body": "<table><tr><td>A</td></tr></table>", "table_caption": []string{"Table 1"}},
		}
		if err := json.NewEncoder(f).Encode(blocks); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	p := New(Config{
		Method:     MethodMinerU,
		HTTPClient: server.Client(),
		MinerU: MinerUConfig{
			APIURL:        server.URL,
			Backend:       "pipeline",
			ParseMethod:   "auto",
			Lang:          "en",
			Formula:       true,
			Table:         true,
			RequestZipOut: true,
		},
	})
	doc, err := p.Parse(context.Background(), "demo.pdf", []byte("%PDF"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"MinerU paragraph", "Chart caption", "Chart bars show visible growth", "Table 1"} {
		if !strings.Contains(doc.Markdown, want) {
			t.Fatalf("markdown missing %q: %q", want, doc.Markdown)
		}
	}
	if doc.Metadata["parser_method"] != MethodMinerU {
		t.Fatalf("metadata = %#v", doc.Metadata)
	}
}

func TestDoclingParserNormalizesRemoteMarkdown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/convert/source" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if _, ok := payload["sources"]; !ok {
			t.Fatalf("payload missing sources: %#v", payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"document": map[string]any{
				"md_content": "# Docling\n\nImage insight from remote parser",
			},
		})
	}))
	defer server.Close()

	p := New(Config{
		Method:     MethodDocling,
		HTTPClient: server.Client(),
		Docling:    DoclingConfig{ServerURL: server.URL},
	})
	doc, err := p.Parse(context.Background(), "doc.pdf", []byte("%PDF"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Markdown != "# Docling\n\nImage insight from remote parser" {
		t.Fatalf("markdown = %q", doc.Markdown)
	}
	if doc.Metadata["parser_method"] != MethodDocling {
		t.Fatalf("metadata = %#v", doc.Metadata)
	}
}

func docxFixture(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipFile(t, zw, "word/document.xml", []byte(`<w:document xmlns:w="urn:test"><w:body><w:p><w:r><w:t>Quarterly revenue</w:t></w:r></w:p></w:body></w:document>`))
	writeZipFile(t, zw, "word/media/chart.png", []byte("\x89PNG\r\n\x1a\nimage"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeZipFile(t *testing.T, zw *zip.Writer, name string, body []byte) {
	t.Helper()
	f, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(body); err != nil {
		t.Fatal(err)
	}
}
