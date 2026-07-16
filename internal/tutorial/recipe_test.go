package tutorial

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestParseRecipeFitsPublishedVisualQuickTier(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	template, err := catalog.Get("visual-document-rag", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	pack, ok := templatePack(template, "quick")
	if !ok {
		t.Fatal("quick pack is absent")
	}
	if _, err := ParseRecipe([]byte(validVisualRecipe), template, pack); err != nil {
		t.Fatal(err)
	}
}

func TestPublishedVisualRecipesMatchCatalog(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	template, err := catalog.Get("visual-document-rag", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, tier := range []string{"quick", "benchmark"} {
		pack, ok := templatePack(template, tier)
		if !ok {
			t.Fatalf("%s pack is absent", tier)
		}
		raw, err := os.ReadFile(filepath.Join("..", "..", "tutorial-recipes", "visual-document-rag", "1.0.0", tier, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseRecipe(raw, template, pack); err != nil {
			t.Fatalf("%s recipe: %v", tier, err)
		}
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

func TestRecipeSourceReaderFetchesOnlyPinnedSource(t *testing.T) {
	content := "abc"
	reader, err := NewRecipeSourceReader(time.Second, t.TempDir(), &http.Client{Transport: recipeRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://huggingface.co/datasets/Qiuchen-Wang/ViDoSeek/resolve/e91a92ba5f38690696c7e66be5c5474b54c6e791/vidoseek.json?download=true" {
			t.Fatalf("source URL = %q", request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(content)), ContentLength: int64(len(content))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	object, err := reader.Fetch(t.Context(), RecipeSourceObject{Path: "vidoseek.json", Bytes: 3, SHA256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"})
	if err != nil {
		t.Fatal(err)
	}
	defer object.Remove()
	stored, err := os.ReadFile(object.TempPath)
	if err != nil || string(stored) != content {
		t.Fatalf("stored=%q err=%v", stored, err)
	}
}

func TestRecipeSourceReaderRejectsUnsafeZIP(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create("../escape.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("unsafe")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(archive.Bytes())
	reader, err := NewRecipeSourceReader(time.Second, t.TempDir(), &http.Client{Transport: recipeRoundTripper(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(archive.Bytes())), ContentLength: int64(archive.Len())}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	_, err = reader.Fetch(t.Context(), RecipeSourceObject{Path: "vidoseek_pdf_document.zip", Bytes: int64(archive.Len()), SHA256: hex.EncodeToString(hash[:])})
	if !errors.Is(err, ErrRecipeArchiveUnsafe) {
		t.Fatalf("err = %v", err)
	}
}

func TestExtractRecipePDFsWritesDeterministicPrivateAssets(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, name := range []string{"z.pdf", "nested/a.pdf", "notes.txt"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "source.zip")
	if err := os.WriteFile(archivePath, archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	assets, err := ExtractRecipePDFs(archivePath, filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 2 || assets[0].Document != "nested/a.pdf" || assets[0].EvidenceID != "nested/a.pdf#1" || assets[1].Document != "z.pdf" {
		t.Fatalf("assets=%#v", assets)
	}
	for _, asset := range assets {
		content, err := os.ReadFile(asset.TempPath)
		if err != nil || int64(len(content)) != asset.Bytes || asset.SHA256 == "" {
			t.Fatalf("asset=%#v content=%q err=%v", asset, content, err)
		}
	}
	if _, err := ExtractRecipePDFs(archivePath, filepath.Dir(assets[0].TempPath)); !errors.Is(err, ErrRecipeArchiveUnsafe) {
		t.Fatalf("reused output error=%v", err)
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

type recipeRoundTripper func(*http.Request) (*http.Response, error)

func (fn recipeRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func visualRecipeTemplate() Template {
	return Template{ID: "visual-document-rag", Version: "1.0.0", Modality: ModalityVisualDocument}
}

func visualRecipePack() PackRef { return PackRef{Tier: "quick", EstimatedBytes: 1 << 30} }

const validVisualRecipe = `{"template_id":"visual-document-rag","version":"1.0.0","tier":"quick","license":{"spdx":"Apache-2.0","source_url":"https://huggingface.co/datasets/Qiuchen-Wang/ViDoSeek","redistributable":true},"source":{"dataset":"Qiuchen-Wang/ViDoSeek","revision":"e91a92ba5f38690696c7e66be5c5474b54c6e791","objects":[{"path":"vidoseek_pdf_document.zip","sha256":"3b999a798ceab38703118e4cc7d9b852f86538d5bb7caad64eb545251ee00454","bytes":758769613},{"path":"vidoseek.json","sha256":"ca4949bfc16231d129cd7565f20e07683854ab5aa8d05f05a86a12a9b71a7fab","bytes":597200}]},"runtime":{"baseline":{"profile":"visual_page","top_k":5},"pages":[{"document":"sample.pdf","page":1,"evidence":"sample.pdf#1"}],"dataset":{"name":"ViDoSeek Quick","items":[{"query":"question","ground_truth":"answer","expected_evidence":["sample.pdf#1"],"split":"eval","weight":1}]}}}`
