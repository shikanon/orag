package kb

import "context"

type KnowledgeBaseRepository interface {
	PutKnowledgeBase(kb KnowledgeBase) error
	ListKnowledgeBases(tenantID string) ([]KnowledgeBase, error)
	GetKnowledgeBase(tenantID, id string) (KnowledgeBase, bool, error)
	DeleteKnowledgeBase(ctx context.Context, tenantID, id string) (bool, error)
}

type ChunkSource interface {
	Chunks(tenantID, kbID string) []Chunk
}

type Store interface {
	KnowledgeBaseRepository
	ChunkSource
	Indexer
}
