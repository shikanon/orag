// Package extensions defines the public, dependency-free contracts used to
// build ORAG integrations. These contracts are beta until ORAG v1.0.
package extensions

import "context"

const ContractVersion = "v0beta1"

type SupportLevel string

const (
	SupportCertified    SupportLevel = "certified"
	SupportCommunity    SupportLevel = "community"
	SupportExperimental SupportLevel = "experimental"
)

type Document struct {
	Name    string
	Content []byte
}

type ParsedDocument struct {
	Text     string
	Metadata map[string]string
}

type Chunk struct {
	ID       string
	Text     string
	Metadata map[string]string
}

type Parser interface {
	Parse(context.Context, Document) (ParsedDocument, error)
}
type Chunker interface {
	Split(context.Context, ParsedDocument) ([]Chunk, error)
}
type Embedder interface {
	Embed(context.Context, []string) ([][]float64, error)
}

type RetrieveRequest struct {
	Query string
	Limit int
}
type RetrievalResult struct {
	ID       string
	Text     string
	Score    float64
	Metadata map[string]string
}
type Retriever interface {
	Retrieve(context.Context, RetrieveRequest) ([]RetrievalResult, error)
}

type RerankRequest struct {
	Query      string
	Candidates []RetrievalResult
}
type Reranker interface {
	Rerank(context.Context, RerankRequest) ([]RetrievalResult, error)
}

type GenerateRequest struct{ Prompt string }
type Generator interface {
	Generate(context.Context, GenerateRequest) (string, error)
}

// ModelProvider identifies a public generator integration. Capability-specific
// contracts such as Embedder and Reranker remain independently composable.
type ModelProvider interface {
	Name() string
	Generator
}

type StorageStatus struct {
	Ready  bool
	Detail string
}
type Storage interface {
	Check(context.Context) (StorageStatus, error)
}
