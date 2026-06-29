package parser

import (
	"context"
	"testing"
)

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
