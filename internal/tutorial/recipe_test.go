package tutorial

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestParseRecipeAcceptsPinnedVisualRecipe(t *testing.T) {
	recipe, err := ParseRecipe([]byte(validVisualRecipe), visualRecipeTemplate(), visualRecipePack())
	if err != nil {
		t.Fatal(err)
	}
	if recipe.Source.Dataset != ViDoSeekDataset || len(recipe.Source.Objects) != 2 {
		t.Fatalf("recipe = %#v", recipe)
	}
}

func TestParseRecipeRejectsSourceDrift(t *testing.T) {
	raw := []byte(`{"template_id":"visual-document-rag","version":"1.0.0","tier":"quick","license":{"spdx":"Apache-2.0","source_url":"https://huggingface.co/datasets/Qiuchen-Wang/ViDoSeek","redistributable":true},"source":{"dataset":"Qiuchen-Wang/ViDoSeek","revision":"main","objects":[]},"runtime":{"baseline":{"profile":"visual_page","top_k":5},"pages":[],"dataset":{"name":"x","items":[]}}}`)
	if _, err := ParseRecipe(raw, visualRecipeTemplate(), visualRecipePack()); !errors.Is(err, ErrRecipeInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseRecipeRejectsUnknownFields(t *testing.T) {
	raw := []byte(validVisualRecipe[:len(validVisualRecipe)-1] + `,"url":"https://example.com"}`)
	if _, err := ParseRecipe(raw, visualRecipeTemplate(), visualRecipePack()); !errors.Is(err, ErrRecipeInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifyRecipeSourceChecksSizeAndChecksum(t *testing.T) {
	object := RecipeSourceObject{Path: "vidoseek.json", Bytes: 3, SHA256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"}
	if err := VerifyRecipeSource(bytes.NewBufferString("abc"), object); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRecipeSource(bytes.NewBufferString("abcd"), object); !errors.Is(err, ErrRecipeSourceSize) {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateRecipeZIPRejectsTraversal(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("../escape.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(entry, "x"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRecipeZIP(reader); !errors.Is(err, ErrRecipeArchiveUnsafe) {
		t.Fatalf("err = %v", err)
	}
}

func visualRecipeTemplate() Template {
	return Template{ID: "visual-document-rag", Version: "1.0.0", Modality: ModalityVisualDocument}
}

func visualRecipePack() PackRef { return PackRef{Tier: "quick", EstimatedBytes: 1 << 30} }

const validVisualRecipe = `{"template_id":"visual-document-rag","version":"1.0.0","tier":"quick","license":{"spdx":"Apache-2.0","source_url":"https://huggingface.co/datasets/Qiuchen-Wang/ViDoSeek","redistributable":true},"source":{"dataset":"Qiuchen-Wang/ViDoSeek","revision":"e91a92ba5f38690696c7e66be5c5474b54c6e791","objects":[{"path":"vidoseek_pdf_document.zip","sha256":"3b999a798ceab38703118e4cc7d9b852f86538d5bb7caad64eb545251ee00454","bytes":758769613},{"path":"vidoseek.json","sha256":"ca4949bfc16231d129cd7565f20e07683854ab5aa8d05f05a86a12a9b71a7fab","bytes":597200}]},"runtime":{"baseline":{"profile":"visual_page","top_k":5},"pages":[{"document":"sample.pdf","page":1,"evidence":"sample.pdf#1"}],"dataset":{"name":"ViDoSeek Quick","items":[{"query":"question","ground_truth":"answer","expected_evidence":["sample.pdf#1"],"split":"eval","weight":1}]}}}`
