package kb

import "context"

type KnowledgeBaseRepository interface {
	PutKnowledgeBase(kb KnowledgeBase)
	ListKnowledgeBases(tenantID string) []KnowledgeBase
	GetKnowledgeBase(tenantID, id string) (KnowledgeBase, bool)
}

type KnowledgeBaseDeleter interface {
	DeleteKnowledgeBase(ctx context.Context, tenantID, id string) (bool, error)
}

type ChunkSource interface {
	Chunks(tenantID, kbID string) []Chunk
}

type Store interface {
	KnowledgeBaseRepository
	KnowledgeBaseDeleter
	ChunkSource
	Indexer
}
