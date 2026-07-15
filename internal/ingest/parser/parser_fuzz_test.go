package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"
)

func FuzzBasicParser(f *testing.F) {
	f.Add(uint8(0), []byte("plain text"))
	f.Add(uint8(1), []byte("<h1>ORAG</h1><p>evaluation-first RAG</p>"))
	f.Add(uint8(2), officeFuzzSeed())
	f.Add(uint8(3), []byte("PK\x03\x04malformed office archive"))
	f.Add(uint8(4), []byte(`{"query":"what is RAG?"}`))

	extensions := [...]string{".txt", ".html", ".docx", ".pptx", ".xlsx", ".json"}
	f.Fuzz(func(t *testing.T, kind uint8, content []byte) {
		name := "fuzz" + extensions[int(kind)%len(extensions)]
		doc, err := (BasicParser{}).Parse(context.Background(), name, content)
		if err != nil {
			return
		}
		if strings.TrimSpace(doc.Markdown) == "" {
			t.Fatal("successful parse returned empty markdown")
		}
		if doc.Metadata["filename"] != name {
			t.Fatalf("filename metadata = %q, want %q", doc.Metadata["filename"], name)
		}
		if doc.Metadata["parser_method"] != MethodBasic {
			t.Fatalf("parser method = %q, want %q", doc.Metadata["parser_method"], MethodBasic)
		}
	})
}

func officeFuzzSeed() []byte {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create("word/document.xml")
	if err != nil {
		panic(err)
	}
	if _, err := file.Write([]byte(`<w:document xmlns:w="urn:test"><w:body><w:p><w:r><w:t>seed document</w:t></w:r></w:p></w:body></w:document>`)); err != nil {
		panic(err)
	}
	if err := writer.Close(); err != nil {
		panic(err)
	}
	return buffer.Bytes()
}
