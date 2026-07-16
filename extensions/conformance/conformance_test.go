package conformance_test

import (
	"context"
	"strings"
	"testing"

	"github.com/shikanon/orag/extensions"
	"github.com/shikanon/orag/extensions/conformance"
)

func TestReferenceExtensionsConform(t *testing.T) {
	ctx := context.Background()
	for name, check := range map[string]func() error{
		"parser":         func() error { return conformance.Parser(ctx, referenceParser{}) },
		"chunker":        func() error { return conformance.Chunker(ctx, referenceChunker{}) },
		"embedder":       func() error { return conformance.Embedder(ctx, referenceEmbedder{}) },
		"retriever":      func() error { return conformance.Retriever(ctx, referenceRetriever{}) },
		"reranker":       func() error { return conformance.Reranker(ctx, referenceReranker{}) },
		"generator":      func() error { return conformance.Generator(ctx, referenceGenerator{}) },
		"model_provider": func() error { return conformance.ModelProvider(ctx, referenceProvider{}) },
		"storage":        func() error { return conformance.Storage(ctx, referenceStorage{}) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := check(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConformanceRejectsInvalidExtensions(t *testing.T) {
	if err := conformance.Embedder(context.Background(), invalidEmbedder{}); err == nil {
		t.Fatal("invalid embedder passed")
	}
	if err := conformance.Reranker(context.Background(), invalidReranker{}); err == nil {
		t.Fatal("invalid reranker passed")
	}
}

type referenceParser struct{}

func (referenceParser) Parse(_ context.Context, document extensions.Document) (extensions.ParsedDocument, error) {
	return extensions.ParsedDocument{Text: string(document.Content), Metadata: map[string]string{"source": document.Name}}, nil
}

type referenceChunker struct{}

func (referenceChunker) Split(_ context.Context, document extensions.ParsedDocument) ([]extensions.Chunk, error) {
	return []extensions.Chunk{{ID: "fixture", Text: document.Text}}, nil
}

type referenceEmbedder struct{}

func (referenceEmbedder) Embed(_ context.Context, texts []string) ([][]float64, error) {
	vectors := make([][]float64, len(texts))
	for index := range texts {
		vectors[index] = []float64{float64(index + 1)}
	}
	return vectors, nil
}

type referenceRetriever struct{}

func (referenceRetriever) Retrieve(_ context.Context, _ extensions.RetrieveRequest) ([]extensions.RetrievalResult, error) {
	return []extensions.RetrievalResult{{ID: "fixture", Text: "retrieved", Score: 1}}, nil
}

type referenceReranker struct{}

func (referenceReranker) Rerank(_ context.Context, request extensions.RerankRequest) ([]extensions.RetrievalResult, error) {
	return append([]extensions.RetrievalResult(nil), request.Candidates...), nil
}

type referenceGenerator struct{}

func (referenceGenerator) Generate(_ context.Context, request extensions.GenerateRequest) (string, error) {
	return strings.TrimSpace(request.Prompt), nil
}

type referenceProvider struct{ referenceGenerator }

func (referenceProvider) Name() string { return "reference" }

type referenceStorage struct{}

func (referenceStorage) Check(context.Context) (extensions.StorageStatus, error) {
	return extensions.StorageStatus{Ready: true, Detail: "fixture"}, nil
}

type invalidEmbedder struct{}

func (invalidEmbedder) Embed(context.Context, []string) ([][]float64, error) {
	return [][]float64{{}}, nil
}

type invalidReranker struct{}

func (invalidReranker) Rerank(context.Context, extensions.RerankRequest) ([]extensions.RetrievalResult, error) {
	return nil, nil
}
