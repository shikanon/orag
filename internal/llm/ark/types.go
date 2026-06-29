package ark

import "context"

type ChatGenerator interface {
	Chat(ctx context.Context, messages []ChatMessage) (string, error)
}

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

type StreamChunk struct {
	Content string
	Done    bool
}

type Reranker interface {
	Rerank(ctx context.Context, query string, docs []RerankDocument, topN int) ([]RerankResult, error)
}

type MultimodalParser interface {
	MultimodalParse(ctx context.Context, name string, content []byte) (string, error)
}
