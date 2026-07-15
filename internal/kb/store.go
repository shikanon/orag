package kb

import "context"

type KnowledgeBaseRepository interface {
	PutKnowledgeBase(ctx context.Context, kb KnowledgeBase) error
	ListKnowledgeBases(ctx context.Context, tenantID string) ([]KnowledgeBase, error)
	GetKnowledgeBase(ctx context.Context, tenantID, id string) (KnowledgeBase, bool, error)
	DeleteKnowledgeBase(ctx context.Context, tenantID, id string) (bool, error)
}

type ProjectKnowledgeBaseRepository interface {
	ListKnowledgeBasesByProject(ctx context.Context, tenantID, projectID string) ([]KnowledgeBase, error)
	GetKnowledgeBaseByProject(ctx context.Context, tenantID, projectID, id string) (KnowledgeBase, bool, error)
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
