// Package conformance provides deterministic, network-free checks for public
// ORAG extension contracts.
package conformance

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/shikanon/orag/extensions"
)

func Parser(ctx context.Context, value extensions.Parser) error {
	if value == nil {
		return fmt.Errorf("parser is nil")
	}
	parsed, err := value.Parse(ctx, extensions.Document{Name: "fixture.txt", Content: []byte("ORAG conformance fixture")})
	if err != nil {
		return fmt.Errorf("parse fixture: %w", err)
	}
	if strings.TrimSpace(parsed.Text) == "" {
		return fmt.Errorf("parser returned empty text")
	}
	return nil
}

func Chunker(ctx context.Context, value extensions.Chunker) error {
	if value == nil {
		return fmt.Errorf("chunker is nil")
	}
	chunks, err := value.Split(ctx, extensions.ParsedDocument{Text: "first paragraph\n\nsecond paragraph"})
	if err != nil {
		return fmt.Errorf("split fixture: %w", err)
	}
	if len(chunks) == 0 {
		return fmt.Errorf("chunker returned no chunks")
	}
	for index, chunk := range chunks {
		if strings.TrimSpace(chunk.Text) == "" {
			return fmt.Errorf("chunk %d has empty text", index)
		}
	}
	return nil
}

func Embedder(ctx context.Context, value extensions.Embedder) error {
	if value == nil {
		return fmt.Errorf("embedder is nil")
	}
	vectors, err := value.Embed(ctx, []string{"alpha", "beta"})
	if err != nil {
		return fmt.Errorf("embed fixture: %w", err)
	}
	if len(vectors) != 2 {
		return fmt.Errorf("embedder returned %d vectors for 2 texts", len(vectors))
	}
	for index, vector := range vectors {
		if len(vector) == 0 {
			return fmt.Errorf("vector %d is empty", index)
		}
		for _, value := range vector {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("vector %d contains non-finite value", index)
			}
		}
	}
	return nil
}

func Retriever(ctx context.Context, value extensions.Retriever) error {
	if value == nil {
		return fmt.Errorf("retriever is nil")
	}
	results, err := value.Retrieve(ctx, extensions.RetrieveRequest{Query: "fixture query", Limit: 2})
	if err != nil {
		return fmt.Errorf("retrieve fixture: %w", err)
	}
	seen := map[string]struct{}{}
	for index, result := range results {
		if strings.TrimSpace(result.ID) == "" || strings.TrimSpace(result.Text) == "" {
			return fmt.Errorf("result %d is incomplete", index)
		}
		if _, ok := seen[result.ID]; ok {
			return fmt.Errorf("duplicate result ID %q", result.ID)
		}
		seen[result.ID] = struct{}{}
		if math.IsNaN(result.Score) || math.IsInf(result.Score, 0) {
			return fmt.Errorf("result %q has non-finite score", result.ID)
		}
	}
	return nil
}

func Reranker(ctx context.Context, value extensions.Reranker) error {
	if value == nil {
		return fmt.Errorf("reranker is nil")
	}
	candidates := []extensions.RetrievalResult{{ID: "a", Text: "alpha", Score: 0.2}, {ID: "b", Text: "beta", Score: 0.1}}
	results, err := value.Rerank(ctx, extensions.RerankRequest{Query: "fixture query", Candidates: candidates})
	if err != nil {
		return fmt.Errorf("rerank fixture: %w", err)
	}
	if len(results) != len(candidates) {
		return fmt.Errorf("reranker returned %d results for %d candidates", len(results), len(candidates))
	}
	seen := map[string]struct{}{}
	for _, result := range results {
		if result.ID != "a" && result.ID != "b" {
			return fmt.Errorf("reranker returned unknown candidate %q", result.ID)
		}
		if _, ok := seen[result.ID]; ok {
			return fmt.Errorf("reranker duplicated candidate %q", result.ID)
		}
		seen[result.ID] = struct{}{}
		if math.IsNaN(result.Score) || math.IsInf(result.Score, 0) {
			return fmt.Errorf("reranker score for %q is non-finite", result.ID)
		}
	}
	return nil
}

func Generator(ctx context.Context, value extensions.Generator) error {
	if value == nil {
		return fmt.Errorf("generator is nil")
	}
	output, err := value.Generate(ctx, extensions.GenerateRequest{Prompt: "Return a concise fixture answer."})
	if err != nil {
		return fmt.Errorf("generate fixture: %w", err)
	}
	if strings.TrimSpace(output) == "" {
		return fmt.Errorf("generator returned empty output")
	}
	return nil
}

func ModelProvider(ctx context.Context, value extensions.ModelProvider) error {
	if value == nil {
		return fmt.Errorf("model provider is nil")
	}
	if strings.TrimSpace(value.Name()) == "" {
		return fmt.Errorf("model provider has empty name")
	}
	return Generator(ctx, value)
}

func Storage(ctx context.Context, value extensions.Storage) error {
	if value == nil {
		return fmt.Errorf("storage is nil")
	}
	status, err := value.Check(ctx)
	if err != nil {
		return fmt.Errorf("storage check: %w", err)
	}
	if !status.Ready {
		return fmt.Errorf("storage is not ready: %s", status.Detail)
	}
	return nil
}
